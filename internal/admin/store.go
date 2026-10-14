// Package admin 提供内嵌 Web 管理界面：账号管理、密钥分发、请求日志、设置编辑。
//
// 设计约束：
//   - 单二进制：UI 通过 go:embed 打包，无外部静态文件依赖
//   - JSON 文件存储：keys / logs / settings 持久化到 data/ 下
//   - 独立认证：PBKDF2-HMAC-SHA256 加盐哈希，Cookie 会话与 /v1 api_key 无关
package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

const (
	sessionCookie = "p2a_admin_session"
	pbkdf2Iter    = 120000
	sessionTTL    = 24 * time.Hour
)

// Store 管理数据的持久化目录。
type Store struct {
	Dir string

	mu                    sync.Mutex
	config                *ConfigState
	keys                  *KeysFile
	logs                  *LogsFile
	session               stringKey // HMAC key for cookie signing
	legacyUnauthenticated bool
}

// stringKey 内部使用的字符串 key 类型。
type stringKey struct {
	value string
}

// ConfigState config.json 的运行时可编辑状态（用于设置页）。
// 通过回调读写，避免直接依赖 cmd/server 的 Config 结构。
type ConfigState struct {
	// GetConfig 返回当前配置的 JSON 表示（供 UI 展示）。
	GetConfig func() map[string]any
	// SaveConfig 接收用户提交的 JSON，写回配置文件。
	// 返回错误则拒绝保存。
	SaveConfig func(raw map[string]any) error
}

// KeysFile 多密钥分发的持久化格式。
type KeysFile struct {
	Keys []APIKey `json:"keys"`
}

// APIKey 单个分发密钥。
type APIKey struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	KeyHash      string   `json:"key_hash"` // SHA-256，明文只在创建时返回
	KeyPreview   string   `json:"key_preview"`
	Enabled      bool     `json:"enabled"`
	MaxRequests  int64    `json:"max_requests"` // 0 = 无限
	UsedRequests int64    `json:"used_requests"`
	ExpiresAt    int64    `json:"expires_at"`  // Unix 秒，0 = 不过期
	ModelAllow   []string `json:"model_allow"` // 空 = 允许全部
	CreatedAt    int64    `json:"created_at"`
	LastUsedAt   int64    `json:"last_used_at"`
	IPAllow      []string `json:"ip_allow"` // CIDR 白名单
	IPDeny       []string `json:"ip_deny"`  // CIDR 黑名单
}

// LogsFile 请求日志的持久化格式。
type LogsFile struct {
	Entries []LogEntry `json:"entries"`
}

// LogEntry 单次请求的审计记录。
type LogEntry struct {
	Time          int64  `json:"time"`
	KeyID         string `json:"key_id"`
	KeyName       string `json:"key_name"`
	UID           string `json:"uid"`
	Model         string `json:"model"`
	Status        int    `json:"status"`
	LatencyMs     int64  `json:"latency_ms"`
	PromptTok     int64  `json:"prompt_tokens"`
	CompletionTok int64  `json:"completion_tokens"`
	Stream        bool   `json:"stream"`
	RemoteIP      string `json:"remote_ip"`
	Error         string `json:"error,omitempty"`
}

// New 构建管理存储并加载已有数据。
func New(dataDir string) (*Store, error) {
	if dataDir == "" {
		dataDir = "./data/admin"
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", abs, err)
	}
	s := &Store{Dir: abs, legacyUnauthenticated: true}
	if err := s.loadKeys(); err != nil {
		return nil, err
	}
	if err := s.loadLogs(); err != nil {
		return nil, err
	}
	secret, err := loadOrCreateSecret(abs)
	if err != nil {
		return nil, err
	}
	s.session = stringKey{value: secret}
	return s, nil
}

// SetConfigState 注入设置页的配置读写回调。
func (s *Store) SetConfigState(state ConfigState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = &state
}

// loadOrCreateSecret 生成或加载 cookie 签名密钥。
func loadOrCreateSecret(dir string) (string, error) {
	fp := filepath.Join(dir, "admin_secret.key")
	if raw, err := os.ReadFile(fp); err == nil {
		return strings.TrimSpace(string(raw)), nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(fp, []byte(secret), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

// ---------------------------------------------------------------------------
// PBKDF2 密码哈希
// ---------------------------------------------------------------------------

// HashPassword 生成 PBKDF2-HMAC-SHA256 加盐哈希，返回 "salt:hash"（hex）。
func HashPassword(password string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	dk := pbkdf2.Key([]byte(password), salt, pbkdf2Iter, 32, sha256.New)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(dk)
}

// VerifyPassword 校验密码是否与哈希匹配。
func VerifyPassword(password, stored string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	got := pbkdf2.Key([]byte(password), salt, pbkdf2Iter, len(want), sha256.New)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---------------------------------------------------------------------------
// JSON 文件持久化
// ---------------------------------------------------------------------------

func (s *Store) loadKeys() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(filepath.Join(s.Dir, "keys.json"))
	if os.IsNotExist(err) {
		s.keys = &KeysFile{}
		return nil
	}
	if err != nil {
		return err
	}
	s.keys = &KeysFile{}
	return json.Unmarshal(raw, s.keys)
}

func (s *Store) saveKeysLocked() error {
	raw, err := json.MarshalIndent(s.keys, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.Dir, "keys.json"), raw)
}

func (s *Store) loadLogs() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(filepath.Join(s.Dir, "logs.json"))
	if os.IsNotExist(err) {
		s.logs = &LogsFile{Entries: []LogEntry{}}
		return nil
	}
	if err != nil {
		return err
	}
	s.logs = &LogsFile{}
	if err := json.Unmarshal(raw, s.logs); err != nil {
		return err
	}
	if s.logs.Entries == nil {
		s.logs.Entries = []LogEntry{}
	}
	return nil
}

func (s *Store) saveLogsLocked() error {
	raw, err := json.MarshalIndent(s.logs, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(s.Dir, "logs.json"), raw)
}

func atomicWrite(path string, raw []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---------------------------------------------------------------------------
// 密钥管理
// ---------------------------------------------------------------------------

// ListKeys 返回所有密钥（不含哈希）。
func (s *Store) ListKeys() []APIKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]APIKey, 0, len(s.keys.Keys))
	for _, k := range s.keys.Keys {
		k.KeyHash = "" // 不外泄
		out = append(out, k)
	}
	return out
}

// CreateKey 创建密钥，返回 (明文, 错误)。
func (s *Store) CreateKey(name string, maxReq int64, expiresAt int64, models []string) (string, *APIKey, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	plain := "p2a-" + hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plain))
	k := &APIKey{
		ID:          fmt.Sprintf("k%d", time.Now().UnixNano()),
		Name:        name,
		KeyHash:     hex.EncodeToString(sum[:]),
		KeyPreview:  plain[:8] + "…" + plain[len(plain)-6:],
		Enabled:     true,
		MaxRequests: maxReq,
		ExpiresAt:   expiresAt,
		ModelAllow:  models,
		CreatedAt:   time.Now().Unix(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys.Keys = append(s.keys.Keys, *k)
	if err := s.saveKeysLocked(); err != nil {
		return "", nil, err
	}
	return plain, k, nil
}

// DeleteKey 删除指定 ID 的密钥。
func (s *Store) DeleteKey(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, k := range s.keys.Keys {
		if k.ID == id {
			s.keys.Keys = append(s.keys.Keys[:i], s.keys.Keys[i+1:]...)
			return s.saveKeysLocked() == nil
		}
	}
	return false
}

// LookupKey 按明文查密钥（用于 /v1 请求鉴权）。
// 同时更新用量与 LastUsedAt。model 为空时跳过白名单检查。
func (s *Store) LookupKey(plain, model string) (*APIKey, bool) {
	if plain == "" {
		return nil, false
	}
	sum := sha256.Sum256([]byte(plain))
	want := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys.Keys {
		k := &s.keys.Keys[i]
		if k.KeyHash != want || !k.Enabled {
			continue
		}
		now := time.Now().Unix()
		if k.ExpiresAt > 0 && now >= k.ExpiresAt {
			continue
		}
		if k.MaxRequests > 0 && k.UsedRequests >= k.MaxRequests {
			continue
		}
		// modelAllow: 空 = 允许全部；有值则必须命中
		if len(k.ModelAllow) > 0 && model != "" {
			found := false
			for _, m := range k.ModelAllow {
				if strings.EqualFold(m, model) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		k.UsedRequests++
		k.LastUsedAt = now
		_ = s.saveKeysLocked()
		out := *k
		out.KeyHash = ""
		return &out, true
	}
	return nil, false
}

// UpdateKey 更新密钥的可编辑字段（不改 key_hash / created_at / key_preview）。
func (s *Store) UpdateKey(id string, fn func(k *APIKey) *APIKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys.Keys {
		if s.keys.Keys[i].ID != id {
			continue
		}
		updated := fn(&s.keys.Keys[i])
		s.keys.Keys[i] = *updated
		return s.saveKeysLocked() == nil
	}
	return false
}

// AddLog 记录一次请求到日志（保留最近 5000 条）。
func (s *Store) AddLog(entry LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs.Entries = append(s.logs.Entries, entry)
	if len(s.logs.Entries) > 5000 {
		s.logs.Entries = s.logs.Entries[len(s.logs.Entries)-5000:]
	}
	_ = s.saveLogsLocked()
}

// ClearLogs 清空请求日志。
func (s *Store) ClearLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs.Entries = []LogEntry{}
	_ = s.saveLogsLocked()
}

// Stats 聚合日志的总请求、错误与最近 30 天每日请求数。
func (s *Store) Stats() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := len(s.logs.Entries)
	errors := 0
	daily := map[string]int64{}
	byKey := map[string]int64{}
	now := time.Now().Unix()
	for _, entry := range s.logs.Entries {
		if entry.Status >= 400 {
			errors++
		}
		if entry.Time >= now-30*86400 {
			day := time.Unix(entry.Time, 0).Format("2006-01-02")
			daily[day]++
		}
		name := entry.KeyName
		if name == "" {
			name = entry.KeyID
		}
		if name == "" {
			name = "legacy"
		}
		byKey[name]++
	}
	return map[string]any{
		"total":  total,
		"errors": errors,
		"daily":  daily,
		"by_key": byKey,
	}
}

// ListLogs 返回日志（倒序，最多 limit 条；支持按 keyID 过滤）。
func (s *Store) ListLogs(limit int, keyID string) []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogEntry, 0, 64)
	// 倒序遍历
	for i := len(s.logs.Entries) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.logs.Entries[i]
		if keyID != "" && e.KeyID != keyID {
			continue
		}
		out = append(out, e)
	}
	return out
}
