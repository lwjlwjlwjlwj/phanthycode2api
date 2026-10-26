package keepalive

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"phanthycode2api/internal/credential"
	"phanthycode2api/internal/phanthy"
	"phanthycode2api/internal/pool"
)

func TestRefreshAllDisablesSessionDead(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"invalid_grant","message":"refresh token revoked"}}`)
	}))
	defer up.Close()

	cred := &credential.Credential{
		UID:          "dead",
		AccessToken:  "old",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(time.Minute), // 落在 30 分钟 skew 内
	}
	p, err := pool.New([]*credential.Credential{cred}, "")
	if err != nil {
		t.Fatal(err)
	}

	client := phanthy.New(phanthy.Options{BaseURL: up.URL, Timeout: 5 * time.Second})
	s := New(client, p, nil, []int{22}, 0)
	s.RefreshAll(context.Background())

	list := p.List(time.Now())
	if len(list) != 1 || !list[0].Disabled {
		t.Fatalf("session-dead account should be disabled: %+v", list)
	}
}

func TestRefreshAllSkipsHealthyToken(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer up.Close()

	cred := &credential.Credential{
		UID:         "ok",
		AccessToken: "at",
		ExpiresAt:   time.Now().Add(10 * time.Hour),
	}
	p, _ := pool.New([]*credential.Credential{cred}, "")
	client := phanthy.New(phanthy.Options{BaseURL: up.URL, Timeout: 5 * time.Second})

	New(client, p, nil, nil, 0).RefreshAll(context.Background())
	if calls != 0 {
		t.Fatalf("healthy token should not be refreshed, upstream calls = %d", calls)
	}
}

func TestProbeBalances(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/phanthy_code/models/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer at-1" {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"balance":9}}`)
	}))
	defer up.Close()

	cred := &credential.Credential{UID: "a", AccessToken: "at-1"}
	p, _ := pool.New([]*credential.Credential{cred}, "")
	client := phanthy.New(phanthy.Options{BaseURL: up.URL, Timeout: 5 * time.Second})

	New(client, p, nil, nil, 0).ProbeBalances(context.Background())

	list := p.List(time.Now())
	if list[0].Balance == nil || *list[0].Balance != 9 {
		t.Fatalf("balance = %v", list[0].Balance)
	}
}
