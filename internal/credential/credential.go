// Package credential 负责账号凭证的解析、派生与原子落盘。
//
// 支持的磁盘格式（均可读，写出统一为嵌套格式）：
//  1. CLI 原生：{"type":"oauth_token","access_token":...,"expires_at":<秒>,"uid":...,"api_key":...}
//  2. 嵌套：    {"auth":{"accessToken":...,"expiresAt":<秒>},"account":{"uid":...},"api_key":...}
//  3. 扁平：    {"accessToken":...,"refreshToken":...,"expiresAt":<秒或毫秒>,"uid":...}
//  4. 官方 CLI：{"claudeAiOauth":{"accessToken":...,"refreshToken":...,"expiresAt":<毫秒>}}
package credential

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Credential 是一个账号的运行时凭证。所有可变字段都由 mu 保护。
type Credential struct {
	mu        sync.Mutex
	refreshMu sync.Mutex // 串行化 token 刷新，避免并发刷新互相覆盖

	UID          string
	Nickname     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	APIKey       string

	path string
}

// Snapshot 是凭证的只读快照，供日志、状态展示与上游调用使用。
type Snapshot struct {
	UID          string
	Nickname     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	APIKey       string
	Path         string
}

// Snapshot 在锁保护下复制一份凭证。
func (c *Credential) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Snapshot{
		UID:          c.UID,
		Nickname:     c.Nickname,
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		ExpiresAt:    c.ExpiresAt,
		APIKey:       c.APIKey,
		Path:         c.path,
	}
}

// RefreshLock 串行化 token 刷新；调用方必须在读-改-写完成后调用 RefreshUnlock。
func (c *Credential) RefreshLock() { c.refreshMu.Lock() }

// RefreshUnlock 与 RefreshLock 配对。
func (c *Credential) RefreshUnlock() { c.refreshMu.Unlock() }

// RefreshTokenValue 返回当前 refresh token 的副本。
func (c *Credential) RefreshTokenValue() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.RefreshToken
}

// SetTokens 更新 token 字段。
func (c *Credential) SetTokens(accessToken, refreshToken string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if accessToken != "" {
		c.AccessToken = accessToken
	}
	if refreshToken != "" {
		c.RefreshToken = refreshToken
	}
	c.ExpiresAt = expiresAt
}

// SetProfile 更新展示信息。
func (c *Credential) SetProfile(uid, nickname string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if uid != "" {
		c.UID = uid
	}
	if nickname != "" {
		c.Nickname = nickname
	}
}

// Expiring 判断 token 是否会在 skew 窗口内过期。
// 过期时间未知（零值）时返回 false，避免无意义刷新。
func (c *Credential) Expiring(now time.Time, skew time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return expiring(c.ExpiresAt, now, skew)
}

func expiring(expiresAt, now time.Time, skew time.Duration) bool {
	if expiresAt.IsZero() {
		return false
	}
	return !now.Add(skew).Before(expiresAt)
}

// Path 返回凭证文件路径。
func (c *Credential) Path() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path
}

// SetPath 设置凭证文件路径。
func (c *Credential) SetPath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = path
}

// saveFormat 是对外写出的嵌套 JSON 结构。
type saveFormat struct {
	Auth struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
	} `json:"auth"`
	Account struct {
		UID      string `json:"uid"`
		Nickname string `json:"nickname"`
	} `json:"account"`
	APIKey string `json:"api_key,omitempty"`
}

// SaveAtomic 以嵌套格式原子写回凭证文件（同目录临时文件 + rename）。
func (c *Credential) SaveAtomic() error {
	snap := c.Snapshot()
	if snap.Path == "" {
		return fmt.Errorf("凭证 %s 没有文件路径，无法保存", snap.UID)
	}

	var out saveFormat
	out.Auth.AccessToken = snap.AccessToken
	out.Auth.RefreshToken = snap.RefreshToken
	if !snap.ExpiresAt.IsZero() {
		out.Auth.ExpiresAt = snap.ExpiresAt.Unix()
	}
	out.Account.UID = snap.UID
	out.Account.Nickname = snap.Nickname
	out.APIKey = snap.APIKey

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化凭证失败: %w", err)
	}
	raw = append(raw, '\n')

	dir := filepath.Dir(snap.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建凭证目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".credential-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时凭证文件失败: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后这里是空操作

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("设置凭证权限失败: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("写入凭证失败: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("同步凭证失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭凭证失败: %w", err)
	}
	if err := os.Rename(tmpName, snap.Path); err != nil {
		return fmt.Errorf("替换凭证文件失败: %w", err)
	}
	return nil
}

// diskFormat 是解析用的超集结构，兼容四种磁盘格式。
type diskFormat struct {
	Type         string `json:"type"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    any    `json:"expires_at"`
	UID          string `json:"uid"`
	Nickname     string `json:"nickname"`
	APIKey       string `json:"api_key"`

	Auth *struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    any    `json:"expiresAt"`
	} `json:"auth"`
	Account *struct {
		UID      string `json:"uid"`
		Nickname string `json:"nickname"`
	} `json:"account"`

	AccessTokenCamel  string `json:"accessToken"`
	RefreshTokenCamel string `json:"refreshToken"`
	ExpiresAtCamel    any    `json:"expiresAt"`

	ClaudeAiOauth *struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    any    `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// Parse 解析一份凭证 JSON。filename 仅用于派生 UID。
func Parse(raw []byte, filename string) (*Credential, error) {
	var d diskFormat
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("凭证不是合法 JSON: %w", err)
	}

	c := &Credential{
		UID:          d.UID,
		Nickname:     d.Nickname,
		AccessToken:  d.AccessToken,
		RefreshToken: d.RefreshToken,
		ExpiresAt:    parseEpoch(d.ExpiresAt),
		APIKey:       d.APIKey,
	}

	if d.Auth != nil {
		if c.AccessToken == "" {
			c.AccessToken = d.Auth.AccessToken
		}
		if c.RefreshToken == "" {
			c.RefreshToken = d.Auth.RefreshToken
		}
		if c.ExpiresAt.IsZero() {
			c.ExpiresAt = parseEpoch(d.Auth.ExpiresAt)
		}
	}
	if d.Account != nil {
		if c.UID == "" {
			c.UID = d.Account.UID
		}
		if c.Nickname == "" {
			c.Nickname = d.Account.Nickname
		}
	}
	if c.AccessToken == "" {
		c.AccessToken = d.AccessTokenCamel
	}
	if c.RefreshToken == "" {
		c.RefreshToken = d.RefreshTokenCamel
	}
	if c.ExpiresAt.IsZero() {
		c.ExpiresAt = parseEpoch(d.ExpiresAtCamel)
	}
	if d.ClaudeAiOauth != nil {
		if c.AccessToken == "" {
			c.AccessToken = d.ClaudeAiOauth.AccessToken
		}
		if c.RefreshToken == "" {
			c.RefreshToken = d.ClaudeAiOauth.RefreshToken
		}
		if c.ExpiresAt.IsZero() {
			c.ExpiresAt = parseEpoch(d.ClaudeAiOauth.ExpiresAt)
		}
	}

	if c.AccessToken == "" && c.APIKey == "" {
		return nil, fmt.Errorf("凭证缺少 access_token 与 api_key")
	}
	if c.UID == "" {
		c.UID = deriveUID(filename, c.AccessToken)
	}
	return c, nil
}

// parseEpoch 解析秒或毫秒时间戳；字符串数字也接受。
func parseEpoch(v any) time.Time {
	var n int64
	switch x := v.(type) {
	case nil:
		return time.Time{}
	case float64:
		n = int64(x)
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return time.Time{}
		}
		n = i
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		if err != nil {
			return time.Time{}
		}
		n = i
	default:
		return time.Time{}
	}
	if n <= 0 {
		return time.Time{}
	}
	if n > 1e12 { // 毫秒（官方 CLI 使用毫秒）
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}

// deriveUID 从文件名（phanthy-<uid>.json）或 token 哈希派生稳定 UID。
func deriveUID(filename, accessToken string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	base = strings.TrimPrefix(base, "phanthy-")
	if base != "" && base != "." && base != "phanthy" && !strings.HasPrefix(base, ".credential") {
		return base
	}
	sum := sha256.Sum256([]byte(accessToken))
	return "acct-" + hex.EncodeToString(sum[:6])
}

// FileName 返回该 UID 的标准凭证文件名。
func FileName(uid string) string {
	safe := make([]rune, 0, len(uid))
	for _, r := range uid {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			safe = append(safe, r)
		default:
			safe = append(safe, '-')
		}
	}
	name := strings.Trim(string(safe), ".-")
	if name == "" {
		name = "account"
	}
	return "phanthy-" + name + ".json"
}

// LoadDir 读取目录下所有 .json 凭证。目录不存在时返回空集合。
// 第二个返回值收集单文件级别的错误，便于调用方记录警告后继续启动。
func LoadDir(dir string) ([]*Credential, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("读取凭证目录失败: %w", err)}
	}

	var (
		creds []*Credential
		errs  []error
		seen  = map[string]bool{}
	)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		c, err := Parse(raw, entry.Name())
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		if seen[c.UID] {
			errs = append(errs, fmt.Errorf("%s: UID %s 重复，已跳过", entry.Name(), c.UID))
			continue
		}
		seen[c.UID] = true
		c.SetPath(path)
		creds = append(creds, c)
	}
	sort.Slice(creds, func(i, j int) bool { return creds[i].UID < creds[j].UID })
	return creds, errs
}
