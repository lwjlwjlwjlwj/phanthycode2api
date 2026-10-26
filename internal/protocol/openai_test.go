package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("解析请求体失败: %v (%s)", err, body)
	}
	return m
}

func TestUpstreamModelAlias(t *testing.T) {
	if got := UpstreamModel("DeepSeek-V4"); got != "Iris-1.0" {
		t.Fatalf("alias = %q", got)
	}
	if got := UpstreamModel("unknown-model"); got != "unknown-model" {
		t.Fatalf("unknown model should pass through, got %q", got)
	}
	if len(ModelList()) != 13 {
		t.Fatalf("model list should contain 13 entries, got %d", len(ModelList()))
	}
}

func TestBuildAnthropicRequestBasic(t *testing.T) {
	maxTokens := 4096
	temp := 0.3
	req := &ChatRequest{
		Model: "DeepSeek-V4",
		Messages: []ChatMessage{
			{Role: "system", Content: json.RawMessage(`"你是助手"`)},
			{Role: "developer", Content: json.RawMessage(`"保持简洁"`)},
			{Role: "user", Content: json.RawMessage(`"你好"`)},
			{Role: "assistant", Content: json.RawMessage(`"在"`)},
			{Role: "user", Content: json.RawMessage(`"继续"`)},
		},
		MaxTokens:   &maxTokens,
		Temperature: &temp,
		Stop:        json.RawMessage(`"END"`),
		User:        "user-42",
	}

	body, err := BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	m := decode(t, body)

	if m["model"] != "Iris-1.0" {
		t.Fatalf("model = %v", m["model"])
	}
	if m["stream"] != true {
		t.Fatal("upstream must always use stream=true")
	}
	if m["max_tokens"] != float64(4096) {
		t.Fatalf("max_tokens = %v", m["max_tokens"])
	}
	if m["system"] != "你是助手\n\n保持简洁" {
		t.Fatalf("system = %q", m["system"])
	}
	stops, _ := m["stop_sequences"].([]any)
	if len(stops) != 1 || stops[0] != "END" {
		t.Fatalf("stop_sequences = %v", stops)
	}
	if meta, ok := m["metadata"].(map[string]any); !ok || meta["user_id"] != "user-42" {
		t.Fatalf("metadata = %v", m["metadata"])
	}

	msgs, _ := m["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages should drop system, got %d", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "user" {
		t.Fatalf("first message role = %v", first["role"])
	}
}

func TestBuildAnthropicRequestToolCalls(t *testing.T) {
	req := &ChatRequest{
		Model: "Kimi K3",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"天气如何"`)},
			{
				Role:    "assistant",
				Content: json.RawMessage(`"我查一下"`),
				ToolCalls: []ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"北京"}`,
					},
				}},
			},
			{
				Role:       "tool",
				ToolCallID: "call_1",
				Content:    json.RawMessage(`"晴，25 度"`),
			},
		},
		Tools: []Tool{{
			Type: "function",
			Function: FunctionDef{
				Name:        "get_weather",
				Description: "查询天气",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			},
		}},
		ToolChoice: json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`),
	}

	body, err := BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	m := decode(t, body)

	assistant := m["messages"].([]any)[1].(map[string]any)
	blocks := assistant["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("assistant blocks = %v", blocks)
	}
	if blocks[0].(map[string]any)["type"] != "text" {
		t.Fatalf("first block = %v", blocks[0])
	}
	toolBlock := blocks[1].(map[string]any)
	if toolBlock["type"] != "tool_use" || toolBlock["id"] != "call_1" || toolBlock["name"] != "get_weather" {
		t.Fatalf("tool_use block = %v", toolBlock)
	}
	input := toolBlock["input"].(map[string]any)
	if input["city"] != "北京" {
		t.Fatalf("tool input = %v", input)
	}

	result := m["messages"].([]any)[2].(map[string]any)
	resultBlock := result["content"].([]any)[0].(map[string]any)
	if resultBlock["type"] != "tool_result" || resultBlock["tool_use_id"] != "call_1" {
		t.Fatalf("tool_result block = %v", resultBlock)
	}

	tools := m["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	choice := m["tool_choice"].(map[string]any)
	if choice["type"] != "tool" || choice["name"] != "get_weather" {
		t.Fatalf("tool_choice = %v", choice)
	}
}

func TestBuildAnthropicRequestToolChoiceNone(t *testing.T) {
	req := &ChatRequest{
		Model:      "GLM-5.1",
		Messages:   []ChatMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools:      []Tool{{Type: "function", Function: FunctionDef{Name: "f"}}},
		ToolChoice: json.RawMessage(`"none"`),
	}
	body, err := BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	m := decode(t, body)
	if _, ok := m["tools"]; ok {
		t.Fatalf("tools should be dropped when tool_choice=none, got %v", m["tools"])
	}
}

func TestBuildAnthropicRequestMaxTokens(t *testing.T) {
	cases := []struct {
		name string
		req  *ChatRequest
		want int
	}{
		{"默认值", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}}, 8192},
		{"优先 max_tokens", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(100), MaxCompletionTokens: intPtr(200)}, 100},
		{"回退 max_completion_tokens", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxCompletionTokens: intPtr(200)}, 200},
		{"超上限改写为 8192", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(999999)}, 8192},
		{"边界 65536 保留", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(65536)}, 65536},
		{"负数改写为 8192", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(-1)}, 8192},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := BuildAnthropicRequest(tc.req)
			if err != nil {
				t.Fatalf("BuildAnthropicRequest: %v", err)
			}
			if got := decode(t, body)["max_tokens"]; got != float64(tc.want) {
				t.Fatalf("max_tokens = %v, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildAnthropicRequestRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		req  *ChatRequest
	}{
		{"空请求", nil},
		{"缺少 model", &ChatRequest{Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}}},
		{"缺少 messages", &ChatRequest{Model: "m"}},
		{"首条不是 user", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "assistant", Content: json.RawMessage(`"x"`)}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildAnthropicRequest(tc.req); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestUserContentBlocks(t *testing.T) {
	req := &ChatRequest{
		Model: "m",
		Messages: []ChatMessage{{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"text","text":"描述这张图"},
				{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}
			]`),
		}},
	}
	body, err := BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	blocks := decode(t, body)["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %v", blocks)
	}
	if blocks[1].(map[string]any)["text"] != "[image]" {
		t.Fatalf("image block should degrade to placeholder, got %v", blocks[1])
	}
}

func TestMapFinishReason(t *testing.T) {
	cases := map[string]string{
		"max_tokens":    "length",
		"tool_use":      "tool_calls",
		"refusal":       "content_filter",
		"end_turn":      "stop",
		"stop_sequence": "stop",
		"pause_turn":    "stop",
	}
	for in, want := range cases {
		if got := MapFinishReason(in, false); got != want {
			t.Fatalf("MapFinishReason(%q) = %q, want %q", in, got, want)
		}
	}
	if got := MapFinishReason("", true); got != "tool_calls" {
		t.Fatalf("empty stop_reason with tools = %q", got)
	}
	if got := MapFinishReason("", false); got != "stop" {
		t.Fatalf("empty stop_reason without tools = %q", got)
	}
}

func TestParseArgumentsFallback(t *testing.T) {
	req := &ChatRequest{
		Model: "m",
		Messages: []ChatMessage{
			{Role: "user", Content: json.RawMessage(`"x"`)},
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID:       "call_1",
				Function: FunctionCall{Name: "f", Arguments: "not-json"},
			}}},
		},
	}
	body, err := BuildAnthropicRequest(req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	blocks := decode(t, body)["messages"].([]any)[1].(map[string]any)["content"].([]any)
	input := blocks[0].(map[string]any)["input"].(map[string]any)
	if len(input) != 0 {
		t.Fatalf("invalid arguments should degrade to empty object, got %v", input)
	}
}

func intPtr(v int) *int { return &v }

func TestEncodeChunk(t *testing.T) {
	frame, err := EncodeChunk(StreamChunk{ID: "x", Object: "chat.completion.chunk"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(frame), "data: {") || !strings.HasSuffix(string(frame), "\n\n") {
		t.Fatalf("unexpected frame: %q", frame)
	}
}
