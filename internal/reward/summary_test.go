package reward

import (
	"encoding/json"
	"testing"
	"time"
)

// measuredSummary 是实测抓到的 activities/summary 响应（已删去无关字段）。
const measuredSummary = `{
  "daily": {
    "status": "granted_today",
    "server_date": "2026-09-29",
    "points": 1500,
    "streak_day": 5,
    "next_streak_day": 5,
    "next_points": 1500,
    "next_eligible_at": "2026-09-29T16:00:00.000Z"
  },
  "credits": {"available": 17690},
  "feature_flags": {"daily_enabled": true}
}`

func decodeSummary(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	return m
}

// TestParseSummary 解析上游权威状态，含阶梯下一档与钱包余额。
func TestParseSummary(t *testing.T) {
	got, ok := ParseSummary(decodeSummary(t, measuredSummary))
	if !ok {
		t.Fatal("ParseSummary 未识别 daily 段")
	}
	if got.Status != "granted_today" || got.ServerDate != "2026-09-29" {
		t.Fatalf("status/server_date 解析错误: %+v", got)
	}
	if got.Points != 1500 || got.NextPoints != 1500 || got.StreakDay != 5 {
		t.Fatalf("积分/连续天数解析错误: %+v", got)
	}
	if got.NextEligibleAt != "2026-09-29T16:00:00.000Z" {
		t.Fatalf("next_eligible_at 解析错误: %q", got.NextEligibleAt)
	}
	if !got.Enabled || !got.HasAvailable || got.Available != 17690 {
		t.Fatalf("feature_flags/credits 解析错误: %+v", got)
	}
}

// TestParseSummaryWithoutDaily 没有 daily 段时报告未识别，调用方据此跳过。
func TestParseSummaryWithoutDaily(t *testing.T) {
	if _, ok := ParseSummary(map[string]any{"credits": map[string]any{"available": 1.0}}); ok {
		t.Fatal("缺少 daily 段时应返回 false")
	}
}

// TestMergeSummaryOverridesLedger 上游权威口径覆盖台账推算值。
func TestMergeSummaryOverridesLedger(t *testing.T) {
	rewards := []map[string]any{
		at("2026-09-26T16:00:00Z", 1200), // 北京时间 9/27
		at("2026-09-27T16:00:00Z", 1500), // 北京时间 9/28
	}
	ledger := Analyze(rewards, beijingTime(2026, time.September, 29, 9))
	if ledger.GrantedToday {
		t.Fatalf("台账只有到 9/28 的批次，9/29 不应算已到账: %+v", ledger)
	}

	state, _ := ParseSummary(decodeSummary(t, measuredSummary))
	got := ledger.MergeSummary(state)
	if !got.GrantedToday || got.TodayPoints != 1500 {
		t.Fatalf("summary 显示已到账，合并后应为已到账 +1500: %+v", got)
	}
	if got.Streak != 5 || got.NextPoints != 1500 || got.Source != "summary" {
		t.Fatalf("合并后字段错误: %+v", got)
	}
	// 台账里的历史累计不应被 summary 抹掉。
	if got.TotalGranted != 2700 || got.GrantedDays != 2 {
		t.Fatalf("台账统计被覆盖: %+v", got)
	}
}

// TestMergeSummaryKeepsLedgerWhenNotGranted 上游说还没发时不虚报已到账。
func TestMergeSummaryKeepsLedgerWhenNotGranted(t *testing.T) {
	ledger := Analyze(nil, beijingTime(2026, time.September, 30, 0))
	got := ledger.MergeSummary(Summary{
		Status:     "claimable",
		ServerDate: "2026-09-30",
		StreakDay:  5,
		NextPoints: 1500,
	})
	if got.GrantedToday || got.TodayPoints != 0 {
		t.Fatalf("未发放不应显示已到账: %+v", got)
	}
	if got.Today != "2026-09-30" || got.NextPoints != 1500 {
		t.Fatalf("业务日/下一档未同步: %+v", got)
	}
}

// TestBeijingDate 业务日按 UTC+8 划分：UTC 16:00 已经进入次日。
func TestBeijingDate(t *testing.T) {
	utc := time.Date(2026, time.September, 29, 16, 0, 0, 0, time.UTC)
	if got := BeijingDate(utc); got != "2026-09-30" {
		t.Fatalf("BeijingDate = %s, want 2026-09-30", got)
	}
}
