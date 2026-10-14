// config.go 加载 JSON 配置 + 环境变量覆盖。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"phanthycode2api/internal/logx"
	"phanthycode2api/internal/upstream"
)

// Config 顶层配置。
type Config struct {
	Listen    string `json:"listen"`     // ":7863"
	APIKey    string `json:"api_key"`    // 空 = 不鉴权
	AuthDir   string `json:"auth_dir"`   // ./auths
	StateFile string `json:"state_file"` // ./data/state.json
	BaseURL   string `json:"base_url"`   // https://code.phanthy.com
	LogLevel  string `json:"log_level"`  // debug | info | error，默认 info

	Admin struct {
		Enabled      bool   `json:"enabled"`
		PasswordHash string `json:"password_hash"` // 空时使用内置默认密码 admin123
		DataDir      string `json:"data_dir"`
	} `json:"admin"`

	Cooldown struct {
		HardCredit  string `json:"hard_credit"`   // "12h"
		SoftRate    string `json:"soft_rate"`     // "60s"
		ErrThresh   int    `json:"err_threshold"` // 默认 3
		ErrCooldown string `json:"err_cooldown"`  // "10m"
	} `json:"cooldown"`

	Schedule struct {
		// KeepaliveInterval 是后台自动保活的检查间隔，默认 "30m"；"0" 关闭。
		// 每轮只刷新 access_token 剩余不足 KeepaliveSkew 的账号。
		KeepaliveInterval string `json:"keepalive_interval"`
		// KeepaliveSkew 是保活提前量，默认 "15m"。
		KeepaliveSkew string `json:"keepalive_skew"`
		// KeepaliveHours 是额外的定点保活小时（本地时区），留空即不设定点保活。
		KeepaliveHours []int `json:"keepalive_hours"`
		// AutoRecover 为真（默认）时，被禁用的账号也会参与保活，刷新成功后自动恢复。
		AutoRecover bool `json:"auto_recover"`
		DailyReward bool `json:"daily_reward"` // 每天自动领取开工奖励，默认 true
		ClaimHour   int  `json:"claim_hour"`   // 领取时刻（北京时间小时），默认 0
		ClaimMinute int  `json:"claim_minute"` // 领取时刻（北京时间分钟），默认 5
	} `json:"schedule"`

	Upstream struct {
		TimeoutSeconds int `json:"timeout_seconds"` // 默认 120
	} `json:"upstream"`

	// Thinking 扩展思考策略。
	// 上游在缺省时会自行开启思考，首字延迟会从 1~2 秒涨到 10 秒以上，
	// 所以这里默认关闭，需要更强推理时再改成 auto / on。
	Thinking struct {
		Mode         string `json:"mode"`          // off | auto | on，默认 off
		BudgetTokens int    `json:"budget_tokens"` // 默认 4096
	} `json:"thinking"`

	// 解析后
	HardCreditDur        time.Duration `json:"-"`
	SoftRateDur          time.Duration `json:"-"`
	ErrCooldownDur       time.Duration `json:"-"`
	KeepaliveIntervalDur time.Duration `json:"-"`
	KeepaliveSkewDur     time.Duration `json:"-"`
}

// Default 默认配置。
func Default() *Config {
	c := &Config{
		Listen:    ":7864",
		APIKey:    "",
		AuthDir:   "./auths",
		StateFile: "./data/state.json",
		BaseURL:   "https://code.phanthy.com",
		LogLevel:  "info",
	}
	c.Admin.Enabled = true
	c.Admin.DataDir = "./data/admin"
	c.Cooldown.HardCredit = "12h"
	c.Cooldown.SoftRate = "30s"
	c.Cooldown.ErrThresh = 5
	c.Cooldown.ErrCooldown = "2m"
	c.Schedule.KeepaliveInterval = "30m"
	c.Schedule.KeepaliveSkew = "15m"
	c.Schedule.KeepaliveHours = []int{}
	c.Schedule.AutoRecover = true
	c.Schedule.DailyReward = true
	c.Schedule.ClaimHour = 0
	c.Schedule.ClaimMinute = 5
	c.Upstream.TimeoutSeconds = 120
	c.Thinking.Mode = "off"
	c.Thinking.BudgetTokens = upstream.DefaultThinkingBudget
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
	if v := os.Getenv("P2A_DAILY_REWARD"); v != "" {
		c.Schedule.DailyReward = v != "0" && !strings.EqualFold(v, "false")
	}
	if v := os.Getenv("P2A_KEEPALIVE_INTERVAL"); v != "" {
		c.Schedule.KeepaliveInterval = v
	}
	if v := os.Getenv("P2A_KEEPALIVE_SKEW"); v != "" {
		c.Schedule.KeepaliveSkew = v
	}
	if v := os.Getenv("P2A_AUTO_RECOVER"); v != "" {
		c.Schedule.AutoRecover = v != "0" && !strings.EqualFold(v, "false")
	}
	if v := os.Getenv("P2A_CLAIM_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Schedule.ClaimHour = n
		}
	}
	if v := os.Getenv("P2A_CLAIM_MINUTE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Schedule.ClaimMinute = n
		}
	}
	if v := os.Getenv("P2A_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Upstream.TimeoutSeconds = n
		}
	}
	if v := os.Getenv("P2A_THINKING_MODE"); v != "" {
		c.Thinking.Mode = v
	}
	if v := os.Getenv("P2A_THINKING_BUDGET"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Thinking.BudgetTokens = n
		}
	}
	if v := os.Getenv("P2A_LOG_LEVEL"); v != "" {
		c.LogLevel = v
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
	if c.Cooldown.ErrThresh <= 0 {
		c.Cooldown.ErrThresh = 5
	}
	if c.KeepaliveIntervalDur, err = parseInterval(c.Schedule.KeepaliveInterval, 30*time.Minute); err != nil {
		return fmt.Errorf("schedule.keepalive_interval: %w", err)
	}
	if c.KeepaliveSkewDur, err = parseInterval(c.Schedule.KeepaliveSkew, 15*time.Minute); err != nil {
		return fmt.Errorf("schedule.keepalive_skew: %w", err)
	}
	if c.Schedule.ClaimHour < 0 || c.Schedule.ClaimHour > 23 {
		c.Schedule.ClaimHour = 0
	}
	if c.Schedule.ClaimMinute < 0 || c.Schedule.ClaimMinute > 59 {
		c.Schedule.ClaimMinute = 5
	}
	if c.Upstream.TimeoutSeconds <= 0 {
		c.Upstream.TimeoutSeconds = 120
	}
	if c.Admin.DataDir == "" {
		c.Admin.Enabled = true
		c.Admin.DataDir = "./data/admin"
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://code.phanthy.com"
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if !strings.HasPrefix(c.Listen, ":") && !strings.Contains(c.Listen, ":") {
		c.Listen = ":" + c.Listen
	}
	normalizedThinking := c.ThinkingOption().Normalize()
	c.Thinking.Mode = string(normalizedThinking.Mode)
	c.Thinking.BudgetTokens = normalizedThinking.Budget
	c.LogLevel = logx.Parse(c.LogLevel).String()
	return nil
}

// LogLevelValue 把配置转成 logx 使用的级别。
func (c *Config) LogLevelValue() logx.Level { return logx.Parse(c.LogLevel) }

// parseInterval 解析调度用的时间间隔字符串。
// 空串取默认值；"0" / "off" / "false" / "never" 表示关闭（返回 0）；
// 负数视为关闭，避免把定时器配成负数后疯狂触发。
func parseInterval(raw string, def time.Duration) (time.Duration, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "":
		return def, nil
	case "0", "off", "false", "no", "never":
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

// ThinkingOption 把配置转成 upstream 使用的思考策略。
func (c *Config) ThinkingOption() upstream.ThinkingOption {
	return upstream.ThinkingOption{
		Mode:   upstream.ThinkingMode(strings.ToLower(strings.TrimSpace(c.Thinking.Mode))),
		Budget: c.Thinking.BudgetTokens,
	}
}
