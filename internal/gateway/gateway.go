// Package gateway 实现网关对外接口：OpenAI 兼容路由、鉴权、账号轮转与响应转发。
package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"phanthycode2api/internal/config"
	"phanthycode2api/internal/phanthy"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/protocol"
)

const (
	// DefaultMaxRotate 是单个请求最多尝试的账号数。
	DefaultMaxRotate = 3
	// DefaultRefreshSkew 是提前刷新的时间窗口（FR-11）。
	DefaultRefreshSkew = 10 * time.Minute
	maxRequestBytes    = 8 << 20 // 请求体上限 8 MiB
)

// Gateway 是 HTTP 入口。
type Gateway struct {
	cfg      *config.Config
	pool     *pool.Pool
	upstream *phanthy.Client
	logger   *log.Logger
	now      func() time.Time

	// MaxRotate 与 RefreshSkew 可在测试中覆盖。
	MaxRotate   int
	RefreshSkew time.Duration
}

// New 构建网关。
func New(cfg *config.Config, p *pool.Pool, upstream *phanthy.Client, logger *log.Logger) *Gateway {
	if logger == nil {
		logger = log.Default()
	}
	return &Gateway{
		cfg:         cfg,
		pool:        p,
		upstream:    upstream,
		logger:      logger,
		now:         time.Now,
		MaxRotate:   DefaultMaxRotate,
		RefreshSkew: DefaultRefreshSkew,
	}
}

// Handler 返回完整路由。
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", g.handleHealthz)
	mux.Handle("/v1/models", g.withAuth(http.HandlerFunc(g.handleModels)))
	mux.Handle("/v1/chat/completions", g.withAuth(http.HandlerFunc(g.handleChat)))
	mux.Handle("/status", g.withAuth(http.HandlerFunc(g.handleStatus)))
	return mux
}

// withAuth 在配置了 api_key 时校验 Authorization 头。
func (g *Gateway) withAuth(next http.Handler) http.Handler {
	key := strings.TrimSpace(g.cfg.APIKey)
	if key == "" {
		return next
	}
	expected := []byte("Bearer " + key)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			writeError(w, http.StatusUnauthorized, "api_error", "invalid_api_key", "missing or invalid API key")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Gateway) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (g *Gateway) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "只支持 GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   protocol.ModelList(),
	})
}

func (g *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "只支持 GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts": g.pool.List(g.now()),
	})
}

// errorEnvelope 是 OpenAI 风格的错误响应。
type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func writeError(w http.ResponseWriter, status int, typ, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorDetail{Message: message, Type: typ, Code: code}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":{"message":"序列化响应失败","type":"api_error","code":"internal_error"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(raw, '\n'))
}
