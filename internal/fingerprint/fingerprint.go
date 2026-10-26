// Package fingerprint 复现官方 CLI（@phanthy/phanthy-codeo）的请求头家族与设备指纹。
// 头名与字段名照抄 tarball 内 dist/tui-worker.js 的 iw={...} / fTG(){...}（M6 证据），
// 大写差异是官方原文，必须原样保留：
//
//	client        -> x-phanthy-client
//	clientSource  -> X-Phanthy-Client-Source
//	device        -> x-phanthy-cli-device
//	product       -> X-Phanthy-Product
//	project       -> x-phanthy-project
//	request       -> x-phanthy-request
//	session       -> x-phanthy-session
//
// 设备指纹与官方 fTG() 一致：base64url(JSON)，含 hostname/username/platform/arch/osRelease、
// ipCandidates(≤32)、networkInterfaces、routes(≤32)、defaultGateway。隐私提醒（报告 S13）：
// 该头携带稳定主机信息，属官方客户端的既有行为，仅用于对齐指纹，不用于其他目的。
package fingerprint

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strings"
)

// 官方头名称（大小写照抄 M6 证据 iw 映射）。
const (
	HeaderClient       = "x-phanthy-client"
	HeaderClientSource = "X-Phanthy-Client-Source"
	HeaderDevice       = "x-phanthy-cli-device"
	HeaderProduct      = "X-Phanthy-Product"
	HeaderProject      = "x-phanthy-project"
	HeaderRequest      = "x-phanthy-request"
	HeaderSession      = "x-phanthy-session"
)

const (
	maxIPCandidates = 32
	maxRoutes       = 32
)

// NetworkInterface 对齐 Node os.networkInterfaces() 的条目字段名。
type NetworkInterface struct {
	Address  string `json:"address"`
	Netmask  string `json:"netmask"`
	Family   string `json:"family"`
	MAC      string `json:"mac"`
	Internal bool   `json:"internal"`
	Cidr     string `json:"cidr,omitempty"`
	ScopeID  int    `json:"scopeid,omitempty"`
}

// DeviceInfo 是官方 fTG() 返回的设备指纹 JSON 结构（字段名逐字对齐）。
type DeviceInfo struct {
	CliVersion        string             `json:"cliVersion"`
	Hostname          string             `json:"hostname"`
	Username          string             `json:"username"`
	Platform          string             `json:"platform"`
	Arch              string             `json:"arch"`
	OSRelease         string             `json:"osRelease"`
	IPCandidates      []string           `json:"ipCandidates"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces"`
	Routes            []string           `json:"routes"`
	DefaultGateway    string             `json:"defaultGateway"`
}

// CollectDevice 收集当前主机指纹。任何单项失败都会静默降级，不阻断整体。
func CollectDevice(version string) DeviceInfo {
	di := DeviceInfo{CliVersion: version}
	di.Hostname, _ = os.Hostname()
	di.Platform = runtime.GOOS
	di.Arch = runtime.GOARCH
	di.Username = currentUsername()
	di.OSRelease = osRelease()
	di.IPCandidates, di.NetworkInterfaces = collectNet()
	di.Routes, di.DefaultGateway = defaultRoutes()
	return di
}

// EncodeDevice 把指纹结构编码为官方格式 x-phanthy-cli-device 的 base64url 值。
func EncodeDevice(di DeviceInfo) string {
	raw, _ := json.Marshal(di)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "claude-code-user" // 官方 CLI 的用户名兜底值
}

func osRelease() string {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return ""
}

func collectNet() ([]string, []NetworkInterface) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}
	var ips []string
	var out []NetworkInterface
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, ipnet, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			if ip4 := ip.To4(); ip4 == nil {
				continue
			}
			if len(ips) < maxIPCandidates {
				ips = append(ips, ip.String())
			}
			ones, _ := ipnet.Mask.Size()
			out = append(out, NetworkInterface{
				Address:  ip.String(),
				Netmask:  maskString(ipnet.Mask),
				Family:   "IPv4",
				MAC:      iface.HardwareAddr.String(),
				Internal: iface.Flags&net.FlagLoopback != 0,
				Cidr:     fmt.Sprintf("%s/%d", ip.String(), ones),
			})
		}
	}
	return ips, out
}

// defaultRoutes 解析 /proc/net/route 的默认路由与默认网关。
func defaultRoutes() ([]string, string) {
	raw, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, ""
	}
	var routes []string
	var gw string
	for _, line := range strings.Split(string(raw), "\n")[1:] {
		fields := strings.Fields(line)
		// iface dest gateway flags ...
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" { // dest 0.0.0.0
			continue
		}
		g := hexToIP(fields[2])
		if gw == "" {
			gw = g
		}
		if len(routes) < maxRoutes {
			routes = append(routes, "default via "+g)
		}
	}
	return routes, gw
}

func maskString(m net.IPMask) string {
	if len(m) != 4 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
}

func hexToIP(h string) string {
	if len(h) != 8 {
		return ""
	}
	b := []byte{0, 0, 0, 0}
	for i := 0; i < 4; i++ {
		v := 0
		if _, err := fmt.Sscanf(h[i*2:i*2+2], "%02x", &v); err != nil {
			return ""
		}
		b[i] = byte(v)
	}
	return net.IP{b[3], b[2], b[1], b[0]}.String()
}

// NewUUID 生成 RFC 4122 v4 UUID（x-phanthy-request 每次请求都应刷新）。
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Config 指纹头配置；DeviceJSON 为空时自动采集主机指纹。
type Config struct {
	Enabled      bool
	CliVersion   string
	Client       string // x-phanthy-client（客户端判别符）
	ClientSource string // X-Phanthy-Client-Source
	Product      string // X-Phanthy-Product
	UserAgent    string
	DeviceJSON   string // 预置 base64url 指纹（覆盖自动采集）
}

// Headers 持有预计算的指纹头，可安全并发用于多个请求。
type Headers struct {
	cfg    Config
	device string
}

// New 构建指纹头。Enabled=false 时返回 nil（调用方按无指纹处理）。
func New(cfg Config) *Headers {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Client == "" {
		cfg.Client = "cli"
	}
	if cfg.ClientSource == "" {
		cfg.ClientSource = "cli"
	}
	if cfg.Product == "" {
		cfg.Product = "phanthy_code"
	}
	if cfg.CliVersion == "" {
		cfg.CliVersion = "0.2.11"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "phanthy-codeo/" + cfg.CliVersion
	}
	device := cfg.DeviceJSON
	if device == "" {
		device = EncodeDevice(CollectDevice(cfg.CliVersion))
	}
	return &Headers{cfg: cfg, device: device}
}

// Enabled 报告指纹是否生效。
func (h *Headers) Enabled() bool { return h != nil }

// Set 把完整指纹家族写入请求头。sessionID 传账号会话标识；projectID 为空则省略
// x-phanthy-project（官方在无项目时同样省略）。
func (h *Headers) Set(req *http.Request, sessionID, projectID string) {
	req.Header.Set(HeaderClient, h.cfg.Client)
	req.Header.Set(HeaderClientSource, h.cfg.ClientSource)
	req.Header.Set(HeaderDevice, h.device)
	req.Header.Set(HeaderProduct, h.cfg.Product)
	req.Header.Set(HeaderRequest, NewUUID())
	req.Header.Set(HeaderSession, sessionID)
	if projectID != "" {
		req.Header.Set(HeaderProject, projectID)
	}
	if h.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", h.cfg.UserAgent)
	}
}