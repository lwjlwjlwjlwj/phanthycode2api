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
)

// DoneFrame 是 OpenAI SSE 流的终止帧。
const DoneFrame = "data: [DONE]\n\n"

// StreamDelta 是 OpenAI chunk 中增量部分。
type StreamDelta struct {
	Role             string           `json:"role,omitempty"`
	Content          string           `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []StreamToolCall `json:"tool_calls,omitempty"`
}

// StreamToolCall 是流式工具调用增量；第一个片段带 id 与 name，后续片段只带 arguments。
type StreamToolCall struct {
	Index    int             `json:"index"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function *StreamFunction `json:"function,omitempty"`
}

// StreamFunction 是工具调用的函数名与参数片段。
type StreamFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// StreamChoice 是 chunk 中的候选。
type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

// StreamChunk 是 OpenAI chat.completion.chunk。
type StreamChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// StreamError 表示上游在 SSE 流内部返回的 error 事件。
// 旧实现直接丢弃该事件，这里必须向上传播。
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

// EncodeChunk 把 chunk 编码为一个 SSE 帧。
func EncodeChunk(chunk StreamChunk) ([]byte, error) {
	raw, err := json.Marshal(chunk)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(raw)+8)
	out = append(out, "data: "...)
	out = append(out, raw...)
	out = append(out, '\n', '\n')
	return out, nil
}

// sseEvent 是上游 Anthropic SSE 事件中会被读取的字段。
type sseEvent struct {
	Type    string `json:"type"`
	Index   *int   `json:"index"`
	Message *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta *struct {
		Type         string `json:"type"`
		Text         string `json:"text"`
		Thinking     string `json:"thinking"`
		PartialJSON  string `json:"partial_json"`
		StopReason   string `json:"stop_reason"`
		StopSequence string `json:"stop_sequence"`
	} `json:"delta"`
	Usage *struct {
		InputTokens  *int `json:"input_tokens"`
		OutputTokens *int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Collector 是唯一的 SSE 收集器：流式与非流式共用同一套事件状态机。
//
// 事件语义来自 re/docs/02-usage-semantics.md §3.2：
//   - message_start        → 首个 role chunk，记录 id / input_tokens
//   - content_block_start  → tool_use 注册工具并发出首个 tool_calls 片段
//   - content_block_delta  → text / thinking / input_json 三类增量
//   - message_delta        → stop_reason 与 output_tokens
//   - error                → 必须作为错误向上传播（修正旧实现丢弃 error 事件的缺陷）
type Collector struct {
	clientModel string
	created     int64

	id       string
	upstream string // 上游返回的 model，仅用于日志
	started  bool

	text      strings.Builder
	reasoning strings.Builder

	toolSlot map[int]int // Anthropic content block index → 工具序号
	tools    []ResponseToolCall
	args     []*strings.Builder

	stopReason   string
	inputTokens  int
	outputTokens int
	sawDelta     bool
	sawTool      bool
}

// NewCollector 构建收集器；clientModel 是客户端请求的模型名，会回填到响应中。
func NewCollector(clientModel string) *Collector {
	return &Collector{
		clientModel: clientModel,
		created:     time.Now().Unix(),
		id:          "chatcmpl-" + randomHex(12),
		toolSlot:    make(map[int]int),
	}
}

// Run 读取 Anthropic SSE 流，把每个 OpenAI chunk 交给 onChunk（可为 nil，表示只聚合）。
// 返回的错误包括底层读取错误与上游流内 error 事件。
func (c *Collector) Run(r io.Reader, onChunk func(StreamChunk) error) error {
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
		chunks, err := c.HandleEvent([]byte(payload))
		if err != nil {
			return err
		}
		if onChunk == nil {
			return nil
		}
		for _, chunk := range chunks {
			if err := onChunk(chunk); err != nil {
				return err
			}
		}
		return nil
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

// HandleEvent 处理单个 SSE data 载荷，返回需要下发的 chunk。
func (c *Collector) HandleEvent(payload []byte) ([]StreamChunk, error) {
	var ev sseEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		// 上游偶尔会夹杂非 JSON 心跳，按噪声丢弃。
		return nil, nil
	}

	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			if ev.Message.ID != "" {
				c.id = ev.Message.ID
			}
			if ev.Message.Model != "" {
				c.upstream = ev.Message.Model
			}
			if ev.Message.Usage != nil {
				c.inputTokens = ev.Message.Usage.InputTokens
				if ev.Message.Usage.OutputTokens > 0 {
					c.outputTokens = ev.Message.Usage.OutputTokens
				}
			}
		}
		if c.started {
			return nil, nil
		}
		c.started = true
		return []StreamChunk{c.chunk(StreamDelta{Role: "assistant"}, nil)}, nil

	case "content_block_start":
		if ev.ContentBlock == nil || ev.ContentBlock.Type != "tool_use" {
			return nil, nil
		}
		idx := indexOf(ev.Index)
		if _, exists := c.toolSlot[idx]; exists {
			return nil, nil
		}
		slot := len(c.tools)
		c.toolSlot[idx] = slot
		id := ev.ContentBlock.ID
		if id == "" {
			id = "toolu_" + randomHex(8)
		}
		c.tools = append(c.tools, ResponseToolCall{
			ID:       id,
			Type:     "function",
			Function: FunctionCall{Name: ev.ContentBlock.Name},
		})
		c.args = append(c.args, &strings.Builder{})
		c.sawTool = true
		return []StreamChunk{c.chunk(StreamDelta{
			ToolCalls: []StreamToolCall{{
				Index:    slot,
				ID:       id,
				Type:     "function",
				Function: &StreamFunction{Name: ev.ContentBlock.Name, Arguments: ""},
			}},
		}, nil)}, nil

	case "content_block_delta":
		if ev.Delta == nil {
			return nil, nil
		}
		idx := indexOf(ev.Index)
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text == "" {
				return nil, nil
			}
			c.text.WriteString(ev.Delta.Text)
			c.sawDelta = true
			return []StreamChunk{c.chunk(StreamDelta{Content: ev.Delta.Text}, nil)}, nil
		case "thinking_delta":
			if ev.Delta.Thinking == "" {
				return nil, nil
			}
			c.reasoning.WriteString(ev.Delta.Thinking)
			c.sawDelta = true
			return []StreamChunk{c.chunk(StreamDelta{ReasoningContent: ev.Delta.Thinking}, nil)}, nil
		case "input_json_delta":
			slot, ok := c.toolSlot[idx]
			if !ok || ev.Delta.PartialJSON == "" {
				return nil, nil
			}
			c.args[slot].WriteString(ev.Delta.PartialJSON)
			c.sawDelta = true
			return []StreamChunk{c.chunk(StreamDelta{
				ToolCalls: []StreamToolCall{{
					Index:    slot,
					Function: &StreamFunction{Arguments: ev.Delta.PartialJSON},
				}},
			}, nil)}, nil
		default:
			// signature_delta 等类型不映射到 OpenAI 输出。
			return nil, nil
		}

	case "message_delta":
		if ev.Delta != nil && ev.Delta.StopReason != "" {
			c.stopReason = ev.Delta.StopReason
		}
		if ev.Usage != nil {
			if ev.Usage.InputTokens != nil {
				c.inputTokens = *ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens != nil {
				c.outputTokens = *ev.Usage.OutputTokens
			}
		}
		return nil, nil

	case "error":
		se := &StreamError{Raw: payload}
		if ev.Error != nil {
			se.Type = ev.Error.Type
			se.Message = ev.Error.Message
		}
		return nil, se

	default:
		// ping / message_stop / content_block_stop 无输出。
		return nil, nil
	}
}

// HasOutput 表示流中是否产生了任何有效内容。
// 空流不得被伪造成成功响应。
func (c *Collector) HasOutput() bool {
	return c.sawDelta || c.sawTool || c.stopReason != ""
}

// FinishReason 返回 OpenAI finish_reason。
func (c *Collector) FinishReason() string {
	return MapFinishReason(c.stopReason, len(c.tools) > 0)
}

// ID 返回响应 id（优先使用上游 message.id）。
func (c *Collector) ID() string { return c.id }

// UpstreamModel 返回上游回填的模型名，未回填时为空。
func (c *Collector) UpstreamModel() string { return c.upstream }

// Usage 返回 token 计数。
func (c *Collector) Usage() (prompt, completion int) {
	return c.inputTokens, c.outputTokens
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
func (c *Collector) FinalChunk() StreamChunk {
	reason := c.FinishReason()
	return c.chunk(StreamDelta{}, &reason)
}

func (c *Collector) chunk(delta StreamDelta, finish *string) StreamChunk {
	return StreamChunk{
		ID:      c.id,
		Object:  "chat.completion.chunk",
		Created: c.created,
		Model:   c.clientModel,
		Choices: []StreamChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finish,
		}},
	}
}

func indexOf(v *int) int {
	if v == nil {
		return 0
	}
	return *v
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
