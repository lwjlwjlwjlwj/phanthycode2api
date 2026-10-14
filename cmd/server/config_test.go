package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParseInterval 覆盖自动保活间隔的解析规则。
func TestParseInterval(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", 30 * time.Minute},
		{"  ", 30 * time.Minute},
		{"10m", 10 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"0", 0},
		{"off", 0},
		{"never", 0},
		{"-5m", 0},
	}
	for _, tc := range cases {
		got, err := parseInterval(tc.raw, 30*time.Minute)
		if err != nil {
			t.Fatalf("parseInterval(%q) 报错: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Errorf("parseInterval(%q) = %s, want %s", tc.raw, got, tc.want)
		}
	}
	if _, err := parseInterval("30", time.Minute); err == nil {
		t.Error("缺少单位的写法应报错，而不是静默取默认值")
	}
}

// TestSaveConfigMapKeepsKeepaliveFields 设置页保存的自动保活字段要真的写进配置文件。
func TestSaveConfigMapKeepsKeepaliveFields(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "config.json")
	raw := map[string]any{
		"listen":     "127.0.0.1:7864",
		"auth_dir":   "./auths",
		"state_file": "./data/state.json",
		"schedule": map[string]any{
			"keepalive_interval": "10m",
			"keepalive_skew":     "5m",
			"auto_recover":       false,
			"keepalive_hours":    []any{3, 15},
			"daily_reward":       true,
			"claim_hour":         0,
			"claim_minute":       5,
		},
	}
	if err := SaveConfigMap(fp, raw); err != nil {
		t.Fatalf("SaveConfigMap: %v", err)
	}
	cfg, err := Load(fp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Schedule.KeepaliveInterval != "10m" || cfg.KeepaliveIntervalDur != 10*time.Minute {
		t.Errorf("keepalive_interval = %q / %s", cfg.Schedule.KeepaliveInterval, cfg.KeepaliveIntervalDur)
	}
	if cfg.KeepaliveSkewDur != 5*time.Minute {
		t.Errorf("keepalive_skew = %s, want 5m", cfg.KeepaliveSkewDur)
	}
	if cfg.Schedule.AutoRecover {
		t.Error("auto_recover 应为 false")
	}
	if len(cfg.Schedule.KeepaliveHours) != 2 || cfg.Schedule.KeepaliveHours[0] != 3 {
		t.Errorf("keepalive_hours = %v", cfg.Schedule.KeepaliveHours)
	}
}

// TestSaveConfigMapRejectsBadInterval 写坏的间隔要当场报错，不能落盘后拖垮下次启动。
func TestSaveConfigMapRejectsBadInterval(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "config.json")
	raw := map[string]any{
		"listen":   "127.0.0.1:7864",
		"schedule": map[string]any{"keepalive_interval": "30"},
	}
	if err := SaveConfigMap(fp, raw); err == nil {
		t.Fatal("无效的 keepalive_interval 应被拒绝")
	}
	if _, err := os.Stat(fp); err == nil {
		t.Error("校验失败时不应写出配置文件")
	}
}

// TestToMapExposesKeepaliveFields 设置页读取的 JSON 必须带上新字段。
func TestToMapExposesKeepaliveFields(t *testing.T) {
	c := Default()
	if err := c.normalize(); err != nil {
		t.Fatalf("normalize: %v", err)
	}
	out, err := json.Marshal(c.ToMap())
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Schedule map[string]any `json:"schedule"`
	}
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"keepalive_interval", "keepalive_skew", "auto_recover"} {
		if _, ok := back.Schedule[key]; !ok {
			t.Errorf("ToMap 缺少 schedule.%s", key)
		}
	}
}
