package credential

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDiskFormats(t *testing.T) {
	cases := []struct {
		name       string
		filename   string
		body       string
		wantUID    string
		wantAT     string
		wantRT     string
		wantExpiry time.Time
		wantKey    string
	}{
		{
			name:       "CLI 原生秒级时间戳",
			filename:   "phanthy-abc.json",
			body:       `{"type":"oauth_token","access_token":"at1","refresh_token":"rt1","expires_at":1700000000,"uid":"u-cli","api_key":"k1"}`,
			wantUID:    "u-cli",
			wantAT:     "at1",
			wantRT:     "rt1",
			wantExpiry: time.Unix(1700000000, 0),
			wantKey:    "k1",
		},
		{
			name:       "嵌套格式",
			filename:   "auth-1.json",
			body:       `{"auth":{"accessToken":"at2","refreshToken":"rt2","expiresAt":1700000100},"account":{"uid":"u-nest","nickname":"小明"}}`,
			wantUID:    "u-nest",
			wantAT:     "at2",
			wantRT:     "rt2",
			wantExpiry: time.Unix(1700000100, 0),
		},
		{
			name:       "扁平毫秒时间戳",
			filename:   "flat.json",
			body:       `{"accessToken":"at3","refreshToken":"rt3","expiresAt":1700000000123,"uid":"u-flat"}`,
			wantUID:    "u-flat",
			wantAT:     "at3",
			wantRT:     "rt3",
			wantExpiry: time.UnixMilli(1700000000123),
		},
		{
			name:       "官方 CLI claudeAiOauth",
			filename:   ".credentials.json",
			body:       `{"claudeAiOauth":{"accessToken":"at4","refreshToken":"rt4","expiresAt":1700000000456}}`,
			wantAT:     "at4",
			wantRT:     "rt4",
			wantExpiry: time.UnixMilli(1700000000456),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cred, err := Parse([]byte(tc.body), tc.filename)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if tc.wantUID != "" && cred.UID != tc.wantUID {
				t.Fatalf("uid = %q, want %q", cred.UID, tc.wantUID)
			}
			if cred.AccessToken != tc.wantAT {
				t.Fatalf("access token = %q", cred.AccessToken)
			}
			if cred.RefreshToken != tc.wantRT {
				t.Fatalf("refresh token = %q", cred.RefreshToken)
			}
			if !cred.ExpiresAt.Equal(tc.wantExpiry) {
				t.Fatalf("expiry = %v, want %v", cred.ExpiresAt, tc.wantExpiry)
			}
			if cred.APIKey != tc.wantKey {
				t.Fatalf("api key = %q", cred.APIKey)
			}
			if tc.wantUID == "" && len(cred.UID) == 0 {
				t.Fatal("uid should be derived from filename or token")
			}
		})
	}
}

func TestParseRejectsEmptyCredential(t *testing.T) {
	if _, err := Parse([]byte(`{"uid":"x"}`), "x.json"); err == nil {
		t.Fatal("credential without token/api_key should be rejected")
	}
}

func TestSaveAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cred := &Credential{
		UID:          "u-save",
		Nickname:     "保存测试",
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		ExpiresAt:    time.Unix(1800000000, 0),
		APIKey:       "key-1",
	}
	path := filepath.Join(dir, FileName("u-save"))
	cred.SetPath(path)
	if err := cred.SaveAtomic(); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("file mode = %o, want 600", mode)
	}

	creds, errs := LoadDir(dir)
	if len(errs) != 0 {
		t.Fatalf("LoadDir errors: %v", errs)
	}
	if len(creds) != 1 {
		t.Fatalf("loaded %d credentials", len(creds))
	}
	got := creds[0]
	if got.UID != "u-save" || got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" || got.APIKey != "key-1" {
		t.Fatalf("round trip mismatch: %+v", got.Snapshot())
	}
	if !got.ExpiresAt.Equal(time.Unix(1800000000, 0)) {
		t.Fatalf("expiry mismatch: %v", got.ExpiresAt)
	}
}

func TestLoadDirIgnoresUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("phanthy-a.json", `{"access_token":"a","uid":"a"}`)
	write("notes.txt", "ignore me")
	write(".hidden.json", `{"access_token":"h","uid":"h"}`)
	write("broken.json", `{`)

	creds, errs := LoadDir(dir)
	if len(creds) != 1 || creds[0].UID != "a" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
	if len(errs) != 1 {
		t.Fatalf("expected exactly one parse error, got %v", errs)
	}
}

func TestExpiring(t *testing.T) {
	now := time.Unix(1000, 0)
	cred := &Credential{ExpiresAt: now.Add(5 * time.Minute)}
	if !cred.Expiring(now, 10*time.Minute) {
		t.Fatal("5m before expiry should be inside the 10m skew")
	}
	if cred.Expiring(now, time.Minute) {
		t.Fatal("5m before expiry should be outside the 1m skew")
	}
	cred = &Credential{}
	if cred.Expiring(now, time.Hour) {
		t.Fatal("unknown expiry should not trigger refresh")
	}
}
