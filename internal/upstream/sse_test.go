package upstream

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAggregate_Basic(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" World"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}
data: [DONE]`

	resp, err := Aggregate(strings.NewReader(sse), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	choices, _ := resp["choices"].([]any)
	if len(choices) == 0 {
		t.Fatal("no choices")
	}
	msg, _ := choices[0].(map[string]any)["message"].(map[string]any)
	if msg == nil {
		t.Fatal("no message")
	}
	content, _ := msg["content"].(string)
	if content != "Hello World" {
		t.Errorf("content = %q, want %q", content, "Hello World")
	}
	finish, _ := choices[0].(map[string]any)["finish_reason"].(string)
	if finish != "end_turn" {
		t.Errorf("finish_reason = %q", finish)
	}
}

func TestAggregate_ToolCalls(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"claude-3-7-sonnet","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"I'll check the weather"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"SF\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":15}}

event: message_stop
data: [DONE]`

	resp, err := Aggregate(strings.NewReader(sse), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	// 序列化回 JSON 便于类型断言（内部可能用 typed struct）
	raw, _ := json.Marshal(resp)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	msg, _ := decoded["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg == nil {
		t.Fatal("no message")
	}
	tcs, _ := msg["tool_calls"].([]any)
	if len(tcs) == 0 {
		t.Logf("full response: %s", raw)
		t.Fatal("no tool_calls")
	}
	tc, _ := tcs[0].(map[string]any)
	if tc["id"] != "toolu_01" {
		t.Errorf("tool id = %v", tc["id"])
	}
	fn, _ := tc["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Errorf("function name = %v", fn["name"])
	}
	args, _ := fn["arguments"].(string)
	if args != `{"city":"SF"}` {
		t.Errorf("arguments = %q, want %q", args, `{"city":"SF"}`)
	}
}

func TestAggregate_Empty(t *testing.T) {
	sse := `data: [DONE]`
	resp, err := Aggregate(strings.NewReader(sse), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	if resp["id"] == "" {
		t.Error("id should not be empty")
	}
}

// TestAggregate_ThinkingNotInContent 回归测试：上游 thinking 片段曾被写进正文，
// 非流式调用方会把思考过程当成答案展示。思考只能出现在 reasoning_content。
func TestAggregate_ThinkingNotInContent(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"deepseek-v4.1-flash","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"用户要我回答 OK，我照做。"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"OK"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":20}}

event: message_stop
data: [DONE]`

	resp, err := Aggregate(strings.NewReader(sse), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	raw, _ := json.Marshal(resp)
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	msg, _ := decoded["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg == nil {
		t.Fatal("no message")
	}
	if content, _ := msg["content"].(string); content != "OK" {
		t.Errorf("content = %q, want %q（思考片段不得混进正文）", content, "OK")
	}
	if reasoning, _ := msg["reasoning_content"].(string); reasoning != "用户要我回答 OK，我照做。" {
		t.Errorf("reasoning_content = %q, want 思考原文", reasoning)
	}
}

// TestAggregate_NoThinkingKeepsContentClean 没有 thinking 块时不应凭空生成 reasoning_content。
func TestAggregate_NoThinkingKeepsContentClean(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_01","type":"message","role":"assistant","content":[],"model":"deepseek-v4.1-flash","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":2}}

event: message_stop
data: [DONE]`

	resp, err := Aggregate(strings.NewReader(sse), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	raw, _ := json.Marshal(resp)
	var decoded map[string]any
	_ = json.Unmarshal(raw, &decoded)
	msg, _ := decoded["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if _, ok := msg["reasoning_content"]; ok {
		t.Error("没有 thinking 块时不应出现 reasoning_content")
	}
	if content, _ := msg["content"].(string); content != "hi" {
		t.Errorf("content = %q, want %q", content, "hi")
	}
}
