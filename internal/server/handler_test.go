package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"phanthycode2api/internal/admin"
	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/upstream"
)

// TestPeekRequest_LargeBodyNotTruncated 是回归测试。
// peekRequest 之前只读 1MB 就把 body 替换成截断内容，超过 1MB 的请求
// （Codex 的大上下文会话）会变成非法 JSON，上游稳定返回
// 400 {"error":{"code":"invalid_request","message":"Request body must be JSON."}}。
func TestPeekRequest_LargeBodyNotTruncated(t *testing.T) {
	filler := strings.Repeat("x", 3<<20) // 3MB，远超过旧的 1MB 截断点
	body := `{"model":"deepseek-v4.1-flash","stream":true,"messages":[{"role":"user","content":"` + filler + `"}]}`

	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	model, stream := peekRequest(r)
	if model != "deepseek-v4.1-flash" || !stream {
		t.Fatalf("peekRequest = (%q, %v), want (deepseek-v4.1-flash, true)", model, stream)
	}

	// 缓存必须完整：调用方随后再读一次，应拿到与原始请求等长的合法 JSON。
	rest, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("重新读取 body: %v", err)
	}
	if len(rest) != len(body) {
		t.Errorf("缓存后 body 长度 = %d, want %d（说明被截断）", len(rest), len(body))
	}
	if !json.Valid(rest) {
		t.Error("缓存后的 body 不是合法 JSON，上游会返回 400 Request body must be JSON.")
	}
}

// TestPeekRequest_SmallBody 小请求体也能正常返回并保留可读 body。
func TestPeekRequest_SmallBody(t *testing.T) {
	body := `{"model":"kimi-k3","stream":false,"messages":[]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	model, stream := peekRequest(r)
	if model != "kimi-k3" || stream {
		t.Fatalf("peekRequest = (%q, %v), want (kimi-k3, false)", model, stream)
	}
	rest, _ := io.ReadAll(r.Body)
	if string(rest) != body {
		t.Errorf("body = %q, want %q", rest, body)
	}
}

// TestPeekRequest_NilBody 空请求不应 panic。
func TestPeekRequest_NilBody(t *testing.T) {
	if model, stream := peekRequest(nil); model != "" || stream {
		t.Errorf("peekRequest(nil) = (%q, %v), want empty", model, stream)
	}
}

// denialSSE 复刻上游拒绝服务时的真实响应：HTTP 200 + text/event-stream，
// 错误只写在事件流里（不到 200 字节，X-Kong-Upstream-Latency 极低）。
const denialSSE = `event: error
data: {"type":"error","error":{"type":"api_error","code":"upstream_permission_denied","message":"Model service access was denied. Choose another model or contact support."}}`

// TestChatCompletions_StreamErrorNotLoggedAs200 是核心回归测试。
//
// 上游用 HTTP 200 承载 SSE error 事件，旧实现只看状态码，于是：
//   - 客户端收到 finish_reason=stop 的空回复（表现为「一直正在思考」）
//   - 管理台记成 status=200、tokens=0/0 的「成功」请求
//
// 修复后必须把上游错误如实返回，并让审计日志的状态码非 200。
func TestChatCompletions_StreamErrorNotLoggedAs200(t *testing.T) {
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, denialSSE)
	}))
	defer upstreamSrv.Close()

	store, err := admin.New(t.TempDir())
	if err != nil {
		t.Fatalf("admin.New: %v", err)
	}
	acctPool := pool.New("")
	acctPool.Add(&auth.Auth{
		UID:         "phanthy-test",
		APIKey:      "sk-test",
		AccessToken: "tok-test",
		ExpiresAt:   time.Now().Add(time.Hour).Unix(),
	})
	h := NewHandler(Config{
		Pool:      acctPool,
		Upstream:  upstream.New(upstreamSrv.URL),
		Admin:     admin.NewHandler(store, "", nil),
		MaxRotate: 2,
	})

	body := `{"model":"deepseek-v4.1-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("HTTP 状态 = %d，上游明确拒绝时不应返回 200，body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"finish_reason":"stop"`) {
		t.Errorf("不得把拒绝伪装成正常结束，body=%s", rec.Body.String())
	}

	logs := store.ListLogs(10, "")
	if len(logs) == 0 {
		t.Fatal("没有写入审计日志")
	}
	if logs[0].Status == http.StatusOK {
		t.Errorf("审计日志 status = 200，被拒绝的请求不能记成成功（tokens=%d/%d）",
			logs[0].PromptTok, logs[0].CompletionTok)
	}
}

// TestChatCompletions_ModelDeniedKeepsAccountHealthy 回归：模型未授权属请求侧问题，
// 不能因此冷却/禁用账号，否则一次错误调用会把唯一可用账号打进冷宫。
func TestChatCompletions_ModelDeniedKeepsAccountHealthy(t *testing.T) {
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, denialSSE)
	}))
	defer upstreamSrv.Close()

	acctPool := pool.New("")
	acctPool.Add(&auth.Auth{
		UID: "phanthy-test", APIKey: "sk-test",
		AccessToken: "tok-test", ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	h := NewHandler(Config{Pool: acctPool, Upstream: upstream.New(upstreamSrv.URL), MaxRotate: 2})

	body := `{"model":"deepseek-v4.1-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	st, ok := acctPool.Status("phanthy-test")
	if !ok {
		t.Fatal("账号不见了")
	}
	if st.Cooling || st.Disabled {
		t.Errorf("模型未授权不应冷却账号：cooling=%v disabled=%v reason=%q", st.Cooling, st.Disabled, st.Reason)
	}

	// 换遍所有账号都被拒，才如实返回「模型不可用」，而不是含糊的 503。
	if rec.Code != http.StatusBadRequest {
		t.Errorf("HTTP 状态 = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "model_not_allowed") {
		t.Errorf("应返回 model_not_allowed，body=%s", rec.Body.String())
	}
}

// TestChatCompletions_DoesNotProbeAPIKeyOnHotPath 回归：上游的 create_api_key
// 已下线，实测会挂几十秒才返回 404 页面。这个探测绝对不能留在请求热路径上
// ——放在那里会让服务启动后的第一个请求白白多等半分钟，而 ChatStream 用
// access_token 兜底本来就能跑通。
func TestChatCompletions_DoesNotProbeAPIKeyOnHotPath(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, okSSE)
	}))
	defer upstreamSrv.Close()

	acctPool := pool.New("")
	// APIKey 留空：旧实现正是靠这个条件在请求里触发 create_api_key 探测。
	acctPool.Add(&auth.Auth{
		UID: "phanthy-test", AccessToken: "tok-test",
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	h := NewHandler(Config{Pool: acctPool, Upstream: upstream.New(upstreamSrv.URL), MaxRotate: 2})

	body := `{"model":"phanthy-pro","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP 状态 = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range paths {
		if strings.Contains(p, "create_api_key") {
			t.Errorf("请求热路径不应探测 create_api_key，实际访问 %s", p)
		}
	}
}

// okSSE 是一段正常的流式回复，用来验证换号之后请求能真正跑通。
const okSSE = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"phanthy-pro","usage":{"input_tokens":5,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":3}}

event: message_stop
data: {"type":"message_stop"}

data: [DONE]`

// TestChatCompletions_ModelDeniedFailsOverToOtherAccount 是核心回归测试。
//
// 上游的「模型服务被拒」是按账号阵发性出现的：实测同一账号同一模型
// 5 分钟前被拒（0/0 tokens）、之后又正常。旧实现一旦某个账号被拒就
// 直接返回 400，客户端看到 424 只能干等——即使池子里还有别的可用账号。
//
// 修复后必须换号重试，并只在所有账号都被拒时才返回 model_not_allowed。
func TestChatCompletions_ModelDeniedFailsOverToOtherAccount(t *testing.T) {
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if r.Header.Get("X-Claude-Code-Session-Id") == "phanthy-a" {
			_, _ = io.WriteString(w, denialSSE)
			return
		}
		_, _ = io.WriteString(w, okSSE)
	}))
	defer upstreamSrv.Close()

	store, err := admin.New(t.TempDir())
	if err != nil {
		t.Fatalf("admin.New: %v", err)
	}
	acctPool := pool.New("")
	for _, uid := range []string{"phanthy-a", "phanthy-b"} {
		acctPool.Add(&auth.Auth{
			UID: uid, APIKey: "sk-" + uid,
			AccessToken: "tok-" + uid, ExpiresAt: time.Now().Add(time.Hour).Unix(),
		})
	}
	h := NewHandler(Config{
		Pool:      acctPool,
		Upstream:  upstream.New(upstreamSrv.URL),
		Admin:     admin.NewHandler(store, "", nil),
		MaxRotate: 2,
	})

	body := `{"model":"phanthy-pro","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP 状态 = %d, want 200（账号 a 被拒应换到账号 b）, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hello") {
		t.Errorf("应返回账号 b 的正文，body=%s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "upstream_permission_denied") {
		t.Errorf("换号成功后不应把账号 a 的拒绝写进流里，body=%s", rec.Body.String())
	}

	logs := store.ListLogs(10, "")
	if len(logs) == 0 {
		t.Fatal("没有写入审计日志")
	}
	if logs[0].Status != http.StatusOK || logs[0].UID != "phanthy-b" {
		t.Errorf("审计日志 = status=%d uid=%s, want 200/phanthy-b", logs[0].Status, logs[0].UID)
	}

	st, ok := acctPool.Status("phanthy-a")
	if !ok {
		t.Fatal("账号 a 不见了")
	}
	if st.Cooling || st.Disabled {
		t.Errorf("被上游拒绝的账号仍应健康：cooling=%v disabled=%v reason=%q", st.Cooling, st.Disabled, st.Reason)
	}
}
