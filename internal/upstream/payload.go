// Payload 转换 OpenAI 与 Anthropic 之间的请求/响应格式。
package upstream

import (
	"encoding/json"
	"strings"
	"time"
)

// openAIReq 入站 OpenAI 格式（简化版，只收本服务需要的字段）。
type openAIReq struct {
	Model       string            `json:"model"`
	Messages    []json.RawMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	TopP        *float64          `json:"top_p,omitempty"`
	Stream      bool              `json:"stream"`
	Tools       []json.RawMessage `json:"tools,omitempty"`
	ToolChoice  json.RawMessage   `json:"tool_choice,omitempty"`
	Stop        []string          `json:"stop,omitempty"`
	User        string            `json:"user,omitempty"`
	// 下游客户端的思考档位：Codex 用 reasoning_effort，OpenAI 新格式用 reasoning.effort。
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Reasoning       struct {
		Effort string `json:"effort,omitempty"`
	} `json:"reasoning,omitempty"`
}

// requestEffort 返回客户端声明的思考档位（优先 reasoning_effort）。
func (r openAIReq) requestEffort() string {
	if strings.TrimSpace(r.ReasoningEffort) != "" {
		return r.ReasoningEffort
	}
	return r.Reasoning.Effort
}

// openAIMsg 消息体片段。
type openAIMsg struct {
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content"` // string 或 []contentPart
	ToolCalls  []json.RawMessage `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Name       string            `json:"name,omitempty"`
}

// contentPart Anthropic 风格 content 数组元素。
type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
	// tool_result
	ToolUseID string      `json:"tool_use_id,omitempty"`
	Content   interface{} `json:"content,omitempty"`
	IsError   bool        `json:"is_error,omitempty"`
}

// anthropicReq Anthropic Messages API 请求结构。
type anthropicReq struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	System        string             `json:"system,omitempty"`
	Messages      []anthropicMsg     `json:"messages"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	Stream        bool               `json:"stream"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
	ToolChoice    json.RawMessage    `json:"tool_choice,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Metadata      map[string]string  `json:"metadata,omitempty"`
	Thinking      *anthropicThinking `json:"thinking,omitempty"`
}

// anthropicThinking 上游扩展思考开关。
type anthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type anthropicMsg struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// CanonicalModel 上游公开模型条目。
type CanonicalModel struct {
	ID      string
	Context int // 上下文窗口（token）
}

// CanonicalModels 上游当前公开可选的模型，与官网定价页一致。
// 既是 /v1/models 的数据源，也是别名表的解析目标。
// 注意：账号具体能用哪些取决于套餐，未开放的会返回 403 model_not_allowed。
var CanonicalModels = []CanonicalModel{
	{"phanthy-fast", 1050000},
	{"phanthy-pro", 1050000},
	{"phanthy-ultra", 1050000},
	{"glm-5.3-flash", 1000000},
	{"glm-5.3", 1000000},
	{"glm-5.2", 1000000},
	{"glm-5.1", 200000},
	{"kimi-k3", 1048576},
	{"kimi-k2.7-code", 262144},
	{"deepseek-v4.1-flash", 1048576},
}

// modelSuffixes 客户端可能携带的上下文后缀。
// 官网客户端会给大上下文模型追加 [1m] / [2m]，但上游 /v1/messages 只认裸模型名，
// 带后缀会直接吃 403 model_not_allowed，因此这里统一剥离。
var modelSuffixes = []string{"[1m]", "[2m]", ":1m", ":2m"}

// normalizeModel 归一化客户端模型名：去首尾空白 → 小写 → 去上下文后缀 →
// 内部空白折成 "-"（让 "Kimi K3" 这类展示名落到 kimi-k3）。
func normalizeModel(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	for _, prefix := range []string{"cn:", "global:"} {
		s = strings.TrimPrefix(s, prefix)
	}
	for _, suffix := range modelSuffixes {
		s = strings.TrimSuffix(s, suffix)
	}
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " \t") {
		s = strings.Join(strings.Fields(s), "-")
	}
	return s
}

// ModelAlias 归一化模型名 → 上游模型 ID（键均已做过 normalizeModel）。
//
// 只需登记「必须改名」的条目：能直接归一化成规范 ID 的展示名
// （"Kimi K3" → kimi-k3、"GLM 5.2" → glm-5.2）不用列，归一化后就是正确 ID。
var ModelAlias = map[string]string{
	// Auto：官网 Auto 落在最经济的公开档。
	"auto":         "phanthy-fast",
	"phanthy-auto": "phanthy-fast",

	// 历史商业命名 → 当前公开 ID。
	"deepseek-v4":             "deepseek-v4.1-flash",
	"deepseek-v4.1":           "deepseek-v4.1-flash",
	"phanthy.com/deepseek-v4": "deepseek-v4.1-flash",

	// Claude 展示名用点号，上游 ID 用连字符。
	"claude-opus-4.8":   "claude-opus-4-8",
	"claude-opus-4.7":   "claude-opus-4-7",
	"claude-sonnet-4.6": "claude-sonnet-4-6",
	"claude-haiku-4.5":  "claude-haiku-4-5",

	// GPT 档位：上游把 phanthy-pro / phanthy-ultra 路由到对应后端（响应回显已确认）。
	"gpt-5.6-sol": "phanthy-pro",
	"gpt-6-astra": "phanthy-ultra",

	// 上游内部代号 → 当前公开 ID。
	// 这些代号曾是旧版映射的目标，但已不在上游可调用目录里，直接透传只会拿到 403。
	"iris-1.0":        "deepseek-v4.1-flash",
	"zeus-1.1-pro":    "phanthy-pro",
	"zeus-1.1":        "phanthy-pro",
	"zeus-1.1-fast":   "phanthy-fast",
	"zeus-1.0-pro":    "phanthy-pro",
	"gaia-1.2":        "claude-opus-4-8",
	"gaia-1.1":        "claude-opus-4-7",
	"gaia-1.0":        "claude-sonnet-4-6",
	"apollo-2.0":      "kimi-k3",
	"apollo-1.1":      "kimi-k2.7-code",
	"apollo-1.0":      "kimi-k2.7-code",
	"metis-1.2-flash": "glm-5.3-flash",
	"metis-1.1":       "glm-5.2",
	"metis-1.0":       "glm-5.1",
}

// ResolveModel 解析客户端模型名 → 上游模型 ID：
// 归一化 → 查别名 → 未命中则返回归一化名（未知模型仍交给上游判定）。
func ResolveModel(name string) string {
	n := normalizeModel(name)
	if up, ok := ModelAlias[n]; ok {
		return up
	}
	return n
}

// ThinkingMode 控制是否把扩展思考（extended thinking）下发给上游。
//
// 上游在收到不带 thinking 字段的请求时会自行开启思考，代价是首字延迟成倍上升，
// 而且在 max_tokens 偏小时会出现「整段预算都花在思考上、正文为空」的结果。
// 因此这里总是显式下发 thinking，把开关交给本服务的配置。
type ThinkingMode string

const (
	// ThinkingOff 始终关闭思考，优先保证响应速度与首字延迟。
	ThinkingOff ThinkingMode = "off"
	// ThinkingOn 始终开启思考，预算取 ThinkingOption.Budget。
	ThinkingOn ThinkingMode = "on"
	// ThinkingAuto 交给客户端的思考档位决定：none/minimal/low 或未声明视为不需要思考，
	// 其余档位（medium/high/...）按 Budget 开启。
	ThinkingAuto ThinkingMode = "auto"
)

// ThinkingOption 思考策略，来源于配置项 thinking。
type ThinkingOption struct {
	Mode   ThinkingMode
	Budget int
}

const (
	// DefaultThinkingBudget 启用思考时的默认预算（token）。
	DefaultThinkingBudget = 4096
	// minThinkingBudget 上游要求预算不能过小，低于该值直接回落到默认预算。
	minThinkingBudget = 1024
	// maxThinkingBudget 预算上限，避免客户端把 max_tokens 撑得过大导致长尾请求。
	maxThinkingBudget = 16384

	// defaultMaxTokens 客户端未给 max_tokens 时的兜底输出上限。
	defaultMaxTokens = 8192
	// maxMaxTokens 允许透传给上游的最大输出上限。
	maxMaxTokens = 65536
)

// Normalize 补齐缺省值，非法模式回落到 ThinkingOff。
func (o ThinkingOption) Normalize() ThinkingOption {
	switch o.Mode {
	case ThinkingOff, ThinkingOn, ThinkingAuto:
	default:
		o.Mode = ThinkingOff
	}
	if o.Budget < minThinkingBudget {
		o.Budget = DefaultThinkingBudget
	}
	if o.Budget > maxThinkingBudget {
		o.Budget = maxThinkingBudget
	}
	return o
}

// resolve 按客户端思考档位决定实际下发的 thinking 字段；永远返回非空值，
// 保证上游不会走「默认开思考」的分支。
func (o ThinkingOption) resolve(effort string) *anthropicThinking {
	o = o.Normalize()
	enabled := o.Mode == ThinkingOn
	if o.Mode == ThinkingAuto {
		switch strings.ToLower(strings.TrimSpace(effort)) {
		case "", "none", "minimal", "low":
			enabled = false
		default:
			enabled = true
		}
	}
	if !enabled {
		return &anthropicThinking{Type: "disabled"}
	}
	return &anthropicThinking{Type: "enabled", BudgetTokens: o.Budget}
}

// PrepareBody 将 OpenAI 请求体转为 Anthropic 请求体；无法解析时原样返回。
// thinking 决定下发给上游的扩展思考开关，见 ThinkingOption。
func PrepareBody(src []byte, thinking ThinkingOption) []byte {
	if len(src) == 0 {
		return src
	}
	var req openAIReq
	if err := json.Unmarshal(src, &req); err != nil {
		return src
	}
	// 强制 stream
	req.Stream = true

	anthro := anthropicReq{
		Model:     ResolveModel(req.Model),
		MaxTokens: req.MaxTokens,
		Stream:    true,
	}

	if anthro.MaxTokens <= 0 {
		anthro.MaxTokens = defaultMaxTokens
	}
	if anthro.MaxTokens > maxMaxTokens {
		anthro.MaxTokens = maxMaxTokens
	}

	// 上游要求 max_tokens 严格大于思考预算，否则直接报参数错误。
	anthro.Thinking = thinking.resolve(req.requestEffort())
	if anthro.Thinking.Type == "enabled" && anthro.MaxTokens <= anthro.Thinking.BudgetTokens {
		anthro.MaxTokens = anthro.Thinking.BudgetTokens + 1024
	}

	if req.Temperature != nil {
		anthro.Temperature = req.Temperature
	}
	if req.TopP != nil {
		anthro.TopP = req.TopP
	}
	if len(req.Stop) > 0 {
		anthro.StopSequences = req.Stop
	}
	if req.User != "" {
		anthro.Metadata = map[string]string{"user_id": req.User}
	}

	// 转换 messages
	var systemBuf strings.Builder
	for _, raw := range req.Messages {
		var msg openAIMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Role {
		case "system", "developer":
			content := extractText(msg.Content)
			if systemBuf.Len() > 0 {
				systemBuf.WriteString("\n\n")
			}
			systemBuf.WriteString(content)
		case "user":
			anthro.Messages = append(anthro.Messages, anthropicMsg{
				Role:    "user",
				Content: toContentParts(msg.Content),
			})
		case "assistant":
			parts := toContentParts(msg.Content)
			// 去掉纯空文本块（assistant 可能只有 tool_calls 没有正文）
			filtered := parts[:0]
			for _, p := range parts {
				if p.Type == "text" && strings.TrimSpace(p.Text) == "" {
					continue
				}
				filtered = append(filtered, p)
			}
			parts = filtered
			// 如果有 tool_calls，追加 tool_use 块
			for _, tcRaw := range msg.ToolCalls {
				var tc struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				}
				if json.Unmarshal(tcRaw, &tc) == nil && tc.ID != "" {
					var input map[string]any
					_ = json.Unmarshal(tc.Function.Arguments, &input)
					parts = append(parts, contentPart{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
			}
			// 如果没有任何内容块，跳过
			if len(parts) == 0 {
				continue
			}
			anthro.Messages = append(anthro.Messages, anthropicMsg{
				Role:    "assistant",
				Content: parts,
			})
		case "tool":
			content := extractText(msg.Content)
			parts := []contentPart{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   content,
				IsError:   false,
			}}
			anthro.Messages = append(anthro.Messages, anthropicMsg{
				Role:    "user",
				Content: parts,
			})
		}
	}

	if systemBuf.Len() > 0 {
		anthro.System = systemBuf.String()
	}

	// 转换 tools
	for _, raw := range req.Tools {
		t := convertTool(raw)
		if t != nil {
			anthro.Tools = append(anthro.Tools, *t)
		}
	}

	// 转换 tool_choice
	if len(req.ToolChoice) > 0 {
		anthro.ToolChoice = convertToolChoice(req.ToolChoice)
	}

	out, err := json.Marshal(anthro)
	if err != nil {
		return src
	}
	return out
}

// extractText 从 content（string 或 []contentPart）中提取纯文本。
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试 string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// 尝试数组
	var parts []contentPart
	if json.Unmarshal(raw, &parts) == nil {
		var buf strings.Builder
		for _, p := range parts {
			if p.Type == "text" || p.Type == "" {
				if buf.Len() > 0 {
					buf.WriteString("\n")
				}
				buf.WriteString(p.Text)
			}
		}
		return buf.String()
	}
	return ""
}

// toContentParts 将 OpenAI content（string 或 []contentPart）转为 Anthropic content 数组。
func toContentParts(raw json.RawMessage) []contentPart {
	if len(raw) == 0 {
		return []contentPart{{Type: "text", Text: ""}}
	}
	// 尝试 string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []contentPart{{Type: "text", Text: s}}
	}
	// 尝试数组
	var parts []contentPart
	if json.Unmarshal(raw, &parts) == nil {
		// 已经是 contentPart 格式，直接返回
		for i := range parts {
			if parts[i].Type == "" {
				parts[i].Type = "text"
			}
		}
		return parts
	}
	// 尝试通用结构
	var rawParts []map[string]any
	if json.Unmarshal(raw, &rawParts) == nil {
		result := make([]contentPart, 0, len(rawParts))
		for _, rp := range rawParts {
			cp := contentPart{}
			if t, ok := rp["type"].(string); ok {
				cp.Type = t
			} else {
				cp.Type = "text"
			}
			if t, ok := rp["text"].(string); ok {
				cp.Text = t
			}
			if _, ok := rp["image_url"]; ok && cp.Type == "image_url" {
				// 简化为 text 占位
				cp.Type = "text"
				cp.Text = "[image]"
			}
			result = append(result, cp)
		}
		return result
	}
	return []contentPart{{Type: "text", Text: string(raw)}}
}

// convertTool 将 OpenAI tool 转为 Anthropic tool。
func convertTool(raw json.RawMessage) *anthropicTool {
	var ot struct {
		Type     string `json:"type"`
		Function struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Parameters  any    `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &ot); err != nil {
		return nil
	}
	name := ot.Function.Name
	if name == "" {
		name = ot.Type
	}
	if name == "" {
		return nil
	}
	inputSchema := ot.Function.Parameters
	if inputSchema == nil {
		inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return &anthropicTool{
		Name:        name,
		Description: ot.Function.Description,
		InputSchema: inputSchema,
	}
}

// convertToolChoice 将 OpenAI tool_choice 转为 Anthropic 格式。
func convertToolChoice(raw json.RawMessage) json.RawMessage {
	// 尝试 string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "none":
			return json.RawMessage(`{"type":"none"}`)
		case "auto":
			return json.RawMessage(`{"type":"auto"}`)
		case "required":
			return json.RawMessage(`{"type":"any"}`)
		default:
			return json.RawMessage(`{"type":"auto"}`)
		}
	}
	// 尝试对象
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		switch strings.ToLower(strings.TrimSpace(obj.Type)) {
		case "none":
			return json.RawMessage(`{"type":"none"}`)
		case "auto":
			return json.RawMessage(`{"type":"auto"}`)
		case "required":
			return json.RawMessage(`{"type":"any"}`)
		case "function":
			if obj.Function.Name != "" {
				out, _ := json.Marshal(map[string]any{
					"type": "tool",
					"name": obj.Function.Name,
				})
				return out
			}
			return json.RawMessage(`{"type":"auto"}`)
		}
	}
	return json.RawMessage(`{"type":"auto"}`)
}

// ---------------------------------------------------------------------------
// 响应转换
// ---------------------------------------------------------------------------

// openAIResp 非流式 OpenAI 响应。
type openAIResp struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
}

type choice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// BuildOpenAIResponse 从 Anthropic 聚合参数构建完整的 OpenAI 非流式响应。
func BuildOpenAIResponse(id, model, role, content, finishReason string, tc []openAIToolCall, u *usage) map[string]any {
	msg := map[string]any{
		"role":    role,
		"content": content,
	}
	if len(tc) > 0 {
		msg["tool_calls"] = tc
	}
	resp := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       msg,
				"finish_reason": finishReason,
			},
		},
	}
	if u != nil {
		resp["usage"] = u
	}
	return resp
}

// UsageTotals 从 Aggregate 生成的响应里取出 token 用量，供审计日志记录。
func UsageTotals(resp map[string]any) (in, out int) {
	if u, ok := resp["usage"].(*usage); ok && u != nil {
		return u.PromptTokens, u.CompletionTokens
	}
	return 0, 0
}

// BuildOpenAIStreamChunk 构建单个流式 chunk。
func BuildOpenAIStreamChunk(id, model string, index int, delta map[string]any, finishReason string) map[string]any {
	return map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         index,
				"delta":         delta,
				"finish_reason": finishReason,
			},
		},
	}
}
