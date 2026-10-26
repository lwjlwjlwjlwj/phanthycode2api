package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/tokenkit"
)

// maxTokens 上限，超过一律改写为 8192（证据 02-usage-semantics.md §2）。
const maxTokensCeiling = 65536

// Error 是协议层产生的、可由网关直接判定为非合法请求的错误。
// 返回的非法请求错误由网关以 400 处理，不回退账号。
var errEmptyRequest = errors.New("请求为空")

// BuildAnthropicRequest 把 OpenAI 请求转换为 Anthropic Messages 请求。
//
// relaykit 负责请求转换（含 system 累积、相邻同角色合并、tool_result 合并、
// tool_choice 映射、tools 序列化、推理适配等），本函数在此之上完成三件
// 逆向证据特有、relaykit 不处理的事：
//   - max_tokens / max_completion_tokens 合法性改写（≤0 或 >65536 → 8192）
//   - 图片块降级为字面量 "[image]"
//   - 强制 stream=true
//
// 模型名按证据 §6.3 原样透传（不做别名映射）：
// relaykit 默认取 textRequest.Model，这里再显式 SetModelName 一次以保持语义清晰。
// 上游始终以 stream=true 调用，非流式响应由本地聚合产生。
func BuildAnthropicRequest(ctx context.Context, req *ChatRequest) ([]byte, error) {
	if req == nil {
		return nil, errEmptyRequest
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model 不能为空")
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("messages 不能为空")
	}

	sanitizeMaxTokens(req)

	gptReq, err := toGeneralOpenAIRequest(req)
	if err != nil {
		return nil, err
	}
	degradeMedia(gptReq)

	meta := &convmeta.Values{
		OriginModelName:   req.Model,
		UpstreamModelName: req.Model,
		IsStream:          true,
		Options: &convmeta.Options{
			Claude: convmeta.ClaudeOptions{
				// 缺 max_tokens 时的兜底（relaykit 不内置兜底，要求宿主注入；
				// new-api 在自身 relay 层用 model_setting 兜底，这里按证据规则给 8192）。
				DefaultMaxTokens: func(string) int { return 8192 },
			},
		},
	}

	claudeReq, err := relayconvert.OpenAIChatRequestToClaudeMessages(ctx, meta, *gptReq)
	if err != nil {
		return nil, err
	}

	// 模型名原样透传（relaykit 默认即透传 textRequest.Model，这里保持显式），
	// 并强制 stream=true（relaykit 按 IsStream 设 stream，这里无条件置真）。
	claudeReq.SetModelName(req.Model)
	stream := true
	claudeReq.Stream = &stream

	return json.Marshal(claudeReq)
}

// sanitizeMaxTokens 把 ≤0 或 >65536 的 max_tokens / max_completion_tokens 改写为 8192。
// relaykit 用 uint 承载这两个字段，负数 JSON 反序列化会报错，因此必须先在此归一化。
// 证据：02-usage-semantics.md §2「accepts 1..65536; anything ≤0 or >65536 is rewritten to 8192」。
func sanitizeMaxTokens(req *ChatRequest) {
	if req.MaxTokens != nil {
		if *req.MaxTokens <= 0 || *req.MaxTokens > maxTokensCeiling {
			*req.MaxTokens = 8192
		}
	}
	if req.MaxCompletionTokens != nil {
		if *req.MaxCompletionTokens <= 0 || *req.MaxCompletionTokens > maxTokensCeiling {
			*req.MaxCompletionTokens = 8192
		}
	}
}

// toGeneralOpenAIRequest 通过 JSON 往返把网关入口 DTO 转换为 relaykit 的请求 DTO。
func toGeneralOpenAIRequest(req *ChatRequest) (*dto.GeneralOpenAIRequest, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var g dto.GeneralOpenAIRequest
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

// degradeMedia 把用户消息中的 image 块降级为字面量 "[image]"。
// 逆向结论不做图片透传；relaykit 的媒体转换需要注册 media resolver，这里直接规避。
func degradeMedia(req *dto.GeneralOpenAIRequest) {
	for i := range req.Messages {
		content, ok := req.Messages[i].Content.([]any)
		if !ok {
			continue
		}
		changed := false
		for j, item := range content {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "image_url" || typ == "image" {
				content[j] = map[string]any{"type": "text", "text": "[image]"}
				changed = true
			}
		}
		if changed {
			req.Messages[i].Content = content
		}
	}
}

// EstimateRequestTokens 用 tokenkit 估算请求的 prompt token 数。
// 仅在 usage 缺失（全为 0）时作为 prompt 侧兜底，对齐 new-api 的
// ResponseText2Usage：prompt 用宿主估算值、completion 用 tokenkit.Estimate、total 求和。
func EstimateRequestTokens(model string, req *ChatRequest) int {
	var b strings.Builder
	for _, m := range req.Messages {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		if text := messageText(m.Content); text != "" {
			b.WriteString(m.Role)
			b.WriteString(": ")
			b.WriteString(text)
		}
	}
	if b.Len() == 0 {
		return 0
	}
	return tokenkit.Estimate(model, b.String())
}

// messageText 提取字符串或内容块数组中的纯文本（含 "[]image]" 占位）。
func messageText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		switch p.Type {
		case "text":
			b.WriteString(p.Text)
		case "image_url", "image":
			b.WriteString("[image]")
		}
	}
	return b.String()
}
