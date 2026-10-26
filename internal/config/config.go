// Package config 负责加载网关配置：先读 JSON 文件，再用 P2A_* 环境变量覆盖。
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是网关的完整配置。
type Config struct {
	Listen    string `json:"listen"`     // 监听地址，如 ":7864"
	APIKey    string `json:"api_key"`    // 非空时对 /v1/* 与 /status 启用 Bearer 鉴权
	AuthDir   string `json:"auth_dir"`   // 账号凭证目录
	StateFile string `json:"state_file"` // 账号池状态持久化文件
	BaseURL   string `json:"base_url"`   // 上游地址，默认 https://code.phanthy.com

	Cooldown    Cooldown    `json:"cooldown"`
	Schedule    Schedule    `json:"schedule"`
	Fingerprint Fingerprint `json:"fingerprint"`
	Upstream    Upstream    `json:"upstream"`

	// 以下字段由 Validate 解析，不参与 JSON 序列化。
	HardCreditDur   time.Duration `json:"-"`
	SoftRateDur     time.Duration `json:"-"`
	ErrCooldownDur  time.Duration `json:"-"`
	ProbeInterval   time.Duration `json:"-"`
	UpstreamTimeout time.Duration `json:"-"`
}

// Cooldown 定义账号冷却策略。
type Cooldown struct {
	HardCredit   string `json:"hard_credit"`   // 积分不足冷却，默认 "12h"
	SoftRate     string `json:"soft_rate"`     // 限流冷却，默认 "60s"
	ErrThreshold int    `json:"err_threshold"` // 连续错误阈值，默认 3
	ErrCooldown  string `json:"err_cooldown"`  // 达到阈值后的冷却，默认 "10m"
}

// Schedule 定义定时任务。
type Schedule struct {
	KeepaliveHours []int  `json:"keepalive_hours"` // 每日 keepalive 的本地小时（0-23），默认 [22]
	ProbeInterval  string `json:"probe_interval"`  // 余额探测周期；"0" 表示关闭
}

// Fingerprint 是官方 CLI 请求头族（M6 证据）的配置。
type Fingerprint struct {
	Enabled      bool   `json:"enabled"`       // 默认 true
	CliVersion   string `json:"cli_version"`   // 默认 "0.2.11"
	Client       string `json:"client"`        // x-phanthy-client
	ClientSource string `json:"client_source"` // X-Phanthy-Client-Source
	Product      string `json:"product"`       // X-Phanthy-Product
	UserAgent    string `json:"user_agent"`    // 留空时使用 phanthy-codeo/<cli_version>
	DeviceJSON   string `json:"device_json"`   // 预留的设备指纹 JSON；留空则自动采集
}

// Upstream 定义上游调用参数。
type Upstream struct {
	TimeoutSeconds int `json:"timeout_seconds"` // 默认 120
}

// Default 返回内置默认配置。
func Default() *Config {
	return &Config{
		Listen:    ":7864",
		AuthDir:   "./auths",
		StateFile: "./data/state.json",
		BaseURL:   "https://code.phanthy.com",
		Cooldown: Cooldown{
			HardCredit:   "12h",
			SoftRate:     "60s",
			ErrThreshold: 3,
			ErrCooldown:  "10m",
		},
		Schedule: Schedule{
			KeepaliveHours: []int{22},
			ProbeInterval:  "0",
		},
		Fingerprint: Fingerprint{
			Enabled:      true,
			CliVersion:   "0.2.11",
			Client:       "cli",
			ClientSource: "cli",
			Product:      "phanthy_code",
		},
		Upstream: Upstream{TimeoutSeconds: 120},
	}
}

// Load 读取配置文件并应用环境变量覆盖，最后校验并解析。
// path 为空时只使用默认值与环境变量；path 非空但文件不存在时返回错误。
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
		if err := json.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件失败: %w", err)
		}
	}
	applyEnv(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv 用 P2A_* 环境变量覆盖配置。
func applyEnv(c *Config) {
	setString := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	setString("P2A_LISTEN", &c.Listen)
	setString("P2A_API_KEY", &c.APIKey)
	setString("P2A_AUTH_DIR", &c.AuthDir)
	setString("P2A_STATE_FILE", &c.StateFile)
	setString("P2A_BASE_URL", &c.BaseURL)
	setString("P2A_HARD_CREDIT", &c.Cooldown.HardCredit)
	setString("P2A_SOFT_RATE", &c.Cooldown.SoftRate)
	setString("P2A_ERR_COOLDOWN", &c.Cooldown.ErrCooldown)
	setString("P2A_PROBE_INTERVAL", &c.Schedule.ProbeInterval)
	setString("P2A_FP_CLI_VERSION", &c.Fingerprint.CliVersion)
	setString("P2A_FP_CLIENT", &c.Fingerprint.Client)
	setString("P2A_FP_CLIENT_SOURCE", &c.Fingerprint.ClientSource)
	setString("P2A_FP_PRODUCT", &c.Fingerprint.Product)
	setString("P2A_FP_USER_AGENT", &c.Fingerprint.UserAgent)
	setString("P2A_FP_DEVICE_JSON", &c.Fingerprint.DeviceJSON)

	if v := os.Getenv("P2A_ERR_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Cooldown.ErrThreshold = n
		}
	}
	if v := os.Getenv("P2A_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Upstream.TimeoutSeconds = n
		}
	}
	if v := os.Getenv("P2A_FP_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Fingerprint.Enabled = b
		}
	}
	if v := os.Getenv("P2A_KEEPALIVE_HOURS"); v != "" {
		hours, err := parseHours(v)
		if err == nil {
			c.Schedule.KeepaliveHours = hours
		}
	}
}

// Validate 校验并规范化配置。
func (c *Config) Validate() error {
	c.Listen = strings.TrimSpace(c.Listen)
	if c.Listen == "" {
		return fmt.Errorf("listen 不能为空")
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("base_url 不是合法 URL: %q", c.BaseURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base_url 必须使用 http/https: %q", c.BaseURL)
	}

	if c.Cooldown.ErrThreshold < 1 {
		c.Cooldown.ErrThreshold = 3
	}
	durations := []struct {
		name  string
		value string
		dst   *time.Duration
		def   time.Duration
	}{
		{"cooldown.hard_credit", c.Cooldown.HardCredit, &c.HardCreditDur, 12 * time.Hour},
		{"cooldown.soft_rate", c.Cooldown.SoftRate, &c.SoftRateDur, time.Minute},
		{"cooldown.err_cooldown", c.Cooldown.ErrCooldown, &c.ErrCooldownDur, 10 * time.Minute},
		{"schedule.probe_interval", c.Schedule.ProbeInterval, &c.ProbeInterval, 0},
	}
	for _, d := range durations {
		parsed, err := time.ParseDuration(strings.TrimSpace(d.value))
		if err != nil {
			if strings.TrimSpace(d.value) == "" {
				parsed = d.def
			} else {
				return fmt.Errorf("%s 不是合法时长: %q", d.name, d.value)
			}
		}
		if parsed < 0 {
			return fmt.Errorf("%s 不能为负: %q", d.name, d.value)
		}
		*d.dst = parsed
	}

	if len(c.Schedule.KeepaliveHours) == 0 {
		c.Schedule.KeepaliveHours = []int{22}
	}
	seen := make(map[int]bool, len(c.Schedule.KeepaliveHours))
	hours := make([]int, 0, len(c.Schedule.KeepaliveHours))
	for _, h := range c.Schedule.KeepaliveHours {
		if h < 0 || h > 23 || seen[h] {
			continue
		}
		seen[h] = true
		hours = append(hours, h)
	}
	if len(hours) == 0 {
		hours = []int{22}
	}
	c.Schedule.KeepaliveHours = hours

	if c.Upstream.TimeoutSeconds <= 0 {
		c.Upstream.TimeoutSeconds = 120
	}
	c.UpstreamTimeout = time.Duration(c.Upstream.TimeoutSeconds) * time.Second

	if strings.TrimSpace(c.Fingerprint.CliVersion) == "" {
		c.Fingerprint.CliVersion = "0.2.11"
	}
	if strings.TrimSpace(c.Fingerprint.Client) == "" {
		c.Fingerprint.Client = "cli"
	}
	if strings.TrimSpace(c.Fingerprint.ClientSource) == "" {
		c.Fingerprint.ClientSource = "cli"
	}
	if strings.TrimSpace(c.Fingerprint.Product) == "" {
		c.Fingerprint.Product = "phanthy_code"
	}
	if strings.TrimSpace(c.Fingerprint.UserAgent) == "" {
		c.Fingerprint.UserAgent = "phanthy-codeo/" + c.Fingerprint.CliVersion
	}

	if strings.TrimSpace(c.AuthDir) == "" {
		return fmt.Errorf("auth_dir 不能为空")
	}
	return nil
}

// parseHours 解析逗号分隔的小时列表；范围与去重由 Validate 负责。
func parseHours(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	hours := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("非法小时: %q", p)
		}
		hours = append(hours, n)
	}
	return hours, nil
}
