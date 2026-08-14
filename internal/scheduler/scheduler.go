// Package scheduler 定时任务：token keepalive。
package scheduler

import (
	"context"
	"errors"
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

	for {
		next := nextFire(time.Now(), s.cfg.KeepaliveHours)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.RunKeepaliveNow()
		}
	}
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