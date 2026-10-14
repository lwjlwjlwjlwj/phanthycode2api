//go:build live

// 手工联调实测脚本（默认不参与构建）：验证「每日开工奖励」的桌面端签名协议
// 在真实上游可用 —— 登记安装 → 查询状态 → 领取，三步都幂等。
//
//	P2A_LIVE_AUTHS=<auths 目录> go test -tags live ./internal/upstream -run TestLive_DailyReward -v -timeout 300s
//
// 可选 P2A_LIVE_DESKTOP_KEY_DIR 指定安装身份目录（默认写到临时目录，不污染工作区）。
// 只读凭证、不做 refresh（refresh 会轮换 refresh_token，影响正在运行的实例）。
package upstream

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"phanthycode2api/internal/auth"
)

func TestLive_DailyReward(t *testing.T) {
	dir := os.Getenv("P2A_LIVE_AUTHS")
	if dir == "" {
		t.Skip("未设置 P2A_LIVE_AUTHS")
	}
	accts, err := auth.LoadDir(dir)
	if err != nil || len(accts) == 0 {
		t.Fatalf("LoadDir(%s): %v (n=%d)", dir, err, len(accts))
	}

	keyDir := os.Getenv("P2A_LIVE_DESKTOP_KEY_DIR")
	if keyDir == "" {
		keyDir = filepath.Join(t.TempDir(), "desktop-keys")
	}

	cli := New("https://code.phanthy.com")
	cli.DesktopKeyDir = keyDir

	for _, acct := range accts {
		acct := acct
		t.Run(acct.UID, func(t *testing.T) {
			id, err := cli.DesktopIdentity(acct.UID)
			if err != nil {
				t.Fatalf("DesktopIdentity: %v", err)
			}
			t.Logf("installation id = %s", id.ID)
			if err := cli.RegisterDesktopInstallation(acct, id); err != nil {
				if deadSession(err) {
					t.Skipf("会话已失效，需重新登录: %v", err)
				}
				t.Fatalf("RegisterDesktopInstallation: %v", err)
			}
			t.Log("登记安装: ok")

			summary, err := cli.ActivitySummary(acct, id)
			if err != nil {
				t.Fatalf("ActivitySummary: %v", err)
			}
			t.Logf("activities/summary = %v", summary)

			daily, _ := summary["daily"].(map[string]any)
			if daily == nil {
				t.Skip("上游未返回 daily 段（该账号可能没有开工奖励资格）")
			}
			if status, _ := daily["status"].(string); status == "granted_today" {
				t.Logf("今日已到账 +%v 积分（连续 %v 天），跳过领取", daily["points"], daily["streak_day"])
				return
			}
			date, _ := daily["server_date"].(string)
			if date == "" {
				t.Fatal("summary 未返回 server_date，无法构造幂等键")
			}
			result, err := cli.ClaimDailyLogin(acct, id, date)
			if err != nil {
				t.Fatalf("ClaimDailyLogin: %v", err)
			}
			t.Logf("领取结果 = %v", result)
		})
	}
}

// deadSession 判断错误是否为「会话已失效」（access_token 过期且刷新失败）。
func deadSession(err error) bool {
	var ue *Error
	return errors.As(err, &ue) && ue.Status == 401
}
