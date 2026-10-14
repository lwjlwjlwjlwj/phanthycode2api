// Package reward 解析账号的「每日开工奖励」（每日登录奖励）。
//
// 两个数据源：
//
//	/api/oauth/activities/summary          权威状态：今日是否已发、连续天数、下一笔可领时间
//	/api/oauth/rewards                     台账：每笔 daily_login 的发放时间与积分
//
// 领取本身由 upstream 包按桌面端签名协议调用
// /api/oauth/activities/daily-login/claim 完成（幂等），本包只负责口径换算与展示。
package reward

import (
	"encoding/json"
	"sort"
	"time"
)

// beijing 是上游划分业务日使用的时区（UTC+8）。
// 奖励在每天北京时间 0 点发放，落在 UTC 上就是前一天 16:00Z。
var beijing = time.FixedZone("CST", 8*60*60)

const dayLayout = "2006-01-02"

// Daily 是账号的每日开工奖励状态，供管理台与日志展示。
type Daily struct {
	Today          string  `json:"today"`            // 当前业务日（北京时间日期）
	GrantedToday   bool    `json:"granted_today"`    // 今天这一笔是否已到账
	TodayPoints    float64 `json:"today_points"`     // 今天到账的积分
	Streak         int     `json:"streak"`           // 连续领取天数（今天未到账时算到昨天为止）
	LastGrantedAt  string  `json:"last_granted_at"`  // 最近一次到账时间（北京时间 RFC3339）
	LastPoints     float64 `json:"last_points"`      // 最近一次到账积分
	NextExpectedAt string  `json:"next_expected_at"` // 下一笔预计到账时间（北京时间 0 点）
	TotalGranted   float64 `json:"total_granted"`    // 历史累计到账积分
	GrantedDays    int     `json:"granted_days"`     // 历史到账天数

	// 以下字段来自上游 activities/summary 的权威口径（比台账推算更准）。
	Status         string  `json:"status,omitempty"`           // granted_today / claimable 等上游状态
	NextPoints     float64 `json:"next_points,omitempty"`      // 下一笔可得积分（按连续天数阶梯）
	NextEligibleAt string  `json:"next_eligible_at,omitempty"` // 下一笔可领时间（RFC3339）
	Source         string  `json:"source,omitempty"`           // ledger（仅台账）| summary（已对齐上游）
}

// Summary 是 /api/oauth/activities/summary 里与本项目相关的字段。
type Summary struct {
	Enabled        bool    // feature_flags.daily_enabled
	Status         string  // daily.status
	ServerDate     string  // daily.server_date（上游业务日）
	Points         float64 // daily.points（今日已发放积分）
	StreakDay      int     // daily.streak_day
	NextPoints     float64 // daily.next_points
	NextEligibleAt string  // daily.next_eligible_at
	Available      float64 // credits.available（钱包可用积分）
	HasAvailable   bool
}

// ParseSummary 从 activities/summary 响应里取出开工奖励与钱包余额。
// 第二个返回值报告响应里是否带 daily 段（缺段说明账号没有开工奖励资格）。
func ParseSummary(summary map[string]any) (Summary, bool) {
	var out Summary
	daily, ok := summary["daily"].(map[string]any)
	if !ok {
		return out, false
	}
	out.Status, _ = daily["status"].(string)
	out.ServerDate, _ = daily["server_date"].(string)
	out.NextEligibleAt, _ = daily["next_eligible_at"].(string)
	out.Points, _ = toFloat(daily["points"])
	out.NextPoints, _ = toFloat(daily["next_points"])
	if streak, ok := toFloat(daily["streak_day"]); ok {
		out.StreakDay = int(streak)
	}
	if flags, ok := summary["feature_flags"].(map[string]any); ok {
		out.Enabled, _ = flags["daily_enabled"].(bool)
	}
	if credits, ok := summary["credits"].(map[string]any); ok {
		if available, ok := toFloat(credits["available"]); ok {
			out.Available = available
			out.HasAvailable = true
		}
	}
	return out, true
}

// MergeSummary 用上游权威状态覆盖台账推算值。
// 台账只记录已发放的批次，推断不出「今天还没发」的原因，也拿不到阶梯下一档积分。
func (d Daily) MergeSummary(s Summary) Daily {
	if s.ServerDate != "" {
		d.Today = s.ServerDate
	}
	d.Status = s.Status
	d.NextPoints = s.NextPoints
	d.NextEligibleAt = s.NextEligibleAt
	d.Source = "summary"
	if s.StreakDay > 0 {
		d.Streak = s.StreakDay
	}
	if s.Status == "granted_today" {
		d.GrantedToday = true
		if s.Points > 0 {
			d.TodayPoints = s.Points
		}
	}
	return d
}

// BeijingDate 返回 t 对应的上游业务日（北京时间日期）。
func BeijingDate(t time.Time) string { return t.In(beijing).Format(dayLayout) }

// Analyze 汇总 rewards 台账里的每日开工奖励。
// now 用于判定「今天」；台账为空时返回可读的默认值。
func Analyze(rewards []map[string]any, now time.Time) Daily {
	pointsByDay := map[string]float64{}
	var last time.Time
	var lastPoints float64
	total := 0.0

	for _, r := range rewards {
		if !isDailyLogin(r) || status(r) != "granted" {
			continue
		}
		points, ok := toFloat(r["points"])
		if !ok || points <= 0 {
			continue
		}
		granted, ok := parseTime(r["granted_at"])
		if !ok {
			continue
		}
		day := granted.In(beijing).Format(dayLayout)
		pointsByDay[day] += points
		total += points
		if granted.After(last) {
			last = granted
			lastPoints = points
		}
	}

	out := Daily{
		NextExpectedAt: nextMidnight(now).Format(time.RFC3339),
		TotalGranted:   total,
		GrantedDays:    len(pointsByDay),
		LastPoints:     lastPoints,
		Source:         "ledger",
	}
	if !last.IsZero() {
		out.LastGrantedAt = last.In(beijing).Format(time.RFC3339)
	}

	today := now.In(beijing).Format(dayLayout)
	out.Today = today
	if points, ok := pointsByDay[today]; ok {
		out.GrantedToday = true
		out.TodayPoints = points
	}
	out.Streak = streak(pointsByDay, now)
	return out
}

// streak 计算连续领取天数：今天已到账就从今天数，否则从昨天数
// （今天 0 点那一笔可能还没到账，但连续记录并未中断）。
func streak(pointsByDay map[string]float64, now time.Time) int {
	cursor := midnight(now)
	if _, ok := pointsByDay[cursor.Format(dayLayout)]; !ok {
		cursor = cursor.AddDate(0, 0, -1)
	}
	days := 0
	for {
		if _, ok := pointsByDay[cursor.Format(dayLayout)]; !ok {
			return days
		}
		days++
		cursor = cursor.AddDate(0, 0, -1)
	}
}

// midnight 返回 now 所在的北京时间当天 0 点。
func midnight(now time.Time) time.Time {
	local := now.In(beijing)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, beijing)
}

// nextMidnight 返回 now 之后最近的一个北京时间 0 点，即下一笔奖励的发放时间。
func nextMidnight(now time.Time) time.Time {
	return midnight(now).AddDate(0, 0, 1)
}

// isDailyLogin 判断一条奖励是否为每日开工奖励。
func isDailyLogin(r map[string]any) bool {
	for _, key := range []string{"reward_type", "reward_code", "reason_code"} {
		if v, _ := r[key].(string); v == "daily_login" {
			return true
		}
	}
	return false
}

func status(r map[string]any) string {
	v, _ := r["status"].(string)
	return v
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

// parseTime 解析上游的 RFC3339 时间字段。
func parseTime(v any) (time.Time, bool) {
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

// SortedDays 返回台账里出现过的业务日，按时间倒序，便于排查漏发。
func SortedDays(rewards []map[string]any) []string {
	seen := map[string]bool{}
	for _, r := range rewards {
		if !isDailyLogin(r) || status(r) != "granted" {
			continue
		}
		granted, ok := parseTime(r["granted_at"])
		if !ok {
			continue
		}
		seen[granted.In(beijing).Format(dayLayout)] = true
	}
	days := make([]string, 0, len(seen))
	for day := range seen {
		days = append(days, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}
