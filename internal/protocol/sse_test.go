package protocol

import (
	"errors"
	"strings"
	"testing"
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
	c := NewCollector("Kimi K3")
	if err := c.Run(strings.NewReader(fixtureStream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !c.HasOutput() {
		t.Fatal("fixture should produce output")
	}

	completion := c.Completion()
	if completion.ID != "msg_1" {
		t.Fatalf("id = %q", completion.ID)
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
	c := NewCollector("DeepSeek-V4")
	var frames []StreamChunk
	err := c.Run(strings.NewReader(fixtureStream), func(chunk StreamChunk) error {
		frames = append(frames, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 期望：role、text、thinking、tool 注册、两段参数 = 6 个 chunk。
	if len(frames) != 6 {
		t.Fatalf("expected 6 chunks, got %d", len(frames))
	}
	if frames[0].Choices[0].Delta.Role != "assistant" {
		t.Fatalf("first chunk role = %q", frames[0].Choices[0].Delta.Role)
	}
	if frames[1].Choices[0].Delta.Content != "你好" {
		t.Fatalf("second chunk content = %q", frames[1].Choices[0].Delta.Content)
	}
	if frames[2].Choices[0].Delta.ReasoningContent != "思考中" {
		t.Fatalf("third chunk reasoning = %q", frames[2].Choices[0].Delta.ReasoningContent)
	}
	toolStart := frames[3].Choices[0].Delta.ToolCalls[0]
	if toolStart.ID != "toolu_1" || toolStart.Function.Name != "get_weather" || toolStart.Index != 0 {
		t.Fatalf("tool start = %+v", toolStart)
	}
	if got := frames[4].Choices[0].Delta.ToolCalls[0].Function.Arguments; got != `{"city":` {
		t.Fatalf("argument fragment = %q", got)
	}

	for _, frame := range frames {
		if frame.Object != "chat.completion.chunk" {
			t.Fatalf("object = %q", frame.Object)
		}
		if frame.Model != "DeepSeek-V4" {
			t.Fatalf("model = %q", frame.Model)
		}
		if frame.Choices[0].FinishReason != nil {
			t.Fatalf("intermediate chunk must not carry finish_reason: %+v", frame)
		}
	}

	final := c.FinalChunk()
	if final.Choices[0].FinishReason == nil || *final.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("final chunk = %+v", final)
	}
}

func TestCollectorErrorEventIsPropagated(t *testing.T) {
	stream := `event: message_start
data: {"type":"message_start","message":{"id":"m","model":"x"}}

event: error
data: {"type":"error","error":{"type":"overloaded_error","message":"upstream is overloaded"}}

`
	c := NewCollector("m")
	err := c.Run(strings.NewReader(stream), func(chunk StreamChunk) error { return nil })
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("expected StreamError, got %v", err)
	}
	if se.Message != "upstream is overloaded" {
		t.Fatalf("message = %q", se.Message)
	}
}

func TestCollectorEmptyStream(t *testing.T) {
	c := NewCollector("m")
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

	c := NewCollector("m")
	if err := c.Run(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	completion := c.Completion()
	if completion.ID != "m2" || completion.Choices[0].Message.Content != "ok" {
		t.Fatalf("completion = %+v", completion)
	}
}

func TestCollectorToolWithoutArguments(t *testing.T) {
	stream := `data: {"type":"message_start","message":{"id":"m"}}

data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"noop"}}

data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}

`
	c := NewCollector("m")
	if err := c.Run(strings.NewReader(stream), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tool := c.Completion().Choices[0].Message.ToolCalls[0]
	if tool.Function.Arguments != "{}" {
		t.Fatalf("empty arguments should be {}, got %q", tool.Function.Arguments)
	}
}
