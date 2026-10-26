package protocol

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
)

func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("解析请求体失败: %v (%s)", err, body)
	}
	return m
}

func TestModelList(t *testing.T) {
	// 名单取自公开定价页 /token-plan 的可用模型列表（Layer A，PUBLIC-MARKETING），共 13 条。
	entries := ModelList()
	if len(entries) != 13 {
		t.Fatalf("model list should contain 13 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Object != "model" || e.OwnedBy != "phanthy" {
			t.Fatalf("entry = %+v", e)
		}
	}
	if entries[0].ID != "Phanthy-fast" {
		t.Fatalf("first entry = %q", entries[0].ID)
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

	body, err := BuildAnthropicRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	m := decode(t, body)

	// 模型名原样透传（证据 §6.3，不做别名映射；Layer B 映射已删除）。
	if m["model"] != "DeepSeek-V4" {
		t.Fatalf("model = %v", m["model"])
	}
	if m["stream"] != true {
		t.Fatal("upstream must always use stream=true")
	}
	if m["max_tokens"] != float64(4096) {
		t.Fatalf("max_tokens = %v", m["max_tokens"])
	}
	// manufacturer/developer 消息被 relaykit 合并进 system 数组（空白拼接，非 \n\n）。
	sys, _ := m["system"].([]any)
	if len(sys) != 1 {
		t.Fatalf("system = %v", m["system"])
	}
	sysText := sys[0].(map[string]any)["text"]
	if sysText != "你是助手 保持简洁" {
		t.Fatalf("system text = %q", sysText)
	}
	stops, _ := m["stop_sequences"].([]any)
	if len(stops) != 1 || stops[0] != "END" {
		t.Fatalf("stop_sequences = %v", stops)
	}
	// relaykit 不透传 metadata.user_id，故不应对其断言。

	msgs, _ := m["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages should drop system, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "user" {
		t.Fatalf("first message role = %v", msgs[0])
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

	body, err := BuildAnthropicRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	m := decode(t, body)

	// 模型名原样透传（不做别名映射）。
	if m["model"] != "Kimi K3" {
		t.Fatalf("model = %v", m["model"])
	}

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

	// tool 结果并入前一条 user 消息，成为带 tool_result 的内容数组。
	result := m["messages"].([]any)[2].(map[string]any)
	if result["role"].(string) != "user" {
		t.Fatalf("tool result should merge into user role, got %v", result["role"])
	}
	resultBlock := result["content"].([]any)[0].(map[string]any)
	if resultBlock["type"] != "tool_result" || resultBlock["tool_use_id"] != "call_1" {
		t.Fatalf("tool_result block = %v", resultBlock)
	}

	tools := m["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	toolDef := tools[0].(map[string]any)
	if toolDef["name"] != "get_weather" {
		t.Fatalf("tool def = %v", toolDef)
	}
	if _, ok := toolDef["input_schema"]; !ok {
		t.Fatalf("tool def missing input_schema: %v", toolDef)
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
	body, err := BuildAnthropicRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	m := decode(t, body)
	// relaykit 对 "none" 仍保留 tools，只把 tool_choice 设为 {type:"none"}。
	if _, ok := m["tools"]; !ok {
		t.Fatal("tools should be retained even when tool_choice=none")
	}
	choice, ok := m["tool_choice"].(map[string]any)
	if !ok || choice["type"] != "none" {
		t.Fatalf("tool_choice = %v", m["tool_choice"])
	}
}

func TestBuildAnthropicRequestMaxTokens(t *testing.T) {
	cases := []struct {
		name string
		req  *ChatRequest
		want int
	}{
		{"默认值", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}}, 8192},
		{"优先 max_completion_tokens", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(100), MaxCompletionTokens: intPtr(200)}, 200},
		{"仅 max_tokens", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(100)}, 100},
		{"仅 max_completion_tokens", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxCompletionTokens: intPtr(200)}, 200},
		{"超上限改写为 8192", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(999999)}, 8192},
		{"多字段超上限改写", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(100), MaxCompletionTokens: intPtr(999999)}, 8192},
		{"边界 65536 保留", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(65536)}, 65536},
		{"负数改写为 8192", &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}, MaxTokens: intPtr(-1)}, 8192},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := BuildAnthropicRequest(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("BuildAnthropicRequest: %v", err)
			}
			if got := decode(t, body)["max_tokens"]; got != float64(tc.want) {
				t.Fatalf("max_tokens = %v, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildAnthropicRequestValidates(t *testing.T) {
	// 这些请求在进入 relaykit 之前即被判为非法（网关以 400 处理）。
	rejects := []struct {
		name string
		req  *ChatRequest
	}{
		{"空请求", nil},
		{"缺少 model", &ChatRequest{Messages: []ChatMessage{{Role: "user", Content: json.RawMessage(`"x"`)}}}},
		{"缺少 messages", &ChatRequest{Model: "m"}},
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildAnthropicRequest(context.Background(), tc.req); err == nil {
				t.Fatal("expected error")
			}
		})
	}

	// 首条不是 user：relaykit 会插入 "..." 占位 user 而非报错。
	accepted := &ChatRequest{Model: "m", Messages: []ChatMessage{{Role: "assistant", Content: json.RawMessage(`"x"`)}}}
	if _, err := BuildAnthropicRequest(context.Background(), accepted); err != nil {
		t.Fatalf("leading assistant message should be accepted via placeholder: %v", err)
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
	body, err := BuildAnthropicRequest(context.Background(), req)
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

func TestFinishReasonMapping(t *testing.T) {
	// 用 relaykit 的 reasonmap 验证 stop_reason → OpenAI finish_reason 映射。
	cases := map[string]string{
		"max_tokens":    "length",
		"tool_use":      "tool_calls",
		"refusal":       "content_filter",
		"end_turn":      "stop",
		"stop_sequence": "stop",
		"pause_turn":    "length",
	}
	for in, want := range cases {
		if got := relayconvert.StopReasonClaudeToOpenAI(in); got != want {
			t.Fatalf("StopReasonClaudeToOpenAI(%q) = %q, want %q", in, got, want)
		}
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
	body, err := BuildAnthropicRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("BuildAnthropicRequest: %v", err)
	}
	blocks := decode(t, body)["messages"].([]any)[1].(map[string]any)["content"].([]any)
	// 无文本的 assistant 消息先被填成 "..." 文本块，tool_use 排在索引 1。
	toolBlock := blocks[1].(map[string]any)
	if toolBlock["type"] != "tool_use" {
		t.Fatalf("expected tool_use block, got %v", blocks)
	}
	// 非法 arguments 退化为空对象；relaykit 的 input 为空 map 时被 omitempty 丢弃。
	if input, ok := toolBlock["input"].(map[string]any); ok && len(input) != 0 {
		t.Fatalf("invalid arguments should degrade to empty object, got %v", input)
	}
}

func intPtr(v int) *int { return &v }

func TestCollectorEncodeChunk(t *testing.T) {
	reason := "stop"
	c := NewCollector("m", 0)
	frame, err := c.EncodeChunk(&dto.ChatCompletionsStreamResponse{
		Object: "chat.completion.chunk",
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{Content: &[]string{"hi"}[0]},
			FinishReason: &reason,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(frame)
	if !strings.HasPrefix(s, "data: {") || !strings.HasSuffix(s, "\n\n") {
		t.Fatalf("unexpected frame: %q", s)
	}
	var decoded SSEChunk
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(s, "data: "), "\n")), &decoded); err != nil {
		t.Fatalf("frame not valid SSE JSON: %v", err)
	}
	if decoded.Object != "chat.completion.chunk" || decoded.Model != "m" {
		t.Fatalf("decoded = %+v", decoded)
	}
}
