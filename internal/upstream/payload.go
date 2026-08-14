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
}

// openAIMsg 消息体片段。
type openAIMsg struct {
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content"`    // string 或 []contentPart
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
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	System      string         `json:"system,omitempty"`
	Messages    []anthropicMsg `json:"messages"`
	Temperature *float64       `json:"temperature,omitempty"`
	TopP        *float64       `json:"top_p,omitempty"`
	Stream      bool           `json:"stream"`
	Tools       []anthropicTool `json:"tools,omitempty"`
	ToolChoice  json.RawMessage `json:"tool_choice,omitempty"`
	StopSequences []string     `json:"stop_sequences,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type anthropicMsg struct {
	Role    string         `json:"role"`
	Content []contentPart  `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// ModelAlias 客户端可见模型名 → 上游真实模型名。
// Phanthy 官网是商业命名（DeepSeek-V4 等），上游 /v1/messages 需要真实模型名（Iris-1.0 等）。
var ModelAlias = map[string]string{
	"DeepSeek-V4":        "Iris-1.0",
	"gpt-5.6-sol":        "Zeus-1.1-pro",
	"gpt-5.6-terra":      "Zeus-1.1",
	"gpt-5.6-luna":       "Zeus-1.1-fast",
	"gpt-5.5":            "Zeus-1.0-pro",
	"Claude Opus 4.8":    "Gaia-1.2",
	"Claude Opus 4.7":    "Gaia-1.1",
	"Claude Sonnet 4.6":  "Gaia-1.0",
	"Kimi K3":            "Apollo-2.0",
	"Kimi-k2.7-code":     "Apollo-1.1",
	"Kimi K2.6":          "Apollo-1.0",
	"GLM 5.2":            "Metis-1.1",
	"GLM-5.1":            "Metis-1.0",
}

// ResolveModel 解析客户端模型名 → 上游真实名；未命中的原样返回。
func ResolveModel(name string) string {
	if up, ok := ModelAlias[name]; ok {
		return up
	}
	return name
}

// PrepareBody 将 OpenAI 请求体转为 Anthropic 请求体；无法解析时原样返回。
func PrepareBody(src []byte) []byte {
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

	if req.MaxTokens <= 0 || req.MaxTokens > 65536 {
		anthro.MaxTokens = 8192
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
						Name      string         `json:"name"`
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
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage  `json:"usage,omitempty"`
}

type choice struct {
	Index        int            `json:"index"`
	Message      openAIMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type openAIMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
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