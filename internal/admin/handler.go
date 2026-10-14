package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// AccountManager 账号操作回调，由 server 注入，避免 admin 直接依赖 pool/upstream。
type AccountManager struct {
	StartOAuth func() (map[string]any, error)
	List       func() []map[string]any
	Add        func(req map[string]any) error
	Delete     func(uid string) error
	Refresh    func(uid string) error
	Enable     func(uid string) error
	Keepalive  func(uid string) error
	Claim      func(uid string) error
}

// Handler 承载 /admin 页面与 /admin/api/* 接口。
type Handler struct {
	store          *Store
	Accounts       *AccountManager
	configPath     string
	passwordHash   *string
	PasswordSet    func() bool
	Verify         func(password string) bool
	ChangePassword func(password string) error
}

// LookupKey 暴露分发密钥查询，供 /v1 网关鉴权。
func (h *Handler) LookupKey(plain, model string) (*APIKey, bool) {
	if h.store == nil {
		return nil, false
	}
	return h.store.LookupKey(plain, model)
}

// NoteRequest 供 /v1 请求完成后写入审计日志。
func (h *Handler) NoteRequest(entry LogEntry) {
	if h.store != nil {
		h.store.AddLog(entry)
	}
}

// LegacyUnauthenticated 报告是否保留空 api_key 时的旧版免鉴权行为。
func (h *Handler) LegacyUnauthenticated() bool {
	return h.store != nil && h.store.legacyUnauthenticated
}

// NewHandler 构建管理端 Handler；密码哈希写入配置路径由 server 提交。
func NewHandler(store *Store, configPath string, passwordHash *string) *Handler {
	return &Handler{store: store, configPath: configPath, passwordHash: passwordHash, PasswordSet: func() bool { return true }}
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(indexHTML))
}

// ServeHTTP 路由 /admin 页面与 API。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/admin") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/admin" {
		h.page(w, r)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/admin/api/") {
		http.NotFound(w, r)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/admin/api/")
	if action == "login" {
		if r.Method != http.MethodPost {
			writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		h.login(w, r)
		return
	}
	if action == "logout" {
		if r.Method != http.MethodPost {
			writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/admin", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if !h.authorized(r) {
		writeAdminJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	segments := strings.Split(strings.Trim(action, "/"), "/")
	switch segments[0] {
	case "me":
		if r.Method != http.MethodGet {
			writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "accounts":
		h.accounts(w, r, segments[1:])
	case "keys":
		h.keys(w, r, segments[1:])
	case "logs":
		if r.Method == http.MethodDelete {
			h.store.ClearLogs()
			writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		if r.Method != http.MethodGet {
			writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 5000 {
			limit = 200
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"entries": h.store.ListLogs(limit, r.URL.Query().Get("key_id"))})
	case "stats":
		if r.Method != http.MethodGet {
			writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}
		writeAdminJSON(w, http.StatusOK, h.store.Stats())
	case "password":
		h.changePassword(w, r)
	case "config":
		h.config(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if !h.PasswordSet() {
		writeAdminJSON(w, http.StatusPreconditionFailed, map[string]any{"error": "admin password is not configured"})
		return
	}
	if !h.Verify(req.Password) {
		writeAdminJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid password"})
		return
	}
	payload, _ := json.Marshal(map[string]int64{"exp": time.Now().Add(sessionTTL).Unix()})
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    encoded + "." + h.sign(encoded),
		Path:     "/admin",
		MaxAge:   int(sessionTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) authorized(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || !hmac.Equal([]byte(h.sign(parts[0])), []byte(parts[1])) {
		return false
	}
	var session struct {
		Exp int64 `json:"exp"`
	}
	return json.Unmarshal(payload, &session) == nil && time.Now().Unix() < session.Exp
}

func (h *Handler) sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(h.store.session.value))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (h *Handler) accounts(w http.ResponseWriter, r *http.Request, tail []string) {
	if h.Accounts == nil {
		writeAdminJSON(w, http.StatusNotImplemented, map[string]any{"error": "account manager not configured"})
		return
	}
	if len(tail) == 0 {
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("action") == "oauth-url" {
				data, err := h.Accounts.StartOAuth()
				if err != nil {
					writeAdminJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
					return
				}
				writeAdminJSON(w, http.StatusOK, data)
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]any{"accounts": h.Accounts.List()})
			return
		}
		if r.Method == http.MethodPost {
			var req map[string]any
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req) != nil {
				writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
				return
			}
			if err := h.Accounts.Add(req); err != nil {
				writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if len(tail) != 2 {
		writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	uid, operation := tail[0], tail[1]
	if r.Method != http.MethodPost {
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var operationFn func(string) error
	switch operation {
	case "delete":
		operationFn = h.Accounts.Delete
	case "refresh":
		operationFn = h.Accounts.Refresh
	case "enable":
		operationFn = h.Accounts.Enable
	case "keepalive":
		operationFn = h.Accounts.Keepalive
	case "claim":
		operationFn = h.Accounts.Claim
	default:
		http.NotFound(w, r)
		return
	}
	if operationFn == nil {
		writeAdminJSON(w, http.StatusNotImplemented, map[string]any{"error": "operation not supported"})
		return
	}
	if err := operationFn(uid); err != nil {
		writeAdminJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) keys(w http.ResponseWriter, r *http.Request, tail []string) {
	if len(tail) == 0 {
		switch r.Method {
		case http.MethodGet:
			writeAdminJSON(w, http.StatusOK, map[string]any{"keys": h.store.ListKeys()})
		case http.MethodPost:
			var req struct {
				Name       string   `json:"name"`
				MaxRequest int64    `json:"max_requests"`
				ExpiresAt  int64    `json:"expires_at"`
				ModelAllow []string `json:"model_allow"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&req) != nil {
				writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
				return
			}
			if strings.TrimSpace(req.Name) == "" {
				req.Name = "unnamed"
			}
			plain, created, err := h.store.CreateKey(req.Name, req.MaxRequest, req.ExpiresAt, req.ModelAllow)
			if err != nil {
				writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
				return
			}
			writeAdminJSON(w, http.StatusOK, map[string]any{"key": plain, "item": created})
		default:
			writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		}
		return
	}
	if len(tail) != 1 {
		http.NotFound(w, r)
		return
	}
	id := tail[0]
	switch r.Method {
	case http.MethodDelete:
		if !h.store.DeleteKey(id) {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "key not found"})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
	case http.MethodPatch:
		var fields map[string]any
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&fields) != nil {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
			return
		}
		if !h.store.UpdateKey(id, func(key *APIKey) *APIKey {
			applyKeyPatch(key, fields)
			return key
		}) {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "key not found"})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func updateConfigFile(path, outputPath, key, value string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	admin, ok := doc["admin"].(map[string]any)
	if !ok {
		admin = map[string]any{}
		doc["admin"] = admin
	}
	admin[key] = value
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, append(encoded, '\n'), 0o600)
}
func applyKeyPatch(key *APIKey, fields map[string]any) {
	if value, ok := fields["enabled"].(bool); ok {
		key.Enabled = value
	}
	if value, ok := fields["name"].(string); ok && strings.TrimSpace(value) != "" {
		key.Name = strings.TrimSpace(value)
	}
	if value, ok := fields["max_requests"].(float64); ok {
		key.MaxRequests = int64(value)
	}
	if value, ok := fields["expires_at"].(float64); ok {
		key.ExpiresAt = int64(value)
	}
	if raw, ok := fields["model_allow"].([]any); ok {
		models := make([]string, 0, len(raw))
		for _, item := range raw {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				models = append(models, strings.TrimSpace(value))
			}
		}
		key.ModelAllow = models
	}
}

func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if len(req.Password) < 8 {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "password must be at least 8 characters"})
		return
	}
	if h.configPath == "" || h.passwordHash == nil {
		writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": "password storage is not configured"})
		return
	}
	hash := HashPassword(req.Password)
	tmp := h.configPath + ".tmp"
	if err := updateConfigFile(h.configPath, tmp, "password_hash", hash); err != nil {
		writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := os.Rename(tmp, h.configPath); err != nil {
		writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	*h.passwordHash = hash
	writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
}
func (h *Handler) config(w http.ResponseWriter, r *http.Request) {
	if h.store.config == nil {
		writeAdminJSON(w, http.StatusNotImplemented, map[string]any{"error": "config callback not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeAdminJSON(w, http.StatusOK, map[string]any{"config": h.store.config.GetConfig()})
	case http.MethodPut:
		var raw map[string]any
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&raw) != nil {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		if err := h.store.config.SaveConfig(raw); err != nil {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func writeAdminJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
