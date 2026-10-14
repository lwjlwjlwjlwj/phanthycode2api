package admin

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash := HashPassword("secret")
	if !VerifyPassword("secret", hash) {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword("wrong", hash) {
		t.Fatal("wrong password should not verify")
	}
}

func TestKeyLifecycleAndLookup(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "admin"))
	if err != nil {
		t.Fatal(err)
	}
	plain, created, err := store.CreateKey("test", 1, 0, []string{"model-a"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || !strings.HasPrefix(plain, "p2a-") {
		t.Fatalf("unexpected key: id=%s plain=%s", created.ID, plain)
	}
	if key, ok := store.LookupKey(plain, "model-a"); !ok || key.ID != created.ID {
		t.Fatal("valid key should be found")
	}
	if _, ok := store.LookupKey(plain, "model-b"); ok {
		t.Fatal("model allowlist should reject model-b")
	}
	if _, ok := store.LookupKey(plain, "model-a"); ok {
		t.Fatal("exhausted key should be rejected")
	}
}

func TestLogsAndStats(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "admin"))
	if err != nil {
		t.Fatal(err)
	}
	store.AddLog(LogEntry{Time: 1, Status: 200, KeyID: "k1", KeyName: "test"})
	store.AddLog(LogEntry{Time: 2, Status: 500, KeyID: "k1", KeyName: "test"})
	if got := len(store.ListLogs(10, "")); got != 2 {
		t.Fatalf("want 2 logs, got %d", got)
	}
	stats := store.Stats()
	if stats["total"] != 2 || stats["errors"] != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	store.ClearLogs()
	if got := len(store.ListLogs(10, "")); got != 0 {
		t.Fatalf("want empty logs, got %d", got)
	}
}
