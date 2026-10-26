// Package scheduler 定时任务：token keepalive + 可选周期性余额探测。
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/upstream"
)

// Config 调度器依赖。
type Config struct {
	Pool           *pool.Pool
	Upstream       *upstream.Client
	KeepaliveHours []int // 默认 [22]
	ProbeInterval  time.Duration // 0 = 关闭余额探测
}

// Scheduler 调度器。
type Scheduler struct {
	cfg Config
}

// New 构建。
func New(cfg Config) *Scheduler {
	if len(cfg.KeepaliveHours) == 0 {
		cfg.KeepaliveHours = []int{22}
	}
	return &Scheduler{cfg: cfg}
}

// nextFire 返回 now 之后最近的一个整点触发时间；hours 为本地小时（0-23）。
func nextFire(now time.Time, hours []int) time.Time {
	var earliest time.Time
	for _, h := range hours {
		t := time.Date(now.Year(), now.Month(), now.Day(), h, 0, 0, 0, now.Location())
		if !t.After(now) {
			t = t.Add(24 * time.Hour)
		}
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

// Run 主循环，阻塞直到 ctx 取消。
func (s *Scheduler) Run(ctx context.Context) {
	// 启动时立即执行一次 keepalive（保证服务起来后账号立即可用）
	log.Printf("scheduler: running initial keepalive")
	s.RunKeepaliveNow()
	if s.cfg.ProbeInterval > 0 {
		log.Printf("scheduler: running initial balance probe")
		s.RunProbeNow()
	}

	var probeTick *time.Ticker
	if s.cfg.ProbeInterval > 0 {
		probeTick = time.NewTicker(s.cfg.ProbeInterval)
		defer probeTick.Stop()
	}

	for {
		next := nextFire(time.Now(), s.cfg.KeepaliveHours)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.RunKeepaliveNow()
		case <-probeTickC(probeTick):
			s.RunProbeNow()
		}
	}
}

// probeTickC 返回探测定时器的触发通道；nil（探测关闭）时返回永不触达的通道。
func probeTickC(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return make(chan time.Time)
	}
	return t.C
}

// RunKeepaliveNow 立即对所有账号刷新 token；session 死亡的自动禁用。
func (s *Scheduler) RunKeepaliveNow() {
	for _, st := range s.cfg.Pool.List() {
		if st.Disabled {
			continue
		}
		a := s.cfg.Pool.AuthByUID(st.UID)
		if a == nil || a.RefreshToken == "" {
			continue
		}
		// 仅在 token 接近过期时刷新
		if !a.NeedsRefresh(30 * time.Minute) {
			continue
		}
		if err := s.cfg.Upstream.RefreshToken(a); err != nil {
			log.Printf("keepalive %s: %v", st.UID, err)
			var ue *upstream.Error
			if errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead {
				s.cfg.Pool.Disable(st.UID, "session dead")
			}
			continue
		}
		// 确保 api_key 有效
		if a.APIKey == "" {
			if err := s.cfg.Upstream.EnsureAPIKey(a); err != nil {
				log.Printf("keepalive %s ensure_api_key: %v", st.UID, err)
			}
		}
		if err := a.SaveAtomic(); err != nil {
			log.Printf("keepalive %s save: %v", st.UID, err)
		}
		log.Printf("keepalive %s: token refreshed", st.UID)
	}
}

// RunProbeNow 对所有账号做余额探测（GET /api/phanthy_code/models/stats，防御性解析）。
// 仅在 token 快过期时先刷新；session 死亡自动禁用。探测间隔 >= 1s（合规于登录态限速）。
func (s *Scheduler) RunProbeNow() {
	for _, st := range s.cfg.Pool.List() {
		if st.Disabled {
			continue
		}
		a := s.cfg.Pool.AuthByUID(st.UID)
		if a == nil || a.AccessToken == "" {
			continue
		}
		if a.NeedsRefresh(30 * time.Minute) {
			if err := s.cfg.Upstream.RefreshToken(a); err != nil {
				log.Printf("probe %s refresh: %v", st.UID, err)
				var ue *upstream.Error
				if errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead {
					s.cfg.Pool.Disable(st.UID, "session dead")
				}
				time.Sleep(time.Second)
				continue
			}
			_ = a.SaveAtomic()
		}
		raw, err := s.cfg.Upstream.FetchModelStats(a)
		if err != nil {
			log.Printf("probe %s stats: %v", st.UID, err)
			var ue *upstream.Error
			if errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead {
				s.cfg.Pool.Disable(st.UID, "session dead")
			}
			time.Sleep(time.Second)
			continue
		}
		bal, ok := ParseBalance(raw)
		if ok {
			s.cfg.Pool.SetBalance(st.UID, bal)
			log.Printf("probe %s balance=%d", st.UID, bal)
		}
		time.Sleep(time.Second)
	}
}

// ParseBalance 防御性解析 modelStats 成功体中的余额字段。
// 成功体 schema 为 BLOCKED-CRED（M6 未观测到 200），这里按常见字段名兜底：
// 顶层或 data 下 credit/balance/points/quota/remaining/remaining_credits。
func ParseBalance(raw []byte) (int64, bool) {
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return 0, false
	}
	keys := []string{"credit", "balance", "points", "quota", "remaining", "remaining_credits", "credit_balance"}
	for _, k := range keys {
		if v, ok := numericOf(root[k]); ok {
			return v, true
		}
	}
	if data, ok := root["data"].(map[string]any); ok {
		for _, k := range keys {
			if v, ok := numericOf(data[k]); ok {
				return v, true
			}
		}
	}
	return 0, false
}

func numericOf(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	case string:
		var i int64
		if _, err := fmt.Sscanf(t, "%d", &i); err == nil {
			return i, true
		}
	}
	return 0, false
}