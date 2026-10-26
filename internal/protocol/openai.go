// Package protocol OpenAI 兼容层的薄封装。
//
// 协议转换（OpenAI Chat ↔ Anthropic Messages）与 SSE 事件状态机不再自研，
// 改由 new-api 开源的 relaykit（github.com/QuantumNous/new-api/relaykit）承载，
// token 估算由 tokenkit（github.com/QuantumNous/new-api/tokenkit）承载。
// 本包仅保留逆向证据特有的、relaykit 不提供的能力：
//   - /v1/models 展示列表（Layer A，PUBLIC-MARKETING 等级，见 M6-public-model-catalog.txt）
//   - max_tokens 合法性改写（≤0 或 >65536 → 8192，证据 02-usage-semantics.md §2）
//   - 图片块降级为字面量 "[image]"
//   - SSE 帧的读写（relaykit 明确由宿主完成）
//
// 模型名处理遵循证据 02-usage-semantics.md §6.3：两种词汇都应作为透传名接受，
// 请求中的 model 原样透传，不做别名映射、不设 allow-list。旧版携带的
// 13 条 客户端→上游ID 映射属于 Layer B（PRIOR-ART / unverified live，
// §6.2），来源是不可信的仓库代码，本重写已按"完全重写、不参考旧版"原则删除。
package protocol

import "encoding/json"

// ModelEntry 是 /v1/models 返回的单个模型。
// 只声明有证据的字段：上下文窗口（200k）来自旧版静态列表且无上游证据
// （§6.4：No upstream evidence），因此不再声明 context_length。
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelList 返回模型展示列表。
//
// 名单逐字取自公开定价页 /token-plan 的"可用模型列表"（PUBLIC-MARKETING 等级，
// re/evidence/M6-public-model-catalog.txt:27-43，sha256 已校验）。这些是营销
// 展示名而非 wire 模型 ID（§6.1：not published：model id / API slug），仅供
// /v1/models 向客户端展示；任意模型名请求都会原样透传（§6.3），本表既不是
// allow-list，也不参与任何名称映射。
func ModelList() []ModelEntry {
	ids := []string{
		"Phanthy-fast",
		"Phanthy-pro",
		"Phanthy-ultra",
		"glm-5.3-flash",
		"glm-5.3",
		"glm-5.2",
		"kimi-k3",
		"kimi-k2.7-code",
		"deepseek-v4.1-flash-1017",
		"Phana-5.5o",
		"Phana-5.5s",
		"Phano-6a",
		"Phano-6.1s",
	}
	out := make([]ModelEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, ModelEntry{
			ID:      id,
			Object:  "model",
			Created: 0,
			OwnedBy: "phanthy",
		})
	}
	return out
}

// ChatRequest 是 OpenAI /v1/chat/completions 请求体的子集。
// 充当网关入口 DTO，随后交由 relaykit 转换为 Anthropic Messages 请求。
type ChatRequest struct {
	Model               string          `json:"model"`
	Messages            []ChatMessage   `json:"messages"`
	MaxTokens           *int            `json:"max_tokens"`
	MaxCompletionTokens *int            `json:"max_completion_tokens"`
	Temperature         *float64        `json:"temperature"`
	TopP                *float64        `json:"top_p"`
	Stream              bool            `json:"stream"`
	Stop                json.RawMessage `json:"stop"`
	Tools               []Tool          `json:"tools"`
	ToolChoice          json.RawMessage `json:"tool_choice"`
	User                string          `json:"user"`
}

// ChatMessage 是 OpenAI 消息。
type ChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []ToolCall      `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
}

// ToolCall 是 OpenAI 的 function 调用。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 是函数名与参数（JSON 字符串）。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool 是 OpenAI 的工具定义。
type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef 是 JSON Schema 形式的函数定义。
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Completion 是 OpenAI 非流式响应。
type Completion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice 是单个候选回复。
type Choice struct {
	Index        int             `json:"index"`
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

// ResponseMessage 是助手消息。
type ResponseMessage struct {
	Role             string             `json:"role"`
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content,omitempty"`
	ToolCalls        []ResponseToolCall `json:"tool_calls,omitempty"`
}

// ResponseToolCall 是 OpenAI 响应中的工具调用。
type ResponseToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// Usage 是 OpenAI token 用量。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
