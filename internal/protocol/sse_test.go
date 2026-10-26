package protocol

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

const fixtureStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","model":"Iris-1.0","usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"思考中"}}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"北京\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":42}}

event: message_stop
data: {"type":"message_stop"}

`

func TestCollectorAggregate(t *testing.T) {
	c := NewCollector("Kimi K3", 0)
	if err := c.Run(strings.NewReader(fixtureStream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !c.HasOutput() {
		t.Fatal("fixture should produce output")
	}

	completion := c.Completion()
	if completion.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if completion.Model != "Kimi K3" {
		t.Fatalf("model = %q", completion.Model)
	}
	if completion.Object != "chat.completion" {
		t.Fatalf("object = %q", completion.Object)
	}
	choice := completion.Choices[0]
	if choice.Message.Content != "你好" {
		t.Fatalf("content = %q", choice.Message.Content)
	}
	if choice.Message.ReasoningContent != "思考中" {
		t.Fatalf("reasoning = %q", choice.Message.ReasoningContent)
	}
	if choice.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("tool calls = %v", choice.Message.ToolCalls)
	}
	tool := choice.Message.ToolCalls[0]
	if tool.ID != "toolu_1" || tool.Function.Name != "get_weather" {
		t.Fatalf("tool = %+v", tool)
	}
	if tool.Function.Arguments != `{"city":"北京"}` {
		t.Fatalf("arguments = %q", tool.Function.Arguments)
	}
	if completion.Usage.PromptTokens != 10 || completion.Usage.CompletionTokens != 42 || completion.Usage.TotalTokens != 52 {
		t.Fatalf("usage = %+v", completion.Usage)
	}
}

func TestCollectorStreamEmitsChunks(t *testing.T) {
	c := NewCollector("DeepSeek-V4", 0)
	var frames []*dto.ChatCompletionsStreamResponse
	err := c.Run(strings.NewReader(fixtureStream), func(chunk *dto.ChatCompletionsStreamResponse) error {
		frames = append(frames, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(frames) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(frames))
	}
	// 找到首个非空文本 chunk（message_start 与 content_block_start 会产生空内容 chunk）。
	var first *string
	for _, f := range frames {
		if f.Choices[0].Delta.Content != nil && *f.Choices[0].Delta.Content != "" {
			first = f.Choices[0].Delta.Content
			break
		}
	}
	if first == nil || *first != "你好" {
		t.Fatalf("first non-empty text chunk = %v", first)
	}

	// 找到 tool_use 起始 chunk。
	var toolStart *dto.ToolCallResponse
	for _, f := range frames {
		for _, tc := range f.Choices[0].Delta.ToolCalls {
			if tc.ID == "toolu_1" {
				toolStart = &tc
			}
		}
	}
	if toolStart == nil {
		t.Fatal("expected a tool_use start chunk")
	}
	if toolStart.Function.Name != "get_weather" || toolStart.Index == nil || *toolStart.Index != 0 {
		t.Fatalf("tool start = %+v", toolStart)
	}

	// 各 chunk 都应为 SSE 编码所需字段回填。
	for _, f := range frames {
		raw, err := c.EncodeChunk(f)
		if err != nil {
			t.Fatalf("EncodeChunk: %v", err)
		}
		if !strings.HasPrefix(string(raw), "data: {") || !strings.HasSuffix(string(raw), "\n\n") {
			t.Fatalf("unexpected frame: %q", raw)
		}
	}

	// fixture 的 message_delta 已携带 stop_reason=tool_use，relaykit 已在流内发过
	// finish_reason chunk，因此 FinalChunk() 应为 nil（避免重复帧）。
	if final := c.FinalChunk(); final != nil {
		t.Fatalf("expected no extra final chunk, got %+v", final)
	}
}

func TestCollectorErrorEventIsPropagated(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"id":"m","model":"x"}}

event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"upstream is overloaded"}}

`
	c := NewCollector("m", 0)
	err := c.Run(strings.NewReader(stream), func(*dto.ChatCompletionsStreamResponse) error { return nil })
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("expected StreamError, got %v", err)
	}
	if se.Message != "upstream is overloaded" {
		t.Fatalf("message = %q", se.Message)
	}
}

func TestCollectorEmptyStream(t *testing.T) {
	c := NewCollector("m", 0)
	if err := c.Run(strings.NewReader(""), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if c.HasOutput() {
		t.Fatal("empty stream must not report output")
	}
}

func TestCollectorIgnoresNoiseAndCRLF(t *testing.T) {
	stream := ": keep-alive\r\n" +
		"event: ping\r\n" +
		"data: {\"type\":\"ping\"}\r\n\r\n" +
		"data: not-json\r\n\r\n" +
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"m2\"}}\r\n\r\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\r\n\r\n" +
		"data: [DONE]\r\n\r\n"

	c := NewCollector("m", 0)
	if err := c.Run(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	completion := c.Completion()
	if completion.ID == "" || completion.Choices[0].Message.Content != "ok" {
		t.Fatalf("completion = %+v", completion)
	}
}

func TestCollectorToolWithoutArguments(t *testing.T) {
	stream := `data: {"type":"message_start","message":{"id":"m"}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"noop"}}

data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}

`
	c := NewCollector("m", 0)
	if err := c.Run(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tool := c.Completion().Choices[0].Message.ToolCalls[0]
	if tool.Function.Arguments != "{}" {
		t.Fatalf("empty arguments should be {}, got %q", tool.Function.Arguments)
	}
}

func TestCollectorFinishReasonStopWithoutDelta(t *testing.T) {
	stream := `data: {"type":"message_start","message":{"id":"m"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

`
	c := NewCollector("m", 0)
	if err := c.Run(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := c.FinishReason(); got != "stop" {
		t.Fatalf("finish_reason = %q, want stop", got)
	}
}

func TestCollectorUsageFallback(t *testing.T) {
	// message_delta 无 usage 时，用 estPrompt + tokenkit.Estimate 补算。
	stream := `data: {"type":"message_start","message":{"id":"m"}}

data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello world"}}

data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}

`
	c := NewCollector("m", 5)
	if err := c.Run(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	usage := c.Completion().Usage
	if usage.PromptTokens != 5 {
		t.Fatalf("prompt tokens = %d, want 5", usage.PromptTokens)
	}
	if usage.CompletionTokens == 0 {
		t.Fatal("completion tokens should be estimated, got 0")
	}
	if usage.TotalTokens != usage.PromptTokens+usage.CompletionTokens {
		t.Fatalf("total mismatch: %+v", usage)
	}
}
