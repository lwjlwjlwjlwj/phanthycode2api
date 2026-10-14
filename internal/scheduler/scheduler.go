// Package scheduler 定时任务：token keepalive + 每日开工奖励自动领取。
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/logx"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/reward"
	"phanthycode2api/internal/upstream"
)

// beijing 是上游划分业务日使用的时区（UTC+8）。
// 开工奖励按北京时间自然日发放，所以领取调度也用同一时区。
var beijing = time.FixedZone("CST", 8*60*60)

// Config 调度器依赖。
type Config struct {
	Pool     *pool.Pool
	Upstream *upstream.Client
	// KeepaliveHours 是额外的定点保活小时（本地时区），默认 [22]。
	// 定时保活现在主要由 KeepaliveInterval 承担，这里保留给「希望每天固定时刻
	// 一定刷一次」的场景，留空即关闭定点保活。
	KeepaliveHours []int
	// KeepaliveInterval 是后台自动保活的检查间隔，默认 30m。
	// 每轮只刷新「access_token 剩余不足 KeepaliveSkew」的账号，0 表示关闭。
	KeepaliveInterval time.Duration
	// KeepaliveSkew 是保活提前量：token 剩余寿命小于它就该刷新，默认 15m。
	KeepaliveSkew time.Duration
	// AutoRecover 为真时，被禁用（session 失效）的账号也会参与保活，
	// 刷新成功后自动重新启用，默认 true。
	AutoRecover bool

	DailyReward bool // 每天自动登记安装并领取开工奖励
	ClaimHour   int  // 领取时刻（北京时间小时），默认 0
	ClaimMinute int  // 领取时刻（北京时间分钟），默认 5
}

// Scheduler 调度器。
type Scheduler struct {
	cfg Config
	// mu 串行化保活 / 领奖，避免后台定时器与手动操作同时对同一账号刷新 token。
	mu sync.Mutex
}

// New 构建。
func New(cfg Config) *Scheduler {
	if cfg.KeepaliveInterval == 0 {
		cfg.KeepaliveInterval = 30 * time.Minute
	}
	if cfg.KeepaliveInterval < 0 {
		cfg.KeepaliveInterval = 0
	}
	if cfg.KeepaliveSkew <= 0 {
		cfg.KeepaliveSkew = 15 * time.Minute
	}
	if cfg.ClaimHour < 0 || cfg.ClaimHour > 23 {
		cfg.ClaimHour = 0
	}
	if cfg.ClaimMinute < 0 || cfg.ClaimMinute > 59 {
		cfg.ClaimMinute = 5
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

// nextDailyClaim 返回 now 之后最近的一个领取时刻（北京时间）。
func nextDailyClaim(now time.Time, hour, minute int) time.Time {
	local := now.In(beijing)
	t := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, beijing)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// Run 主循环，阻塞直到 ctx 取消。
func (s *Scheduler) Run(ctx context.Context) {
	// 启动时立即执行一次 keepalive（保证服务起来后账号立即可用）
	logx.Infof("scheduler: running initial keepalive")
	s.RunKeepaliveNow()

	if s.cfg.DailyReward {
		// 启动即补领：进程可能是在北京时间 0 点之后才起来的。
		logx.Infof("scheduler: 每日开工奖励自动领取已启用（每天北京时间 %02d:%02d）", s.cfg.ClaimHour, s.cfg.ClaimMinute)
		s.RunDailyClaimNow()
	}
	if s.cfg.KeepaliveInterval > 0 {
		logx.Infof("scheduler: 自动保活已启用（每 %s 检查一次，token 剩余不足 %s 即刷新，自动恢复=%v）",
			s.cfg.KeepaliveInterval, s.cfg.KeepaliveSkew, s.cfg.AutoRecover)
	}

	// 后台保活定时器：与定点触发并列，保证服务空转时账号也不会掉线。
	var keepC <-chan time.Time
	if s.cfg.KeepaliveInterval > 0 {
		ticker := time.NewTicker(s.cfg.KeepaliveInterval)
		defer ticker.Stop()
		keepC = ticker.C
	}
	for {
		now := time.Now()
		var nextKeep time.Time
		if len(s.cfg.KeepaliveHours) > 0 {
			nextKeep = nextFire(now, s.cfg.KeepaliveHours)
		}
		nextClaim := time.Time{}
		if s.cfg.DailyReward {
			nextClaim = nextDailyClaim(now, s.cfg.ClaimHour, s.cfg.ClaimMinute)
		}
		target := nextClaim
		if target.IsZero() || (!nextKeep.IsZero() && nextKeep.Before(target)) {
			target = nextKeep
		}
		var timerC <-chan time.Time
		var timer *time.Timer
		if !target.IsZero() {
			timer = time.NewTimer(time.Until(target))
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-keepC:
			if timer != nil {
				timer.Stop()
			}
			s.runKeepalive(false)
		case <-timerC:
			// 定时器可能比目标时刻略早触发，留 1 秒余量再判定。
			due := time.Now().Add(time.Second)
			if !nextClaim.IsZero() && !nextClaim.After(due) {
				s.RunDailyClaimNow()
			}
			if !nextKeep.IsZero() && !nextKeep.After(due) {
				s.RunKeepaliveNow()
			}
		}
	}
}

// RunKeepaliveNow 立即对所有账号刷新 token：启动、定点保活与手动触发都走这里。
// 被禁用（session 失效）的账号在 AutoRecover 开启时也会重试，成功后自动恢复。
func (s *Scheduler) RunKeepaliveNow() {
	s.runKeepalive(true)
}

// runKeepalive 遍历账号刷新 token；force 为真时忽略「是否接近过期」的判断。
func (s *Scheduler) runKeepalive(force bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	refreshed, recovered, failed := 0, 0, 0
	for _, st := range s.cfg.Pool.List() {
		if st.Disabled && !s.cfg.AutoRecover {
			continue
		}
		a := s.cfg.Pool.AuthByUID(st.UID)
		if a == nil {
			continue
		}
		// 被禁用的账号要主动重试：它的 access_token 可能还没过期，
		// 但会话已失效，只有真的换一次 token 才能判断是否恢复。
		ok, err := s.refreshToken(a, force || st.Disabled)
		if err != nil {
			failed++
			var ue *upstream.Error
			sessionDead := errors.As(err, &ue) && ue.Kind == upstream.ErrSessionDead
			if sessionDead && !st.Disabled {
				s.cfg.Pool.Disable(st.UID, "session dead")
			}
			// 只在「刚坏掉」时报错，避免坏账号每轮都刷屏。
			if !st.Disabled {
				logx.Errorf("keepalive %s: %v", st.UID, err)
			} else {
				logx.Debugf("keepalive %s: 仍不可用: %v", st.UID, err)
			}
			continue
		}
		if !ok {
			continue
		}
		refreshed++
		if st.Disabled {
			s.cfg.Pool.Enable(st.UID)
			recovered++
			logx.Infof("keepalive %s: session 已恢复，重新启用", st.UID)
			continue
		}
		logx.Debugf("keepalive %s: token refreshed", st.UID)
	}
	if failed > 0 || recovered > 0 {
		logx.Infof("keepalive: 本轮结束（刷新 %d · 恢复 %d · 失败 %d）", refreshed, recovered, failed)
	}
}

// refreshToken 刷新单个账号的 access_token，返回是否真的刷新了。
// 刷新成功即视为一次有效保活，记到账号状态里供管理台展示。
func (s *Scheduler) refreshToken(a *auth.Auth, force bool) (bool, error) {
	if a == nil || strings.TrimSpace(a.RefreshToken) == "" {
		return false, nil
	}
	if !force && !a.NeedsRefresh(s.cfg.KeepaliveSkew) {
		return false, nil
	}
	if err := s.cfg.Upstream.RefreshToken(a); err != nil {
		return false, err
	}
	// 确保 api_key 有效
	if a.APIKey == "" {
		if err := s.cfg.Upstream.EnsureAPIKey(a); err != nil {
			logx.Debugf("keepalive %s ensure_api_key: %v", a.UID, err)
		}
	}
	if err := a.SaveAtomic(); err != nil {
		logx.Errorf("keepalive %s save: %v", a.UID, err)
	}
	s.cfg.Pool.NoteKeepalive(a.UID)
	return true, nil
}

// RunDailyClaimNow 为每个账号登记桌面端安装并领取当天开工奖励。
// 领取接口幂等（当天已发放时返回 already_granted），可安全地重复执行。
func (s *Scheduler) RunDailyClaimNow() {
	s.mu.Lock()
	defer s.mu.Unlock()
	ok, skipped, failed := 0, 0, 0
	for _, st := range s.cfg.Pool.List() {
		account := s.cfg.Pool.AuthByUID(st.UID)
		if account == nil {
			continue
		}
		// 被禁用（会话失效等）的账号不再参与挑号，但 access_token 往往还有几小时有效期，
		// 这时候照样能领开工奖励。领到就是白赚，所以只要 token 没过期就试一次，
		// 避免因为一次 keepalive 失败把连续天数断掉。
		if account.ExpiresAt > 0 && time.Now().Unix() >= account.ExpiresAt {
			logx.Debugf("开工奖励 %s: access_token 已过期，跳过", account.Nickname)
			skipped++
			continue
		}
		if err := s.claimDaily(account); err != nil {
			failed++
			continue
		}
		ok++
	}
	if ok+skipped+failed > 0 {
		logx.Infof("开工奖励: 本轮结束（成功 %d · 跳过 %d · 失败 %d）", ok, skipped, failed)
	}
}

// ClaimForUID 为单个账号补领当天的开工奖励，供管理台「领取」按钮调用。
func (s *Scheduler) ClaimForUID(uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.cfg.Pool.AuthByUID(uid)
	if account == nil {
		return fmt.Errorf("account not found")
	}
	return s.claimDaily(account)
}

// claimDaily 登记安装 → 查询状态 → 领取，三步都幂等。
// 安装身份按账号区分：上游的 installation id 全局唯一且绑定首个登记的账号。
// 每一步都会记日志，返回值供管理台手动领取时回显。
func (s *Scheduler) claimDaily(account *auth.Auth) error {
	id, err := s.cfg.Upstream.DesktopIdentity(account.UID)
	if err != nil {
		logx.Errorf("开工奖励 %s: 加载桌面端身份失败: %v", account.Nickname, err)
		return fmt.Errorf("加载桌面端身份失败: %w", err)
	}
	if err := s.cfg.Upstream.RegisterDesktopInstallation(account, id); err != nil {
		logx.Errorf("开工奖励 %s: 登记桌面端安装失败: %v", account.Nickname, err)
		return fmt.Errorf("登记桌面端安装失败: %w", err)
	}
	summary, err := s.cfg.Upstream.ActivitySummary(account, id)
	if err != nil {
		logx.Errorf("开工奖励 %s: 查询状态失败: %v", account.Nickname, err)
		return fmt.Errorf("查询开工奖励状态失败: %w", err)
	}
	state, ok := reward.ParseSummary(summary)
	if !ok {
		logx.Debugf("开工奖励 %s: 上游未返回 daily 段，跳过", account.Nickname)
		return fmt.Errorf("上游未返回 daily 段（账号可能没有开工奖励资格）")
	}
	if state.Status == "granted_today" {
		logx.Infof("开工奖励 %s: %s 已到账 +%.0f 积分（连续 %d 天%s）",
			account.Nickname, state.ServerDate, state.Points, state.StreakDay, nextPointsNote(state.NextPoints))
		return nil
	}
	serverDate := state.ServerDate
	if serverDate == "" {
		serverDate = reward.BeijingDate(time.Now())
	}
	result, err := s.cfg.Upstream.ClaimDailyLogin(account, id, serverDate)
	if err != nil {
		logx.Errorf("开工奖励 %s: 领取失败: %v", account.Nickname, err)
		return fmt.Errorf("领取失败: %w", err)
	}
	status, _ := result["status"].(string)
	if status == "" {
		status = "ok"
	}
	points, _ := toFloat(result["points"])
	if points <= 0 {
		points = state.Points
	}
	logx.Infof("开工奖励 %s: %s 领取结果 %s +%.0f 积分%s",
		account.Nickname, serverDate, status, points, nextPointsNote(state.NextPoints))
	return nil
}

// nextPointsNote 拼出「下一档积分」提示，缺数据时返回空串。
func nextPointsNote(next float64) string {
	if next <= 0 {
		return ""
	}
	return "，下一档 " + trimFloat(next) + " 积分"
}

// trimFloat 去掉小数末尾的 0，便于日志阅读。
func trimFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// toFloat 兼容 JSON 反序列化后的各种数值类型。
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
