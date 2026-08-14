// Package server 暴露 OpenAI 兼容 HTTP 接口，内部驱动 pool 挑号 + upstream 转发。
package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/upstream"
)

// Config handler 依赖。
type Config struct {
	Pool         *pool.Pool
	Upstream     *upstream.Client
	APIKey       string        // 空 = 不鉴权
	MaxRotate    int           // 单请求最多换号次数，默认 3
	HardCooldown time.Duration // 积分不足冷却，默认 12h
	SoftCooldown time.Duration // 429 冷却，默认 60s
	ErrThreshold int           // 连续其他错误冷却阈值，默认 3
	ErrCooldown  time.Duration // 错误冷却时长，默认 10m
	RefreshSkew  time.Duration // token 提前刷新窗口，默认 10m
}

// Handler 主路由。
type Handler struct {
	cfg Config
	mux *http.ServeMux
}

// NewHandler 构建 handler。
func NewHandler(cfg Config) *Handler {
	if cfg.MaxRotate <= 0 {
		cfg.MaxRotate = 3
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
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.APIKey != "" {
			authz := r.Header.Get("Authorization")
			if !strings.HasPrefix(authz, "Bearer ") || strings.TrimPrefix(authz, "Bearer ") != h.cfg.APIKey {
				writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
				return
			}
		}
		next(w, r)
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

// 静态模型表（客户端可见的商业命名）。
var staticModels = []map[string]any{
	{"id": "DeepSeek-V4", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "gpt-5.6-sol", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "gpt-5.6-terra", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "gpt-5.6-luna", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "gpt-5.5", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "Claude Opus 4.8", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "Claude Opus 4.7", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "Claude Sonnet 4.6", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "Kimi K3", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "Kimi-k2.7-code", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "Kimi K2.6", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "GLM 5.2", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
	{"id": "GLM-5.1", "object": "model", "created": 1753600000, "owned_by": "phanthy", "context_length": 200000},
}

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   staticModels,
	})
}

func (h *Handler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "read body: "+err.Error())
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
	for i := 0; i < h.cfg.MaxRotate; i++ {
		acct := h.cfg.Pool.PickExcluding(tried)
		if acct == nil {
			break
		}
		tried[acct.UID] = true

		// token 临近过期 → 先 refresh（失败冷却换号）
		if acct.NeedsRefresh(h.cfg.RefreshSkew) {
			if err := h.cfg.Upstream.RefreshToken(acct); err != nil {
				lastErr = err
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

		// 确保 api_key 可用（失败不阻塞，ChatStream 会用 access_token 兜底）
		if acct.APIKey == "" {
			if err := h.cfg.Upstream.EnsureAPIKey(acct); err != nil {
				log.Printf("ensure_api_key uid=%s: %v (will try access_token)", acct.UID, err)
			}
		}

		// 准备请求体（OpenAI → Anthropic 转换）
		anthroBody := upstream.PrepareBody(body)

		rc, status, respBody, terr := h.cfg.Upstream.ChatStream(acct, anthroBody)
		if terr != nil {
			lastErr = terr
			h.cfg.Pool.NoteError(acct.UID, h.cfg.ErrThreshold, h.cfg.ErrCooldown)
			continue
		}
		if status >= 400 {
			kind := upstream.Classify(status, string(respBody))
			switch kind {
			case upstream.ErrHardCredit:
				h.cfg.Pool.Cooldown(acct.UID, pool.CoolHard, h.cfg.HardCooldown, "积分不足")
				lastErr = &upstream.Error{Kind: kind, Status: status, Msg: string(respBody)}
				continue
			case upstream.ErrSoftRate:
				h.cfg.Pool.Cooldown(acct.UID, pool.CoolSoft, h.cfg.SoftCooldown, "429 rate limit")
				lastErr = &upstream.Error{Kind: kind, Status: status, Msg: string(respBody)}
				continue
			case upstream.ErrSessionDead:
				h.cfg.Pool.Disable(acct.UID, "session dead")
				lastErr = &upstream.Error{Kind: kind, Status: status, Msg: string(respBody)}
				continue
			case upstream.ErrNotFound:
				h.cfg.Pool.Cooldown(acct.UID, pool.CoolSoft, h.cfg.SoftCooldown, "upstream 404")
				lastErr = &upstream.Error{Kind: kind, Status: status, Msg: string(respBody)}
				continue
			default:
				h.cfg.Pool.NoteError(acct.UID, h.cfg.ErrThreshold, h.cfg.ErrCooldown)
				lastErr = &upstream.Error{Kind: kind, Status: status, Msg: string(respBody)}
				continue
			}
		}
		defer rc.Close()
		h.cfg.Pool.NoteSuccess(acct.UID)

		if peek.Stream {
			if err := upstream.Stream(w, rc, peek.Model); err != nil {
				log.Printf("stream error: %v", err)
			}
			return
		}
		resp, err := upstream.Aggregate(rc, peek.Model)
		if err != nil {
			writeOpenAIError(w, http.StatusBadGateway, "upstream_parse", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	msg := "all accounts unavailable (cooling/disabled)"
	if lastErr != nil {
		msg += ": " + lastErr.Error()
	}
	writeOpenAIError(w, http.StatusServiceUnavailable, "no_healthy_account", msg)
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

// Ensure imports are used (sync/pool are used via Config).
var _ = sync.Mutex{}