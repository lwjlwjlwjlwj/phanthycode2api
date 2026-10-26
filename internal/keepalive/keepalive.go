// Package keepalive 实现 token 定时保活与可选的余额探测。
package keepalive

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"phanthycode2api/internal/phanthy"
	"phanthycode2api/internal/pool"
)

// DefaultSkew 是 keepalive 的提前刷新窗口（FR-14）。
const DefaultSkew = 30 * time.Minute

// Scheduler 负责周期性刷新凭证与探测余额。
type Scheduler struct {
	client        *phanthy.Client
	pool          *pool.Pool
	logger        *log.Logger
	skew          time.Duration
	hours         []int
	probeInterval time.Duration
	now           func() time.Time
}

// New 构建调度器。hours 为本地时间的小时列表；probeInterval<=0 表示关闭余额探测。
func New(client *phanthy.Client, p *pool.Pool, logger *log.Logger, hours []int, probeInterval time.Duration) *Scheduler {
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		client:        client,
		pool:          p,
		logger:        logger,
		skew:          DefaultSkew,
		hours:         hours,
		probeInterval: probeInterval,
		now:           time.Now,
	}
}

// Start 在后台启动调度循环；ctx 取消后退出。
func (s *Scheduler) Start(ctx context.Context) {
	go s.Run(ctx)
}

// Run 先立即执行一次 keepalive，然后按分钟检查是否到达配置的小时；
// 余额探测按 probeInterval 独立触发。
func (s *Scheduler) Run(ctx context.Context) {
	s.RefreshAll(ctx)

	lastHourKey := ""
	minute := time.NewTicker(time.Minute)
	defer minute.Stop()

	var probe *time.Ticker
	if s.probeInterval > 0 {
		probe = time.NewTicker(s.probeInterval)
		defer probe.Stop()
		// 启动后立刻做一次余额探测，避免等到第一个周期才有数据。
		s.ProbeBalances(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-minute.C:
			now := s.now()
			if !s.matchHour(now.Hour()) {
				continue
			}
			key := fmt.Sprintf("%04d-%02d-%02d-%02d", now.Year(), now.Month(), now.Day(), now.Hour())
			if key == lastHourKey {
				continue
			}
			lastHourKey = key
			s.RefreshAll(ctx)
		case <-probeChan(probe):
			s.ProbeBalances(ctx)
		}
	}
}

// probeChan 把可能为 nil 的 ticker 转换为可 select 的通道。
func probeChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

func (s *Scheduler) matchHour(hour int) bool {
	for _, h := range s.hours {
		if h == hour {
			return true
		}
	}
	return false
}

// RefreshAll 扫描所有未被禁用的账号，刷新即将过期的 token。
func (s *Scheduler) RefreshAll(ctx context.Context) {
	now := s.now()
	disabled := make(map[string]bool)
	for _, st := range s.pool.List(now) {
		if st.Disabled {
			disabled[st.UID] = true
		}
	}

	for _, cred := range s.pool.Credentials() {
		uid := cred.UID
		if disabled[uid] {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if err := s.client.RefreshCredential(ctx, cred, s.skew); err != nil {
			var ue *phanthy.Error
			if errors.As(err, &ue) && ue.Kind == phanthy.KindSessionDead {
				s.logger.Printf("keepalive: 账号 %s 登录态失效，已禁用: %v", uid, err)
				s.pool.Disable(uid, "session_dead")
				continue
			}
			s.logger.Printf("keepalive: 账号 %s 刷新失败: %v", uid, err)
			continue
		}
	}
}

// ProbeBalances 通过 /api/phanthy_code/models/stats 更新余额；两次请求之间至少间隔 1 秒。
func (s *Scheduler) ProbeBalances(ctx context.Context) {
	creds := s.pool.Credentials()
	for i, cred := range creds {
		if ctx.Err() != nil {
			return
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
		snap := cred.Snapshot()
		if snap.AccessToken == "" {
			continue
		}
		body, err := s.client.ModelStats(ctx, phanthy.Auth{AccessToken: snap.AccessToken, UID: snap.UID})
		if err != nil {
			s.logger.Printf("余额探测: 账号 %s 失败: %v", snap.UID, err)
			continue
		}
		if balance := phanthy.ExtractBalance(body); balance != nil {
			s.pool.SetBalance(snap.UID, balance)
			s.logger.Printf("余额探测: 账号 %s 余额=%d", snap.UID, *balance)
		}
	}
}
