// config.go 加载 JSON 配置 + 环境变量覆盖。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 顶层配置。
type Config struct {
	Listen    string `json:"listen"`     // ":7863"
	APIKey    string `json:"api_key"`    // 空 = 不鉴权
	AuthDir   string `json:"auth_dir"`   // ./auths
	StateFile string `json:"state_file"` // ./data/state.json
	BaseURL   string `json:"base_url"`   // https://code.phanthy.com

	Cooldown struct {
		HardCredit  string `json:"hard_credit"`   // "12h"
		SoftRate    string `json:"soft_rate"`     // "60s"
		ErrThresh   int    `json:"err_threshold"` // 默认 3
		ErrCooldown string `json:"err_cooldown"`  // "10m"
	} `json:"cooldown"`

	Schedule struct {
		KeepaliveHours []int  `json:"keepalive_hours"` // [22]
		ProbeInterval  string `json:"probe_interval"`  // 余额探测周期 "30m"；"0" = 关闭
	} `json:"schedule"`

	Fingerprint struct {
		Enabled      bool   `json:"enabled"` // 默认 true：发送官方 CLI 指纹头（M6 证据）
		CliVersion   string `json:"cli_version"`
		Client       string `json:"client"`        // x-phanthy-client
		ClientSource string `json:"client_source"` // X-Phanthy-Client-Source
		Product      string `json:"product"`       // X-Phanthy-Product
		UserAgent    string `json:"user_agent"`
		DeviceJSON   string `json:"device_json"` // 预置 base64url 指纹（留空自动采集）
	} `json:"fingerprint"`

	Upstream struct {
		TimeoutSeconds int `json:"timeout_seconds"` // 默认 120
	} `json:"upstream"`

	// 解析后
	HardCreditDur  time.Duration `json:"-"`
	SoftRateDur    time.Duration `json:"-"`
	ErrCooldownDur time.Duration `json:"-"`
	ProbeDur       time.Duration `json:"-"`
}

// Default 默认配置。
func Default() *Config {
	c := &Config{
		Listen:    ":7863",
		APIKey:    "",
		AuthDir:   "./auths",
		StateFile: "./data/state.json",
		BaseURL:   "https://code.phanthy.com",
	}
	c.Cooldown.HardCredit = "12h"
	c.Cooldown.SoftRate = "60s"
	c.Cooldown.ErrThresh = 3
	c.Cooldown.ErrCooldown = "10m"
	c.Schedule.KeepaliveHours = []int{22}
	c.Schedule.ProbeInterval = "0"
	c.Fingerprint.Enabled = true
	c.Fingerprint.CliVersion = "0.2.11" // 与官方 npm 包版本对齐
	c.Upstream.TimeoutSeconds = 120
	return c
}

// Load 从文件读，再用 P2A_* env 覆盖。
func Load(path string) (*Config, error) {
	c := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}
		if err := json.Unmarshal(raw, c); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}
	applyEnv(c)
	if err := c.normalize(); err != nil {
		return nil, err
	}
	return c, nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("P2A_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("P2A_API_KEY"); v != "" {
		c.APIKey = v
	}
	if v := os.Getenv("P2A_AUTH_DIR"); v != "" {
		c.AuthDir = v
	}
	if v := os.Getenv("P2A_STATE_FILE"); v != "" {
		c.StateFile = v
	}
	if v := os.Getenv("P2A_BASE_URL"); v != "" {
		c.BaseURL = v
	}
	if v := os.Getenv("P2A_HARD_CREDIT"); v != "" {
		c.Cooldown.HardCredit = v
	}
	if v := os.Getenv("P2A_SOFT_RATE"); v != "" {
		c.Cooldown.SoftRate = v
	}
	if v := os.Getenv("P2A_ERR_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Cooldown.ErrThresh = n
		}
	}
	if v := os.Getenv("P2A_ERR_COOLDOWN"); v != "" {
		c.Cooldown.ErrCooldown = v
	}
	if v := os.Getenv("P2A_PROBE_INTERVAL"); v != "" {
		c.Schedule.ProbeInterval = v
	}
	if v := os.Getenv("P2A_FP_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.Fingerprint.Enabled = b
		}
	}
	if v := os.Getenv("P2A_FP_CLI_VERSION"); v != "" {
		c.Fingerprint.CliVersion = v
	}
	if v := os.Getenv("P2A_FP_CLIENT"); v != "" {
		c.Fingerprint.Client = v
	}
	if v := os.Getenv("P2A_FP_CLIENT_SOURCE"); v != "" {
		c.Fingerprint.ClientSource = v
	}
	if v := os.Getenv("P2A_FP_PRODUCT"); v != "" {
		c.Fingerprint.Product = v
	}
	if v := os.Getenv("P2A_FP_USER_AGENT"); v != "" {
		c.Fingerprint.UserAgent = v
	}
	if v := os.Getenv("P2A_FP_DEVICE_JSON"); v != "" {
		c.Fingerprint.DeviceJSON = v
	}
	if v := os.Getenv("P2A_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Upstream.TimeoutSeconds = n
		}
	}
}

func (c *Config) normalize() error {
	var err error
	if c.HardCreditDur, err = time.ParseDuration(c.Cooldown.HardCredit); err != nil {
		return fmt.Errorf("cooldown.hard_credit: %w", err)
	}
	if c.SoftRateDur, err = time.ParseDuration(c.Cooldown.SoftRate); err != nil {
		return fmt.Errorf("cooldown.soft_rate: %w", err)
	}
	if c.ErrCooldownDur, err = time.ParseDuration(c.Cooldown.ErrCooldown); err != nil {
		return fmt.Errorf("cooldown.err_cooldown: %w", err)
	}
	if c.ProbeDur, err = time.ParseDuration(c.Schedule.ProbeInterval); err != nil {
		return fmt.Errorf("schedule.probe_interval: %w", err)
	}
	if c.Cooldown.ErrThresh <= 0 {
		c.Cooldown.ErrThresh = 3
	}
	if c.Upstream.TimeoutSeconds <= 0 {
		c.Upstream.TimeoutSeconds = 120
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://code.phanthy.com"
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if !strings.HasPrefix(c.Listen, ":") && !strings.Contains(c.Listen, ":") {
		c.Listen = ":" + c.Listen
	}
	return nil
}
