// Package phanthy 封装对 code.phanthy.com 的全部上游调用。
//
// 请求头名与大小写来自 M6 对官方 CLI @phanthy/phanthy-codeo@0.2.11 的静态提取
// （re/evidence/M6-cli-config.txt:58-73）：其中两个刻意大写 X-Phanthy，五个为小写 x-phanthy-。
package phanthy

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
	"sort"
	"strings"
)

// 官方 CLI 的七个请求头名（大小写必须原样保留）。
const (
	HeaderClient       = "x-phanthy-client"
	HeaderClientSource = "X-Phanthy-Client-Source"
	HeaderDevice       = "x-phanthy-cli-device"
	HeaderProduct      = "X-Phanthy-Product"
	HeaderProject      = "x-phanthy-project"
	HeaderRequest      = "x-phanthy-request"
	HeaderSession      = "x-phanthy-session"
)

// FingerprintConfig 是指纹生成参数。
type FingerprintConfig struct {
	Enabled      bool
	CliVersion   string
	Client       string
	ClientSource string
	Product      string
	UserAgent    string
	DeviceJSON   string // 原始 JSON 字符串；留空则自动采集
}

// Fingerprint 预先计算稳定的设备指纹，并在每个请求上写入官方 CLI 请求头。
type Fingerprint struct {
	client       string
	clientSource string
	product      string
	userAgent    string
	device       string
}

// NewFingerprint 构建指纹；Enabled=false 时返回 nil，调用方无需判空分支。
func NewFingerprint(cfg FingerprintConfig) *Fingerprint {
	if !cfg.Enabled {
		return nil
	}
	deviceJSON := strings.TrimSpace(cfg.DeviceJSON)
	if deviceJSON == "" {
		if raw, err := json.Marshal(collectDevice(cfg.CliVersion)); err == nil {
			deviceJSON = string(raw)
		}
	}

	client := cfg.Client
	if client == "" {
		client = "cli"
	}
	clientSource := cfg.ClientSource
	if clientSource == "" {
		clientSource = "cli"
	}
	product := cfg.Product
	if product == "" {
		product = "phanthy_code"
	}
	userAgent := cfg.UserAgent
	if userAgent == "" {
		version := cfg.CliVersion
		if version == "" {
			version = "0.2.11"
		}
		userAgent = "phanthy-codeo/" + version
	}

	return &Fingerprint{
		client:       client,
		clientSource: clientSource,
		product:      product,
		userAgent:    userAgent,
		device:       base64.RawURLEncoding.EncodeToString([]byte(deviceJSON)),
	}
}

// Apply 在推理请求上写入完整请求头族。
// sessionID 对应 x-phanthy-session；projectID 为空时不发送 x-phanthy-project（对齐官方“无项目”行为）。
func (f *Fingerprint) Apply(req *http.Request, sessionID, projectID string) {
	if f == nil {
		return
	}
	h := req.Header
	h.Set(HeaderClient, f.client)
	h.Set(HeaderClientSource, f.clientSource)
	h.Set(HeaderDevice, f.device)
	h.Set(HeaderProduct, f.product)
	h.Set(HeaderRequest, NewUUID())
	if sessionID != "" {
		h.Set(HeaderSession, sessionID)
	}
	if projectID != "" {
		h.Set(HeaderProject, projectID)
	}
	f.setUserAgent(req)
}

// ApplyProductOnly 只写 X-Phanthy-Product 与 User-Agent。
// 对齐官方 CLI 的两个模型发现端点：它们只带 x-phanthy-product（M6-cli-config.txt:168-186）。
func (f *Fingerprint) ApplyProductOnly(req *http.Request) {
	if f == nil {
		return
	}
	req.Header.Set(HeaderProduct, f.product)
	f.setUserAgent(req)
}

func (f *Fingerprint) setUserAgent(req *http.Request) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", f.userAgent)
	}
}

// DeviceInfo 是 x-phanthy-cli-device 的 JSON 载荷。
type DeviceInfo struct {
	CliVersion        string          `json:"cliVersion"`
	Hostname          string          `json:"hostname"`
	Username          string          `json:"username"`
	Platform          string          `json:"platform"`
	Arch              string          `json:"arch"`
	OSRelease         string          `json:"osRelease"`
	IPCandidates      []string        `json:"ipCandidates"`
	NetworkInterfaces []InterfaceInfo `json:"networkInterfaces"`
	Routes            []string        `json:"routes"`
	DefaultGateway    string          `json:"defaultGateway"`
}

// InterfaceInfo 描述一张网卡的 IPv4 地址。
type InterfaceInfo struct {
	Name     string `json:"name"`
	Address  string `json:"address"`
	Internal bool   `json:"internal"`
}

const maxDeviceItems = 32

// collectDevice 采集本机信息，用于构造 base64url(JSON) 设备指纹。
// 采集失败时对应字段为空，不影响请求发送。
func collectDevice(cliVersion string) DeviceInfo {
	info := DeviceInfo{
		CliVersion: cliVersion,
		Platform:   runtime.GOOS,
		Arch:       runtime.GOARCH,
	}
	info.Hostname, _ = os.Hostname()
	info.Username = currentUsername()
	info.OSRelease = osRelease()

	type ipEntry struct {
		addr     string
		internal bool
	}
	var ips []ipEntry
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipnet, ok := addr.(*net.IPNet)
				if !ok {
					continue
				}
				v4 := ipnet.IP.To4()
				if v4 == nil || v4.IsLoopback() {
					continue
				}
				entry := ipEntry{addr: v4.String(), internal: iface.Flags&net.FlagLoopback != 0}
				ips = append(ips, entry)
				if len(info.NetworkInterfaces) < maxDeviceItems {
					ones, _ := ipnet.Mask.Size()
					info.NetworkInterfaces = append(info.NetworkInterfaces, InterfaceInfo{
						Name:     iface.Name,
						Address:  fmt.Sprintf("%s/%d", v4.String(), ones),
						Internal: iface.Flags&net.FlagLoopback != 0,
					})
				}
			}
		}
	}
	// 官方客户端把非内部地址排在前面。
	sort.SliceStable(ips, func(i, j int) bool { return !ips[i].internal && ips[j].internal })
	for _, ip := range ips {
		if len(info.IPCandidates) >= maxDeviceItems {
			break
		}
		info.IPCandidates = append(info.IPCandidates, ip.addr)
	}

	info.Routes, info.DefaultGateway = defaultRoutes()
	return info
}

func currentUsername() string {
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	if v := os.Getenv("USERNAME"); v != "" {
		return v
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "claude-code-user"
}

func osRelease() string {
	raw, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return ""
}

// defaultRoutes 解析 Linux 路由表，返回默认路由与网关；其他平台返回空。
func defaultRoutes() ([]string, string) {
	raw, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, ""
	}
	var routes []string
	gateway := ""
	for i, line := range strings.Split(string(raw), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// fields: Iface Destination Gateway Flags ...
		if fields[1] != "00000000" {
			continue
		}
		gw := parseHexIPv4(fields[2])
		if len(routes) < maxDeviceItems {
			r := fmt.Sprintf("default via %s dev %s", gw, fields[0])
			routes = append(routes, r)
		}
		if gateway == "" {
			gateway = gw
		}
	}
	return routes, gateway
}

func parseHexIPv4(hex string) string {
	if len(hex) != 8 {
		return hex
	}
	ip := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		var b byte
		if _, err := fmt.Sscanf(hex[i*2:i*2+2], "%02x", &b); err != nil {
			return hex
		}
		// /proc/net/route 是小端十六进制
		ip[3-i] = b
	}
	return ip.String()
}

// NewUUID 生成 RFC 4122 v4 UUID，用于 x-phanthy-request。
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 失败是极端环境错误，退化为时间无关的固定格式没有意义；
		// 这里仍返回一个格式合法的 UUID，避免请求构造失败。
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
