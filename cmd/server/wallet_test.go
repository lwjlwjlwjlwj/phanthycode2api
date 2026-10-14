package main

import (
	"testing"
	"time"
)

// TestBuildWalletExpiredLotDoesNotAbsorbLaterUsage 固化「套餐」页的额度口径：
// 已过期的批次既不计入钱包总额，也只能抵扣它有效期内的消耗。
func TestBuildWalletExpiredLotDoesNotAbsorbLaterUsage(t *testing.T) {
	now := time.Now()
	reward := func(kind, status string, points float64, grantedDays, expiresDays int) map[string]any {
		return map[string]any{
			"reward_type": kind,
			"status":      status,
			"points":      points,
			"granted_at":  now.AddDate(0, 0, grantedDays).UTC().Format(time.RFC3339),
			"expires_at":  now.AddDate(0, 0, expiresDays).UTC().Format(time.RFC3339),
		}
	}
	rewards := []map[string]any{
		reward("daily_login", "granted", 100, -40, -10), // 已过期，只能吃掉它有效期内的 100
		reward("daily_login", "granted", 300, -20, 10),  // 有效
		reward("long_task_feedback", "granted", 50, -2, 28),
		reward("referral_inviter", "pending", 5000, 0, 0),
	}
	summary := map[string]any{
		"usage_by_day_and_model": []any{
			map[string]any{"date": now.AddDate(0, 0, -30).Format("2006-01-02"), "cost_points": 80.0},
			map[string]any{"date": now.AddDate(0, 0, -15).Format("2006-01-02"), "cost_points": 40.0},
			map[string]any{"date": now.AddDate(0, 0, -1).Format("2006-01-02"), "cost_points": 30.0},
		},
	}
	usage := map[string]any{
		"credits_remaining": 500.0,
		"credits_total":     500.0,
		"credits_used":      0.0,
		"credits_reset_at":  now.AddDate(0, 0, 20).UTC().Format(time.RFC3339),
	}

	pools, pending := buildWallet("体验版", usage, rewards, summary)
	if pending != 5000 {
		t.Fatalf("pending = %v, want 5000", pending)
	}
	byKey := map[string]walletPool{}
	for _, pool := range pools {
		byKey[pool.Key] = pool
	}
	if len(pools) != 3 {
		t.Fatalf("pools = %d (%v), want 3 (plan + 2 未过期奖励池)", len(pools), byKey)
	}
	if plan := byKey["plan"]; plan.Label != "体验版" || plan.Total != 500 || plan.Remaining != 500 {
		t.Fatalf("plan pool = %+v", plan)
	}
	// -30d 的 80 分落在过期批次上；-15d 的 40 分先扣它剩下的 20，溢出的 20 由有效批次承担；
	// -1d 的 30 分全部由有效批次承担。所以有效批次的已用是 50，而不是把三天 150 分全算上。
	if daily := byKey["daily_login"]; daily.Total != 300 || daily.Used != 50 || daily.Remaining != 250 {
		t.Fatalf("daily_login pool = %+v, want 300/50/250", daily)
	}
	if act := byKey["long_task_feedback"]; act.Total != 50 || act.Used != 0 || act.Remaining != 50 {
		t.Fatalf("activity pool = %+v, want 50/0/50", act)
	}
	for _, pool := range pools[1:] {
		if !pool.Estimate {
			t.Fatalf("reward pool %s 应标记为估算", pool.Key)
		}
	}
}

// TestBuildWalletKeepsPendingOutOfWallet 待发放奖励不进钱包，只作为 pending 返回。
func TestBuildWalletKeepsPendingOutOfWallet(t *testing.T) {
	rewards := []map[string]any{
		{"reward_type": "daily_login", "status": "pending", "points": 300.0},
	}
	pools, pending := buildWallet("", map[string]any{"credits_remaining": 10.0}, rewards, nil)
	if pending != 300 {
		t.Fatalf("pending = %v, want 300", pending)
	}
	if len(pools) != 1 || pools[0].Key != "plan" || pools[0].Label != "套餐额度" {
		t.Fatalf("pools = %+v", pools)
	}
}
