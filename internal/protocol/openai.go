// Package protocol OpenAII 兼容层的薄封装。
//
// 协议转换（OpenAI Chat ↔ Anthropic Messages）与 SSE 事件状态机不再自研，
// 改由 new-api 开源的 relaykit（github.com/QuantumNous/new-api/relaykit）承载，
// token 估算由 tokenkit（github.com/QuantumNous/new-api/tokenkit）承载。
// 本包仅保留逆向证据特有的、relaykit 不提供的能力：
//   - 客户端模型别名 → 上游模型 ID（ModelAliases / UpstreamModel / ModelList）
//   - max_tokens 合法性改写（≤0 或 >65536 → 8192，证据 02-usage-semantics.md §2）
//   - 图片块降级为字面量 "[image]"
//   - SSE 帧的读写（relaykit 明确由宿主完成）
package protocol

import "encoding/json"

// 客户端模型别名 → 上游模型 ID。
// 该映射来自逆向报告 Layer B（PRIOR-ART / 未在线验证），未知名称原样透传。
var ModelAliases = map[string]string{
	"DeepSeek-V4":       "Iris-1.0",
	"gpt-5.6-sol":       "Zeus-1.1-pro",
	"gpt-5.6-terra":     "Zeus-1.1",
	"gpt-5.6-luna":      "Zeus-1.1-fast",
	"gpt-5.5":           "Zeus-1.0-pro",
	"Claude Opus 4.8":   "Gaia-1.2",
	"Claude Opus 4.7":   "Gaia-1.1",
	"Claude Sonnet 4.6": "Gaia-1.0",
	"Kimi K3":           "Apollo-2.0",
	"Kimi-k2.7-code":    "Apollo-1.1",
	"Kimi K2.6":         "Apollo-1.0",
	"GLM 5.2":           "Metis-1.1",
	"GLM-5.1":           "Metis-1.0",
}

// UpstreamModel 把客户端模型名映射为上游 ID；未知名称原样返回。
func UpstreamModel(name string) string {
	if v, ok := ModelAliases[name]; ok {
		return v
	}
	return name
}

// ModelEntry 是 /v1/models 返回的单个模型。
type ModelEntry struct {
	ID            string `json:"id"`
	Object        string `json:"object"`
	Created       int64  `json:"created"`
	OwnedBy       string `json:"owned_by"`
	ContextLength int    `json:"context_length"`
}

// ModelList 返回 13 个客户端模型别名（顺序稳定）。
func ModelList() []ModelEntry {
	ids := []string{
		"DeepSeek-V4",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.5",
		"Claude Opus 4.8",
		"Claude Opus 4.7",
		"Claude Sonnet 4.6",
		"Kimi K3",
		"Kimi-k2.7-code",
		"Kimi K2.6",
		"GLM 5.2",
		"GLM-5.1",
	}
	out := make([]ModelEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, ModelEntry{
			ID:            id,
			Object:        "model",
			Created:       0,
			OwnedBy:       "phanthy",
			ContextLength: 200000,
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
