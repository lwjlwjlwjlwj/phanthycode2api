package scheduler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/upstream"
)

// TestNextDailyClaimUsesBeijingMidnight 领取时刻按北京时间计算，
// 与上游划分业务日的时区保持一致。
func TestNextDailyClaimUsesBeijingMidnight(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "北京时间当天 0 点前 → 当天",
			now:  time.Date(2026, time.September, 29, 13, 0, 0, 0, time.UTC), // 北京 9/29 21:00
			want: "2026-09-30T00:05:00+08:00",
		},
		{
			name: "北京时间 0 点已过 → 次日",
			now:  time.Date(2026, time.September, 30, 0, 6, 0, 0, beijing),
			want: "2026-10-01T00:05:00+08:00",
		},
		{
			name: "UTC 16:00 正好跨日",
			now:  time.Date(2026, time.September, 29, 15, 59, 0, 0, time.UTC),
			want: "2026-09-30T00:05:00+08:00",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextDailyClaim(tc.now, 0, 5).In(beijing).Format(time.RFC3339)
			if got != tc.want {
				t.Fatalf("nextDailyClaim = %s, want %s", got, tc.want)
			}
			if !nextDailyClaim(tc.now, 0, 5).After(tc.now) {
				t.Fatalf("触发时刻必须晚于 now")
			}
		})
	}
}

// TestNextFireKeepsKeepaliveBehaviour 保留原有 keepalive 整点逻辑。
func TestNextFireKeepsKeepaliveBehaviour(t *testing.T) {
	now := time.Date(2026, time.September, 29, 23, 30, 0, 0, time.UTC)
	got := nextFire(now, []int{22})
	if got.Hour() != 22 || !got.After(now) {
		t.Fatalf("nextFire = %s, want 次日 22:00", got)
	}
	if got.Day() != 30 {
		t.Fatalf("22 点已过应顺延到次日，实际 %s", got)
	}
}

// fakeUpstream 起一个只实现 /oauth/token 的假上游，返回刷新调用次数。
// status 非 200 时模拟刷新失败。
func fakeUpstream(t *testing.T, status int) (*upstream.Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&calls, 1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_grant","message":"Authorization code is invalid or expired."}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    1800,
		})
	}))
	t.Cleanup(srv.Close)
	return upstream.New(srv.URL), &calls
}

// testAccount 造一个可落盘的账号；预置 api_key 以跳过 EnsureAPIKey 的网络调用。
func testAccount(t *testing.T, uid string, ttl time.Duration) *auth.Auth {
	t.Helper()
	return &auth.Auth{
		UID:          uid,
		Nickname:     uid,
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		ExpiresAt:    time.Now().Add(ttl).Unix(),
		APIKey:       "sk-ant-existing",
		FilePath:     filepath.Join(t.TempDir(), uid+".json"),
	}
}

// TestRunKeepaliveRefreshesExpiringToken 剩余寿命进入提前量时自动刷新并记账。
func TestRunKeepaliveRefreshesExpiringToken(t *testing.T) {
	up, calls := fakeUpstream(t, http.StatusOK)
	a := testAccount(t, "u1", time.Minute) // 1 分钟后过期，落在 15m 提前量内
	p := pool.New("")
	p.Add(a)
	s := New(Config{Pool: p, Upstream: up, KeepaliveSkew: 15 * time.Minute, AutoRecover: true})

	s.runKeepalive(false)

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if a.AccessToken != "new-access-token" {
		t.Errorf("access_token = %q, want refreshed", a.AccessToken)
	}
	st, _ := p.Status("u1")
	if st.LastKeepaliveAt.IsZero() {
		t.Error("保活成功后应写入 LastKeepaliveAt")
	}
}

// TestRunKeepaliveSkipsFreshToken token 还很久才过期时不做无谓刷新。
func TestRunKeepaliveSkipsFreshToken(t *testing.T) {
	up, calls := fakeUpstream(t, http.StatusOK)
	a := testAccount(t, "u1", time.Hour)
	p := pool.New("")
	p.Add(a)
	s := New(Config{Pool: p, Upstream: up, KeepaliveSkew: 15 * time.Minute, AutoRecover: true})

	s.runKeepalive(false)

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("refresh calls = %d, want 0（token 未接近过期）", got)
	}
	if a.AccessToken != "old-access-token" {
		t.Errorf("access_token 不该被改写，实际 %q", a.AccessToken)
	}
}

// TestRunKeepaliveNowForcesRefresh 手动/启动保活不看过期时间，直接换新 token。
func TestRunKeepaliveNowForcesRefresh(t *testing.T) {
	up, calls := fakeUpstream(t, http.StatusOK)
	a := testAccount(t, "u1", time.Hour)
	p := pool.New("")
	p.Add(a)
	s := New(Config{Pool: p, Upstream: up, KeepaliveSkew: 15 * time.Minute, AutoRecover: true})

	s.RunKeepaliveNow()

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
}

// TestRunKeepaliveRecoversDisabledAccount 自动恢复开启时，禁用账号也会重试并恢复。
func TestRunKeepaliveRecoversDisabledAccount(t *testing.T) {
	up, calls := fakeUpstream(t, http.StatusOK)
	a := testAccount(t, "u1", time.Hour) // token 未接近过期，只有主动重试才会刷新
	p := pool.New("")
	p.Add(a)
	p.Disable("u1", "session dead")
	s := New(Config{Pool: p, Upstream: up, KeepaliveSkew: 15 * time.Minute, AutoRecover: true})

	s.runKeepalive(false)

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("禁用账号也应重试刷新，calls = %d", got)
	}
	st, _ := p.Status("u1")
	if st.Disabled {
		t.Error("刷新成功后应自动重新启用")
	}
}

// TestRunKeepaliveLeavesDisabledAloneWhenAutoRecoverOff 关闭自动恢复后不碰禁用账号。
func TestRunKeepaliveLeavesDisabledAloneWhenAutoRecoverOff(t *testing.T) {
	up, calls := fakeUpstream(t, http.StatusOK)
	a := testAccount(t, "u1", time.Minute)
	p := pool.New("")
	p.Add(a)
	p.Disable("u1", "session dead")
	s := New(Config{Pool: p, Upstream: up, KeepaliveSkew: 15 * time.Minute, AutoRecover: false})

	s.runKeepalive(false)

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("关闭自动恢复时不该刷新禁用账号，calls = %d", got)
	}
	st, _ := p.Status("u1")
	if !st.Disabled {
		t.Error("禁用状态应保持不变")
	}
}

// TestRunKeepaliveDisablesOnDeadSession 刷新被上游判定会话失效时自动禁用。
func TestRunKeepaliveDisablesOnDeadSession(t *testing.T) {
	up, _ := fakeUpstream(t, http.StatusBadRequest)
	a := testAccount(t, "u1", time.Minute)
	p := pool.New("")
	p.Add(a)
	s := New(Config{Pool: p, Upstream: up, KeepaliveSkew: 15 * time.Minute, AutoRecover: true})

	s.runKeepalive(false)

	st, _ := p.Status("u1")
	if !st.Disabled {
		t.Error("invalid_grant 应把账号标记为禁用")
	}
}
