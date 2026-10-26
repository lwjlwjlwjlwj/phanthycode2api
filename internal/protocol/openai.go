// Package protocol 负责 OpenAI Chat Completions 与 Anthropic Messages 之间的协议转换。
package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ChatRequest 是 OpenAI /v1/chat/completions 请求体的子集。
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

// anthropicRequest 是发往 /v1/messages 的请求体。
type anthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []anthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	Stream        bool               `json:"stream"`
	Tools         []anthropicTool    `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Metadata      *anthropicMeta     `json:"metadata,omitempty"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type anthropicMeta struct {
	UserID string `json:"user_id"`
}

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

// BuildAnthropicRequest 把 OpenAI 请求转换为 Anthropic Messages 请求体。
// 上游始终以 stream=true 调用，非流式响应由本地聚合产生。
func BuildAnthropicRequest(req *ChatRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("请求为空")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("model 不能为空")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages 不能为空")
	}

	ar := anthropicRequest{
		Model:       UpstreamModel(req.Model),
		MaxTokens:   maxTokens(req),
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      true,
	}

	var systemParts []string
	var messages []anthropicMessage

	appendMessage := func(role string, blocks []anthropicBlock) {
		if len(blocks) == 0 {
			return
		}
		if n := len(messages); n > 0 && messages[n-1].Role == role {
			messages[n-1].Content = append(messages[n-1].Content, blocks...)
			return
		}
		messages = append(messages, anthropicMessage{Role: role, Content: blocks})
	}

	for _, msg := range req.Messages {
		switch strings.ToLower(msg.Role) {
		case "system", "developer":
			if text := contentText(msg.Content); text != "" {
				systemParts = append(systemParts, text)
			}
		case "user":
			blocks := userBlocks(msg.Content)
			if len(blocks) > 0 {
				appendMessage("user", blocks)
			}
		case "assistant":
			var blocks []anthropicBlock
			if text := contentText(msg.Content); text != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: text})
			}
			for _, tc := range msg.ToolCalls {
				name := tc.Function.Name
				if name == "" {
					continue
				}
				blocks = append(blocks, anthropicBlock{
					Type:  "tool_use",
					ID:    toolUseID(tc.ID),
					Name:  name,
					Input: parseArguments(tc.Function.Arguments),
				})
			}
			if len(blocks) > 0 {
				appendMessage("assistant", blocks)
			}
		case "tool", "function":
			if msg.ToolCallID == "" {
				// 缺少 tool_call_id 时退化为普通用户文本，避免构造非法块。
				if text := contentText(msg.Content); text != "" {
					appendMessage("user", []anthropicBlock{{Type: "text", Text: text}})
				}
				continue
			}
			appendMessage("user", []anthropicBlock{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   contentText(msg.Content),
			}})
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages 中没有可发送的 user/assistant 消息")
	}
	if messages[0].Role != "user" {
		return nil, fmt.Errorf("Anthropic Messages 要求第一条消息为 user")
	}
	ar.Messages = messages
	ar.System = strings.Join(systemParts, "\n\n")

	if tools, choice := buildTools(req); len(tools) > 0 {
		ar.Tools = tools
		ar.ToolChoice = choice
	}
	ar.StopSequences = parseStop(req.Stop)
	if strings.TrimSpace(req.User) != "" {
		ar.Metadata = &anthropicMeta{UserID: req.User}
	}

	return json.Marshal(ar)
}

func maxTokens(req *ChatRequest) int {
	value := 0
	if req.MaxTokens != nil {
		value = *req.MaxTokens
	} else if req.MaxCompletionTokens != nil {
		value = *req.MaxCompletionTokens
	}
	// 证据：02-usage-semantics.md §2「accepts 1..65536; ≤0 或 >65536 一律改写为 8192」
	if value <= 0 || value > 65536 {
		return 8192
	}
	return value
}

// contentText 提取字符串或内容块数组中的纯文本。
func contentText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
		return ""
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "")
}

// userBlocks 把 OpenAI 用户内容转换为 Anthropic 内容块。
// 图片块按逆向结论降级为字面量 "[image]"（不做图片透传）。
func userBlocks(raw json.RawMessage) []anthropicBlock {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil && s != "" {
			return []anthropicBlock{{Type: "text", Text: s}}
		}
		return nil
	}

	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return nil
	}
	blocks := make([]anthropicBlock, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			if p.Text != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: p.Text})
			}
		case "image_url", "image":
			blocks = append(blocks, anthropicBlock{Type: "text", Text: "[image]"})
		default:
			if p.Text != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: p.Text})
			}
		}
	}
	return blocks
}

func toolUseID(id string) string {
	if id != "" {
		return id
	}
	return "toolu_" + strings.ReplaceAll(newShortID(), "-", "")
}

// parseArguments 把 OpenAI 的 JSON 字符串参数转为对象；非法时返回空对象。
func parseArguments(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return map[string]any{}
	}
	if _, ok := v.(map[string]any); !ok {
		return map[string]any{}
	}
	return v
}

func buildTools(req *ChatRequest) ([]anthropicTool, any) {
	if len(req.Tools) == 0 {
		return nil, nil
	}
	tools := make([]anthropicTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t.Type != "" && t.Type != "function" {
			continue
		}
		if strings.TrimSpace(t.Function.Name) == "" {
			continue
		}
		schema := any(map[string]any{"type": "object", "properties": map[string]any{}})
		raw := bytes.TrimSpace(t.Function.Parameters)
		if len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err == nil {
				schema = parsed
			}
		}
		tools = append(tools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}
	if len(tools) == 0 {
		return nil, nil
	}
	choice := parseToolChoice(req.ToolChoice)
	if choice == toolChoiceNone {
		return nil, nil
	}
	return tools, choice
}

// toolChoiceNone 是内部哨兵值，表示“禁用工具”。
var toolChoiceNone = struct{ tag string }{tag: "none"}

func parseToolChoice(raw json.RawMessage) any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil
		}
		switch s {
		case "none":
			return toolChoiceNone
		case "auto":
			return map[string]any{"type": "auto"}
		case "required", "any":
			return map[string]any{"type": "any"}
		}
		return nil
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil
	}
	if obj.Type == "tool" && obj.Name != "" {
		return map[string]any{"type": "tool", "name": obj.Name}
	}
	if obj.Type == "function" && obj.Function.Name != "" {
		return map[string]any{"type": "tool", "name": obj.Function.Name}
	}
	return nil
}

func parseStop(raw json.RawMessage) []string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil && s != "" {
			return []string{s}
		}
		return nil
	}
	var list []string
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, s := range list {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
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

// MapFinishReason 把 Anthropic stop_reason 映射为 OpenAI finish_reason。
func MapFinishReason(stopReason string, hasTools bool) string {
	switch stopReason {
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	case "end_turn", "stop_sequence", "pause_turn":
		return "stop"
	case "":
		if hasTools {
			return "tool_calls"
		}
		return "stop"
	default:
		return "stop"
	}
}

func newShortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
