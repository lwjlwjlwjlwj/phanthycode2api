// sse.go 将上游 Anthropic SSE 流转换为 OpenAI SSE 流（或聚合成单个响应）。
package upstream

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"phanthycode2api/internal/logx"
)

// ErrEvent 是上游在 SSE 流里下发的 error 事件。
// 上游拒绝服务时仍返回 HTTP 200，错误只存在于事件流中：
// 不解析它就会把「被拒绝」静默当成「空回复」，管理台日志里表现为 status=200 tokens=0/0。
type ErrEvent struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ErrEvent) Error() string {
	if e == nil {
		return "upstream stream error"
	}
	label := e.Code
	if label == "" {
		label = e.Type
	}
	if label == "" {
		return e.Message
	}
	if e.Message == "" {
		return label
	}
	return label + ": " + e.Message
}

// anthroEvent Anthropic SSE 事件通用结构（使用 map 避免字段名冲突）。
type anthroEvent struct {
	Type    string                     `json:"type"`
	Raw     map[string]json.RawMessage `json:"-"`
	Message *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
	Index        int `json:"index"`
	ContentBlock *struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input any    `json:"input"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		Thinking    string `json:"thinking"`
	} `json:"delta,omitempty"`
	Usage *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Error *ErrEvent `json:"error,omitempty"`
}

// parseAnthroEvent 解析 Anthropic SSE 事件，正确处理 message_delta 的 delta 字段。
func parseAnthroEvent(raw []byte) *anthroEvent {
	var ev struct {
		Type string                     `json:"type"`
		Rest map[string]json.RawMessage `json:"-"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil
	}
	// 用 Map 解析所有字段
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil
	}
	ev.Rest = all
	result := &anthroEvent{Type: ev.Type}
	_ = json.Unmarshal(raw, result)

	// 特殊处理 message_delta：delta 字段在这里是 {stop_reason, stop_sequence} 而非 {type, text}
	// 而非 content_block_delta 的 {type, text_delta, partial_json}
	// 通过单独的字段解析
	return result
}

// getStopReason 从 message_delta 事件中提取 stop_reason。
func getStopReason(raw []byte) string {
	var delta struct {
		Delta struct {
			StopReason   string `json:"stop_reason"`
			StopSequence string `json:"stop_sequence"`
		} `json:"delta"`
	}
	if json.Unmarshal(raw, &delta) == nil {
		return delta.Delta.StopReason
	}
	return ""
}

// getUsage 解析 message_delta 里的 usage，并分别报告两个字段是否真的出现过。
// 上游只在下发变化过的字段（实测 message_delta 通常带 output_tokens，
// 有时带 input_tokens），用零值无条件覆盖会把 message_start 里拿到的
// input_tokens 抹成 0，审计日志就会变成 tokens=0/<out>。
func getUsage(raw []byte) (in int, hasIn bool, out int, hasOut bool) {
	var u struct {
		Usage map[string]json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &u); err != nil || u.Usage == nil {
		return 0, false, 0, false
	}
	if v, ok := u.Usage["input_tokens"]; ok {
		_ = json.Unmarshal(v, &in)
		hasIn = true
	}
	if v, ok := u.Usage["output_tokens"]; ok {
		_ = json.Unmarshal(v, &out)
		hasOut = true
	}
	return in, hasIn, out, hasOut
}

// StreamError 表示上游在事件流里下发的错误。
//
// 上游拒绝服务时不会用 4xx/5xx 响应，而是照样返回 HTTP 200 +
// text/event-stream，把真正的错误放进 `event: error` 事件里：
//
//	event: error
//	data: {"type":"error","error":{"code":"upstream_permission_denied",...}}
//
// 只看 HTTP 状态码的代理会把这种「被拒绝」当成「正常结束的空回复」：
// 客户端收不到任何内容却看到 finish_reason=stop（表现为一直转圈），
// 审计日志里则是一条 status=200、tokens=0/0 的「成功」记录。
type StreamError struct {
	Event *ErrEvent
	Kind  ErrKind
	// Committed 表示响应已经写出（含响应头与正文），无法再改写 HTTP 状态码或
	// 追加 JSON 错误体，调用方只能把错误记进审计日志并结束本次响应。
	Committed bool
}

func (e *StreamError) Error() string {
	if e == nil {
		return "upstream stream error"
	}
	return e.Event.Error()
}

// Message 返回上游给出的可读原因；没有事件时回落到 Kind，便于写进日志与响应。
func (e *StreamError) Message() string {
	if e == nil || e.Event == nil {
		return "upstream stream error"
	}
	if e.Event.Message != "" {
		return e.Event.Message
	}
	return e.Event.Code
}

// Unwrap 让 errors.As(err, &[]*ErrEvent) 能取到原始事件。
func (e *StreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Event
}

// streamState 流式转换状态。
type streamState struct {
	id      string
	model   string
	gotRole bool
	textBuf strings.Builder
	// thinkingBuf 单独累积上游思考内容。它属于推理过程，不能混进正文，
	// 否则非流式调用方会把思考片段当成答案。
	thinkingBuf strings.Builder
	toolBufs    map[int]*toolAccum // index → tool 累积
	toolSeq     []int
	usageIn     int
	usageOut    int
	stopReason  string
	// streamErr 上游在事件流里下发的错误，收尾时按失败返回而不是补 finish_reason。
	streamErr *ErrEvent
	// emitted 是否已经给下游写过至少一个 SSE 事件：决定还能不能改 HTTP 状态码。
	emitted bool
	// contentSeen 是否产出过正文/思考/工具调用，便于区分「空回复」与「没解析到」。
	contentSeen bool
}

type toolAccum struct {
	id   string
	name string
	args strings.Builder
}

// Stream 将上游 Anthropic SSE 流实时转换为 OpenAI SSE 流写回 w。
// 需要请求方已知 model（入参）。
// 返回值 usageIn/usageOut 是上游上报的 token 用量，供审计日志记录。
func Stream(w http.ResponseWriter, r io.Reader, model string) (usageIn, usageOut int, err error) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)

	st := &streamState{model: model, toolBufs: map[int]*toolAccum{}}
	br := bufio.NewReaderSize(r, 64*1024)

	writeEvent := func(obj map[string]any) error {
		raw, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
			return err
		}
		st.emitted = true
		if fl != nil {
			fl.Flush()
		}
		return nil
	}

	for {
		line, err := br.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimPrefix(line, "data: ")
				if payload != "[DONE]" {
					var ev anthroEvent
					if json.Unmarshal([]byte(payload), &ev) == nil {
						handleEvent(st, &ev, payload, writeEvent)
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return st.usageIn, st.usageOut, err
		}
	}
	// 上游用 HTTP 200 承载 SSE，拒绝服务时错误只在事件流里。
	// 这种情况绝不能补 finish_reason: stop —— 那等于把「被上游拒绝」伪装成
	// 「正常结束」，客户端会一直等正文（表现为一直转圈），审计日志也会把它
	// 记成一条 200 的成功请求。
	if st.streamErr != nil {
		failure := &StreamError{
			Event:     st.streamErr,
			Kind:      ClassifyStreamErr(st.streamErr.Code, st.streamErr.Message),
			Committed: st.emitted,
		}
		if st.emitted {
			// 已经给下游发过 chunk，状态码改不动了，至少把错误如实写进事件流，
			// 不要让调用方以为这是一次正常结束。
			_ = writeEvent(map[string]any{
				"error": map[string]any{
					"message": st.streamErr.Message,
					"type":    "api_error",
					"code":    st.streamErr.Code,
				},
			})
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			if fl != nil {
				fl.Flush()
			}
		}
		return st.usageIn, st.usageOut, failure
	}
	if !st.contentSeen && st.usageOut == 0 {
		// 200 却没有正文也没有 usage：上游偶尔这样静默吞掉请求，
		// 客户端看到的就是「一直没内容」，与拒绝的表现一致，留条日志便于定位。
		logx.Errorf("stream: 上游 200 但既无正文也无 usage（model=%q），疑似被静默拒绝", st.model)
	}
	// 结束：补 finish_reason + [DONE]
	fr := st.stopReason
	if fr == "" {
		if len(st.toolSeq) > 0 {
			fr = "tool_calls"
		} else {
			fr = "stop"
		}
	}
	if err := writeEvent(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{}, fr)); err != nil {
		return st.usageIn, st.usageOut, err
	}
	_, err = io.WriteString(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
	return st.usageIn, st.usageOut, err
}

func handleEvent(st *streamState, ev *anthroEvent, raw string, emit func(map[string]any) error) {
	if ev == nil {
		return
	}
	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			if st.id == "" {
				st.id = ev.Message.ID
			}
			if st.model == "" {
				st.model = ev.Message.Model
			}
			st.usageIn = ev.Message.Usage.InputTokens
		}
		// 首个 chunk：声明 role
		_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{"role": "assistant"}, ""))
		st.gotRole = true

	case "content_block_start":
		if ev.ContentBlock == nil {
			return
		}
		switch ev.ContentBlock.Type {
		case "text":
			// 不需要额外动作；delta 会带文本
		case "tool_use":
			idx := ev.Index
			if _, seen := st.toolBufs[idx]; !seen {
				st.toolBufs[idx] = &toolAccum{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				st.toolSeq = append(st.toolSeq, idx)
				st.contentSeen = true
				_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{
					"tool_calls": []any{map[string]any{
						"index": idx,
						"id":    ev.ContentBlock.ID,
						"type":  "function",
						"function": map[string]any{
							"name":      ev.ContentBlock.Name,
							"arguments": "",
						},
					}},
				}, ""))
			}
		}

	case "content_block_delta":
		if ev.Delta == nil {
			return
		}
		switch ev.Delta.Type {
		case "text_delta":
			if ev.Delta.Text != "" {
				st.textBuf.WriteString(ev.Delta.Text)
				st.contentSeen = true
				_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{
					"content": ev.Delta.Text,
				}, ""))
			}
		case "thinking_delta":
			if ev.Delta.Thinking != "" {
				st.contentSeen = true
				_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{
					"reasoning_content": ev.Delta.Thinking,
				}, ""))
			}
		case "input_json_delta":
			idx := ev.Index
			if acc, ok := st.toolBufs[idx]; ok && ev.Delta.PartialJSON != "" {
				acc.args.WriteString(ev.Delta.PartialJSON)
				st.contentSeen = true
				_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{
					"tool_calls": []any{map[string]any{
						"index": idx,
						"function": map[string]any{
							"arguments": ev.Delta.PartialJSON,
						},
					}},
				}, ""))
			}
		}
	case "message_delta":
		if sr := getStopReason([]byte(raw)); sr != "" {
			st.stopReason = sr
		}
		st.applyUsage(raw)
	case "message_stop":
		// 不做处理，外层循环结束前统一补 finish chunk
	case "error":
		// 上游拒绝服务：HTTP 状态仍是 200，错误只在这里出现。
		if ev.Error != nil {
			st.streamErr = ev.Error
		} else {
			st.streamErr = &ErrEvent{Type: "error", Code: "upstream_error", Message: truncate(raw, 300)}
		}
	}
}

// applyUsage 把 message_delta 的 usage 合并进状态：只覆盖真正下发的字段，
// 避免用零值抹掉 message_start 里已经拿到的 input_tokens。
func (st *streamState) applyUsage(raw string) {
	in, hasIn, out, hasOut := getUsage([]byte(raw))
	if hasIn && in > 0 {
		st.usageIn = in
	}
	if hasOut {
		st.usageOut = out
	}
}

// Aggregate 读取完整 Anthropic SSE 流，聚合为 OpenAI 非流式 chat.completion 响应。
// modelOverride 非空时覆盖响应中的 model 字段（用于回显客户端请求的模型名）。
func Aggregate(r io.Reader, modelOverride string) (map[string]any, error) {
	st := &streamState{toolBufs: map[int]*toolAccum{}, usageIn: 0, usageOut: 0}
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimPrefix(line, "data: ")
				if payload != "[DONE]" {
					var ev anthroEvent
					if json.Unmarshal([]byte(payload), &ev) == nil {
						collectEvent(st, &ev, payload)
					}
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	// 同 Stream：错误可能只存在于事件流里，不能当成一次正常的空回复。
	if st.streamErr != nil {
		return nil, &StreamError{
			Event: st.streamErr,
			Kind:  ClassifyStreamErr(st.streamErr.Code, st.streamErr.Message),
		}
	}
	if !st.contentSeen && st.usageOut == 0 {
		logx.Errorf("aggregate: 上游 200 但既无正文也无 usage（model=%q），疑似被静默拒绝", st.model)
	}
	if st.id == "" {
		st.id = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	finish := st.stopReason
	if finish == "" {
		if len(st.toolSeq) > 0 {
			finish = "tool_calls"
		} else {
			finish = "stop"
		}
	}
	msg := openAIMessage{
		Role:    "assistant",
		Content: st.textBuf.String(),
	}
	if len(st.toolSeq) > 0 {
		msg.ToolCalls = st.buildToolCalls()
	}
	u := &usage{
		PromptTokens:     st.usageIn,
		CompletionTokens: st.usageOut,
		TotalTokens:      st.usageIn + st.usageOut,
	}
	if modelOverride != "" {
		st.model = modelOverride
	}
	resp := BuildOpenAIResponse(st.id, st.model, "assistant", msg.Content, finish, msg.ToolCalls, u)
	if reasoning := st.thinkingBuf.String(); reasoning != "" {
		// 推理过程与流式路径保持一致：单独放在 reasoning_content，
		// 不混进 content，避免调用方把思考片段当成答案展示。
		if choices, ok := resp["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if message, ok := choice["message"].(map[string]any); ok {
					message["reasoning_content"] = reasoning
				}
			}
		}
	}
	return resp, nil
}

func collectEvent(st *streamState, ev *anthroEvent, raw string) {
	if ev == nil {
		return
	}
	switch ev.Type {
	case "message_start":
		if ev.Message != nil {
			if st.id == "" {
				st.id = ev.Message.ID
			}
			if st.model == "" {
				st.model = ev.Message.Model
			}
			st.usageIn = ev.Message.Usage.InputTokens
		}
	case "content_block_start":
		if ev.ContentBlock == nil {
			return
		}
		if ev.ContentBlock.Type == "tool_use" {
			idx := ev.Index
			if _, seen := st.toolBufs[idx]; !seen {
				st.toolBufs[idx] = &toolAccum{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				st.toolSeq = append(st.toolSeq, idx)
				st.contentSeen = true
			}
		}
	case "content_block_delta":
		if ev.Delta == nil {
			return
		}
		switch ev.Delta.Type {
		case "text_delta":
			st.textBuf.WriteString(ev.Delta.Text)
			if ev.Delta.Text != "" {
				st.contentSeen = true
			}
		case "thinking_delta":
			st.thinkingBuf.WriteString(ev.Delta.Thinking)
			if ev.Delta.Thinking != "" {
				st.contentSeen = true
			}
		case "input_json_delta":
			idx := ev.Index
			if acc, ok := st.toolBufs[idx]; ok && ev.Delta.PartialJSON != "" {
				acc.args.WriteString(ev.Delta.PartialJSON)
				st.contentSeen = true
			}
		}
	case "message_delta":
		if sr := getStopReason([]byte(raw)); sr != "" {
			st.stopReason = sr
		}
		st.applyUsage(raw)
	case "error":
		if ev.Error != nil {
			st.streamErr = ev.Error
		} else {
			st.streamErr = &ErrEvent{Type: "error", Code: "upstream_error", Message: truncate(raw, 300)}
		}
	}
}

func (st *streamState) buildToolCalls() []openAIToolCall {
	// 按 toolSeq 顺序排序
	out := make([]openAIToolCall, 0, len(st.toolSeq))
	for _, idx := range st.toolSeq {
		acc := st.toolBufs[idx]
		if acc == nil {
			continue
		}
		tc := openAIToolCall{ID: acc.id, Type: "function"}
		tc.Function.Name = acc.name
		tc.Function.Arguments = acc.args.String()
		out = append(out, tc)
	}
	return out
}
