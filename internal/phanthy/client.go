package phanthy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"phanthycode2api/internal/credential"
)

const (
	// ClientID 是官方 CLI 使用的公开客户端 id（M6 静态证据 lq6）。
	ClientID = "phanthy-code-cli"
	// RedirectURI 是官方 CLI 注册的回调地址。
	RedirectURI = "https://code.phanthy.com/oauth/code/success"
	// AuthorizeURL 是 OAuth 授权端点（M6-cli-config.txt:74-99）。
	AuthorizeURL = "https://code.phanthy.com/oauth/authorize"
	// OAuthBeta 是 OAuth 端点附带的 anthropic-beta 值。
	OAuthBeta = "oauth-2025-04-20"

	maxResponseBody = 1 << 20 // 错误/JSON 响应读取上限 1 MiB
)

// Scopes 是官方 CLI 请求的 6 个 scope（M6-cli-config.txt:74-99）。
// 服务端 discovery 文档只公布 5 个，org:create_api_key 未公开。
var Scopes = []string{
	"org:create_api_key",
	"user:profile",
	"user:inference",
	"user:sessions:claude_code",
	"user:mcp_servers",
	"user:file_upload",
}

// Auth 是一次上游调用所需的凭据。
type Auth struct {
	AccessToken string
	APIKey      string
	UID         string
}

// Token 是 /oauth/token 的解析结果。
type Token struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

// Expiry 计算过期时间；上游未返回 expires_in 时保守按 1 小时估计。
func (t *Token) Expiry(now time.Time) time.Time {
	ttl := t.ExpiresIn
	if ttl <= 0 {
		ttl = time.Hour
	}
	return now.Add(ttl)
}

// Options 构建 Client。
type Options struct {
	BaseURL     string
	Timeout     time.Duration
	Fingerprint *Fingerprint
	HTTPClient  *http.Client // 测试可注入
}

// Client 是对 code.phanthy.com 的 HTTP 客户端。
type Client struct {
	BaseURL string
	HTTP    *http.Client
	FP      *Fingerprint
}

// New 构建客户端。
func New(opts Options) *Client {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base == "" {
		base = "https://code.phanthy.com"
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 120 * time.Second
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConnsPerHost = 16
		transport.IdleConnTimeout = 90 * time.Second
		transport.ForceAttemptHTTP2 = true
		httpClient = &http.Client{Timeout: timeout, Transport: transport}
	}
	return &Client{BaseURL: base, HTTP: httpClient, FP: opts.Fingerprint}
}

func (c *Client) url(path string) string {
	return c.BaseURL + path
}

// ExchangeCode 用授权码换取 token（PKCE）。
func (c *Client) ExchangeCode(ctx context.Context, code, verifier, redirectURI string) (*Token, error) {
	if redirectURI == "" {
		redirectURI = RedirectURI
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
		"client_id":     {ClientID},
	}
	body, err := c.tokenPost(ctx, form)
	if err != nil {
		return nil, err
	}
	return parseToken(body)
}

// RefreshToken 用 refresh token 换取新 token。
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, &Error{Kind: KindSessionDead, Message: "账号没有 refresh_token"}
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {ClientID},
	}
	body, err := c.tokenPost(ctx, form)
	if err != nil {
		return nil, err
	}
	return parseToken(body)
}

// RefreshCredential 在 token 进入 skew 窗口时刷新并原子落盘。
// 无 refresh_token 或上游返回 invalid_grant 时返回 KindSessionDead 错误。
func (c *Client) RefreshCredential(ctx context.Context, cred *credential.Credential, skew time.Duration) error {
	if !cred.Expiring(time.Now(), skew) {
		return nil
	}
	cred.RefreshLock()
	defer cred.RefreshUnlock()
	// 双检：可能已被其他协程刷新。
	if !cred.Expiring(time.Now(), skew) {
		return nil
	}
	tok, err := c.RefreshToken(ctx, cred.RefreshTokenValue())
	if err != nil {
		return err
	}
	cred.SetTokens(tok.AccessToken, tok.RefreshToken, tok.Expiry(time.Now()))
	if err := cred.SaveAtomic(); err != nil {
		return fmt.Errorf("刷新后保存凭证失败: %w", err)
	}
	return nil
}

// TokenResponse 是 /oauth/token 的原始 JSON 结构。
type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    json.Number `json:"expires_in"`
}

func parseToken(body []byte) (*Token, error) {
	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, &Error{Kind: KindClient, Message: "解析 token 响应失败", Cause: err, Body: body}
	}
	if tr.AccessToken == "" {
		return nil, &Error{Kind: KindClient, Message: "token 响应缺少 access_token", Body: body}
	}
	tok := &Token{AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken}
	if v, err := tr.ExpiresIn.Float64(); err == nil && v > 0 {
		tok.ExpiresIn = time.Duration(v * float64(time.Second))
	}
	return tok, nil
}

func (c *Client) tokenPost(ctx context.Context, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/oauth/token"), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, &Error{Kind: KindClient, Message: "构造 token 请求失败", Cause: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-beta", OAuthBeta)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, newTransportError(err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, newTransportError(err)
	}
	if resp.StatusCode >= 400 {
		return nil, newHTTPError(resp.StatusCode, body)
	}
	// 上游可能用 HTTP 200 返回错误信封（prior-art 假设，做防御处理）。
	if code, message := ParseEnvelope(body); code != "" {
		return nil, &Error{
			Status:  resp.StatusCode,
			Code:    code,
			Message: message,
			Kind:    classifyOAuthCode(code),
			Body:    body,
		}
	}
	return body, nil
}

func classifyOAuthCode(code string) Kind {
	switch strings.ToLower(code) {
	case "invalid_grant", "missing_login", "invalid_token":
		return KindSessionDead
	default:
		return KindClient
	}
}

// Profile 获取账号资料。成功 schema 未被验证，调用方应防御性解析。
func (c *Client) Profile(ctx context.Context, accessToken string) (map[string]any, error) {
	if accessToken == "" {
		return nil, &Error{Kind: KindSessionDead, Message: "账号没有 access_token"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/oauth/profile"), nil)
	if err != nil {
		return nil, &Error{Kind: KindClient, Message: "构造 profile 请求失败", Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-beta", OAuthBeta)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, newTransportError(err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, newTransportError(err)
	}
	if resp.StatusCode >= 400 {
		return nil, newHTTPError(resp.StatusCode, body)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &Error{Kind: KindClient, Message: "profile 响应不是 JSON 对象", Cause: err, Body: body}
	}
	return out, nil
}

// Messages 发送 Anthropic Messages 请求；成功时返回尚未读取的 200 响应体（SSE）。
// 非 2xx 会在本方法内读取并关闭响应体，返回带分类的 *Error。
func (c *Client) Messages(ctx context.Context, auth Auth, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/v1/messages"), bytes.NewReader(body))
	if err != nil {
		return nil, &Error{Kind: KindClient, Message: "构造 messages 请求失败", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	if !applyInferenceAuth(req, auth) {
		return nil, &Error{Kind: KindSessionDead, Message: "账号没有 access_token 或 api_key"}
	}
	c.FP.Apply(req, auth.UID, "")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, newTransportError(err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, readErr := readLimited(resp.Body)
		if readErr != nil {
			return nil, newTransportError(readErr)
		}
		return nil, newHTTPError(resp.StatusCode, errBody)
	}
	return resp, nil
}

// applyInferenceAuth 按 M6 证据优先使用 Bearer；没有 access_token 时才退回 x-api-key。
func applyInferenceAuth(req *http.Request, auth Auth) bool {
	if strings.TrimSpace(auth.AccessToken) != "" {
		req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
		return true
	}
	if strings.TrimSpace(auth.APIKey) != "" {
		req.Header.Set("x-api-key", auth.APIKey)
		return true
	}
	return false
}

// Models 获取模型目录原始 JSON（成功 schema 未验证，调用方按需使用）。
func (c *Client) Models(ctx context.Context, auth Auth) ([]byte, error) {
	return c.discoveryGet(ctx, "/api/phanthy_code/models", auth, "")
}

// ModelStats 获取模型统计原始 JSON（余额探测的数据来源，schema 未验证）。
func (c *Client) ModelStats(ctx context.Context, auth Auth) ([]byte, error) {
	return c.discoveryGet(ctx, "/api/phanthy_code/models/stats", auth, "application/json")
}

func (c *Client) discoveryGet(ctx context.Context, path string, auth Auth, accept string) ([]byte, error) {
	if strings.TrimSpace(auth.AccessToken) == "" {
		return nil, &Error{Kind: KindSessionDead, Message: "模型端点需要 Bearer access_token"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, &Error{Kind: KindClient, Message: "构造模型请求失败", Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	c.FP.ApplyProductOnly(req)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, newTransportError(err)
	}
	defer resp.Body.Close()
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, newTransportError(err)
	}
	if resp.StatusCode >= 400 {
		return nil, newHTTPError(resp.StatusCode, body)
	}
	return body, nil
}

func readLimited(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxResponseBody))
}
