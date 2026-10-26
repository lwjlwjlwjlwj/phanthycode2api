package protocol

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/tokenkit"
)

// DoneFrame 是 OpenAI SSE 流的终止帧。
const DoneFrame = "data: [DONE]\n\n"

// StreamError 表示上游在 SSE 流内部返回的 error 事件。
// relaykit 通过 ClaudeResponse.GetClaudeError() 识别该事件，这里向上传播。
type StreamError struct {
	Type    string
	Message string
	Raw     []byte
}

func (e *StreamError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("上游流内错误 type=%s message=%s", e.Type, e.Message)
	}
	return fmt.Sprintf("上游流内错误: %s", truncate(e.Raw, 256))
}

// SSEChunk 是 OpenAI chat.completion.chunk 的 SSE 帧载荷。
// relaykit 的 ChatCompletionsStreamResponse 携带 system_fingerprint/usage 等
// 内部字段且无 omitempty，这里用白名单结构体编码，只暴露 OpenAI 兼容字段。
type SSEChunk struct {
	ID      string                                    `json:"id"`
	Object  string                                    `json:"object"`
	Created int64                                     `json:"created"`
	Model   string                                    `json:"model"`
	Choices []dto.ChatCompletionsStreamResponseChoice `json:"choices"`
}

// Collector 是唯一的流式收集器：流式与非流式共用同一套 relaykit 事件状态机。
//
// relaykit 承担事件解析的复杂部分：
//   - ClaudeToChatStreamState.ConvertChunk：Anthropic content block index →
//     OpenAI 稠密 tool_calls index 的映射与校验
//   - StreamResponseClaude2OpenAI：单个事件 → OpenAI 流式 chunk
//   - FormatClaudeResponseInfo：跨事件累积 id / model / ResponseText / usage
//
// 本收集器在 relaykit 之上只做宿主职责：SSE 帧读写、聚合非流式响应、
// 以及按 new-api 的 ResponseText2Usage 规则用 tokenkit 补算缺失的 usage。
type Collector struct {
	clientModel string
	created     int64

	id        string
	state     *relayconvert.ClaudeToChatStreamState
	info      relayconvert.ClaudeResponseInfo
	estPrompt int

	text      strings.Builder
	reasoning strings.Builder

	tools []ResponseToolCall
	args  []*strings.Builder

	stopReason string
	sawFinish  bool
	sawDelta   bool
	sawTool    bool
}

// NewCollector 构建收集器；clientModel 是客户端请求的模型名（回填响应），
// estPrompt 是请求侧 token 估算值（usage 缺失时的 prompt 兜底）。
func NewCollector(clientModel string, estPrompt int) *Collector {
	return &Collector{
		clientModel: clientModel,
		created:     time.Now().Unix(),
		id:          "chatcmpl-" + randomHex(12),
		state:       relayconvert.NewClaudeToChatStreamState(),
		estPrompt:   estPrompt,
	}
}

// Run 读取 Anthropic SSE 流，把每个 OpenAI chunk 交给 onChunk（可为 nil，表示只聚合）。
// 返回的错误包括底层读取错误、上游流内 error 事件与 relaykit 的转换错误。
func (c *Collector) Run(r io.Reader, onChunk func(*dto.ChatCompletionsStreamResponse) error) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var data []string

	dispatch := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		if strings.TrimSpace(payload) == "" || payload == "[DONE]" {
			return nil
		}
		var ev dto.ClaudeResponse
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			// 上游偶尔夹杂非 JSON 心跳，按噪声丢弃。
			return nil
		}
		return c.HandleEvent(&ev, onChunk)
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				if dispatchErr := dispatch(); dispatchErr != nil {
					return dispatchErr
				}
			case strings.HasPrefix(trimmed, ":"):
				// SSE 注释行
			case strings.HasPrefix(trimmed, "data:"):
				v := strings.TrimPrefix(trimmed, "data:")
				v = strings.TrimPrefix(v, " ")
				data = append(data, v)
			default:
				// event: / id: / retry: 等字段不参与解析，JSON type 才是权威。
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	// 容忍最后一个事件没有以空行结尾。
	return dispatch()
}

// HandleEvent 处理单个上游事件。
func (c *Collector) HandleEvent(ev *dto.ClaudeResponse, onChunk func(*dto.ChatCompletionsStreamResponse) error) error {
	if ev == nil {
		return nil
	}
	// error 事件必须向上传播（修正旧实现丢弃 error 事件的缺陷）。
	if claudeErr := ev.GetClaudeError(); claudeErr != nil {
		return &StreamError{Type: claudeErr.Type, Message: claudeErr.Message}
	}

	chunk, err := c.state.ConvertChunk(ev)
	if err != nil {
		return err
	}
	relayconvert.FormatClaudeResponseInfo(ev, chunk, &c.info)
	if chunk == nil {
		return nil
	}
	c.accumulate(chunk)
	if onChunk != nil {
		return onChunk(chunk)
	}
	return nil
}

// accumulate 把单个 OpenAI chunk 聚合进非流式状态。
func (c *Collector) accumulate(oai *dto.ChatCompletionsStreamResponse) {
	if oai == nil || len(oai.Choices) == 0 {
		return
	}
	if oai.Id != "" {
		c.id = oai.Id
	}
	choice := oai.Choices[0]
	delta := choice.Delta

	if delta.Content != nil && *delta.Content != "" {
		c.text.WriteString(*delta.Content)
		c.sawDelta = true
	}
	if rc := delta.GetReasoningContent(); rc != "" {
		c.reasoning.WriteString(rc)
		c.sawDelta = true
	}
	for _, tc := range delta.ToolCalls {
		idx := 0
		if tc.Index != nil {
			idx = *tc.Index
		}
		for len(c.tools) <= idx {
			c.tools = append(c.tools, ResponseToolCall{})
			c.args = append(c.args, &strings.Builder{})
		}
		if c.tools[idx].ID == "" {
			c.tools[idx] = ResponseToolCall{
				ID:       tc.ID,
				Type:     toolTypeName(tc.Type),
				Function: FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
			}
			c.sawTool = true
		} else {
			// 后续片段只带 arguments 增量。
			c.args[idx].WriteString(tc.Function.Arguments)
		}
	}
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		c.stopReason = *choice.FinishReason
		c.sawFinish = true
	}
}

func toolTypeName(t any) string {
	if s, ok := t.(string); ok && s != "" {
		return s
	}
	return "function"
}

// HasOutput 表示流中是否产生了任何有效内容。
// 空流不得被伪造成成功响应。
func (c *Collector) HasOutput() bool {
	return c.sawDelta || c.sawTool || c.stopReason != ""
}

// FinishReason 返回 OpenAI finish_reason；未收到时按证据规则兜底。
func (c *Collector) FinishReason() string {
	if c.stopReason != "" {
		return c.stopReason
	}
	if len(c.tools) > 0 {
		return "tool_calls"
	}
	return "stop"
}

// ID 返回响应 id（优先使用上游 message.id）。
func (c *Collector) ID() string { return c.id }

// Created 返回响应时间戳。
func (c *Collector) Created() int64 { return c.created }

// UpstreamModel 返回上游回填的模型名，未回填时为空。
func (c *Collector) UpstreamModel() string { return c.info.Model }

// Usage 返回 OpenAI 语义的 token 计数（含 cache 折算与缺失补算）。
func (c *Collector) Usage() (prompt, completion int) {
	if c.info.Usage == nil {
		return 0, 0
	}
	mapped := relayconvert.UsageFromClaudeUsage(c.info.Usage)
	if mapped.PromptTokens != 0 || mapped.CompletionTokens != 0 {
		return mapped.PromptTokens, mapped.CompletionTokens
	}
	// 对齐 new-api HandleStreamFinalResponse 的 ResponseText2Usage：
	// prompt 用宿主估算值、completion 用 tokenkit.Estimate、total 求和，仅全 0 时补。
	// 估算所用模型名优先取上游回填名，未回填时退回客户端请求名（两者均为透传名）。
	prompt = c.estPrompt
	model := c.UpstreamModel()
	if model == "" {
		model = c.clientModel
	}
	completion = tokenkit.Estimate(model, c.info.ResponseText.String())
	return prompt, completion
}

// Completion 把已收集的状态组装为 OpenAI 非流式响应。
func (c *Collector) Completion() *Completion {
	msg := ResponseMessage{
		Role:             "assistant",
		Content:          c.text.String(),
		ReasoningContent: c.reasoning.String(),
	}
	if len(c.tools) > 0 {
		tools := make([]ResponseToolCall, len(c.tools))
		for i, tool := range c.tools {
			args := c.args[i].String()
			if args == "" {
				args = "{}"
			}
			tools[i] = ResponseToolCall{
				ID:       tool.ID,
				Type:     tool.Type,
				Function: FunctionCall{Name: tool.Function.Name, Arguments: args},
			}
		}
		msg.ToolCalls = tools
	}

	prompt, completion := c.Usage()
	return &Completion{
		ID:      c.id,
		Object:  "chat.completion",
		Created: c.created,
		Model:   c.clientModel,
		Choices: []Choice{{
			Index:        0,
			Message:      msg,
			FinishReason: c.FinishReason(),
		}},
		Usage: Usage{
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      prompt + completion,
		},
	}
}

// FinalChunk 返回带 finish_reason 的终止 chunk。
// 上游 message_delta 已带 finish_reason 时返回 nil（避免重复帧）。
func (c *Collector) FinalChunk() *dto.ChatCompletionsStreamResponse {
	if c.sawFinish {
		return nil
	}
	reason := c.FinishReason()
	return &dto.ChatCompletionsStreamResponse{
		Object: "chat.completion.chunk",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			FinishReason: &reason,
		}},
	}
}

// EncodeChunk 把 relaykit 生成的 OpenAI 流式响应编码为一个 SSE data 帧，
// 并回填宿主侧 id / created / 客户端模型名（relaykit 不保证这些字段非空）。
func (c *Collector) EncodeChunk(oai *dto.ChatCompletionsStreamResponse) ([]byte, error) {
	if oai == nil {
		return nil, errors.New("空 chunk")
	}
	frame := SSEChunk{
		ID:      oai.Id,
		Object:  oai.Object,
		Created: oai.Created,
		Model:   oai.Model,
		Choices: oai.Choices,
	}
	if frame.ID == "" {
		frame.ID = c.id
	}
	if frame.Created == 0 {
		frame.Created = c.created
	}
	if frame.Model == "" {
		frame.Model = c.clientModel
	}
	if frame.Object == "" {
		frame.Object = "chat.completion.chunk"
	}
	raw, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(raw)+8)
	out = append(out, "data: "...)
	out = append(out, raw...)
	out = append(out, '\n', '\n')
	return out, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
