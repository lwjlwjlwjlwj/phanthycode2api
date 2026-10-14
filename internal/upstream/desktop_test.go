package upstream

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"phanthycode2api/internal/auth"
)

// 从官方桌面端实测抓包得到的固定向量，用于锁定安装标识派生算法。
// 上一轮尝试过 SPKI / hex / UUID 等派生方式，服务端一律不认，只有
// 「raw 公钥的 sha256」才能对上，所以这里做回归测试。
const (
	knownSeed = "ABB944288E7DB56FDD184CFBDC063F64EEE40D54E160B26800EF389B0B897528"
	knownSPKI = "MCowBQYDK2VwAyEA39URCRYgVrmewQTe6qZzVwPa_4V86zfZAqTLvb7Oflw"
	knownID   = "di_fTnh4RH5p_fBDePi-mawH4ptXGsVVsV3UyLlyI5Mlro"
)

func knownIdentity(t *testing.T) *DesktopIdentity {
	t.Helper()
	seed, err := hex.DecodeString(knownSeed)
	if err != nil {
		t.Fatalf("decode seed: %v", err)
	}
	return newDesktopIdentity(ed25519.NewKeyFromSeed(seed))
}

func TestInstallationIDMatchesUpstream(t *testing.T) {
	id := knownIdentity(t)
	if id.ID != knownID {
		t.Fatalf("installation id = %s, want %s", id.ID, knownID)
	}
	spki, err := id.publicKeySPKI()
	if err != nil {
		t.Fatalf("publicKeySPKI: %v", err)
	}
	if spki != knownSPKI {
		t.Fatalf("public_key = %s, want %s", spki, knownSPKI)
	}
}

// TestSignaturePayload 独立复算 7 段 payload 并用公钥验签，锁定格式与字段顺序。
func TestSignaturePayload(t *testing.T) {
	id := knownIdentity(t)
	pub := id.key.Public().(ed25519.PublicKey)
	const ts, nonce = "1700000000000", "bm9uY2U"
	body := []byte(`{"public_key":"x"}`)
	idem := "register:" + knownID

	verify := func(signature, want string) {
		t.Helper()
		raw, err := base64.RawURLEncoding.DecodeString(signature)
		if err != nil {
			t.Fatalf("decode signature: %v", err)
		}
		if !ed25519.Verify(pub, []byte(want), raw) {
			t.Fatalf("签名与预期 payload 不匹配：%q", want)
		}
	}

	sum := sha256.Sum256(body)
	verify(id.sign(http.MethodPost, pathDesktopRegister, ts, nonce, idem, body), strings.Join([]string{
		"v1", http.MethodPost, pathDesktopRegister, ts, nonce,
		base64.RawURLEncoding.EncodeToString(sum[:]), idem,
	}, "\n"))

	// GET 请求没有 body，摘要按空串计算，idempotency-key 段留空。
	empty := sha256.Sum256(nil)
	verify(id.sign(http.MethodGet, pathActivitySummary, ts, nonce, "", nil), strings.Join([]string{
		"v1", http.MethodGet, pathActivitySummary, ts, nonce,
		base64.RawURLEncoding.EncodeToString(empty[:]), "",
	}, "\n"))
}

// TestLoadOrCreateDesktopIdentity 身份落盘后可复用，不会每次启动都换设备。
func TestLoadOrCreateDesktopIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "desktop-key.json")
	first, err := LoadOrCreateDesktopIdentity(path)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := LoadOrCreateDesktopIdentity(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("重新加载得到不同身份：%s vs %s", first.ID, second.ID)
	}
	if !strings.HasPrefix(first.ID, "di_") {
		t.Fatalf("installation id 前缀错误：%s", first.ID)
	}
}

// TestDesktopIdentityIsPerAccount 每个账号必须拿到不同的安装身份：
// 上游的 installation id 全局唯一，共用一份身份时第二个账号会被判
// desktop_installation_required（实测 401），拿不到 activities/summary。
func TestDesktopIdentityIsPerAccount(t *testing.T) {
	cli := New("http://127.0.0.1:1")
	cli.DesktopKeyDir = t.TempDir()

	first, err := cli.DesktopIdentity("phanthy-1001")
	if err != nil {
		t.Fatalf("DesktopIdentity(1001): %v", err)
	}
	second, err := cli.DesktopIdentity("phanthy-1002")
	if err != nil {
		t.Fatalf("DesktopIdentity(1002): %v", err)
	}
	if first.ID == second.ID {
		t.Fatalf("两个账号拿到同一个安装身份：%s", first.ID)
	}
	again, err := cli.DesktopIdentity("phanthy-1001")
	if err != nil {
		t.Fatalf("DesktopIdentity(1001) 二次: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("同一账号换了身份：%s → %s", first.ID, again.ID)
	}
	// 落盘文件名必须能安全承载上游返回的 uid。
	if _, err := cli.DesktopIdentity("../escape/../x y"); err != nil {
		t.Fatalf("异常 uid 不应报错: %v", err)
	}
	entries, err := os.ReadDir(cli.DesktopKeyDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("落盘文件数 = %d, want 3", len(entries))
	}
}

// TestClaimDailyLoginSendsSignedRequest 校验领取请求的路径、幂等键与签名头。
func TestClaimDailyLoginSendsSignedRequest(t *testing.T) {
	var got struct {
		path, idem, installID, ts, nonce, sig, beta, auth string
		body                                              []byte
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.idem = r.Header.Get("idempotency-key")
		got.installID = r.Header.Get("x-desktop-installation-id")
		got.ts = r.Header.Get("x-desktop-timestamp")
		got.nonce = r.Header.Get("x-desktop-nonce")
		got.sig = r.Header.Get("x-desktop-signature")
		got.beta = r.Header.Get("anthropic-beta")
		got.auth = r.Header.Get("Authorization")
		got.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"already_granted","points":1500}`))
	}))
	defer srv.Close()

	cli := New(srv.URL)
	cli.DesktopKeyDir = t.TempDir()
	id, err := cli.DesktopIdentity("u1")
	if err != nil {
		t.Fatalf("DesktopIdentity: %v", err)
	}
	account := &auth.Auth{UID: "u1", AccessToken: "tok"}
	result, err := cli.ClaimDailyLogin(account, id, "2026-09-29")
	if err != nil {
		t.Fatalf("ClaimDailyLogin: %v", err)
	}
	if result["status"] != "already_granted" {
		t.Fatalf("status = %v, want already_granted", result["status"])
	}
	if got.path != pathClaimDailyLogin {
		t.Fatalf("path = %s, want %s", got.path, pathClaimDailyLogin)
	}
	if want := "daily:" + id.ID + ":2026-09-29"; got.idem != want {
		t.Fatalf("idempotency-key = %s, want %s", got.idem, want)
	}
	if got.installID != id.ID {
		t.Fatalf("x-desktop-installation-id = %s, want %s", got.installID, id.ID)
	}
	if got.beta != "oauth-2025-04-20" || got.auth != "Bearer tok" {
		t.Fatalf("鉴权头错误：beta=%q auth=%q", got.beta, got.auth)
	}
	if string(got.body) != "{}" {
		t.Fatalf("body = %q, want {}", got.body)
	}
	sum := sha256.Sum256(got.body)
	payload := strings.Join([]string{
		"v1", http.MethodPost, pathClaimDailyLogin, got.ts, got.nonce,
		base64.RawURLEncoding.EncodeToString(sum[:]), got.idem,
	}, "\n")
	sig, err := base64.RawURLEncoding.DecodeString(got.sig)
	if err != nil {
		t.Fatalf("decode x-desktop-signature: %v", err)
	}
	if !ed25519.Verify(id.key.Public().(ed25519.PublicKey), []byte(payload), sig) {
		t.Fatalf("签名校验失败，payload=%q", payload)
	}
}

// TestRegisterDesktopInstallationTreats409AsSuccess 已登记过不应被当成错误。
func TestRegisterDesktopInstallationTreats409AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"conflict","message":"already registered"}}`))
	}))
	defer srv.Close()

	cli := New(srv.URL)
	cli.DesktopKeyDir = t.TempDir()
	id, err := cli.DesktopIdentity("u1")
	if err != nil {
		t.Fatalf("DesktopIdentity: %v", err)
	}
	if err := cli.RegisterDesktopInstallation(&auth.Auth{UID: "u1", AccessToken: "tok"}, id); err != nil {
		t.Fatalf("409 应视为幂等成功，实际: %v", err)
	}
}
