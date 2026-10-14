package upstream

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"phanthycode2api/internal/auth"
	"phanthycode2api/internal/logx"
)

// DesktopVersion 是登记桌面端安装时上报的客户端版本。
// 上游据此判断安装是否受支持，取值需与官方桌面端保持一致。
const DesktopVersion = "2.33.2"

// desktopEdition 是登记请求里的发行版标识（Phanthy Code 桌面端固定值）。
const desktopEdition = "phanthy_code"

// 上游的每日开工奖励接口挂在 /api/oauth 下，路径集中在此便于对照。
const (
	pathDesktopRegister = "/api/oauth/desktop-installations/register"
	pathActivitySummary = "/api/oauth/activities/summary"
	pathClaimDailyLogin = "/api/oauth/activities/daily-login/claim"
)

// DesktopIdentity 是「桌面端安装」身份：一对 Ed25519 密钥 + 由公钥派生的安装标识。
//
// 上游要求 activities / desktop-installations 系列接口携带签名头，
// 签名密钥即安装身份本身：同一份密钥重复登记是幂等的，换密钥等于换一台设备。
type DesktopIdentity struct {
	key ed25519.PrivateKey
	// ID 是上游要求的安装标识：di_ + b64u(sha256(raw 32 字节公钥))。
	// 注意必须对 raw 公钥求哈希，用 SPKI / hex 都会得到服务端不认的 id。
	ID string
}

// desktopKeyFile 是安装身份的落盘格式（仅存 seed，公钥可随时派生）。
type desktopKeyFile struct {
	Seed           string `json:"seed_hex"`
	InstallationID string `json:"installation_id"`
}

// InstallationID 由 raw Ed25519 公钥派生上游的安装标识。
func InstallationID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "di_" + base64.RawURLEncoding.EncodeToString(sum[:])
}

// LoadOrCreateDesktopIdentity 读取安装身份；文件不存在或损坏时生成一份并落盘。
// 身份只需生成一次，之后长期复用（换文件等于换一台设备）。
func LoadOrCreateDesktopIdentity(path string) (*DesktopIdentity, error) {
	if raw, err := os.ReadFile(path); err == nil {
		var f desktopKeyFile
		if json.Unmarshal(raw, &f) == nil {
			if seed, decErr := hex.DecodeString(strings.TrimSpace(f.Seed)); decErr == nil && len(seed) == ed25519.SeedSize {
				return newDesktopIdentity(ed25519.NewKeyFromSeed(seed)), nil
			}
		}
		logx.Debugf("desktop identity %s 无法解析，重新生成", path)
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate desktop key: %w", err)
	}
	id := newDesktopIdentity(key)
	if strings.TrimSpace(path) == "" {
		return id, nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create desktop key dir: %w", err)
		}
	}
	doc, err := json.MarshalIndent(desktopKeyFile{
		Seed:           hex.EncodeToString(key.Seed()),
		InstallationID: id.ID,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(doc, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("write desktop key: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, fmt.Errorf("save desktop key: %w", err)
	}
	logx.Infof("desktop identity 已生成：%s（%s）", id.ID, path)
	return id, nil
}

func newDesktopIdentity(key ed25519.PrivateKey) *DesktopIdentity {
	return &DesktopIdentity{key: key, ID: InstallationID(key.Public().(ed25519.PublicKey))}
}

// publicKeySPKI 返回 b64u(SPKI) 形态的公钥，即登记请求的 public_key 字段。
func (d *DesktopIdentity) publicKeySPKI() (string, error) {
	der, err := x509.MarshalPKIXPublicKey(d.key.Public())
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(der), nil
}

// sign 按上游约定生成签名：
//
//	v1\n{METHOD}\n{path}\n{ts}\n{nonce}\n{b64u(sha256(body))}\n{idempotencyKey}
//
// GET 请求的 body 视为空串；ts 为毫秒时间戳字符串。
func (d *DesktopIdentity) sign(method, path, ts, nonce, idem string, body []byte) string {
	sum := sha256.Sum256(body)
	payload := strings.Join([]string{
		"v1", method, path, ts, nonce,
		base64.RawURLEncoding.EncodeToString(sum[:]), idem,
	}, "\n")
	return base64.RawURLEncoding.EncodeToString(ed25519.Sign(d.key, []byte(payload)))
}

// desktopKeyPath 返回某个账号的安装身份文件路径。
// uid 会被规整成安全文件名，避免上游返回的奇怪字符逃出目录。
func (c *Client) desktopKeyPath(uid string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '_'
		}
	}, uid)
	if safe == "" {
		safe = "default"
	}
	return filepath.Join(c.DesktopKeyDir, safe+".json")
}

// DesktopIdentity 惰性加载（首次生成）某个账号的安装身份，进程内按 uid 缓存。
//
// 每个账号必须用各自的身份：上游的 installation id 全局唯一，登记后绑定到
// 首个账号，其他账号再用同一个 id 调 activities/* 会被判 desktop_installation_required。
func (c *Client) DesktopIdentity(uid string) (*DesktopIdentity, error) {
	c.desktopMu.Lock()
	defer c.desktopMu.Unlock()
	if id, ok := c.desktopIDs[uid]; ok {
		return id, nil
	}
	id, err := LoadOrCreateDesktopIdentity(c.desktopKeyPath(uid))
	if err != nil {
		return nil, err
	}
	if c.desktopIDs == nil {
		c.desktopIDs = map[string]*DesktopIdentity{}
	}
	c.desktopIDs[uid] = id
	return id, nil
}

// desktopRequest 按上游要求签名并发起请求，返回状态码与响应体。
// 401 时刷新一次 access_token 重试（与官方 sidecar 行为一致）。
func (c *Client) desktopRequest(ctx context.Context, a *auth.Auth, id *DesktopIdentity, method, path string, body []byte, idem string) (int, []byte, error) {
	status, raw, err := c.desktopAttempt(ctx, a, id, method, path, body, idem)
	if err != nil {
		return status, raw, err
	}
	if status != http.StatusUnauthorized || a.RefreshToken == "" {
		return status, raw, nil
	}
	if refreshErr := c.RefreshToken(a); refreshErr != nil {
		logx.Debugf("desktop %s %s: 401 且刷新失败: %v", method, path, refreshErr)
		return status, raw, nil
	}
	if saveErr := a.SaveAtomic(); saveErr != nil {
		logx.Errorf("desktop %s: 刷新后保存失败: %v", a.UID, saveErr)
	}
	logx.Debugf("desktop %s %s: token 已刷新，重试一次", method, path)
	return c.desktopAttempt(ctx, a, id, method, path, body, idem)
}

func (c *Client) desktopAttempt(ctx context.Context, a *auth.Auth, id *DesktopIdentity, method, path string, body []byte, idem string) (int, []byte, error) {
	if id == nil {
		return 0, nil, fmt.Errorf("desktop identity 未加载")
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce, err := randomNonce()
	if err != nil {
		return 0, nil, err
	}

	var reader io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseAPIURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
	req.Header.Set("anthropic-beta", c.OAuthBetaHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "phanthycode2api/1.0")
	req.Header.Set("x-desktop-installation-id", id.ID)
	req.Header.Set("x-desktop-timestamp", ts)
	req.Header.Set("x-desktop-nonce", nonce)
	req.Header.Set("x-desktop-signature", id.sign(method, path, ts, nonce, idem, body))
	if idem != "" {
		req.Header.Set("idempotency-key", idem)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// RegisterDesktopInstallation 向上游登记本机安装身份。
// 已登记过时上游返回 409，属幂等成功，不算错误。
func (c *Client) RegisterDesktopInstallation(a *auth.Auth, id *DesktopIdentity) error {
	spki, err := id.publicKeySPKI()
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{
		"public_key": spki,
		"edition":    desktopEdition,
		"platform":   runtime.GOOS,
		"version":    DesktopVersion,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, raw, err := c.desktopRequest(ctx, a, id, http.MethodPost, pathDesktopRegister, body, "register:"+id.ID)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		logx.Debugf("desktop register %s: 已登记（409），跳过", a.UID)
		return nil
	}
	if status >= 400 {
		return &Error{Kind: Classify(status, string(raw)), Status: status, Msg: truncate(string(raw), 200)}
	}
	logx.Debugf("desktop register %s: %s", a.UID, truncate(string(raw), 200))
	return nil
}

// ActivitySummary 查询开工奖励状态（含上游权威的可用积分）。
func (c *Client) ActivitySummary(a *auth.Auth, id *DesktopIdentity) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	status, raw, err := c.desktopRequest(ctx, a, id, http.MethodGet, pathActivitySummary, nil, "")
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, &Error{Kind: Classify(status, string(raw)), Status: status, Msg: truncate(string(raw), 200)}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, &Error{Kind: ErrClient, Status: status, Msg: "activities/summary non-json response"}
	}
	return m, nil
}

// ClaimDailyLogin 领取指定业务日的开工奖励。
// 该接口幂等：当天已发放时返回 already_granted，重复调用不会重复发放。
func (c *Client) ClaimDailyLogin(a *auth.Auth, id *DesktopIdentity, serverDate string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body := []byte("{}")
	idem := fmt.Sprintf("daily:%s:%s", id.ID, serverDate)
	status, raw, err := c.desktopRequest(ctx, a, id, http.MethodPost, pathClaimDailyLogin, body, idem)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, &Error{Kind: Classify(status, string(raw)), Status: status, Msg: truncate(string(raw), 200)}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{"status": "granted"}, nil
	}
	return m, nil
}

// randomNonce 生成签名用的随机串（b64u，18 字节熵）。
func randomNonce() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
