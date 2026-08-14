// Package upstream 封装对 Phanthy（code.phanthy.com）上游的全部 HTTP 调用
// （chat / oauth / api_key），以及错误分类（驱动 pool 冷却状态机）。
package upstream

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"phanthycode2api/internal/auth"
)

// ErrKind 错误分类，pool 据此决定冷却时长。
type ErrKind int

const (
	ErrNone        ErrKind = iota // 成功
	ErrHardCredit                 // 积分/余额不足 → 长冷却
	ErrSoftRate                   // 429 软限流 → 短冷却
	ErrSessionDead                // 401/会话失效 → 禁用
	ErrNotFound                   // 404 上游偶发 → 短冷却不累计 errCount（防雪崩）
	ErrServer                     // 5xx 上游故障
	ErrClient                     // 其他 4xx / 业务错误
)

func (k ErrKind) String() string {
	switch k {
	case ErrHardCredit:
		return "hard_credit"
	case ErrSoftRate:
		return "soft_rate"
	case ErrSessionDead:
		return "session_dead"
	case ErrNotFound:
		return "not_found"
	case ErrServer:
		return "server"
	case ErrClient:
		return "client"
	default:
		return "none"
	}
}

// Error 带分类的上游错误。
type Error struct {
	Kind   ErrKind
	Status int
	Msg    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("upstream %s (http %d): %s", e.Kind, e.Status, e.Msg)
}

// hardMarkers 积分不足关键词（小写比较 + 原文比较双通道）。
var hardMarkers = []string{
	"insufficient credit", "no credit", "credit exhausted", "out of credit",
	"quota exceeded", "quota exhaust", "payment required", "credit not enough",
	"not enough credit", "credit balance is too low", "billing",
	"积分不足", "额度不足", "余额不足", "积分用完", "额度用尽", "没有积分",
}

// sessionDeadMarkers 会话失效关键词。
var sessionDeadMarkers = []string{
	"OAuth token has been revoked", "missing_login", "invalid_grant",
	"session expired", "unauthorized", "invalid access token",
}

// Classify 按 HTTP 状态码 + body 判定错误类别。
func Classify(status int, body string) ErrKind {
	if status == http.StatusPaymentRequired {
		return ErrHardCredit
	}
	lower := strings.ToLower(body)
	for _, m := range hardMarkers {
		if strings.Contains(lower, strings.ToLower(m)) || strings.Contains(body, m) {
			return ErrHardCredit
		}
	}
	for _, m := range sessionDeadMarkers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return ErrSessionDead
		}
	}
	if status == http.StatusUnauthorized {
		return ErrSessionDead
	}
	if status == http.StatusTooManyRequests {
		return ErrSoftRate
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= 500 {
		return ErrServer
	}
	if status >= 400 {
		return ErrClient
	}
	return ErrNone
}

// Client 上游 HTTP 客户端。Base 字段可覆盖便于测试／私有部署。
type Client struct {
	HTTP *http.Client

	BaseAPIURL      string // https://code.phanthy.com
	OAuthTokenURL   string // {Base}/oauth/token
	APIKeyURL       string // {Base}/api/oauth/phanthy_cli/create_api_key
	ProfileURL      string // {Base}/api/oauth/profile
	ClientID        string // phanthy-code-cli
	OAuthBetaHeader string // oauth-2025-04-20
}

// New 生产默认值。配置连接池减少 TLS 握手。
func New(baseURL string) *Client {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		base = "https://code.phanthy.com"
	}
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Client{
		HTTP:            &http.Client{Timeout: 300 * time.Second, Transport: tr},
		BaseAPIURL:      base,
		OAuthTokenURL:   base + "/oauth/token",
		APIKeyURL:       base + "/api/oauth/phanthy_cli/create_api_key",
		ProfileURL:      base + "/api/oauth/profile",
		ClientID:        "phanthy-code-cli",
		OAuthBetaHeader: "oauth-2025-04-20",
	}
}

// oauthEnvelope 上游 token/profile 响应统一信封。
type oauthEnvelope struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// formPost 发表单请求并解析公共信封。
func (c *Client) formPost(url string, form map[string]string, extraHdrs map[string]string) (map[string]any, error) {
	// 解析错误信封
	body, err := c.rawPost(url, "application/x-www-form-urlencoded", encodeForm(form), extraHdrs)
	if err != nil {
		return nil, err
	}
	var env oauthEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Error != nil {
		msg := env.Error.Message
		if msg == "" {
			msg = env.Error.Code
		}
		kind := Classify(200, env.Error.Code+" "+msg)
		if kind == ErrNone {
			kind = ErrClient
		}
		return nil, &Error{Kind: kind, Status: 200, Msg: fmt.Sprintf("code=%s msg=%s", env.Error.Code, msg)}
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, &Error{Kind: ErrClient, Status: 200, Msg: fmt.Sprintf("non-json oauth response: %s", truncate(string(body), 200))}
	}
	return m, nil
}

// rawPost 通用 POST 请求，返回 body；HTTP 非 2xx 时返回 *Error。
func (c *Client) rawPost(url, contentType string, body []byte, extraHdrs map[string]string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "phanthycode2api/1.0")
	for k, v := range extraHdrs {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		kind := Classify(resp.StatusCode, string(raw))
		return nil, &Error{Kind: kind, Status: resp.StatusCode, Msg: truncate(string(raw), 200)}
	}
	return raw, nil
}

func encodeForm(form map[string]string) []byte {
	var b strings.Builder
	first := true
	for k, v := range form {
		if !first {
			b.WriteString("&")
		}
		first = false
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(urlQueryEscape(v))
	}
	return []byte(b.String())
}

// urlQueryEscape 简单转义（避免引入 net/url 增加耦合时用于表单字段）。
func urlQueryEscape(s string) string {
	r := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"+", "%2B",
		"&", "%26",
		"=", "%3D",
		"#", "%23",
	)
	return r.Replace(s)
}

// RefreshToken 用 refresh_token 换取新 access_token；成功时更新 a 的字段，
// 调用方负责 SaveAtomic。全程持 a 锁，防止并发 SaveAtomic 读半更新 token。
func (c *Client) RefreshToken(a *auth.Auth) error {
	a.Lock()
	defer a.Unlock()
	if strings.TrimSpace(a.RefreshToken) == "" {
		return fmt.Errorf("no refreshToken")
	}
	form := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": a.RefreshToken,
		"client_id":     c.ClientID,
	}
	extra := map[string]string{"anthropic-beta": c.OAuthBetaHeader}
	m, err := c.formPost(c.OAuthTokenURL, form, extra)
	if err != nil {
		return err
	}
	token, _ := m["access_token"].(string)
	if token == "" {
		return &Error{Kind: ErrSessionDead, Status: 0, Msg: "refresh_failed: no access_token in response — re-login required"}
	}
	a.AccessToken = token
	if rt, ok := m["refresh_token"].(string); ok && rt != "" {
		a.RefreshToken = rt
	}
	if exp, ok := toInt64(m["expires_in"]); ok && exp > 0 {
		a.ExpiresAt = time.Now().Add(time.Duration(exp) * time.Second).Unix()
	}
	return nil
}

// CreateAPIKey 用 OAuth access_token 换取 chat 专用的 API key（x-api-key）。
func (c *Client) CreateAPIKey(a *auth.Auth) (string, error) {
	a.Lock()
	defer a.Unlock()
	if strings.TrimSpace(a.AccessToken) == "" {
		return "", fmt.Errorf("no accessToken")
	}
	req, err := http.NewRequest(http.MethodPost, c.APIKeyURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	req.Header.Set("anthropic-beta", c.OAuthBetaHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "phanthycode2api/1.0")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		kind := Classify(resp.StatusCode, string(raw))
		return "", &Error{Kind: kind, Status: resp.StatusCode, Msg: truncate(string(raw), 200)}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", &Error{Kind: ErrClient, Status: resp.StatusCode, Msg: "create_api_key non-json response"}
	}
	// 兼容 api_key / apiKey / data.api_key 三种形态
	if k, ok := m["api_key"].(string); ok && k != "" {
		return k, nil
	}
	if k, ok := m["apiKey"].(string); ok && k != "" {
		return k, nil
	}
	if d, ok := m["data"].(map[string]any); ok {
		if k, ok := d["api_key"].(string); ok && k != "" {
			return k, nil
		}
		if k, ok := d["apiKey"].(string); ok && k != "" {
			return k, nil
		}
	}
	return "", &Error{Kind: ErrClient, Status: resp.StatusCode, Msg: "create_api_key: no api_key in response"}
}

// EnsureAPIKey 若账号缺少 api_key 则自动调用 create_api_key 补齐，并原子写回。
func (c *Client) EnsureAPIKey(a *auth.Auth) error {
	a.Lock()
	if a.APIKey != "" {
		a.Unlock()
		return nil
	}
	a.Unlock()
	key, err := c.CreateAPIKey(a)
	if err != nil {
		return err
	}
	a.Lock()
	a.APIKey = key
	a.Unlock()
	if err := a.SaveAtomic(); err != nil {
		log.Printf("ensure_api_key save uid=%s: %v", a.UID, err)
	}
	return nil
}

// ChatStream 发 chat 请求并返回原始 SSE body 流（调用方负责 Close）。
// 非 2xx 时 rc 为 nil、body 为上游响应体、err 为 nil；只有传输层失败才返回 err。
// 认证方式：优先 api_key（x-api-key），其次 access_token（Bearer）。
func (c *Client) ChatStream(a *auth.Auth, body []byte) (rc io.ReadCloser, status int, respBody []byte, err error) {
	url := c.BaseAPIURL + "/v1/messages"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("x-app", "cli")
	req.Header.Set("User-Agent", "phanthycode2api/1.0")
	if strings.TrimSpace(a.APIKey) != "" {
		req.Header.Set("x-api-key", a.APIKey)
	} else if strings.TrimSpace(a.AccessToken) != "" {
		req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	} else {
		return nil, 401, []byte(`{"error":{"code":"missing_login","message":"no api_key or access_token"}}`), nil
	}
	if a.UID != "" {
		req.Header.Set("X-Claude-Code-Session-Id", a.UID)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		log.Printf("chat_stream uid=%s: transport error: %v", a.UID, err)
		return nil, 0, nil, err
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		kind := Classify(resp.StatusCode, string(raw))
		log.Printf("chat_stream uid=%s: upstream %d %s body=%s", a.UID, resp.StatusCode, kind, truncate(string(raw), 200))
		return nil, resp.StatusCode, raw, nil
	}
	return resp.Body, resp.StatusCode, nil, nil
}

// FetchProfile 查询账号信息（可用于状态展示、积分判断）。
func (c *Client) FetchProfile(a *auth.Auth) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, c.ProfileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "phanthycode2api/1.0")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		kind := Classify(resp.StatusCode, string(raw))
		return nil, &Error{Kind: kind, Status: resp.StatusCode, Msg: truncate(string(raw), 200)}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// toInt64 尝试把 any 转成 int64。
func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	case int:
		return int64(t), true
	case int64:
		return t, true
	}
	return 0, false
}