// Package auth 解析 Phanthy 账号凭证文件（支持嵌套形/扁平形/CLI 原生双形态），
// 提供原子写回与 api_key 关联。
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Auth 是归一化后的账号凭证。
// access_token 用于 OAuth 流程（刷新、创建 api_key），
// api_key 用于调用 /v1/messages。
type Auth struct {
	mu sync.Mutex

	AccessToken  string // OAuth access token
	RefreshToken string // OAuth refresh token
	ExpiresAt    int64  // access_token 过期时间（Unix 秒）
	APIKey       string // create_api_key 返回的 api key（可空，运行时自动补）
	UID          string // 账号标识（文件主键）
	Nickname     string
	FilePath     string // 来源文件；refresh 后原子写回此处
}

// Lock 供同进程内其他包（upstream）改写 Auth 字段期间加锁。
func (a *Auth) Lock() { a.mu.Lock() }

// Unlock 释放 a.Lock 获取的锁。
func (a *Auth) Unlock() { a.mu.Unlock() }

// NeedsRefresh 报告 access_token 是否将在 within 内过期（或已过期/无 expiry）。
func (a *Auth) NeedsRefresh(within time.Duration) bool {
	if a.ExpiresAt <= 0 {
		return true
	}
	return time.Now().Add(within).Unix() >= a.ExpiresAt
}

// Parse 兼容三种磁盘形态：
//
//	CLI 原生   {"type":"oauth_token","access_token":...,"refresh_token":...,"expires_at":...}
//	嵌套形     {"auth":{...},"account":{...}}          （本服务写回格式）
//	扁平形     {"accessToken":...,"api_key":...}       （面板手建）
func Parse(raw []byte) (*Auth, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty auth storage")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("storage_parse_error: %w", err)
	}

	// 形态一：CLI 原生（type=oauth_token）
	if typ, _ := string(probe["type"]), false; typ == "oauth_token" || len(probe["type"]) > 0 {
		var c struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresAt    int64  `json:"expires_at"`
		}
		// probe 中确实有 type 字段才走 CLI 原生解析
		if _, ok := probe["type"]; ok {
			var c2 struct {
				Type         string `json:"type"`
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				ExpiresAt    int64  `json:"expires_at"`
				UID          string `json:"uid"`
				Nickname     string `json:"nickname"`
				APIKey       string `json:"api_key"`
			}
			if err := json.Unmarshal(raw, &c2); err != nil {
				return nil, fmt.Errorf("storage_parse_error: %w", err)
			}
			c.AccessToken = c2.AccessToken
			c.RefreshToken = c2.RefreshToken
			c.ExpiresAt = c2.ExpiresAt
			if c2.APIKey != "" {
				return &Auth{
					AccessToken:  c2.AccessToken,
					RefreshToken: c2.RefreshToken,
					ExpiresAt:    c2.ExpiresAt,
					APIKey:       c2.APIKey,
					UID:          c2.UID,
					Nickname:     c2.Nickname,
				}, nil
			}
			return &Auth{
				AccessToken:  c.AccessToken,
				RefreshToken: c.RefreshToken,
				ExpiresAt:    c.ExpiresAt,
				UID:          c2.UID,
				Nickname:     c2.Nickname,
			}, nil
		}
	}

	// 形态二：嵌套形
	if _, nested := probe["auth"]; nested {
		var n struct {
			Auth struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
				ExpiresAt    int64  `json:"expiresAt"`
			} `json:"auth"`
			Account struct {
				UID      string `json:"uid"`
				Nickname string `json:"nickname"`
			} `json:"account"`
			APIKey string `json:"api_key"`
		}
		if err := json.Unmarshal(raw, &n); err != nil {
			return nil, fmt.Errorf("storage_parse_error: %w", err)
		}
		return &Auth{
			AccessToken:  n.Auth.AccessToken,
			RefreshToken: n.Auth.RefreshToken,
			ExpiresAt:    n.Auth.ExpiresAt,
			APIKey:       n.APIKey,
			UID:          n.Account.UID,
			Nickname:     n.Account.Nickname,
		}, nil
	}

	// 形态三：扁平形
	var f struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
		APIKey       string `json:"api_key"`
		UID          string `json:"uid"`
		Nickname     string `json:"nickname"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("storage_parse_error: %w", err)
	}
	return &Auth{
		AccessToken:  f.AccessToken,
		RefreshToken: f.RefreshToken,
		ExpiresAt:    f.ExpiresAt,
		APIKey:       f.APIKey,
		UID:          f.UID,
		Nickname:     f.Nickname,
	}, nil
}

// SaveAtomic 以嵌套形原子写回 FilePath（tmp + rename），保持可读格式。
// 加锁外壳：防止与 RefreshToken 并发写回半更新 token。
func (a *Auth) SaveAtomic() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.saveAtomicLocked()
}

// saveAtomicLocked 是 SaveAtomic 的持锁内部版本；调用方必须已持有 a.mu。
func (a *Auth) saveAtomicLocked() error {
	if a.FilePath == "" {
		return fmt.Errorf("no FilePath set")
	}
	doc := map[string]any{
		"auth": map[string]any{
			"accessToken":  a.AccessToken,
			"refreshToken": a.RefreshToken,
			"expiresAt":    a.ExpiresAt,
		},
		"account": map[string]any{
			"uid":      a.UID,
			"nickname": a.Nickname,
		},
		"api_key": a.APIKey,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.FilePath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.FilePath)
}

// LoadDir 扫描 dir 下 phanthy*.json；解析失败的静默跳过（启动日志由调用方统计）。
func LoadDir(dir string) ([]*Auth, error) {
	files, err := filepath.Glob(filepath.Join(dir, "phanthy*.json"))
	if err != nil {
		return nil, err
	}
	var out []*Auth
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		a, err := Parse(raw)
		if err != nil {
			continue
		}
		if strings.TrimSpace(a.AccessToken) == "" {
			continue
		}
		if a.UID == "" {
			a.UID = strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		}
		a.FilePath = f
		out = append(out, a)
	}
	return out, nil
}