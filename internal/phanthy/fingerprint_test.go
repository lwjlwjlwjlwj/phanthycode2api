package phanthy

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
)

func TestFingerprintHeaders(t *testing.T) {
	fp := NewFingerprint(FingerprintConfig{
		Enabled:      true,
		CliVersion:   "0.2.11",
		Client:       "cli",
		ClientSource: "cli",
		Product:      "phanthy_code",
		DeviceJSON:   `{"cliVersion":"0.2.11","hostname":"h"}`,
	})
	if fp == nil {
		t.Fatal("expected fingerprint")
	}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", nil)
	fp.Apply(req, "session-1", "")

	want := map[string]string{
		"x-phanthy-client":        "cli",
		"X-Phanthy-Client-Source": "cli",
		"X-Phanthy-Product":       "phanthy_code",
		"x-phanthy-session":       "session-1",
		"User-Agent":              "phanthy-codeo/0.2.11",
	}
	for key, value := range want {
		if got := req.Header.Get(key); got != value {
			t.Fatalf("header %s = %q, want %q", key, got, value)
		}
	}
	if got := req.Header.Get("x-phanthy-project"); got != "" {
		t.Fatalf("project header should be omitted, got %q", got)
	}

	// 设备指纹必须是 base64url(JSON)，且可还原出原始字段。
	device := req.Header.Get("x-phanthy-cli-device")
	raw, err := base64.RawURLEncoding.DecodeString(device)
	if err != nil {
		t.Fatalf("device header is not base64url: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("device header is not JSON: %v", err)
	}
	if decoded["hostname"] != "h" {
		t.Fatalf("unexpected device payload: %v", decoded)
	}

	// x-phanthy-request 必须是 UUID v4 格式。
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if got := req.Header.Get("x-phanthy-request"); !uuidRe.MatchString(got) {
		t.Fatalf("x-phanthy-request = %q, want UUID v4", got)
	}
}

func TestFingerprintDisabled(t *testing.T) {
	fp := NewFingerprint(FingerprintConfig{Enabled: false})
	if fp != nil {
		t.Fatal("disabled fingerprint should be nil")
	}
	// nil 接收者调用必须安全。
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	fp.Apply(req, "s", "p")
	fp.ApplyProductOnly(req)
	if len(req.Header) != 0 {
		t.Fatalf("nil fingerprint should not set headers: %v", req.Header)
	}
}

func TestApplyProductOnly(t *testing.T) {
	fp := NewFingerprint(FingerprintConfig{
		Enabled:    true,
		CliVersion: "0.2.11",
		Client:     "cli",
		Product:    "phanthy_code",
		DeviceJSON: `{}`,
	})
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/phanthy_code/models", nil)
	fp.ApplyProductOnly(req)

	if got := req.Header.Get("X-Phanthy-Product"); got != "phanthy_code" {
		t.Fatalf("product = %q", got)
	}
	for _, key := range []string{"x-phanthy-client", "x-phanthy-cli-device", "x-phanthy-request", "x-phanthy-session", "X-Phanthy-Client-Source"} {
		if got := req.Header.Get(key); got != "" {
			t.Fatalf("discovery request should not carry %s, got %q", key, got)
		}
	}
}

func TestNewUUIDFormat(t *testing.T) {
	a := NewUUID()
	if a == NewUUID() {
		t.Fatal("UUID should be random per call")
	}
	if len(a) != 36 {
		t.Fatalf("UUID length = %d", len(a))
	}
}
