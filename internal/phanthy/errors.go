package phanthy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Kind 是上游错误的分类，决定账号池的状态机动作。
type Kind int

const (
	// KindNone 表示成功。
	KindNone Kind = iota
	// KindHardCredit 表示积分不足（402 或余额/配额相关文案）。
	KindHardCredit
	// KindSessionDead 表示登录态失效，账号应被永久禁用。
	KindSessionDead
	// KindSoftRate 表示限流（429）。
	KindSoftRate
	// KindNotFound 表示路径/模型不存在（404）。
	KindNotFound
	// KindServer 表示上游 5xx。
	KindServer
	// KindClient 表示请求本身有问题（400/403/422 等）。
	KindClient
)

func (k Kind) String() string {
	switch k {
	case KindNone:
		return "none"
	case KindHardCredit:
		return "hard_credit"
	case KindSessionDead:
		return "session_dead"
	case KindSoftRate:
		return "soft_rate"
	case KindNotFound:
		return "not_found"
	case KindServer:
		return "server_error"
	case KindClient:
		return "client_error"
	default:
		return "unknown"
	}
}

// Error 携带上游状态码、错误信封与分类。
type Error struct {
	Status  int
	Code    string
	Message string
	Kind    Kind
	Body    []byte
	Cause   error
}

func (e *Error) Error() string {
	parts := make([]string, 0, 3)
	if e.Status != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.Status))
	}
	if e.Kind != KindNone {
		parts = append(parts, "kind="+e.Kind.String())
	}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	if e.Message != "" {
		parts = append(parts, "message="+e.Message)
	}
	if e.Cause != nil {
		parts = append(parts, "cause="+e.Cause.Error())
	}
	if len(parts) == 0 {
		return "上游请求失败"
	}
	return strings.Join(parts, " ")
}

// Unwrap 支持 errors.Is/As 链。
func (e *Error) Unwrap() error { return e.Cause }

// 余额/配额相关标记（英文与中文都覆盖，来源：re/evidence 中的 prior-art 标记集合）。
var hardMarkers = []string{
	"insufficient credit",
	"no credit",
	"credit exhausted",
	"out of credit",
	"quota exceeded",
	"quota exhaust",
	"payment required",
	"credit not enough",
	"not enough credit",
	"credit balance is too low",
	"billing",
	"积分不足",
	"额度不足",
	"余额不足",
	"积分用完",
	"额度用尽",
	"没有积分",
}

// 登录态失效标记。
var sessionMarkers = []string{
	"oauth token has been revoked",
	"missing_login",
	"invalid_grant",
	"session expired",
	"unauthorized",
	"invalid access token",
}

// Classify 按固定优先级分类：402 → 余额标记 → 登录态标记 → 401 → 429 → 404 → 5xx → 4xx。
func Classify(status int, body []byte) Kind {
	if status == http.StatusPaymentRequired {
		return KindHardCredit
	}
	text := strings.ToLower(string(body))
	for _, m := range hardMarkers {
		if strings.Contains(text, strings.ToLower(m)) {
			return KindHardCredit
		}
	}
	for _, m := range sessionMarkers {
		if strings.Contains(text, strings.ToLower(m)) {
			return KindSessionDead
		}
	}
	switch {
	case status == http.StatusUnauthorized:
		return KindSessionDead
	case status == http.StatusTooManyRequests:
		return KindSoftRate
	case status == http.StatusNotFound:
		return KindNotFound
	case status >= 500:
		return KindServer
	case status >= 400:
		return KindClient
	default:
		return KindNone
	}
}

// envelope 是上游统一的错误信封：{"error":{"code":...,"message":...}}。
type envelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ParseEnvelope 尝试解析错误信封；解析失败时返回空字段。
func ParseEnvelope(body []byte) (code, message string) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", ""
	}
	return env.Error.Code, env.Error.Message
}

// newHTTPError 由状态码与响应体构造分类错误。
func newHTTPError(status int, body []byte) *Error {
	code, message := ParseEnvelope(body)
	kind := Classify(status, body)
	if message == "" {
		message = truncate(string(body), 512)
	}
	return &Error{
		Status:  status,
		Code:    code,
		Message: message,
		Kind:    kind,
		Body:    body,
	}
}

// newTransportError 由网络错误构造分类错误。
func newTransportError(err error) *Error {
	return &Error{Kind: KindServer, Message: "上游连接失败", Cause: err}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
