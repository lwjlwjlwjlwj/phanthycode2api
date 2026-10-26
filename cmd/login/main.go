// Command login 实现 PhanthyCode 的 PKCE 半自动登录：先生成授权链接，再用授权码换 token。
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"phanthycode2api/internal/credential"
	"phanthycode2api/internal/phanthy"
)

func main() {
	step := flag.String("step", "url", "步骤：url（生成授权链接）或 exchange（用授权码换 token）")
	code := flag.String("code", "", "授权码；redirect_uri 查询串中的 code 参数")
	state := flag.String("state", "", "回调返回的 state，用于校验本次登录流程")
	authDir := flag.String("auth-dir", envOr("P2A_AUTH_DIR", "./auths"), "凭证输出目录")
	baseURL := flag.String("base-url", envOr("P2A_BASE_URL", "https://code.phanthy.com"), "上游地址")
	metaFile := flag.String("meta-file", ".login-verifier", "PKCE verifier/state 临时文件（0600）")
	flag.Parse()

	switch strings.ToLower(strings.TrimSpace(*step)) {
	case "url":
		if err := stepURL(*baseURL, *metaFile); err != nil {
			log.Fatalf("生成授权链接失败: %v", err)
		}
	case "exchange":
		if strings.TrimSpace(*code) == "" {
			log.Fatalf("缺少 -code 参数")
		}
		if err := stepExchange(*baseURL, *authDir, *metaFile, *code, *state); err != nil {
			log.Fatalf("换取凭证失败: %v", err)
		}
	default:
		log.Fatalf("未知 step: %q（可选 url / exchange）", *step)
	}
}

// loginMeta 是两次运行之间传递的 PKCE 参数。
type loginMeta struct {
	Verifier string `json:"verifier"`
	State    string `json:"state"`
}

func stepURL(baseURL, metaFile string) error {
	verifier := randomURLSafe(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomURLSafe(16)

	meta := loginMeta{Verifier: verifier, State: state}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(metaFile, raw, 0o600); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", metaFile, err)
	}

	query := url.Values{
		"client_id":             {phanthy.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {phanthy.RedirectURI},
		"scope":                 {strings.Join(phanthy.Scopes, " ")},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	fmt.Println("在浏览器中打开以下链接并完成登录授权：")
	fmt.Printf("\n%s/oauth/authorize?%s\n\n", strings.TrimRight(baseURL, "/"), query.Encode())
	fmt.Println("授权后浏览器会跳转到 redirect_uri，从地址栏复制 code 与 state，然后执行：")
	fmt.Printf("  go run ./cmd/login -step=exchange -code=<code> -state=%s\n", state)
	return nil
}

func stepExchange(baseURL, authDir, metaFile, code, gotState string) error {
	raw, err := os.ReadFile(metaFile)
	if err != nil {
		return fmt.Errorf("读取 %s 失败（请先执行 -step=url）: %w", metaFile, err)
	}
	var meta loginMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("解析 %s 失败: %w", metaFile, err)
	}
	if meta.Verifier == "" {
		return fmt.Errorf("%s 中缺少 verifier", metaFile)
	}
	// 校验 state，避免把别处的授权码贴进本次流程（证据：02-usage-semantics.md §1.2）。
	if meta.State == "" {
		return fmt.Errorf("%s 中缺少 state，请重新执行 -step=url", metaFile)
	}
	if gotState == "" {
		return fmt.Errorf("缺少 -state 参数（从回调地址复制）")
	}
	if gotState != meta.State {
		return fmt.Errorf("state 不匹配：期望 %s，实际 %s", meta.State, gotState)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := phanthy.New(phanthy.Options{BaseURL: baseURL})
	token, err := client.ExchangeCode(ctx, strings.TrimSpace(code), meta.Verifier, "")
	if err != nil {
		return err
	}

	uid, nickname := "", ""
	if profile, err := client.Profile(ctx, token.AccessToken); err == nil {
		uid, nickname = extractProfile(profile)
	} else {
		log.Printf("提示: 获取账号资料失败（不影响凭证保存）: %v", err)
	}
	if uid == "" {
		uid = fallbackUID(token.AccessToken)
	}
	if nickname == "" {
		nickname = uid
	}

	cred := &credential.Credential{
		UID:          uid,
		Nickname:     nickname,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.Expiry(time.Now()),
	}
	path := filepath.Join(authDir, credential.FileName(uid))
	cred.SetPath(path)
	if err := cred.SaveAtomic(); err != nil {
		return err
	}

	_ = os.Remove(metaFile)

	fmt.Printf("凭证已保存: %s\n", path)
	fmt.Printf("账号: %s（%s）\n", nickname, uid)
	fmt.Println("重启服务即可使用该账号。")
	return nil
}

// extractProfile 防御性解析 profile 响应；成功 schema 未被验证。
func extractProfile(profile map[string]any) (uid, nickname string) {
	uid = lookupString(profile, "uid", "id", "sub", "account_uid", "user_id")
	nickname = lookupString(profile, "nickname", "name", "display_name", "username", "email")

	nested := []string{"account", "user", "profile", "data"}
	for _, key := range nested {
		child, ok := profile[key].(map[string]any)
		if !ok {
			continue
		}
		if uid == "" {
			uid = lookupString(child, "uid", "id", "uuid", "sub")
		}
		if nickname == "" {
			nickname = lookupString(child, "nickname", "name", "display_name", "username", "email")
		}
	}
	return uid, nickname
}

func lookupString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// fallbackUID 在既无 uid 又无 profile 时用 token 哈希派生稳定标识。
func fallbackUID(accessToken string) string {
	sum := sha256.Sum256([]byte(accessToken))
	return "acct-" + hex.EncodeToString(sum[:6])
}

func randomURLSafe(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("安全随机数生成失败: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
