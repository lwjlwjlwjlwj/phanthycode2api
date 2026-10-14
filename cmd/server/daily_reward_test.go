package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/logx"
	"phanthycode2api/internal/reward"
	"phanthycode2api/internal/upstream"
)

// captureLog 把标准日志重定向到缓冲区，便于断言提示文案。
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prevWriter, prevFlags := log.Writer(), log.Flags()
	prevLevel := logx.Current()
	log.SetOutput(buf)
	log.SetFlags(0)
	logx.SetLevel(logx.LevelInfo)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
		logx.SetLevel(prevLevel)
	})
	return buf
}

// TestNotifyDailyRewardLogsOncePerDay 到账当天只提示一次，次日到账再提示一次。
func TestNotifyDailyRewardLogsOncePerDay(t *testing.T) {
	buf := captureLog(t)
	account := &auth.Auth{UID: "phanthy-notify-1", Nickname: "phanthy-0928"}
	today := reward.Daily{Today: "2026-09-28", GrantedToday: true, TodayPoints: 1500, Streak: 3, TotalGranted: 3000}

	notifyDailyReward(account, today)
	notifyDailyReward(account, today)

	out := buf.String()
	if got := strings.Count(out, "开工奖励"); got != 1 {
		t.Fatalf("同一业务日提示 %d 次，want 1；输出：%q", got, out)
	}
	if !strings.Contains(out, "已到账 +1500 积分") {
		t.Fatalf("提示缺少积分信息：%q", out)
	}
	if !strings.Contains(out, "phanthy-0928") || !strings.Contains(out, "2026-09-28") {
		t.Fatalf("提示缺少昵称或业务日：%q", out)
	}

	buf.Reset()
	notifyDailyReward(account, reward.Daily{Today: "2026-09-29"})
	if buf.Len() != 0 {
		t.Fatalf("未到账不应提示，实际：%q", buf.String())
	}

	notifyDailyReward(account, reward.Daily{Today: "2026-09-29", GrantedToday: true, TodayPoints: 1500, Streak: 4})
	if buf.Len() == 0 {
		t.Fatal("次日到账应当提示")
	}
}

// TestNotifyDailyRewardSkipsIncomplete 缺少业务日或未到账时不提示。
func TestNotifyDailyRewardSkipsIncomplete(t *testing.T) {
	buf := captureLog(t)
	account := &auth.Auth{UID: "phanthy-notify-2", Nickname: "phanthy-0929"}

	notifyDailyReward(account, reward.Daily{GrantedToday: true, TodayPoints: 1500})
	notifyDailyReward(account, reward.Daily{Today: "2026-09-29"})

	if buf.Len() != 0 {
		t.Fatalf("不应提示，实际：%q", buf.String())
	}
}

// summaryStub 起一个假上游，只应答 activities/summary。
func summaryStub(t *testing.T, body string, status int) (*upstream.Client, *bool) {
	t.Helper()
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/activities/summary" {
			http.NotFound(w, r)
			return
		}
		hit = true
		if r.Header.Get("x-desktop-signature") == "" || r.Header.Get("x-desktop-installation-id") == "" {
			t.Errorf("activities/summary 缺少签名头: %v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	client := upstream.New(srv.URL)
	client.DesktopKeyDir = t.TempDir()
	return client, &hit
}

// TestAttachDailyPrefersSummary 管理台口径：钱包余额取上游 credits.available，
// 今日状态取 activities/summary；台账为空也不写成「暂无记录」。
func TestAttachDailyPrefersSummary(t *testing.T) {
	const body = `{"daily":{"status":"granted_today","server_date":"2026-09-29","points":1500,` +
		`"streak_day":5,"next_points":1500,"next_eligible_at":"2026-09-29T16:00:00.000Z"},` +
		`"credits":{"available":17690},"feature_flags":{"daily_enabled":true}}`
	client, hit := summaryStub(t, body, http.StatusOK)
	account := &auth.Auth{UID: "phanthy-attach-1", Nickname: "phanthy-0929"}

	buf := captureLog(t)
	data := map[string]any{}
	attachDaily(client, account, data, nil, false)

	if !*hit {
		t.Fatal("未请求 activities/summary")
	}
	if got := data["credits_available"]; got != float64(17690) {
		t.Fatalf("credits_available = %#v, want 17690", got)
	}
	daily, ok := data["daily"].(reward.Daily)
	if !ok {
		t.Fatalf("daily 未写入: %#v", data)
	}
	if !daily.GrantedToday || daily.TodayPoints != 1500 || daily.Streak != 5 || daily.Source != "summary" {
		t.Fatalf("daily 口径不对: %+v", daily)
	}
	if !strings.Contains(buf.String(), "已到账 +1500 积分") {
		t.Fatalf("应输出到账提示：%q", buf.String())
	}
}

// TestAttachDailyRecordsSummaryError 拿不到 summary 时只记错误，
// 不能凭空造一条「暂无记录」的台账结果。
func TestAttachDailyRecordsSummaryError(t *testing.T) {
	client, _ := summaryStub(t, `{"error":{"code":"expired_access_token"}}`, http.StatusUnauthorized)
	account := &auth.Auth{UID: "phanthy-attach-2", Nickname: "phanthy-0929"}

	buf := captureLog(t)
	data := map[string]any{}
	attachDaily(client, account, data, nil, false)

	if _, ok := data["daily"]; ok {
		t.Fatalf("summary 失败且无台账时不应写入 daily: %#v", data)
	}
	if msg, _ := data["daily_summary_error"].(string); !strings.Contains(msg, "401") {
		t.Fatalf("daily_summary_error = %q, want 含 401", msg)
	}
	if strings.Contains(buf.String(), "开工奖励") {
		t.Fatalf("未到账不应提示开工奖励，实际：%q", buf.String())
	}
}
