package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"phanthycode2api/internal/config"
	"phanthycode2api/internal/credential"
	"phanthycode2api/internal/phanthy"
	"phanthycode2api/internal/pool"
)

const successSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_test","model":"Iris-1.0","usage":{"input_tokens":3,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}

`

type testEnv struct {
	gateway  *Gateway
	server   *httptest.Server
	upstream *httptest.Server
	pool     *pool.Pool
	lastReq  *http.Request
	lastBody map[string]any
}

func newTestEnv(t *testing.T, apiKey string, upstream http.HandlerFunc, creds ...*credential.Credential) *testEnv {
	t.Helper()

	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)

	cfg := config.Default()
	cfg.APIKey = apiKey
	cfg.BaseURL = up.URL
	if err := cfg.Validate(); err != nil {
		t.Fatalf("配置校验失败: %v", err)
	}

	p, err := pool.New(creds, filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("账号池: %v", err)
	}

	fp := phanthy.NewFingerprint(phanthy.FingerprintConfig{
		Enabled:    true,
		CliVersion: "0.2.11",
		Client:     "cli",
		Product:    "phanthy_code",
		DeviceJSON: `{"cliVersion":"0.2.11","hostname":"test"}`,
	})
	client := phanthy.New(phanthy.Options{BaseURL: up.URL, Timeout: 5 * time.Second, Fingerprint: fp})
	gw := New(cfg, p, client, nil)

	server := httptest.NewServer(gw.Handler())
	t.Cleanup(server.Close)

	return &testEnv{gateway: gw, server: server, upstream: up, pool: p}
}

func testCred(uid string) *credential.Credential {
	return &credential.Credential{UID: uid, AccessToken: "token-" + uid}
}

func postChat(t *testing.T, base, apiKey string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	return resp
}

func TestHealthz(t *testing.T) {
	env := newTestEnv(t, "secret", func(w http.ResponseWriter, r *http.Request) {}, testCred("a"))
	resp, err := http.Get(env.server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("healthz = %d %q", resp.StatusCode, body)
	}
}

func TestAuthRequired(t *testing.T) {
	env := newTestEnv(t, "secret", func(w http.ResponseWriter, r *http.Request) {}, testCred("a"))

	resp, err := http.Get(env.server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var envl errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envl); err != nil {
		t.Fatal(err)
	}
	if envl.Error.Code != "invalid_api_key" {
		t.Fatalf("code = %q", envl.Error.Code)
	}
}

func TestModels(t *testing.T) {
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {}, testCred("a"))
	resp, err := http.Get(env.server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" || len(out.Data) != 13 {
		t.Fatalf("models = %+v", out)
	}
	if out.Data[0].ID != "DeepSeek-V4" || out.Data[0].OwnedBy != "phanthy" {
		t.Fatalf("first model = %+v", out.Data[0])
	}
}

func TestChatNonStream(t *testing.T) {
	var gotHeaders http.Header
	var gotBody map[string]any

	env := newTestEnv(t, "secret", func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, successSSE)
	}, testCred("acct-1"))

	resp := postChat(t, env.server.URL, "secret", map[string]any{
		"model": "DeepSeek-V4",
		"messages": []map[string]any{
			{"role": "user", "content": "你好"},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content type = %q", ct)
	}

	var completion struct {
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		t.Fatal(err)
	}
	if completion.Object != "chat.completion" || completion.Model != "DeepSeek-V4" {
		t.Fatalf("completion = %+v", completion)
	}
	if completion.Choices[0].Message.Content != "你好" || completion.Choices[0].FinishReason != "stop" {
		t.Fatalf("choice = %+v", completion.Choices[0])
	}
	if completion.Usage.PromptTokens != 3 || completion.Usage.CompletionTokens != 5 {
		t.Fatalf("usage = %+v", completion.Usage)
	}

	// 上游必须收到别名映射、stream=true 与官方指纹头。
	if gotBody["model"] != "Iris-1.0" || gotBody["stream"] != true {
		t.Fatalf("upstream body = %v", gotBody)
	}
	if got := gotHeaders.Get("Authorization"); got != "Bearer token-acct-1" {
		t.Fatalf("upstream auth = %q", got)
	}
	if got := gotHeaders.Get("x-phanthy-client"); got != "cli" {
		t.Fatalf("x-phanthy-client = %q", got)
	}
	if got := gotHeaders.Get("X-Phanthy-Client-Source"); got != "cli" {
		t.Fatalf("client source = %q", got)
	}
	if got := gotHeaders.Get("X-Phanthy-Product"); got != "phanthy_code" {
		t.Fatalf("product = %q", got)
	}
	if got := gotHeaders.Get("x-phanthy-cli-device"); got == "" {
		t.Fatal("device header missing")
	}
	if got := gotHeaders.Get("x-phanthy-request"); got == "" {
		t.Fatal("request id header missing")
	}
	if got := gotHeaders.Get("x-phanthy-session"); got != "acct-1" {
		t.Fatalf("session header = %q", got)
	}
}

func TestChatStream(t *testing.T) {
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, successSSE)
	}, testCred("a"))

	resp := postChat(t, env.server.URL, "", map[string]any{
		"model":  "DeepSeek-V4",
		"stream": true,
		"messages": []map[string]any{
			{"role": "user", "content": "你好"},
		},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	if !strings.Contains(text, `"role":"assistant"`) || !strings.Contains(text, `"content":"你好"`) {
		t.Fatalf("stream content = %s", text)
	}
	if !strings.Contains(text, `"finish_reason":"stop"`) {
		t.Fatalf("missing finish chunk: %s", text)
	}
	if !strings.HasSuffix(text, "data: [DONE]\n\n") {
		t.Fatalf("stream must end with [DONE]: %s", text)
	}
	if strings.Count(text, "data: [DONE]") != 1 {
		t.Fatalf("expected exactly one DONE marker: %s", text)
	}
}

func TestChatRotatesOnRateLimit(t *testing.T) {
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer token-a" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"code":"rate_limited","message":"too many requests"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, successSSE)
	}, testCred("a"), testCred("b"))

	resp := postChat(t, env.server.URL, "", map[string]any{
		"model":    "DeepSeek-V4",
		"messages": []map[string]any{{"role": "user", "content": "你好"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}

	status := env.pool.List(time.Now())
	if len(status) != 2 {
		t.Fatalf("status = %+v", status)
	}
	if status[0].UID != "a" || !status[0].Cooling {
		t.Fatalf("account a should be cooling: %+v", status[0])
	}
	if status[1].UID != "b" || status[1].Cooling {
		t.Fatalf("account b should be healthy: %+v", status[1])
	}
}

func TestChatNoHealthyAccount(t *testing.T) {
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		t.Error("empty pool must not call upstream")
	})

	resp := postChat(t, env.server.URL, "", map[string]any{
		"model":    "DeepSeek-V4",
		"messages": []map[string]any{{"role": "user", "content": "你好"}},
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var envl errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envl); err != nil {
		t.Fatal(err)
	}
	if envl.Error.Code != "no_healthy_account" {
		t.Fatalf("code = %q", envl.Error.Code)
	}
}

func TestChatUpstreamClientError(t *testing.T) {
	var calls int
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"invalid_request","message":"bad input"}}`)
	}, testCred("a"), testCred("b"))

	resp := postChat(t, env.server.URL, "", map[string]any{
		"model":    "DeepSeek-V4",
		"messages": []map[string]any{{"role": "user", "content": "你好"}},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("client error should not rotate accounts, calls = %d", calls)
	}
	var envl errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envl); err != nil {
		t.Fatal(err)
	}
	if envl.Error.Code != "upstream_client_error" {
		t.Fatalf("code = %q", envl.Error.Code)
	}
	for _, st := range env.pool.List(time.Now()) {
		if st.Cooling || st.ErrCount != 0 {
			t.Fatalf("account should not be penalised: %+v", st)
		}
	}
}

func TestChatInvalidRequest(t *testing.T) {
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid request must not reach upstream")
	}, testCred("a"))

	cases := []map[string]any{
		{"messages": []map[string]any{{"role": "user", "content": "x"}}},
		{"model": "DeepSeek-V4"},
		{"model": "DeepSeek-V4", "messages": []map[string]any{{"role": "assistant", "content": "x"}}},
	}
	for _, body := range cases {
		resp := postChat(t, env.server.URL, "", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d for %v", resp.StatusCode, body)
		}
		resp.Body.Close()
	}
}

func TestStatusEndpoint(t *testing.T) {
	env := newTestEnv(t, "", func(w http.ResponseWriter, r *http.Request) {}, testCred("a"))
	env.pool.SetBalance("a", int64Ptr(7))

	resp, err := http.Get(env.server.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Accounts []struct {
			UID      string `json:"uid"`
			Balance  *int64 `json:"balance"`
			Disabled bool   `json:"disabled"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Accounts) != 1 || out.Accounts[0].UID != "a" {
		t.Fatalf("accounts = %+v", out.Accounts)
	}
	if out.Accounts[0].Balance == nil || *out.Accounts[0].Balance != 7 {
		t.Fatalf("balance = %v", out.Accounts[0].Balance)
	}
}
