// login 登录工具（两步模式）：生成 PKCE 授权链接 → 浏览器授权 →
// 获取 code → 交换 token + 获取 api_key → 落盘 auths/ 即生效。
//
// 两步模式：
//   step=url（默认）：生成授权 URL，打开浏览器，打印 verifier，退出
//   step=exchange -code=xxx：用 verifier 交换 token，获取 api_key，落盘
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	baseURL  = flag.String("base-url", "https://code.phanthy.com", "Phanthy base URL")
	outDir   = flag.String("out", "auths", "output directory for auth files")
	clientID = flag.String("client-id", "phanthy-code-cli", "OAuth client id")
	step     = flag.String("step", "url", "step: url or exchange")
	code     = flag.String("code", "", "authorization code (for step=exchange)")
	verifier = flag.String("verifier", "", "PKCE verifier from step=url output")
)

const verifierFile = ".login-verifier"

func main() {
	flag.Parse()

	switch *step {
	case "url":
		stepURL()
	case "exchange":
		stepExchange()
	default:
		fatal("未知 step: %s (可选: url, exchange)", *step)
	}
}

// stepURL 生成 PKCE verifier，输出授权 URL，打开浏览器，将 verifier 写入文件。
func stepURL() {
	v, err := randomString(48)
	if err != nil {
		fatal("generate verifier: %v", err)
	}
	sum := sha256.Sum256([]byte(v))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authURL := buildAuthorizeURL(*baseURL, *clientID, challenge)
	fmt.Printf("授权 URL（请在浏览器中打开）：\n\n   %s\n\n", authURL)
	openBrowser(authURL)

	// 保存 verifier 供下一步使用
	if err := os.WriteFile(verifierFile, []byte(v), 0o600); err != nil {
		fatal("保存 verifier 失败: %v", err)
	}
	fmt.Printf("Verifier 已保存到 %s\n", verifierFile)
	fmt.Println()
	fmt.Println("请在浏览器中完成授权。")
	fmt.Println("授权成功后，页面会跳转到 https://code.phanthy.com/oauth/code/success")
	fmt.Println("页面上会显示授权码，请复制该 code 值，然后运行：")
	fmt.Printf("   go run ./cmd/login -step=exchange -code=<复制的code>\n")
}

// stepExchange 用 code + verifier 交换 token 并保存凭证。
func stepExchange() {
	if *code == "" {
		fatal("请提供 -code 参数")
	}
	v := *verifier
	if v == "" {
		raw, err := os.ReadFile(verifierFile)
		if err != nil {
			fatal("读取 verifier 文件失败 (请先运行 step=url): %v", err)
		}
		v = strings.TrimSpace(string(raw))
	}
	if v == "" {
		fatal("verifier 为空")
	}

	// 交换 token
	tr, err := exchangeToken(*code, v)
	if err != nil {
		fatal("token 交换失败: %v\n提示：若为 invalid_grant，说明 code 已过期，请重新运行 step=url", err)
	}

	// 获取 api_key（prod 上游该端点固定 404，官方 CLI 同样静默忽略，用 access_token 兜底）
	fmt.Println("获取 api_key ...")
	apiKey, err := createAPIKey(tr.AccessToken)
	if err != nil {
		fmt.Printf("  创建 api_key 失败（服务启动时会自动补齐）: %v\n", err)
	} else if apiKey == "" {
		fmt.Println("  api_key 暂不可用（上游返回 404），将使用 access_token 兜底，不影响使用")
	}

	// 落盘
	uid := fmt.Sprintf("phanthy-%d", time.Now().Unix())
	expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second).Unix()
	doc := map[string]any{
		"auth": map[string]any{
			"accessToken":  tr.AccessToken,
			"refreshToken": tr.RefreshToken,
			"expiresAt":    expiresAt,
		},
		"account": map[string]any{
			"uid":      uid,
			"nickname": "phanthy-" + time.Now().Format("0102"),
		},
		"api_key": apiKey,
	}
	_ = os.MkdirAll(*outDir, 0o755)
	out := filepath.Join(*outDir, uid+".json")
	raw, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(out, raw, 0o600); err != nil {
		fatal("写文件失败: %v", err)
	}
	fmt.Printf("\n登录成功！凭证已保存: %s\n", out)
	fmt.Println("重启 phanthycode2api 服务即生效。")

	// 清理 verifier 文件
	os.Remove(verifierFile)
}

// ---------------- 工具函数 ----------------

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func buildAuthorizeURL(base, clientID, challenge string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", "https://code.phanthy.com/oauth/code/success")
	q.Set("scope", "user:inference user:profile user:sessions:claude_code")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", "p2a-login")
	return strings.TrimRight(base, "/") + "/oauth/authorize?" + q.Encode()
}

// extractCode 从用户输入中提取 code（支持完整 URL 或裸 code）。
func extractCode(s string) string {
	if u, err := url.Parse(s); err == nil && u.Scheme != "" {
		return u.Query().Get("code")
	}
	// 可能是 "code=xxx" 或裸 code
	if i := strings.Index(s, "code="); i >= 0 {
		s = s[i+5:]
		if j := strings.IndexAny(s, "& "); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func exchangeToken(code, verifier string) (*tokenResp, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", "https://code.phanthy.com/oauth/code/success")
	form.Set("client_id", *clientID)

	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(*baseURL, "/")+"/oauth/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		raw = append(raw, buf[:n]...)
		if err != nil {
			break
		}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, trunc(string(raw), 200))
	}
	var tr tokenResp
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("parse token response: %v", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("no access_token in response: %s", trunc(string(raw), 200))
	}
	return &tr, nil
}

func createAPIKey(accessToken string) (string, error) {
	req, err := http.NewRequest(http.MethodPost,
		strings.TrimRight(*baseURL, "/")+"/api/oauth/phanthy_cli/create_api_key", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		raw = append(raw, buf[:n]...)
		if err != nil {
			break
		}
	}
	if resp.StatusCode >= 400 {
		// 上游 create_api_key 端点在 prod 固定返回 404（官方 CLI 亦如此），
		// 用 access_token 兜底即可，静默忽略，不打印错误。
		if resp.StatusCode == http.StatusNotFound {
			return "", nil
		}
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, trunc(string(raw), 200))
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	// 对齐官方 CLI：api_key 实际在 data.raw_key 字段
	if d, ok := m["data"].(map[string]any); ok {
		for _, k := range []string{"raw_key", "api_key", "apiKey"} {
			if v, ok := d[k].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	// 兜底：直接在顶层查找
	for _, k := range []string{"api_key", "apiKey", "raw_key"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("no api_key in response: %s", trunc(string(raw), 200))
}

func openBrowser(u string) {
	if os.Getenv("OS") != "" && strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
		_ = exec.Command("cmd", "/c", "start", "", u).Start()
		return
	}
	_ = exec.Command("open", u).Start()
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}