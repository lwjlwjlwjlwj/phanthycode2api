package upstream

import (
	"encoding/json"
	"testing"
)

func TestPrepareBody_Basic(t *testing.T) {
	in := `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 4096,
		"stream": true,
		"messages": [
			{"role": "system", "content": "You are helpful"},
			{"role": "user", "content": "hi"}
		],
		"temperature": 0.5
	}`
	out := PrepareBody([]byte(in))

	var parsed struct {
		Model       string          `json:"model"`
		MaxTokens   int             `json:"max_tokens"`
		System      string          `json:"system"`
		Stream      bool            `json:"stream"`
		Temperature float64         `json:"temperature"`
		Messages    []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %s", parsed.Model)
	}
	if parsed.System != "You are helpful" {
		t.Errorf("system = %q", parsed.System)
	}
	if !parsed.Stream {
		t.Error("stream must be true")
	}
	if len(parsed.Messages) != 1 || parsed.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", parsed.Messages)
	}
	if len(parsed.Messages[0].Content) != 1 || parsed.Messages[0].Content[0].Text != "hi" {
		t.Errorf("content = %+v", parsed.Messages[0].Content)
	}
}

func TestPrepareBody_Tools(t *testing.T) {
	in := `{
		"model": "claude-3-7-sonnet",
		"messages": [{"role": "user", "content": "what's the weather"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
			}
		}],
		"tool_choice": {"type": "function", "function": {"name": "get_weather"}}
	}`
	out := PrepareBody([]byte(in))

	var parsed struct {
		Tools      []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema any    `json:"input_schema"`
		} `json:"tools"`
		ToolChoice struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"tool_choice"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Tools) != 1 {
		t.Fatalf("tools = %+v", parsed.Tools)
	}
	if parsed.Tools[0].Name != "get_weather" {
		t.Errorf("tool name = %s", parsed.Tools[0].Name)
	}
	if parsed.ToolChoice.Type != "tool" || parsed.ToolChoice.Name != "get_weather" {
		t.Errorf("tool_choice = %+v", parsed.ToolChoice)
	}
}

func TestPrepareBody_ToolChoiceStrings(t *testing.T) {
	cases := map[string]string{
		`"none"`:     "none",
		`"auto"`:     "auto",
		`"required"`: "any",
	}
	for in, want := range cases {
		body := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}],"tool_choice":` + in + `}`)
		out := PrepareBody(body)
		var parsed struct {
			ToolChoice json.RawMessage `json:"tool_choice"`
		}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var got struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(parsed.ToolChoice, &got)
		if got.Type != want {
			t.Errorf("tool_choice %s → type=%q, want %q", in, got.Type, want)
		}
	}
}

func TestPrepareBody_ToolResults(t *testing.T) {
	in := `{
		"model": "claude-3-7-sonnet",
		"messages": [
			{"role": "user", "content": "weather in SF?"},
			{"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"SF\"}"}}]},
			{"role": "tool", "tool_call_id": "call_1", "content": "Sunny"}
		]
	}`
	out := PrepareBody([]byte(in))

	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				Text      string `json:"text"`
				ToolUseID string `json:"tool_use_id"`
				Name      string `json:"name"`
				ID        string `json:"id"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(parsed.Messages))
	}
	assistant := parsed.Messages[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 1 || assistant.Content[0].Type != "tool_use" {
		t.Errorf("assistant msg = %+v", assistant)
	}
	toolMsg := parsed.Messages[2]
	if toolMsg.Role != "user" || toolMsg.Content[0].Type != "tool_result" || toolMsg.Content[0].ToolUseID != "call_1" {
		t.Errorf("tool msg = %+v", toolMsg)
	}
}

func BenchmarkPrepareBody(b *testing.B) {
	in := []byte(`{"model":"claude-sonnet-4-6","max_tokens":4096,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	for i := 0; i < b.N; i++ {
		_ = PrepareBody(in)
	}
}

func TestResolveModel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"DeepSeek-V4", "Iris-1.0"},
		{"gpt-5.6-sol", "Zeus-1.1-pro"},
		{"gpt-5.6-terra", "Zeus-1.1"},
		{"gpt-5.6-luna", "Zeus-1.1-fast"},
		{"gpt-5.5", "Zeus-1.0-pro"},
		{"Claude Opus 4.8", "Gaia-1.2"},
		{"Claude Opus 4.7", "Gaia-1.1"},
		{"Claude Sonnet 4.6", "Gaia-1.0"},
		{"Kimi K3", "Apollo-2.0"},
		{"Kimi-k2.7-code", "Apollo-1.1"},
		{"Kimi K2.6", "Apollo-1.0"},
		{"GLM 5.2", "Metis-1.1"},
		{"GLM-5.1", "Metis-1.0"},
		{"unknown-model", "unknown-model"},
		{"", ""},
	}
	for _, c := range cases {
		got := ResolveModel(c.input)
		if got != c.want {
			t.Errorf("ResolveModel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}