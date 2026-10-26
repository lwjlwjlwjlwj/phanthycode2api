// Package pool 维护账号池：健康检查、冷却、禁用、错误计数、LRU 轮转与余额感知。
package pool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"phanthycode2api/internal/credential"
)

// CooldownKind 表示冷却原因，仅用于日志与状态展示。
type CooldownKind int

const (
	// CooldownHard 积分不足。
	CooldownHard CooldownKind = iota
	// CooldownSoft 限流或短时不可用。
	CooldownSoft
	// CooldownError 连续错误达到阈值。
	CooldownError
)

func (k CooldownKind) String() string {
	switch k {
	case CooldownHard:
		return "hard_credit"
	case CooldownSoft:
		return "soft_rate"
	case CooldownError:
		return "consecutive_errors"
	default:
		return "unknown"
	}
}

// AccountStatus 是 /status 对外暴露的账号快照，不含任何 token。
type AccountStatus struct {
	UID       string     `json:"uid"`
	Nickname  string     `json:"nickname"`
	HasAPIKey bool       `json:"has_api_key"`
	Cooling   bool       `json:"cooling"`
	Until     *time.Time `json:"until,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Disabled  bool       `json:"disabled"`
	ErrCount  int        `json:"err_count"`
	Balance   *int64     `json:"balance,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
}

type entry struct {
	cred     *credential.Credential
	disabled bool
	reason   string
	until    time.Time
	errCount int
	balance  *int64
	lastUsed time.Time
}

// Pool 是并发安全的账号池。
type Pool struct {
	mu        sync.Mutex
	entries   map[string]*entry
	stateFile string
}

// New 构建账号池，并加载已持久化的冷却/禁用状态。
func New(creds []*credential.Credential, stateFile string) (*Pool, error) {
	p := &Pool{
		entries:   make(map[string]*entry, len(creds)),
		stateFile: stateFile,
	}
	for _, c := range creds {
		if c == nil || c.UID == "" {
			continue
		}
		p.entries[c.UID] = &entry{cred: c}
	}
	if stateFile != "" {
		if err := p.loadState(); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// Len 返回账号数量。
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

// Credentials 返回所有凭证（用于 keepalive / 探测），顺序按 UID。
func (p *Pool) Credentials() []*credential.Credential {
	p.mu.Lock()
	defer p.mu.Unlock()
	uids := make([]string, 0, len(p.entries))
	for uid := range p.entries {
		uids = append(uids, uid)
	}
	sort.Strings(uids)
	out := make([]*credential.Credential, 0, len(uids))
	for _, uid := range uids {
		out = append(out, p.entries[uid].cred)
	}
	return out
}

// Pick 选择一个当前可用的账号；没有可用账号时返回 nil。
func (p *Pool) Pick(now time.Time) *credential.Credential {
	return p.PickExcluding(now, nil)
}

// PickExcluding 选择当前可用且不在 excluded 中的账号。
// 排序规则：余额为 0 的账号降级 → lastUsed 越早越优先（LRU）→ UID 字典序。
func (p *Pool) PickExcluding(now time.Time, excluded map[string]bool) *credential.Credential {
	p.mu.Lock()
	defer p.mu.Unlock()

	var candidates []*entry
	for uid, e := range p.entries {
		if excluded != nil && excluded[uid] {
			continue
		}
		if !e.usable(now) {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool { return prefer(candidates[i], candidates[j]) })
	return candidates[0].cred
}

func (e *entry) usable(now time.Time) bool {
	if e.disabled {
		return false
	}
	if e.until.IsZero() {
		return true
	}
	return !now.Before(e.until)
}

func prefer(a, b *entry) bool {
	aZero := a.balance != nil && *a.balance == 0
	bZero := b.balance != nil && *b.balance == 0
	if aZero != bZero {
		return !aZero
	}
	if !a.lastUsed.Equal(b.lastUsed) {
		return a.lastUsed.Before(b.lastUsed)
	}
	return a.cred.UID < b.cred.UID
}

// MarkUsed 记录账号被使用的时间（仅内存，随下一次持久化落盘）。
func (p *Pool) MarkUsed(uid string, at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.entries[uid]; ok {
		e.lastUsed = at
	}
}

// Cooldown 将账号冷却一段时间。
func (p *Pool) Cooldown(uid string, kind CooldownKind, dur time.Duration, reason string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[uid]
	if !ok {
		return
	}
	if reason == "" {
		reason = kind.String()
	}
	if dur > 0 {
		e.until = now.Add(dur)
	}
	e.reason = reason
	p.saveLocked()
}

// Disable 永久禁用账号（session 死亡等），直到人工重新登录。
func (p *Pool) Disable(uid, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[uid]
	if !ok {
		return
	}
	e.disabled = true
	e.reason = reason
	p.saveLocked()
}

// NoteError 记录一次非成功调用；达到阈值后按 errCooldown 冷却。
// 返回是否触发了冷却。
func (p *Pool) NoteError(uid string, threshold int, errCooldown time.Duration, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[uid]
	if !ok {
		return false
	}
	if threshold < 1 {
		threshold = 1
	}
	e.errCount++
	cooled := false
	if e.errCount >= threshold {
		e.until = now.Add(errCooldown)
		e.reason = "连续错误"
		e.errCount = 0
		cooled = true
	}
	p.saveLocked()
	return cooled
}

// NoteSuccess 重置连续错误计数。
func (p *Pool) NoteSuccess(uid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[uid]
	if !ok {
		return
	}
	if e.errCount != 0 {
		e.errCount = 0
		p.saveLocked()
	}
}

// SetBalance 更新账号余额（nil 表示未知）。
func (p *Pool) SetBalance(uid string, balance *int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[uid]
	if !ok {
		return
	}
	if balance == nil {
		if e.balance == nil {
			return
		}
		e.balance = nil
	} else {
		v := *balance
		if e.balance != nil && *e.balance == v {
			return
		}
		e.balance = &v
	}
	p.saveLocked()
}

// List 返回所有账号的对外状态，按 UID 排序。
func (p *Pool) List(now time.Time) []AccountStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	uids := make([]string, 0, len(p.entries))
	for uid := range p.entries {
		uids = append(uids, uid)
	}
	sort.Strings(uids)

	out := make([]AccountStatus, 0, len(uids))
	for _, uid := range uids {
		e := p.entries[uid]
		snap := e.cred.Snapshot()
		st := AccountStatus{
			UID:       uid,
			Nickname:  snap.Nickname,
			HasAPIKey: snap.APIKey != "",
			Disabled:  e.disabled,
			Reason:    e.reason,
			ErrCount:  e.errCount,
		}
		if !e.until.IsZero() && now.Before(e.until) {
			until := e.until
			st.Cooling = true
			st.Until = &until
		}
		if e.balance != nil {
			v := *e.balance
			st.Balance = &v
		}
		if !e.lastUsed.IsZero() {
			used := e.lastUsed
			st.LastUsed = &used
		}
		out = append(out, st)
	}
	return out
}

// persistedState 是 state_file 的磁盘格式。
type persistedState struct {
	Accounts map[string]persistedAccount `json:"accounts"`
}

type persistedAccount struct {
	Disabled bool       `json:"disabled,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	Until    *time.Time `json:"until,omitempty"`
	ErrCount int        `json:"err_count,omitempty"`
	Balance  *int64     `json:"balance,omitempty"`
	LastUsed *time.Time `json:"last_used,omitempty"`
}

func (p *Pool) loadState() error {
	raw, err := os.ReadFile(p.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取状态文件失败: %w", err)
	}
	var st persistedState
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("解析状态文件失败: %w", err)
	}
	for uid, saved := range st.Accounts {
		e, ok := p.entries[uid]
		if !ok {
			continue // 凭证已删除的账号，忽略其残留状态
		}
		e.disabled = saved.Disabled
		e.reason = saved.Reason
		if saved.Until != nil {
			e.until = *saved.Until
		}
		e.errCount = saved.ErrCount
		if saved.Balance != nil {
			v := *saved.Balance
			e.balance = &v
		}
		if saved.LastUsed != nil {
			e.lastUsed = *saved.LastUsed
		}
	}
	return nil
}

func (p *Pool) saveLocked() {
	if p.stateFile == "" {
		return
	}
	st := persistedState{Accounts: make(map[string]persistedAccount, len(p.entries))}
	for uid, e := range p.entries {
		if !e.disabled && e.reason == "" && e.until.IsZero() && e.errCount == 0 && e.balance == nil && e.lastUsed.IsZero() {
			continue
		}
		saved := persistedAccount{
			Disabled: e.disabled,
			Reason:   e.reason,
			ErrCount: e.errCount,
		}
		if !e.until.IsZero() {
			until := e.until
			saved.Until = &until
		}
		if e.balance != nil {
			v := *e.balance
			saved.Balance = &v
		}
		if !e.lastUsed.IsZero() {
			used := e.lastUsed
			saved.LastUsed = &used
		}
		st.Accounts[uid] = saved
	}

	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	raw = append(raw, '\n')

	dir := filepath.Dir(p.stateFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmpName, p.stateFile)
}
