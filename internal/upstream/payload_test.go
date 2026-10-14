package upstream

import (
	"encoding/json"
	"strings"
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
	out := PrepareBody([]byte(in), ThinkingOption{Mode: ThinkingOff})

	var parsed struct {
		Model       string  `json:"model"`
		MaxTokens   int     `json:"max_tokens"`
		System      string  `json:"system"`
		Stream      bool    `json:"stream"`
		Temperature float64 `json:"temperature"`
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
	out := PrepareBody([]byte(in), ThinkingOption{Mode: ThinkingOff})

	var parsed struct {
		Tools []struct {
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
		out := PrepareBody(body, ThinkingOption{Mode: ThinkingOff})
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
	out := PrepareBody([]byte(in), ThinkingOption{Mode: ThinkingOff})

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
		_ = PrepareBody(in, ThinkingOption{Mode: ThinkingOff})
	}
}

func TestResolveModel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// 规范 ID：原样透传（大小写归一化）。
		{"kimi-k3", "kimi-k3"},
		{"Kimi-k3", "kimi-k3"},
		{"glm-5.2", "glm-5.2"},
		{"glm-5.3-flash", "glm-5.3-flash"},
		{"deepseek-v4.1-flash", "deepseek-v4.1-flash"},
		{"phanthy-pro", "phanthy-pro"},

		// 展示名：内部空白折成连字符后落到规范 ID。
		{"Kimi K3", "kimi-k3"},
		{"GLM 5.2", "glm-5.2"},
		{"  Kimi-k2.7-code  ", "kimi-k2.7-code"},

		// 上下文后缀：上游只认裸模型名，必须剥离。
		{"kimi-k3[1m]", "kimi-k3"},
		{"glm-5.2[2m]", "glm-5.2"},
		{"kimi-k3:1m", "kimi-k3"},

		// Auto：落到最经济的公开档。
		{"auto", "phanthy-fast"},

		// 历史商业命名 → 当前公开 ID。
		{"DeepSeek-V4", "deepseek-v4.1-flash"},
		{"Claude Opus 4.8", "claude-opus-4-8"},
		{"claude-sonnet-4.6", "claude-sonnet-4-6"},
		{"gpt-5.6-sol", "phanthy-pro"},

		// 上游内部代号（已不在公开目录）→ 当前公开 ID，避免 403。
		{"Iris-1.0", "deepseek-v4.1-flash"},
		{"Zeus-1.1-pro", "phanthy-pro"},
		{"Gaia-1.2", "claude-opus-4-8"},
		{"Apollo-2.0", "kimi-k3"},
		{"Metis-1.1", "glm-5.2"},

		// 未知模型：返回归一化名，交给上游判定。
		{"unknown-model", "unknown-model"},
		{"Unknown Model", "unknown-model"},
		{"", ""},
	}
	for _, c := range cases {
		got := ResolveModel(c.input)
		if got != c.want {
			t.Errorf("ResolveModel(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestCanonicalModelsAreResolvable 保证 /v1/models 里的每个 ID 都能被解析回来，
// 即别名表不会把某个规范 ID 改写掉，否则列表与实际调用会不一致。
func TestCanonicalModelsAreResolvable(t *testing.T) {
	for _, m := range CanonicalModels {
		if got := ResolveModel(m.ID); got != m.ID {
			t.Errorf("ResolveModel(%q) = %q, want 规范 ID 不被改写", m.ID, got)
		}
	}
}

// TestModelAliasTargetsAreReal 防止别名表指向已下线的上游代号。
// 旧版映射把客户端模型翻译成 iris/zeus/gaia/apollo/metis 这类内部代号，
// 而这些代号已不在上游可调用目录中，导致所有请求吃 403；这里锁死方向。
func TestModelAliasTargetsAreReal(t *testing.T) {
	deadPrefixes := []string{"iris-", "zeus-", "gaia-", "apollo-", "metis-"}
	for from, to := range ModelAlias {
		for _, prefix := range deadPrefixes {
			if strings.HasPrefix(to, prefix) {
				t.Errorf("ModelAlias[%q] = %q 指向已下线的内部代号，会稳定拿到 403", from, to)
			}
		}
		// 键必须是归一化后的形态，否则客户端实际发送的名字查不到。
		if from != normalizeModel(from) {
			t.Errorf("ModelAlias 键 %q 未归一化（应为 %q）", from, normalizeModel(from))
		}
	}
}

// TestNormalizeModel 确认归一化与官网客户端的处理顺序一致。
func TestNormalizeModel(t *testing.T) {
	cases := map[string]string{
		"  Kimi-K3  ":     "kimi-k3",
		"GLM 5.2":         "glm-5.2",
		"kimi-k3[1m]":     "kimi-k3",
		"kimi-k3[2M]":     "kimi-k3",
		"claude-opus-4.8": "claude-opus-4.8",
		"":                "",
	}
	for in, want := range cases {
		if got := normalizeModel(in); got != want {
			t.Errorf("normalizeModel(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- 扩展思考（extended thinking）策略 ---

// thinkingOf 解析 PrepareBody 输出里的 thinking 字段。
func thinkingOf(t *testing.T, out []byte) anthropicThinking {
	t.Helper()
	var parsed struct {
		Thinking *anthropicThinking `json:"thinking"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v (out=%s)", err, out)
	}
	if parsed.Thinking == nil {
		t.Fatalf("payload 未下发 thinking 字段：%s", out)
	}
	return *parsed.Thinking
}

// TestPrepareBody_ThinkingAlwaysExplicit 锁死「永远显式下发 thinking」这一前提。
// 上游收不到该字段时会自行开启思考，首字延迟从 1~2 秒涨到 10 秒以上。
func TestPrepareBody_ThinkingAlwaysExplicit(t *testing.T) {
	in := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"}]}`)
	for _, opt := range []ThinkingOption{
		{Mode: ThinkingOff},
		{Mode: ThinkingOn},
		{Mode: ThinkingAuto},
		{},
	} {
		if got := thinkingOf(t, PrepareBody(in, opt)); got.Type != "disabled" && got.Type != "enabled" {
			t.Errorf("option %+v: thinking.type = %q, want disabled/enabled", opt, got.Type)
		}
	}
}

// TestPrepareBody_ThinkingAuto 覆盖「客户端档位 -> 思考开关」的映射。
func TestPrepareBody_ThinkingAuto(t *testing.T) {
	cases := []struct {
		name   string
		effort string
		nested bool
		want   string
	}{
		{name: "未声明档位", effort: "", want: "disabled"},
		{name: "none", effort: "none", want: "disabled"},
		{name: "minimal", effort: "minimal", want: "disabled"},
		{name: "low", effort: "low", want: "disabled"},
		{name: "medium", effort: "medium", want: "enabled"},
		{name: "high", effort: "high", want: "enabled"},
		{name: "大写档位归一化", effort: "HIGH", want: "enabled"},
		{name: "嵌套 reasoning.effort", effort: "high", nested: true, want: "enabled"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := `{"model":"m","messages":[{"role":"user","content":"hi"}]`
			if c.effort != "" {
				if c.nested {
					body += `,"reasoning":{"effort":"` + c.effort + `"}`
				} else {
					body += `,"reasoning_effort":"` + c.effort + `"`
				}
			}
			body += `}`
			got := thinkingOf(t, PrepareBody([]byte(body), ThinkingOption{Mode: ThinkingAuto, Budget: 2048}))
			if got.Type != c.want {
				t.Errorf("effort=%q -> thinking.type = %q, want %q", c.effort, got.Type, c.want)
			}
		})
	}
}

// TestPrepareBody_ThinkingAutoTopLevelWins 顶层 reasoning_effort 优先于嵌套字段。
func TestPrepareBody_ThinkingAutoTopLevelWins(t *testing.T) {
	body := []byte(`{"model":"m","reasoning_effort":"low","reasoning":{"effort":"high"},"messages":[]}`)
	if got := thinkingOf(t, PrepareBody(body, ThinkingOption{Mode: ThinkingAuto})); got.Type != "disabled" {
		t.Errorf("thinking.type = %q, want disabled（顶层 low 应覆盖嵌套 high）", got.Type)
	}
}

// TestPrepareBody_ThinkingOnBudget 开启思考时必须带预算，且 max_tokens 要大于预算。
func TestPrepareBody_ThinkingOnBudget(t *testing.T) {
	in := []byte(`{"model":"m","max_tokens":1000,"messages":[{"role":"user","content":"hi"}]}`)
	out := PrepareBody(in, ThinkingOption{Mode: ThinkingOn, Budget: 4096})
	got := thinkingOf(t, out)
	if got.Type != "enabled" || got.BudgetTokens != 4096 {
		t.Fatalf("thinking = %+v, want enabled/4096", got)
	}
	var parsed struct {
		MaxTokens int `json:"max_tokens"`
	}
	_ = json.Unmarshal(out, &parsed)
	if parsed.MaxTokens <= got.BudgetTokens {
		t.Errorf("max_tokens = %d 必须大于 thinking.budget_tokens = %d，否则上游报参数错误",
			parsed.MaxTokens, got.BudgetTokens)
	}
}

// TestPrepareBody_ThinkingOffOmitsBudget 关闭思考时不带预算，避免上游误读。
func TestPrepareBody_ThinkingOffOmitsBudget(t *testing.T) {
	out := PrepareBody([]byte(`{"model":"m","messages":[]}`), ThinkingOption{Mode: ThinkingOff, Budget: 4096})
	if !strings.Contains(string(out), `"thinking":{"type":"disabled"}`) {
		t.Errorf("thinking 应为 {\"type\":\"disabled\"}，实际输出：%s", out)
	}
}

// TestPrepareBody_MaxTokensFallback 未给 max_tokens 时补默认值，超大值截到上限。
func TestPrepareBody_MaxTokensFallback(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"缺省补默认值", `{"model":"m","messages":[]}`, defaultMaxTokens},
		{"合法值透传", `{"model":"m","max_tokens":2048,"messages":[]}`, 2048},
		{"超大值截断", `{"model":"m","max_tokens":100000,"messages":[]}`, maxMaxTokens},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var parsed struct {
				MaxTokens int `json:"max_tokens"`
			}
			_ = json.Unmarshal(PrepareBody([]byte(c.in), ThinkingOption{Mode: ThinkingOff}), &parsed)
			if parsed.MaxTokens != c.want {
				t.Errorf("max_tokens = %d, want %d", parsed.MaxTokens, c.want)
			}
		})
	}
}

// TestThinkingOptionNormalize 非法或越界配置回落到安全值。
func TestThinkingOptionNormalize(t *testing.T) {
	cases := []struct {
		in   ThinkingOption
		want ThinkingOption
	}{
		{ThinkingOption{Mode: "bogus"}, ThinkingOption{Mode: ThinkingOff, Budget: DefaultThinkingBudget}},
		{ThinkingOption{Mode: ThinkingOff}, ThinkingOption{Mode: ThinkingOff, Budget: DefaultThinkingBudget}},
		{ThinkingOption{Mode: ThinkingOn, Budget: 10}, ThinkingOption{Mode: ThinkingOn, Budget: DefaultThinkingBudget}},
		{ThinkingOption{Mode: ThinkingAuto, Budget: 999999}, ThinkingOption{Mode: ThinkingAuto, Budget: maxThinkingBudget}},
		{ThinkingOption{Mode: ThinkingAuto, Budget: 4096}, ThinkingOption{Mode: ThinkingAuto, Budget: 4096}},
	}
	for _, c := range cases {
		if got := c.in.Normalize(); got != c.want {
			t.Errorf("%+v.Normalize() = %+v, want %+v", c.in, got, c.want)
		}
	}
}
