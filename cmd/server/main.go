// Package main phanthycode2api 入口：加载配置、构建 pool、起调度器与 HTTP 服务。
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"phanthycode2api/internal/admin"
	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/logx"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/reward"
	"phanthycode2api/internal/scheduler"
	"phanthycode2api/internal/server"
	"phanthycode2api/internal/upstream"
)

func main() {
	cfgPath := flag.String("config", "config.json", "path to config json")
	flag.Parse()

	baseDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatalf("resolve app directory: %v", err)
	}
	if !filepath.IsAbs(*cfgPath) {
		candidate := filepath.Join(baseDir, *cfgPath)
		if _, statErr := os.Stat(candidate); statErr == nil {
			*cfgPath = candidate
		} else if _, cwdStatErr := os.Stat(*cfgPath); cwdStatErr != nil {
			*cfgPath = candidate
		}
	}

	cfg, err := Load(*cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("config %s not found; create default config and run from app directory", *cfgPath)
			cfgPath, cfgErr := EnsureDefaultConfig(*cfgPath, baseDir)
			if cfgErr != nil {
				log.Fatalf("create default config: %v", cfgErr)
			}
			cfg, err = Load(cfgPath)
		}
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
	}
	if err := MakeRelativePathsAbsolute(cfg, baseDir); err != nil {
		log.Fatalf("resolve config paths: %v", err)
	}
	logx.SetLevel(cfg.LogLevelValue())

	auths, err := auth.LoadDir(cfg.AuthDir)
	if err != nil {
		log.Fatalf("load auths: %v", err)
	}
	logx.Infof("loaded %d account(s) from %s", len(auths), cfg.AuthDir)

	p := pool.New(cfg.StateFile)
	for _, a := range auths {
		p.Add(a)
	}

	up := upstream.New(cfg.BaseURL)
	up.HTTP.Timeout = time.Duration(cfg.Upstream.TimeoutSeconds) * time.Second
	// 桌面端安装身份（每日开工奖励签名用）与 state.json 放在同一数据目录，每个账号一份。
	up.DesktopKeyDir = filepath.Join(filepath.Dir(cfg.StateFile), "desktop-keys")

	sch := scheduler.New(scheduler.Config{
		Pool:              p,
		Upstream:          up,
		KeepaliveHours:    cfg.Schedule.KeepaliveHours,
		KeepaliveInterval: cfg.KeepaliveIntervalDur,
		KeepaliveSkew:     cfg.KeepaliveSkewDur,
		AutoRecover:       cfg.Schedule.AutoRecover,
		DailyReward:       cfg.Schedule.DailyReward,
		ClaimHour:         cfg.Schedule.ClaimHour,
		ClaimMinute:       cfg.Schedule.ClaimMinute,
	})

	if !cfg.Admin.Enabled && cfg.Admin.PasswordHash == "" {
		cfg.Admin.Enabled = true
		logx.Infof("admin password not configured; enabling /admin with default password admin123")
	}

	adminStore, err := admin.New(cfg.Admin.DataDir)
	if err != nil {
		log.Fatalf("load admin data: %v", err)
	}
	var adminHandler *admin.Handler
	if cfg.Admin.Enabled {
		adminStore.SetConfigState(admin.ConfigState{
			// 从磁盘重读，让设置页保存后立刻看到新值；
			// 回放启动时的内存快照会让「保存成功」看起来像没生效。
			GetConfig: func() map[string]any {
				fresh, err := Load(*cfgPath)
				if err == nil {
					err = MakeRelativePathsAbsolute(fresh, baseDir)
				}
				if err != nil {
					return cfg.ToMap()
				}
				return fresh.ToMap()
			},
			SaveConfig: func(raw map[string]any) error { return SaveConfigMap(*cfgPath, raw) },
		})
		adminHandler = admin.NewHandler(adminStore, *cfgPath, &cfg.Admin.PasswordHash)
		adminHandler.Verify = func(password string) bool {
			if cfg.Admin.PasswordHash == "" {
				return subtle.ConstantTimeCompare([]byte(password), []byte(defaultAdminPassword)) == 1
			}
			return admin.VerifyPassword(password, cfg.Admin.PasswordHash)
		}
		adminHandler.Accounts = &admin.AccountManager{
			StartOAuth: func() (map[string]any, error) { return StartOAuthLogin(cfg.BaseURL, cfg.AuthDir) },
			List: func() []map[string]any {
				out := []map[string]any{}
				for _, status := range p.List() {
					item := map[string]any{
						"uid": status.UID, "nickname": status.Nickname, "has_api_key": status.HasAPI,
						"cooling": status.Cooling, "until": status.Until, "reason": status.Reason,
						"disabled": status.Disabled, "err_count": status.ErrCount,
						"last_keepalive_at": status.LastKeepaliveAt,
					}
					if account := p.AuthByUID(status.UID); account != nil {
						for key, value := range accountUsage(up, account) {
							item[key] = value
						}
					}
					out = append(out, item)
				}
				return out
			},
			Add: func(req map[string]any) error {
				code, _ := req["code"].(string)
				verifier, _ := req["verifier"].(string)
				account, err := ExchangeAndSaveAccount(upstream.New(cfg.BaseURL), cfg.AuthDir, code, verifier)
				if err != nil {
					return err
				}
				p.Add(account)
				return nil
			},
			Delete:    func(uid string) error { return DeleteAccount(cfg.AuthDir, p, uid) },
			Refresh:   func(uid string) error { return RefreshAccount(upstream.New(cfg.BaseURL), p, uid) },
			Enable:    func(uid string) error { p.Enable(uid); return nil },
			Keepalive: func(uid string) error { return KeepaliveAccount(upstream.New(cfg.BaseURL), p, uid) },
			Claim:     sch.ClaimForUID,
		}
	}

	h := server.NewHandler(server.Config{
		Pool:         p,
		Upstream:     up,
		APIKey:       cfg.APIKey,
		HardCooldown: cfg.HardCreditDur,
		SoftCooldown: cfg.SoftRateDur,
		ErrThreshold: cfg.Cooldown.ErrThresh,
		ErrCooldown:  cfg.ErrCooldownDur,
		Admin:        adminHandler,
		Thinking:     cfg.ThinkingOption(),
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sch.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           h,
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	logx.Infof("phanthycode2api listening on %s (api_key=%v, admin=%v, thinking=%s, log_level=%s)",
		cfg.Listen, cfg.APIKey != "", adminHandler != nil, cfg.ThinkingOption().Normalize().Mode, logx.Current())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
	logx.Infof("bye")
}

type usageCacheEntry struct {
	data map[string]any
	at   time.Time
}

var usageCache = struct {
	sync.Mutex
	items map[string]usageCacheEntry
}{items: map[string]usageCacheEntry{}}

// dailyRewardSeen 记录每个账号最近提示过的业务日，保证「开工奖励到账」同一天只提示一次。
var dailyRewardSeen = struct {
	sync.Mutex
	days map[string]string
}{days: map[string]string{}}

// usageCacheTTL 管理页额度缓存时长：刷新一个账号要打 3 个上游接口，缓存久一点才不至于把刷新按成压测。
const usageCacheTTL = 5 * time.Minute

// rewardLabels 奖励类型 → 官网「套餐」页里的额度池名称。
var rewardLabels = map[string]string{
	"daily_login":              "每日登录奖励",
	"long_task_feedback":       "活动奖励",
	"token_factory_activation": "代币工厂奖励",
	"referral_inviter":         "推荐奖励",
}

// creditLot 是奖励台账里的一批额度，一笔已发放的奖励对应一个批次。
type creditLot struct {
	kind      string
	points    float64
	used      float64
	granted   time.Time
	hasGrant  bool
	expiry    time.Time
	hasExpiry bool
}

// usageDay 是某一天的积分消耗，由用量明细按天汇总而来。
type usageDay struct {
	start time.Time
	end   time.Time
	cost  float64
}

// usageByDay 把用量明细按自然日汇总成消耗序列（上游按天聚合，没有更细的时间点）。
func usageByDay(summary map[string]any) []usageDay {
	rows, _ := summary["usage_by_day_and_model"].([]any)
	buckets := map[string]*usageDay{}
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			continue
		}
		date, _ := item["date"].(string)
		if date == "" {
			continue
		}
		bucket := buckets[date]
		if bucket == nil {
			start, err := time.Parse("2006-01-02", date)
			if err != nil {
				continue
			}
			bucket = &usageDay{start: start.UTC(), end: start.UTC().Add(24 * time.Hour)}
			buckets[date] = bucket
			order = append(order, date)
		}
		cost, _ := toFloat(item["cost_points"])
		bucket.cost += cost
	}
	days := make([]usageDay, 0, len(order))
	for _, date := range order {
		days = append(days, *buckets[date])
	}
	return days
}

// lotOrderKey 给出批次的抵扣顺序：先发放的先扣。
func lotOrderKey(lot creditLot) time.Time {
	if lot.hasGrant {
		return lot.granted
	}
	if lot.hasExpiry {
		return lot.expiry
	}
	return time.Time{}
}

// lotAliveOn 判断批次在某一天是否处于有效期内。
func lotAliveOn(lot *creditLot, day usageDay) bool {
	if lot.hasExpiry && !lot.expiry.After(day.start) {
		return false
	}
	if lot.hasGrant && !lot.granted.Before(day.end) {
		return false
	}
	return true
}

// chargeLots 按「先发放先扣」把每天的消耗摊到当天仍然有效的批次上。
// 批次过期时没用完的部分直接作废，所以只能用「消耗发生当天还在有效期内」的批次来抵扣；
// 否则会把早已作废的额度也算成已用，和官网「套餐」页的剩余量对不上。
func chargeLots(lots []creditLot, days []usageDay) {
	for _, day := range days {
		remaining := day.cost
		for i := range lots {
			if remaining <= 0 {
				break
			}
			lot := &lots[i]
			capacity := lot.points - lot.used
			if capacity <= 0 || !lotAliveOn(lot, day) {
				continue
			}
			take := math.Min(remaining, capacity)
			lot.used += take
			remaining -= take
		}
	}
}

// walletPool 是「套餐」页里的一行额度池。
type walletPool struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
	Used      float64 `json:"used"`
	Lots      int     `json:"lots"`
	ExpiresAt string  `json:"expires_at,omitempty"`
	Estimate  bool    `json:"estimate,omitempty"`
}

// accountUsage 汇总账号额度，口径对齐官网「套餐」页：
// 钱包总额 = 套餐池 + 各奖励池（未过期批次）之和，剩余 = 总额 - 已用。
// 批次级的扣减账本上游没有公开接口，奖励池的「已用」由用量汇总按先到期先扣还原，因此标记为估算。
func accountUsage(client *upstream.Client, account *auth.Auth) map[string]any {
	if account == nil {
		return map[string]any{"credits_error": "account not found"}
	}
	usageCache.Lock()
	if entry, ok := usageCache.items[account.UID]; ok && time.Since(entry.at) < usageCacheTTL {
		data := entry.data
		usageCache.Unlock()
		return data
	}
	usageCache.Unlock()

	raw, err := client.FetchUsage(account)
	if err != nil && account.NeedsRefresh(10*time.Minute) {
		if refreshErr := client.RefreshToken(account); refreshErr == nil {
			_ = account.SaveAtomic()
			raw, err = client.FetchUsage(account)
		}
	}
	if err != nil {
		data := map[string]any{"credits_error": err.Error()}
		// 上游账号接口挂了也照常汇总开工奖励：管理台据此判断「今天到底领到没有、要不要重新登录」。
		attachDaily(client, account, data, nil, false)
		return cacheUsage(account.UID, data)
	}

	data := extractUsage(raw)

	summary, _ := client.FetchUsageSummary(account)
	planName := ""
	if plan, ok := summary["plan"].(map[string]any); ok {
		planName, _ = plan["name"].(string)
		if planName != "" {
			data["credits_plan"] = planName
		}
		if expires, _ := plan["expires_at"].(string); expires != "" {
			data["credits_plan_expires_at"] = expires
		}
	}
	consumed := sumCostPoints(summary)
	if consumed > 0 {
		data["credits_consumed"] = consumed
	}

	rewards, rewardsErr := client.FetchRewards(account)
	if rewardsErr != nil && len(rewards) == 0 {
		data["credits_rewards_error"] = rewardsErr.Error()
	}
	attachDaily(client, account, data, rewards, rewardsErr == nil || len(rewards) > 0)

	pools, pending := buildWallet(planName, data, rewards, summary)
	if len(pools) == 0 {
		if _, ok := data["credits_error"]; !ok {
			data["credits_error"] = "no usage data"
		}
		return cacheUsage(account.UID, data)
	}
	total, left, approx := 0.0, 0.0, false
	for _, pool := range pools {
		total += pool.Total
		left += pool.Remaining
		if pool.Estimate {
			approx = true
		}
	}
	data["wallet_pools"] = pools
	data["wallet_total"] = total
	data["wallet_remaining"] = left
	data["wallet_used"] = total - left
	data["wallet_approx"] = approx
	if pending > 0 {
		data["wallet_pending"] = pending
	}
	return cacheUsage(account.UID, data)
}

// attachDaily 汇总每日开工奖励，写进管理页用的 data：
// 台账（/api/oauth/rewards）给历史累计，activities/summary 给上游权威的今日状态与阶梯下一档。
// ledgerOK 为 false 表示台账没读到（账号接口已失败），此时只信 summary，不再拿空台账冒充「无记录」。
func attachDaily(client *upstream.Client, account *auth.Auth, data map[string]any, rewards []map[string]any, ledgerOK bool) {
	daily := reward.Daily{Today: reward.BeijingDate(time.Now())}
	if ledgerOK {
		daily = reward.Analyze(rewards, time.Now())
	}
	id, idErr := client.DesktopIdentity(account.UID)
	if idErr != nil {
		data["daily_summary_error"] = "桌面端安装身份不可用: " + idErr.Error()
	} else if summary, sumErr := client.ActivitySummary(account, id); sumErr == nil {
		if state, ok := reward.ParseSummary(summary); ok {
			daily = daily.MergeSummary(state)
			// credits.available 是上游钱包余额，与官网「钱包积分」逐分一致。
			if state.HasAvailable {
				data["credits_available"] = state.Available
			}
		}
	} else {
		data["daily_summary_error"] = sumErr.Error()
	}
	if ledgerOK || daily.Source == "summary" {
		data["daily"] = daily
		notifyDailyReward(account, daily)
	}
}

// cacheUsage 写入管理页缓存并返回结果。
func cacheUsage(uid string, data map[string]any) map[string]any {
	usageCache.Lock()
	usageCache.items[uid] = usageCacheEntry{data: data, at: time.Now()}
	usageCache.Unlock()
	return data
}

// notifyDailyReward 在每日开工奖励到账时提示一次。
// 领取由 scheduler 每天自动完成，这里负责在管理台读到「已到账」时补一条日志，
// 每个账号每个业务日只提示一次。
func notifyDailyReward(account *auth.Auth, daily reward.Daily) {
	if !daily.GrantedToday || daily.Today == "" {
		return
	}
	dailyRewardSeen.Lock()
	seen := dailyRewardSeen.days[account.UID]
	dailyRewardSeen.days[account.UID] = daily.Today
	dailyRewardSeen.Unlock()
	if seen == daily.Today {
		return
	}
	logx.Infof("开工奖励 %s: %s 已到账 +%.0f 积分（连续 %d 天，累计 %.0f）",
		account.Nickname, daily.Today, daily.TodayPoints, daily.Streak, daily.TotalGranted)
}

// buildWallet 把套餐池与奖励批次整理成「套餐」页的额度池列表。
// 第二个返回值是 pending 状态的待发放积分（官网同样不计入钱包余额）。
func buildWallet(planName string, usage map[string]any, rewards []map[string]any, summary map[string]any) ([]walletPool, float64) {
	pools := make([]walletPool, 0, 4)
	if remaining, ok := toFloat(usage["credits_remaining"]); ok {
		total, _ := toFloat(usage["credits_total"])
		used, _ := toFloat(usage["credits_used"])
		label := planName
		if label == "" {
			label = "套餐额度"
		}
		expires, _ := usage["credits_reset_at"].(string)
		pools = append(pools, walletPool{
			Key: "plan", Label: label, Total: total, Remaining: remaining, Used: used,
			Lots: 1, ExpiresAt: expires,
		})
	}

	lots := make([]creditLot, 0, len(rewards))
	pending := 0.0
	for _, reward := range rewards {
		points, ok := toFloat(reward["points"])
		if !ok || points <= 0 {
			continue
		}
		status, _ := reward["status"].(string)
		if status != "granted" {
			if status == "pending" {
				pending += points
			}
			continue
		}
		kind, _ := reward["reward_type"].(string)
		if kind == "" {
			kind = "other"
		}
		lot := creditLot{kind: kind, points: points}
		lot.granted, lot.hasGrant = parseUpstreamTime(reward["granted_at"])
		lot.expiry, lot.hasExpiry = parseUpstreamTime(reward["expires_at"])
		lots = append(lots, lot)
	}
	if len(lots) == 0 {
		return pools, pending
	}
	sort.SliceStable(lots, func(i, j int) bool {
		return lotOrderKey(lots[i]).Before(lotOrderKey(lots[j]))
	})
	chargeLots(lots, usageByDay(summary))

	now := time.Now()
	type rewardPool struct {
		total, used float64
		lots        int
		earliest    time.Time
		hasEarliest bool
	}
	groups := map[string]*rewardPool{}
	order := make([]string, 0, 4)
	for _, lot := range lots {
		if lot.hasExpiry && !lot.expiry.After(now) {
			continue // 已过期批次不再计入钱包
		}
		group := groups[lot.kind]
		if group == nil {
			group = &rewardPool{}
			groups[lot.kind] = group
			order = append(order, lot.kind)
		}
		group.total += lot.points
		group.used += lot.used
		group.lots++
		if lot.hasExpiry && (!group.hasEarliest || lot.expiry.Before(group.earliest)) {
			group.earliest = lot.expiry
			group.hasEarliest = true
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, right := groups[order[i]], groups[order[j]]
		if !left.hasEarliest {
			return false
		}
		if !right.hasEarliest {
			return true
		}
		return left.earliest.Before(right.earliest)
	})
	for _, kind := range order {
		group := groups[kind]
		label := rewardLabels[kind]
		if label == "" {
			label = kind
		}
		pool := walletPool{
			Key: kind, Label: label, Total: group.total, Lots: group.lots,
			Used: group.used, Remaining: group.total - group.used, Estimate: true,
		}
		if group.hasEarliest {
			pool.ExpiresAt = group.earliest.UTC().Format(time.RFC3339)
		}
		pools = append(pools, pool)
	}
	return pools, pending
}

// sumCostPoints 汇总用量明细里的消耗积分（上游只返回最近一段时间的明细）。
func sumCostPoints(summary map[string]any) float64 {
	rows, _ := summary["usage_by_day_and_model"].([]any)
	total := 0.0
	for _, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if points, ok := toFloat(item["cost_points"]); ok {
			total += points
		}
	}
	return total
}

// extractUsage 从 /api/oauth/usage 响应中取出管理页需要的积分字段。
func extractUsage(raw map[string]any) map[string]any {
	plan, _ := raw["current_plan"].(map[string]any)
	if plan == nil {
		plan, _ = raw["seven_day"].(map[string]any)
	}
	out := map[string]any{}
	if plan == nil {
		out["credits_error"] = "no usage data"
		return out
	}
	out["credits_remaining"] = plan["remaining_credits"]
	out["credits_total"] = plan["total_credits"]
	out["credits_used"] = plan["used_credits"]
	out["credits_percent"] = plan["remaining_percentage"]
	out["credits_reset_at"] = plan["resets_at"]
	out["credits_window"] = plan["window_kind"]
	return out
}

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

// parseUpstreamTime 解析上游的 RFC3339 时间字段。
func parseUpstreamTime(v any) (time.Time, bool) {
	text, ok := v.(string)
	if !ok || text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

const defaultAdminPassword = "admin123"

// EnsureDefaultConfig 在可执行文件目录创建开箱即用的 config.json。
func EnsureDefaultConfig(path, baseDir string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	cfg := Default()
	if err := MakeRelativePathsAbsolute(cfg, baseDir); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// MakeRelativePathsAbsolute 让双击运行时始终读写 exe 旁边的 auths/ 与 data/。
func MakeRelativePathsAbsolute(cfg *Config, baseDir string) error {
	resolve := func(value string) (string, error) {
		if value == "" || filepath.IsAbs(value) {
			return value, nil
		}
		return filepath.Abs(filepath.Join(baseDir, value))
	}
	var err error
	if cfg.AuthDir, err = resolve(cfg.AuthDir); err != nil {
		return err
	}
	if cfg.StateFile, err = resolve(cfg.StateFile); err != nil {
		return err
	}
	if cfg.Admin.DataDir, err = resolve(cfg.Admin.DataDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.AuthDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.StateFile), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(cfg.Admin.DataDir, 0o755)
}

// ToMap 导出当前配置，供管理端设置页展示。
func (c *Config) ToMap() map[string]any {
	raw, _ := json.Marshal(c)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	out["hard_credit_duration"] = c.HardCreditDur.String()
	out["soft_rate_duration"] = c.SoftRateDur.String()
	out["err_cooldown_duration"] = c.ErrCooldownDur.String()
	if adminConfig, ok := out["admin"].(map[string]any); ok {
		if adminConfig["password_hash"] != "" {
			adminConfig["password_hash"] = "***"
		}
	}
	return out
}

// SaveConfigMap 保存设置页提交的配置；listen 与 admin 保留原值，避免网页误改。
func SaveConfigMap(path string, raw map[string]any) error {
	existing := map[string]any{}
	if current, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(current, &existing)
	}
	out := map[string]any{}
	for key, value := range raw {
		switch key {
		case "listen", "admin", "hard_credit_duration", "soft_rate_duration", "err_cooldown_duration":
			continue
		}
		out[key] = value
	}
	if listen, ok := raw["listen"].(string); ok && strings.TrimSpace(listen) != "" {
		listen = strings.TrimSpace(listen)
		if !strings.Contains(listen, ":") {
			listen = ":" + listen
		}
		out["listen"] = listen
	} else if listen, ok := existing["listen"]; ok {
		out["listen"] = listen
	}
	if adminConfig, ok := existing["admin"]; ok {
		out["admin"] = adminConfig
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	// 先按启动路径校验再落盘：写错一个字段（例如 keepalive_interval 填成 "30"）
	// 会让服务下次启动直接失败，不如当场报错让用户改。
	if err := validateConfigBytes(encoded); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// validateConfigBytes 用与启动一致的解析路径校验即将写入的配置。
func validateConfigBytes(raw []byte) error {
	c := Default()
	if err := json.Unmarshal(raw, c); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return c.normalize()
}

// StartOAuthLogin 生成 PKCE verifier 和授权链接；verifier 暂存到账号目录内，等待 code 提交。
func StartOAuthLogin(baseURL, authDir string) (map[string]any, error) {
	verifier, err := randomPKCEVerifier()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	values := url.Values{}
	values.Set("client_id", "phanthy-code-cli")
	values.Set("response_type", "code")
	values.Set("redirect_uri", "https://code.phanthy.com/oauth/code/success")
	values.Set("scope", "user:inference user:profile user:sessions:claude_code")
	values.Set("code_challenge", challenge)
	values.Set("code_challenge_method", "S256")
	values.Set("state", "p2a-admin-login")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(authDir, ".admin-login-verifier"), []byte(verifier), 0o600); err != nil {
		return nil, err
	}
	return map[string]any{
		"url": strings.TrimRight(baseURL, "/") + "/oauth/authorize?" + values.Encode(),
	}, nil
}

// ExchangeAndSaveAccount 用授权码换取 token 并写入账号目录。
func ExchangeAndSaveAccount(client *upstream.Client, dir, rawCode, rawVerifier string) (*auth.Auth, error) {
	code := extractCode(rawCode)
	if code == "" {
		return nil, fmt.Errorf("authorization code is empty")
	}
	verifier := strings.TrimSpace(rawVerifier)
	if verifier == "" {
		raw, err := os.ReadFile(filepath.Join(dir, ".admin-login-verifier"))
		if err != nil {
			return nil, fmt.Errorf("read verifier: %w", err)
		}
		verifier = strings.TrimSpace(string(raw))
	}
	if verifier == "" {
		return nil, fmt.Errorf("PKCE verifier is empty")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", "https://code.phanthy.com/oauth/code/success")
	form.Set("client_id", "phanthy-code-cli")
	req, _ := http.NewRequest(http.MethodPost, client.OAuthTokenURL, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-beta", client.OAuthBetaHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, err
	}
	if token.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in response")
	}
	uid := fmt.Sprintf("phanthy-%d", time.Now().Unix())
	account := &auth.Auth{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).Unix(),
		UID:          uid,
		Nickname:     auth.UniqueNickname(dir, uid, "phanthy-"+time.Now().Format("0102")),
	}
	if err := saveAuth(dir, account); err != nil {
		return nil, err
	}
	_ = client.EnsureAPIKey(account)
	if err := account.SaveAtomic(); err != nil {
		return nil, err
	}
	return account, nil
}

// DeleteAccount 删除凭证文件并从账号池移除。
func DeleteAccount(dir string, pool *pool.Pool, uid string) error {
	if uid == "" || strings.ContainsAny(uid, `/\`) {
		return fmt.Errorf("invalid account id")
	}
	matches, err := filepath.Glob(filepath.Join(dir, uid+".json"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if filepath.Base(path) != uid+".json" {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	// 即使凭证文件已不存在，也要清掉账号池里的残留状态。
	pool.Remove(uid)
	return nil
}

// RefreshAccount 手动刷新指定账号的 OAuth token。
func RefreshAccount(client *upstream.Client, pool *pool.Pool, uid string) error {
	acct := pool.AuthByUID(uid)
	if acct == nil {
		return fmt.Errorf("account not found")
	}
	if err := client.RefreshToken(acct); err != nil {
		return err
	}
	if err := acct.SaveAtomic(); err != nil {
		return err
	}
	pool.Enable(uid)
	return nil
}

// KeepaliveAccount 拉取账号 profile，用于校验会话并触发上游活跃度。
func KeepaliveAccount(client *upstream.Client, pool *pool.Pool, uid string) error {
	acct := pool.AuthByUID(uid)
	if acct == nil {
		return fmt.Errorf("account not found")
	}
	// 手动「保活」= 立刻换一份新 token，不等到快过期才刷。
	if strings.TrimSpace(acct.RefreshToken) != "" {
		if err := client.RefreshToken(acct); err != nil {
			return err
		}
		if err := acct.SaveAtomic(); err != nil {
			return err
		}
		pool.NoteKeepalive(uid)
	}
	if _, err := client.FetchProfile(acct); err != nil {
		return err
	}
	pool.Enable(uid)
	return nil
}

// saveAuth 写入新账号凭证。
func saveAuth(dir string, account *auth.Auth) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	account.FilePath = filepath.Join(dir, account.UID+".json")
	return account.SaveAtomic()
}

// extractCode 提取完整回调 URL 或裸授权码中的纯 code。
// randomPKCEVerifier 生成浏览器授权流程使用的 verifier。
func randomPKCEVerifier() (string, error) {
	buf := make([]byte, 48)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
func extractCode(input string) string {
	input = strings.TrimSpace(input)
	if parsed, err := url.Parse(input); err == nil && parsed.Query().Get("code") != "" {
		return parsed.Query().Get("code")
	}
	if index := strings.IndexByte(input, '#'); index >= 0 {
		input = input[:index]
	}
	if index := strings.Index(input, "code="); index >= 0 {
		input = input[index+5:]
	}
	if index := strings.IndexAny(input, "& "); index >= 0 {
		input = input[:index]
	}
	return strings.TrimSpace(input)
}
