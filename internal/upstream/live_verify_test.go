//go:build live

// 手工联调实测脚本（默认不参与构建）：验证上游对未授权模型确实返回
// HTTP 200 + SSE error 事件，且本包能把它识别成 StreamError。
//
//	P2A_LIVE_AUTHS=<auths 目录> go test -tags live ./internal/upstream -run TestLive -v -timeout 300s
//
// 只读凭证、不做 refresh（refresh 会轮换 refresh_token，影响正在运行的实例）。
package upstream

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"phanthycode2api/internal/auth"
)

func TestLive_ModelDeniedIsSurfaced(t *testing.T) {
	dir := os.Getenv("P2A_LIVE_AUTHS")
	if dir == "" {
		t.Skip("未设置 P2A_LIVE_AUTHS")
	}
	accts, err := auth.LoadDir(dir)
	if err != nil || len(accts) == 0 {
		t.Fatalf("LoadDir(%s): %v (n=%d)", dir, err, len(accts))
	}
	cli := New("https://code.phanthy.com")
	acct := accts[0]
	t.Logf("account uid=%s api_key=%v access_token=%v expires_in=%s",
		acct.UID, acct.APIKey != "", acct.AccessToken != "",
		time.Until(time.Unix(acct.ExpiresAt, 0)).Round(time.Second))

	for _, model := range []string{"deepseek-v4.1-flash", "phanthy-pro"} {
		t.Run(model, func(t *testing.T) {
			openaiBody := `{"model":"` + model + `","max_tokens":32,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
			anthro := PrepareBody([]byte(openaiBody), ThinkingOption{Mode: ThinkingOff})

			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()

			rc, status, respBody, err := cli.ChatStream(ctx, acct, anthro)
			if err != nil {
				t.Fatalf("ChatStream: %v", err)
			}
			t.Logf("HTTP status = %d", status)
			if status >= 400 {
				t.Logf("上游直接返回 HTTP 错误：%s", truncate(string(respBody), 300))
				return
			}
			defer rc.Close()

			resp, aerr := Aggregate(rc, model)
			if aerr != nil {
				var se *StreamError
				if !errors.As(aerr, &se) {
					t.Fatalf("Aggregate err = %T %v, want *StreamError", aerr, aerr)
				}
				t.Logf("识别为 StreamError: kind=%s code=%s committed=%v msg=%q",
					se.Kind, se.Event.Code, se.Committed, truncate(se.Event.Message, 160))
				return
			}
			in, out := UsageTotals(resp)
			if in == 0 && out == 0 {
				t.Errorf("静默成功但 tokens=0/0：这正是要修的 bug")
			}
			t.Logf("正常返回 tokens=%d/%d", in, out)
		})
	}
}
