package fingerprint

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_Disabled_ReturnsNil(t *testing.T) {
	if h := New(Config{Enabled: false}); h != nil {
		t.Fatal("disabled fingerprint should be nil")
	}
}

func TestNew_Defaults(t *testing.T) {
	h := New(Config{Enabled: true})
	if h == nil {
		t.Fatal("enabled fingerprint should not be nil")
	}
	if h.cfg.Client != "cli" {
		t.Errorf("client default = %q", h.cfg.Client)
	}
	if h.cfg.ClientSource != "cli" {
		t.Errorf("clientSource default = %q", h.cfg.ClientSource)
	}
	if h.cfg.Product != "phanthy_code" {
		t.Errorf("product default = %q", h.cfg.Product)
	}
	if h.cfg.UserAgent != "phanthy-codeo/0.2.11" {
		t.Errorf("userAgent default = %q", h.cfg.UserAgent)
	}
	if h.device == "" {
		t.Error("device header should not be empty")
	}
}

func TestDeviceHeader_DecodesAsJSON(t *testing.T) {
	h := New(Config{Enabled: true, CliVersion: "0.2.11"})
	raw, err := base64.RawURLEncoding.DecodeString(h.device)
	if err != nil {
		t.Fatalf("device not base64url: %v", err)
	}
	var di DeviceInfo
	if err := json.Unmarshal(raw, &di); err != nil {
		t.Fatalf("device not JSON: %v", err)
	}
	if di.CliVersion != "0.2.11" {
		t.Errorf("cliVersion = %q", di.CliVersion)
	}
	if di.Platform == "" || di.Arch == "" {
		t.Errorf("platform/arch missing: %+v", di)
	}
	if di.Hostname == "" || di.Username == "" {
		t.Errorf("hostname/username missing: %+v", di)
	}
}

func TestDeviceHeader_Stable(t *testing.T) {
	c1 := New(Config{Enabled: true})
	c2 := New(Config{Enabled: true})
	if c1.device != c2.device {
		t.Error("device header should be stable across instances (same host)")
	}
}

func TestSet_AllSevenHeaders(t *testing.T) {
	h := New(Config{Enabled: true, Client: "code", ClientSource: "telemetry", Product: "phanthy_codeo"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.Set(req, "sess-1", "proj-9")

	checks := map[string]string{
		HeaderClient:       "code",
		HeaderClientSource: "telemetry",
		HeaderDevice:       h.device,
		HeaderProduct:      "phanthy_codeo",
		HeaderRequest:      "", // 每次刷新，仅验证存在
		HeaderSession:      "sess-1",
		HeaderProject:      "proj-9",
	}
	for name, want := range checks {
		got := req.Header.Get(name)
		if got == "" {
			t.Errorf("header %s missing", name)
		}
		if want != "" && got != want {
			t.Errorf("header %s = %q, want %q", name, got, want)
		}
	}
	if req.Header.Get("User-Agent") != "phanthy-codeo/0.2.11" {
		t.Errorf("User-Agent = %q", req.Header.Get("User-Agent"))
	}
}

func TestSet_RequestIDRefreshesPerCall(t *testing.T) {
	h := New(Config{Enabled: true})
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	h.Set(r1, "s", "")
	h.Set(r2, "s", "")
	if r1.Header.Get(HeaderRequest) == r2.Header.Get(HeaderRequest) {
		t.Error("x-phanthy-request should be a fresh UUID per request")
	}
}

func TestSet_ProjectOmittedWhenEmpty(t *testing.T) {
	h := New(Config{Enabled: true})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.Set(req, "s", "")
	if req.Header.Get(HeaderProject) != "" {
		t.Errorf("project header should be omitted when empty, got %q", req.Header.Get(HeaderProject))
	}
}

func TestNewUUID_Format(t *testing.T) {
	u := NewUUID()
	if len(u) != 36 {
		t.Fatalf("uuid length = %d, want 36", len(u))
	}
	// RFC 4122 v4: 第 15 个字符是 '4'
	if u[14] != '4' {
		t.Errorf("uuid not v4: %s", u)
	}
}

func TestDeviceJSON_Override(t *testing.T) {
	h := New(Config{Enabled: true, DeviceJSON: "c3RhdGljLXByZXNldA"})
	if h.device != "c3RhdGljLXByZXNldA" {
		t.Errorf("device override not honored: %q", h.device)
	}
}
