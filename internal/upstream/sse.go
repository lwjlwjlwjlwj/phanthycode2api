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
)

// anthroEvent Anthropic SSE 事件通用结构（使用 map 避免字段名冲突）。
type anthroEvent struct {
	Type    string                 `json:"type"`
	Raw     map[string]json.RawMessage `json:"-"`
	Message *struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
	Index       int  `json:"index"`
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
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// parseAnthroEvent 解析 Anthropic SSE 事件，正确处理 message_delta 的 delta 字段。
func parseAnthroEvent(raw []byte) *anthroEvent {
	var ev struct {
		Type string `json:"type"`
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

// getUsage 从 message_delta 事件中提取 usage。
func getUsage(raw []byte) (int, int) {
	var u struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &u) == nil {
		return u.Usage.InputTokens, u.Usage.OutputTokens
	}
	return 0, 0
}

// streamState 流式转换状态。
type streamState struct {
	id       string
	model    string
	gotRole  bool
	textBuf  strings.Builder
	toolBufs map[int]*toolAccum // index → tool 累积
	toolSeq  []int
	usageIn  int
	usageOut int
	stopReason string
}

type toolAccum struct {
	id         string
	name       string
	args       strings.Builder
}

// Stream 将上游 Anthropic SSE 流实时转换为 OpenAI SSE 流写回 w。
// 需要请求方已知 model（入参）。
func Stream(w http.ResponseWriter, r io.Reader, model string) error {
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
			return err
		}
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
		return err
	}
	_, err := io.WriteString(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
	return err
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
				_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{
					"content": ev.Delta.Text,
				}, ""))
			}
		case "thinking_delta":
			if ev.Delta.Thinking != "" {
				_ = emit(BuildOpenAIStreamChunk(st.id, st.model, 0, map[string]any{
					"reasoning_content": ev.Delta.Thinking,
				}, ""))
			}
		case "input_json_delta":
			idx := ev.Index
			if acc, ok := st.toolBufs[idx]; ok && ev.Delta.PartialJSON != "" {
				acc.args.WriteString(ev.Delta.PartialJSON)
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
		if in, out := getUsage([]byte(raw)); out > 0 {
			st.usageOut = out
			st.usageIn = in
		}
	case "message_stop":
		// 不做处理，外层循环结束前统一补 finish chunk
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
	return BuildOpenAIResponse(st.id, st.model, "assistant", msg.Content, finish, msg.ToolCalls, u), nil
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
			}
		}
	case "content_block_delta":
		if ev.Delta == nil {
			return
		}
		switch ev.Delta.Type {
		case "text_delta":
			st.textBuf.WriteString(ev.Delta.Text)
		case "thinking_delta":
			st.textBuf.WriteString(ev.Delta.Thinking)
		case "input_json_delta":
			idx := ev.Index
			if acc, ok := st.toolBufs[idx]; ok && ev.Delta.PartialJSON != "" {
				acc.args.WriteString(ev.Delta.PartialJSON)
			}
		}
	case "message_delta":
		if sr := getStopReason([]byte(raw)); sr != "" {
			st.stopReason = sr
		}
		if in, out := getUsage([]byte(raw)); out > 0 {
			st.usageOut = out
			st.usageIn = in
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