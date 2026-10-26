package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	t.Setenv("P2A_LISTEN", "")
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if cfg.Listen != ":7864" || cfg.BaseURL != "https://code.phanthy.com" {
		t.Fatalf("defaults = %+v", cfg)
	}
	if cfg.HardCreditDur != 12*time.Hour || cfg.SoftRateDur != time.Minute || cfg.ErrCooldownDur != 10*time.Minute {
		t.Fatalf("durations = %v %v %v", cfg.HardCreditDur, cfg.SoftRateDur, cfg.ErrCooldownDur)
	}
	if cfg.Fingerprint.UserAgent != "phanthy-codeo/0.2.11" {
		t.Fatalf("user agent = %q", cfg.Fingerprint.UserAgent)
	}
	if len(cfg.Schedule.KeepaliveHours) != 1 || cfg.Schedule.KeepaliveHours[0] != 22 {
		t.Fatalf("keepalive hours = %v", cfg.Schedule.KeepaliveHours)
	}
	if cfg.Fingerprint.Enabled != true || cfg.ProbeInterval != 0 {
		t.Fatalf("fingerprint enabled=%v probe=%v", cfg.Fingerprint.Enabled, cfg.ProbeInterval)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("P2A_LISTEN", ":9999")
	t.Setenv("P2A_API_KEY", "secret")
	t.Setenv("P2A_HARD_CREDIT", "1h30m")
	t.Setenv("P2A_ERR_THRESHOLD", "5")
	t.Setenv("P2A_TIMEOUT_SECONDS", "30")
	t.Setenv("P2A_KEEPALIVE_HOURS", "3, 9,9,99")
	t.Setenv("P2A_FP_ENABLED", "false")
	t.Setenv("P2A_PROBE_INTERVAL", "15m")

	cfg := Default()
	applyEnv(cfg)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if cfg.Listen != ":9999" || cfg.APIKey != "secret" {
		t.Fatalf("listen/api key = %q %q", cfg.Listen, cfg.APIKey)
	}
	if cfg.HardCreditDur != 90*time.Minute {
		t.Fatalf("hard credit = %v", cfg.HardCreditDur)
	}
	if cfg.Cooldown.ErrThreshold != 5 || cfg.UpstreamTimeout != 30*time.Second {
		t.Fatalf("threshold/timeout = %d %v", cfg.Cooldown.ErrThreshold, cfg.UpstreamTimeout)
	}
	if cfg.Fingerprint.Enabled {
		t.Fatal("fingerprint should be disabled by env")
	}
	if cfg.ProbeInterval != 15*time.Minute {
		t.Fatalf("probe interval = %v", cfg.ProbeInterval)
	}
	if len(cfg.Schedule.KeepaliveHours) != 2 || cfg.Schedule.KeepaliveHours[0] != 3 || cfg.Schedule.KeepaliveHours[1] != 9 {
		t.Fatalf("keepalive hours = %v", cfg.Schedule.KeepaliveHours)
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"listen":":7000","api_key":"k","cooldown":{"hard_credit":"2h"},"fingerprint":{"enabled":true,"cli_version":"9.9.9"}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"P2A_LISTEN", "P2A_API_KEY", "P2A_HARD_CREDIT", "P2A_FP_CLI_VERSION"} {
		t.Setenv(key, "")
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":7000" || cfg.APIKey != "k" || cfg.HardCreditDur != 2*time.Hour {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Fingerprint.UserAgent != "phanthy-codeo/9.9.9" {
		t.Fatalf("user agent = %q", cfg.Fingerprint.UserAgent)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing config file should error")
	}
	cfg := Default()
	cfg.BaseURL = "ftp://example.com"
	if err := cfg.Validate(); err == nil {
		t.Fatal("non-http base_url should error")
	}
	cfg = Default()
	cfg.Cooldown.SoftRate = "not-a-duration"
	if err := cfg.Validate(); err == nil {
		t.Fatal("invalid duration should error")
	}
}
