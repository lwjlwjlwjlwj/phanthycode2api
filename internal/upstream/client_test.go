package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/fingerprint"
)

func mkAuthFull(uid, token, apiKey string) *auth.Auth {
	return &auth.Auth{UID: uid, AccessToken: token, APIKey: apiKey}
}

// 记录请求头供断言。
type reqCapture struct {
	headers http.Header
	body    string
}

func TestChatStream_BearerPreferred_WithFingerprint(t *testing.T) {
	var got reqCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		got.body = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.FP = fingerprint.New(fingerprint.Config{Enabled: true})
	a := mkAuthFull("u1", "token-bearer", "key-legacy")

	rc, status, respBody, err := c.ChatStream(a, []byte(`{"model":"x","max_tokens":1}`))
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer rc.Close()
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if respBody != nil {
		t.Fatalf("unexpected respBody")
	}

	// Bearer 优先：即使有 api_key 也发 Bearer
	if got.headers.Get("Authorization") != "Bearer token-bearer" {
		t.Errorf("Authorization = %q, want Bearer token-bearer", got.headers.Get("Authorization"))
	}
	if got.headers.Get("x-api-key") != "" {
		t.Errorf("x-api-key should not be sent when token exists, got %q", got.headers.Get("x-api-key"))
	}
	// 指纹头族
	for _, h := range []string{
		fingerprint.HeaderClient,
		fingerprint.HeaderClientSource,
		fingerprint.HeaderDevice,
		fingerprint.HeaderProduct,
		fingerprint.HeaderRequest,
		fingerprint.HeaderSession,
	} {
		if got.headers.Get(h) == "" {
			t.Errorf("fingerprint header %s missing", h)
		}
	}
	if got.headers.Get(fingerprint.HeaderSession) != "u1" {
		t.Errorf("session = %q, want u1", got.headers.Get(fingerprint.HeaderSession))
	}
	if got.headers.Get("User-Agent") != "phanthy-codeo/0.2.11" {
		t.Errorf("User-Agent = %q", got.headers.Get("User-Agent"))
	}
}

func TestChatStream_APIKeyFallback_WhenNoToken(t *testing.T) {
	var got reqCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer srv.Close()

	c := New(srv.URL) // 无指纹：回退历史行为 + x-api-key
	a := mkAuthFull("u1", "", "legacy-key")

	rc, status, _, err := c.ChatStream(a, []byte(`{"model":"x","max_tokens":1}`))
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer rc.Close()
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if got.headers.Get("x-api-key") != "legacy-key" {
		t.Errorf("x-api-key = %q", got.headers.Get("x-api-key"))
	}
	if got.headers.Get("Authorization") != "" {
		t.Errorf("Authorization should be empty for key-only account")
	}
}

func TestChatStream_MissingCreds_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	a := mkAuthFull("u1", "", "")
	rc, status, respBody, err := c.ChatStream(a, []byte(`{}`))
	if rc != nil {
		rc.Close()
		t.Error("rc should be nil for missing creds")
	}
	if err != nil {
		t.Fatalf("err should be nil (status-path), got %v", err)
	}
	if status != 401 {
		t.Errorf("status = %d, want 401", status)
	}
	if !strings.Contains(string(respBody), "missing_login") {
		t.Errorf("body = %s", respBody)
	}
}

func TestFetchModelStats_SendsBearerAndProduct(t *testing.T) {
	var got reqCapture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.headers = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"balance":123}}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.FP = fingerprint.New(fingerprint.Config{Enabled: true})
	a := mkAuthFull("u1", "tok", "")

	raw, err := c.FetchModelStats(a)
	if err != nil {
		t.Fatalf("FetchModelStats: %v", err)
	}
	if string(raw) != `{"data":{"balance":123}}` {
		t.Errorf("raw = %s", raw)
	}
	if got.headers.Get("Authorization") != "Bearer tok" {
		t.Errorf("Authorization = %q", got.headers.Get("Authorization"))
	}
	if got.headers.Get(fingerprint.HeaderProduct) != "phanthy_code" {
		t.Errorf("product = %q", got.headers.Get(fingerprint.HeaderProduct))
	}
	if got.headers.Get(fingerprint.HeaderRequest) == "" {
		t.Error("x-phanthy-request missing")
	}
}

func TestFetchModels_SendsBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/phanthy_code/models" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"x"}]`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	c.FP = fingerprint.New(fingerprint.Config{Enabled: true})
	a := mkAuthFull("u1", "tok", "")
	raw, err := c.FetchModels(a)
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if string(raw) != `[{"id":"x"}]` {
		t.Errorf("raw = %s", raw)
	}
}
