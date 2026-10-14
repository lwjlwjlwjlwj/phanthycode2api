// Package server 暴露 OpenAI 兼容 HTTP 接口，内部驱动 pool 挑号 + upstream 转发。
package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"phanthycode2api/internal/admin"
	"phanthycode2api/internal/logx"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/upstream"
)

// maxBodyBytes 单次请求体上限。
// Codex 在大上下文下会发送数 MB 的会话历史，上限给足并显式报错，避免静默截断。
const maxBodyBytes = 32 << 20

// Config handler 依赖。
type Config struct {
	Pool         *pool.Pool
	Upstream     *upstream.Client
	APIKey       string                  // 空 = 不鉴权
	MaxRotate    int                     // 单请求最多换号次数，默认 2
	HardCooldown time.Duration           // 积分不足冷却，默认 12h
	SoftCooldown time.Duration           // 429 冷却，默认 60s
	ErrThreshold int                     // 连续其他错误冷却阈值，默认 3
	ErrCooldown  time.Duration           // 错误冷却时长，默认 10m
	Admin        *admin.Handler          // 可空；非空时启用 /admin 与分发密钥
	RefreshSkew  time.Duration           // token 提前刷新窗口，默认 10m
	Thinking     upstream.ThinkingOption // 扩展思考策略，见 upstream.ThinkingOption
}

// Handler 主路由。
type Handler struct {
	cfg Config
	mux *http.ServeMux
}

// NewHandler 构建 handler。
func NewHandler(cfg Config) *Handler {
	if cfg.MaxRotate <= 0 {
		// 单请求最多换号两次：上游超时类失败重试很少成功，
		// 换号次数过多只会把尾部等待时间成倍拉长。
		cfg.MaxRotate = 2
	}
	if cfg.HardCooldown <= 0 {
		cfg.HardCooldown = 12 * time.Hour
	}
	if cfg.SoftCooldown <= 0 {
		cfg.SoftCooldown = 60 * time.Second
	}
	if cfg.ErrThreshold <= 0 {
		cfg.ErrThreshold = 3
	}
	if cfg.ErrCooldown <= 0 {
		cfg.ErrCooldown = 10 * time.Minute
	}
	if cfg.RefreshSkew <= 0 {
		cfg.RefreshSkew = 10 * time.Minute
	}
	h := &Handler{cfg: cfg, mux: http.NewServeMux()}
	h.mux.HandleFunc("POST /v1/chat/completions", h.withAuth(h.chatCompletions))
	h.mux.HandleFunc("GET /v1/models", h.withAuth(h.models))
	h.mux.HandleFunc("GET /status", h.withAuth(h.status))
	h.mux.HandleFunc("GET /healthz", h.healthz)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Admin != nil && (r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/")) {
		h.cfg.Admin.ServeHTTP(w, r)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plain := ""
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(authz, "Bearer ") {
			plain = strings.TrimPrefix(authz, "Bearer ")
		}
		model, stream := peekRequest(r)
		if h.cfg.APIKey == "" && h.cfg.Admin == nil {
			next(w, r)
			return
		}
		if h.cfg.Admin != nil {
			if key, ok := h.cfg.Admin.LookupKey(plain, model); ok {
				h.serveWithLog(w, r, next, model, stream, key.ID, key.Name)
				return
			}
		}
		if h.cfg.APIKey != "" && subtle.ConstantTimeCompare([]byte(plain), []byte(h.cfg.APIKey)) == 1 {
			h.serveWithLog(w, r, next, model, stream, "", "")
			return
		}
		if h.cfg.APIKey == "" && (h.cfg.Admin == nil || h.cfg.Admin.LegacyUnauthenticated()) {
			h.serveWithLog(w, r, next, model, stream, "", "")
			return
		}
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
	}
}

// serveWithLog 记录 /v1/chat/completions 的模型、状态与耗时。
func (h *Handler) serveWithLog(w http.ResponseWriter, r *http.Request, next http.HandlerFunc, model string, stream bool, keyID, keyName string) {
	if h.cfg.Admin == nil || r.URL.Path != "/v1/chat/completions" {
		next(w, r)
		return
	}
	start := time.Now()
	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	next(recorder, r)
	latency := time.Since(start).Milliseconds()
	h.cfg.Admin.NoteRequest(admin.LogEntry{
		Time:          time.Now().Unix(),
		KeyID:         keyID,
		KeyName:       keyName,
		UID:           recorder.uid,
		Model:         model,
		Status:        recorder.status,
		LatencyMs:     latency,
		PromptTok:     int64(recorder.promptTok),
		CompletionTok: int64(recorder.completionTok),
		Stream:        stream,
		RemoteIP:      remoteIP(r),
	})
	key := keyName
	if key == "" {
		key = "-"
	}
	logx.Infof("req key=%s uid=%s model=%s status=%d %dms tokens=%d/%d stream=%v",
		key, orDash(recorder.uid), orDash(model), recorder.status, latency,
		recorder.promptTok, recorder.completionTok, stream)
}

type statusRecorder struct {
	http.ResponseWriter
	status        int
	uid           string
	promptTok     int
	completionTok int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(data []byte) (int, error) {
	return w.ResponseWriter.Write(data)
}

func (w *statusRecorder) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts": h.cfg.Pool.List(),
	})
}

// modelsCreated 是模型列表的固定创建时间（秒），保证响应稳定可缓存。
const modelsCreated = 1753600000

// modelEntries 由 upstream.CanonicalModels 生成，保证 /v1/models
// 与真实可调用的上游模型保持一致。
var modelEntries = buildModelEntries()

func buildModelEntries() []map[string]any {
	out := make([]map[string]any, 0, len(upstream.CanonicalModels))
	for _, m := range upstream.CanonicalModels {
		out = append(out, map[string]any{
			"id": m.ID, "object": "model", "created": modelsCreated,
			"owned_by": "phanthy", "context_length": m.Context,
		})
	}
	return out
}

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   modelEntries,
	})
}

func (h *Handler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "read body: "+err.Error())
		return
	}
	if len(body) == 0 {
		// 探测类空请求（部分启动器会这样探活）不必打扰上游。
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "empty request body")
		return
	}
	if len(body) > maxBodyBytes {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request_too_large",
			fmt.Sprintf("request body exceeds %d MB", maxBodyBytes>>20))
		return
	}

	// 探测是否流式
	var peek struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &peek)

	tried := map[string]bool{}
	var lastErr error
	// lastDenied 记录「上游拒绝该模型」的原始原因。模型被拒可能是单账号套餐
	// 差异，也可能是上游瞬时抖动（实测同账号同模型 5 分钟前被拒、之后正常），
	// 所以先换号试一遍，只有所有账号都被拒才把错误交给客户端。
	var lastDenied string
	var served *statusRecorder
	if rec, ok := w.(*statusRecorder); ok {
		served = rec
	}
	maxAttempts := h.cfg.MaxRotate
	// widen 在模型被拒时把预算放宽到健康账号数：上游拒绝是秒回，
	// 多试几个号比直接断定「套餐不含该模型」准确得多。
	widen := func() {
		if n := h.cfg.Pool.HealthyCount(); n > maxAttempts {
			maxAttempts = n
		}
	}
	for i := 0; i < maxAttempts; i++ {
		acct := h.cfg.Pool.PickExcluding(tried)
		if acct == nil {
			break
		}
		tried[acct.UID] = true
		logx.Debugf("chat attempt=%d/%d uid=%s model=%s", i+1, maxAttempts, acct.UID, peek.Model)

		// token 临近过期 → 先 refresh（失败冷却换号）
		if acct.NeedsRefresh(h.cfg.RefreshSkew) {
			if err := h.cfg.Upstream.RefreshToken(acct); err != nil {
				lastErr = err
				logx.Debugf("chat uid=%s refresh failed: %v", acct.UID, err)
				var ue *upstream.Error
				if errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead {
					h.cfg.Pool.Disable(acct.UID, "refresh session dead")
				} else {
					h.cfg.Pool.Cooldown(acct.UID, pool.CoolErr, h.cfg.ErrCooldown, "refresh: "+err.Error())
				}
				continue
			}
			_ = acct.SaveAtomic()
		}

		// 这里刻意不调 EnsureAPIKey：上游的 create_api_key 接口已下线，实测会
		// 挂几十秒才返回 404 页面（`upstream.timeout_seconds` 内都算「成功」返回），
		// 放在热路径上会让对象启动后的第一个请求白白多等半分钟。
		// ChatStream 会用 access_token 兜底，api_key 只是优化项，交给
		// keepalive 在后台慢慢补，不影响请求正确性。

		// 准备请求体（OpenAI → Anthropic 转换，并按配置下发 thinking 开关）
		anthroBody := upstream.PrepareBody(body, h.cfg.Thinking)

		rc, status, respBody, terr := h.cfg.Upstream.ChatStream(r.Context(), acct, anthroBody)
		if terr != nil {
			lastErr = terr
			h.cfg.Pool.NoteError(acct.UID, h.cfg.ErrThreshold, h.cfg.ErrCooldown)
			continue
		}
		if status >= 400 {
			kind := upstream.Classify(status, string(respBody))
			lastErr = &upstream.Error{Kind: kind, Status: status, Msg: string(respBody)}
			if h.applyFailure(acct.UID, kind) {
				// 该账号不被允许用这个模型：属请求侧问题，账号仍然健康，
				// 不冷却、不计错误。换号再试，别让单个账号的套餐挡掉整个请求。
				lastDenied = strings.TrimSpace(string(respBody))
				widen()
				logx.Infof("chat uid=%s model=%s 上游未授权该模型（http %d %s），换号重试 %d/%d",
					acct.UID, orDash(peek.Model), status, kind, i+1, maxAttempts)
				continue
			}
			logx.Debugf("chat uid=%s upstream status=%d kind=%s", acct.UID, status, kind)
			continue
		}
		defer rc.Close()
		if served != nil {
			served.uid = acct.UID
		}

		if peek.Stream {
			in, out, serr := upstream.Stream(w, rc, peek.Model)
			if served != nil {
				served.promptTok, served.completionTok = in, out
			}
			if serr == nil {
				h.cfg.Pool.NoteSuccess(acct.UID)
				return
			}
			var se *upstream.StreamError
			if errors.As(serr, &se) {
				if se.Kind == upstream.ErrModelDenied && !se.Committed {
					// 还没给下游写过任何字节，可以安全换号重来。
					lastDenied = se.Message()
					widen()
					logx.Infof("chat uid=%s model=%s 上游未授权该模型（%s %s），换号重试 %d/%d",
						acct.UID, orDash(peek.Model), se.Kind, clip(se.Message(), 200), i+1, maxAttempts)
				}
				if h.failStream(w, served, acct.UID, se) {
					return
				}
				lastErr = serr
				continue
			}
			// 写回下游失败（客户端中断等）：上游本身没问题，不再换号。
			logx.Errorf("stream uid=%s: %v", acct.UID, serr)
			return
		}
		resp, err := upstream.Aggregate(rc, peek.Model)
		if err != nil {
			var se *upstream.StreamError
			if errors.As(err, &se) {
				if se.Kind == upstream.ErrModelDenied && !se.Committed {
					lastDenied = se.Message()
					widen()
					logx.Infof("chat uid=%s model=%s 上游未授权该模型（%s %s），换号重试 %d/%d",
						acct.UID, orDash(peek.Model), se.Kind, clip(se.Message(), 200), i+1, maxAttempts)
				}
				if h.failStream(w, served, acct.UID, se) {
					return
				}
				lastErr = err
				continue
			}
			logx.Errorf("aggregate uid=%s: %v", acct.UID, err)
			writeOpenAIError(w, http.StatusBadGateway, "upstream_parse", err.Error())
			return
		}
		h.cfg.Pool.NoteSuccess(acct.UID)
		if served != nil {
			served.promptTok, served.completionTok = upstream.UsageTotals(resp)
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if lastDenied != "" {
		// 所有健康账号都被上游拒绝该模型。明确告诉调用方这是模型/套餐问题，
		// 而不是「账号不可用」——后者会让客户端去重试同样的请求。
		writeOpenAIError(w, http.StatusBadRequest, "model_not_allowed",
			fmt.Sprintf("model %q was rejected by all %d tried account(s): %s",
				peek.Model, len(tried), lastDenied))
		return
	}
	msg := "all accounts unavailable (cooling/disabled)"
	if lastErr != nil {
		msg += ": " + lastErr.Error()
	}
	writeOpenAIError(w, http.StatusServiceUnavailable, "no_healthy_account", msg)
}

// applyFailure 按上游错误分类更新账号池状态。
//
// 返回 true 表示这是「请求侧」问题（当前账号的套餐不含该模型）：账号本身健康，
// 不能冷却/计错，但值得换号再试——套餐按账号而异，且上游会瞬时拒绝。
func (h *Handler) applyFailure(uid string, kind upstream.ErrKind) bool {
	switch kind {
	case upstream.ErrModelDenied:
		// 上游按套餐放行模型：同一账号对 phanthy-pro 正常，对未授权模型秒拒。
		// 这是模型权限问题，不是账号故障，冷却/计错都会误伤可用账号。
		return true
	case upstream.ErrHardCredit:
		h.cfg.Pool.Cooldown(uid, pool.CoolHard, h.cfg.HardCooldown, "积分不足")
	case upstream.ErrSoftRate:
		h.cfg.Pool.Cooldown(uid, pool.CoolSoft, h.cfg.SoftCooldown, "429 rate limit")
	case upstream.ErrSessionDead:
		h.cfg.Pool.Disable(uid, "session dead")
	case upstream.ErrNotFound:
		// 上游 404 多为瞬时抖动，短冷却即可，不累计 errCount 以免雪崩。
		h.cfg.Pool.Cooldown(uid, pool.CoolSoft, h.cfg.SoftCooldown, "upstream 404")
	default:
		h.cfg.Pool.NoteError(uid, h.cfg.ErrThreshold, h.cfg.ErrCooldown)
	}
	return false
}

// failStream 处理上游用 HTTP 200 + `event: error` 下发的拒绝。
//
// 上游拒绝服务时不会返回 4xx/5xx，错误只存在于 SSE 事件流里。旧实现只看状态码，
// 于是把「被拒绝」当成「正常结束的空回复」，客户端一直转圈、审计日志记成
// status=200/tokens=0/0 的成功请求。这里按分类如实处理并修正审计状态。
//
// 注意「模型未授权」要区别对待：账号本身是健康的，而且换号重试可能成功
// （套餐按账号而异，上游也会瞬时拒绝），所以只要还没给下游写过任何字节，
// 就返回 false 让主循环换号重试，绝不在这里下结论。
//
// 返回 true 表示已经就地响应或收尾，调用方应直接返回；false 表示可换号重试。
func (h *Handler) failStream(w http.ResponseWriter, served *statusRecorder, uid string, se *upstream.StreamError) bool {
	if se == nil {
		return false
	}
	ev := se.Event
	code, msg := "upstream_error", ""
	if ev != nil {
		if ev.Code != "" {
			code = ev.Code
		}
		msg = ev.Message
	}
	if msg == "" {
		msg = code
	}
	logx.Errorf("stream uid=%s kind=%s code=%s committed=%v msg=%q",
		uid, se.Kind, code, se.Committed, clip(msg, 300))

	if se.Kind == upstream.ErrModelDenied {
		// 账号健康：不冷却、不计错。
		if !se.Committed {
			// 一个字节都没下发，HTTP 状态码还能改，交给主循环换号重试。
			return false
		}
		// 正文已经开始下发，状态码改不动了；Stream 已把 error 事件写进流里，
		// 这里只把审计状态从 200 修正掉，避免这次拒绝在管理台上隐身。
		if served != nil && served.status < 400 {
			served.status = http.StatusBadGateway
		}
		logx.Errorf("stream uid=%s 已开始下发正文，无法换号重试", uid)
		return true
	}

	fatal := h.applyFailure(uid, se.Kind)

	if se.Committed {
		// 响应头/正文已经发给下游，HTTP 状态码改不动了。
		// 至少别让审计日志继续记成 200 成功，否则这个 bug 又隐身了。
		if served != nil && served.status < 400 {
			served.status = http.StatusBadGateway
		}
		return true
	}
	if !fatal {
		// 可恢复错误：交给主循环换号重试。
		return false
	}

	status, outCode := http.StatusBadGateway, code
	switch se.Kind {
	case upstream.ErrHardCredit:
		status, outCode = http.StatusPaymentRequired, "insufficient_credit"
	case upstream.ErrSessionDead:
		status, outCode = http.StatusBadGateway, "upstream_session_dead"
	}
	writeOpenAIError(w, status, outCode, "upstream: "+msg)
	return true
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	raw, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeOpenAIError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "api_error",
			"code":    code,
		},
	})
}

func peekRequest(r *http.Request) (string, bool) {
	if r == nil || r.Body == nil {
		return "", false
	}
	// 一次性读完并缓存：只读开头一段会把大请求体截断成非法 JSON，
	// 而截断后的 JSON 正是上游 400「Request body must be JSON.」的来源。
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return "", false
	}
	var payload struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &payload)
	return payload.Model, payload.Stream
}
func remoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// orDash 把空字符串显示成 "-"，避免日志里出现 key= 这种断掉的字段。
func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// clip 截断过长的日志文本，避免把整段 HTML 错误页写进日志。
func clip(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
