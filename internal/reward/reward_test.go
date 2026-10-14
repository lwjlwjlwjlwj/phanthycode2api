package reward

import (
	"testing"
	"time"
)

// at 构造一条已发放的每日开工奖励台账记录。
func at(grantedAt string, points float64) map[string]any {
	return map[string]any{
		"reward_type":        "daily_login",
		"reward_code":        "daily_login",
		"status":             "granted",
		"points":             points,
		"granted_at":         grantedAt,
		"source_object_type": "DesktopInstallation",
	}
}

// beijingTime 构造带 +08:00 偏移的本地时间，模拟国内机器的时钟。
func beijingTime(y int, m time.Month, d, hour int) time.Time {
	return time.Date(y, m, d, hour, 0, 0, 0, beijing)
}

// TestAnalyzeCountsGrantAtBeijingMidnight 上游在 UTC 16:00 发放，落在北京时间次日 0 点，
// 这一笔应算作「当天」的开工奖励，而不是前一天。
func TestAnalyzeCountsGrantAtBeijingMidnight(t *testing.T) {
	rewards := []map[string]any{
		at("2026-09-26T16:00:00Z", 1500),
		at("2026-09-27T16:00:00Z", 1500),
	}

	got := Analyze(rewards, beijingTime(2026, time.September, 28, 13))

	if !got.GrantedToday {
		t.Fatal("GrantedToday = false, want true")
	}
	if got.TodayPoints != 1500 {
		t.Fatalf("TodayPoints = %v, want 1500", got.TodayPoints)
	}
	if got.Streak != 2 {
		t.Fatalf("Streak = %d, want 2", got.Streak)
	}
	if got.TotalGranted != 3000 {
		t.Fatalf("TotalGranted = %v, want 3000", got.TotalGranted)
	}
	if got.GrantedDays != 2 {
		t.Fatalf("GrantedDays = %d, want 2", got.GrantedDays)
	}
	if got.Today != "2026-09-28" {
		t.Fatalf("Today = %q, want 2026-09-28", got.Today)
	}
}

// TestAnalyzeKeepsStreakWhenTodayPending 今天的还没到账时，连续天数算到昨天，不归零。
func TestAnalyzeKeepsStreakWhenTodayPending(t *testing.T) {
	rewards := []map[string]any{
		at("2026-09-25T16:00:00Z", 1200),
		at("2026-09-26T16:00:00Z", 1500),
		at("2026-09-27T16:00:00Z", 1500),
	}

	// 9-29 当天，此时 9-29 的奖励是 9-28T16:00Z，尚未出现。
	got := Analyze(rewards, beijingTime(2026, time.September, 29, 9))

	if got.GrantedToday {
		t.Fatal("GrantedToday = true, want false")
	}
	if got.Streak != 3 {
		t.Fatalf("Streak = %d, want 3", got.Streak)
	}
	if got.TodayPoints != 0 {
		t.Fatalf("TodayPoints = %v, want 0", got.TodayPoints)
	}
}

// TestAnalyzeResetsStreakAfterGap 断签后连续天数从最近一段重新算。
func TestAnalyzeResetsStreakAfterGap(t *testing.T) {
	rewards := []map[string]any{
		at("2026-09-20T16:00:00Z", 300),
		at("2026-09-21T16:00:00Z", 600),
		// 9-22、9-23 断签
		at("2026-09-23T16:00:00Z", 300),
		at("2026-09-24T16:00:00Z", 600),
		at("2026-09-25T16:00:00Z", 900),
	}

	// 最近一笔 9-25T16:00Z 落在北京时间 9-26，所以「今天」是 9-26。
	got := Analyze(rewards, beijingTime(2026, time.September, 26, 20))

	if !got.GrantedToday {
		t.Fatal("GrantedToday = false, want true")
	}
	if got.Streak != 3 {
		t.Fatalf("Streak = %d, want 3", got.Streak)
	}
}

// TestAnalyzeIgnoresOtherRewards 推荐/活动奖励不计入开工奖励，pending 也不算到账。
func TestAnalyzeIgnoresOtherRewards(t *testing.T) {
	rewards := []map[string]any{
		{"reward_type": "referral_inviter", "status": "pending", "points": 5000, "granted_at": "2026-09-28T02:00:00Z"},
		{"reward_type": "long_task_feedback", "status": "granted", "points": 100, "granted_at": "2026-09-28T02:00:00Z"},
		{"reward_type": "daily_login", "status": "pending", "points": 1500, "granted_at": "2026-09-28T02:00:00Z"},
	}

	got := Analyze(rewards, beijingTime(2026, time.September, 28, 13))

	if got.GrantedToday || got.GrantedDays != 0 || got.TotalGranted != 0 {
		t.Fatalf("Analyze = %+v, want all zero", got)
	}
}

// TestAnalyzeReportsNextGrantAtBeijingMidnight 下一笔按北京时间 0 点推算。
func TestAnalyzeReportsNextGrantAtBeijingMidnight(t *testing.T) {
	got := Analyze(nil, beijingTime(2026, time.September, 28, 13))

	want := time.Date(2026, time.September, 29, 0, 0, 0, 0, beijing).Format(time.RFC3339)
	if got.NextExpectedAt != want {
		t.Fatalf("NextExpectedAt = %q, want %q", got.NextExpectedAt, want)
	}
	if got.Today != "2026-09-28" {
		t.Fatalf("Today = %q, want 2026-09-28", got.Today)
	}
}

// TestAnalyzeSumsDuplicateGrantsOnSameDay 同一天出现多条记录时合并成一天。
func TestAnalyzeSumsDuplicateGrantsOnSameDay(t *testing.T) {
	// 补录（北京时间 9-28 08:05）与正常发放（北京时间 9-28 00:00）落在同一业务日。
	rewards := []map[string]any{
		at("2026-09-28T00:05:00Z", 600),
		at("2026-09-27T16:00:00Z", 900),
	}

	got := Analyze(rewards, beijingTime(2026, time.September, 28, 13))

	if got.TodayPoints != 1500 {
		t.Fatalf("TodayPoints = %v, want 1500", got.TodayPoints)
	}
	if got.GrantedDays != 1 {
		t.Fatalf("GrantedDays = %d, want 1", got.GrantedDays)
	}
	if got.TotalGranted != 1500 {
		t.Fatalf("TotalGranted = %v, want 1500", got.TotalGranted)
	}
}
