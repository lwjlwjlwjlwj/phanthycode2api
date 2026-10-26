package pool

import (
	"testing"
	"time"

	"phanthycode2api/internal/auth"
)

func mkAuth(uid, apiKey string) *auth.Auth {
	return &auth.Auth{UID: uid, APIKey: apiKey}
}

func TestPick_LRU_Rotates(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	p.Add(mkAuth("a2", "k2"))

	// 未使用时两个 lastUsed 均为零 → 按 uid 取 a1
	if got := p.Pick(); got == nil || got.UID != "a1" {
		t.Fatalf("first pick = %v, want a1 (uid tie-break)", got)
	}
	p.MarkUsed("a1")

	// a1 已用 → 轮转到 a2
	if got := p.Pick(); got == nil || got.UID != "a2" {
		t.Fatalf("second pick = %v, want a2 (LRU rotation)", got)
	}
	p.MarkUsed("a2")

	// 全部用过 → 回到最早使用的 a1
	if got := p.Pick(); got == nil || got.UID != "a1" {
		t.Fatalf("third pick = %v, want a1 (least-recently-used)", got)
	}
}

func TestPick_BalanceZero_Deprioritized(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	p.SetBalance("a1", 0)
	p.Add(mkAuth("a2", "k2"))

	// a1 余额为 0 已知 → 应跳过，先取 a2
	if got := p.Pick(); got == nil || got.UID != "a2" {
		t.Fatalf("pick = %v, want a2 (zero-balance deprioritized)", got)
	}
}

func TestSetBalance_Persists(t *testing.T) {
	dir := t.TempDir()
	fp := dir + "/state.json"

	p := New(fp)
	p.Add(mkAuth("a1", "k1"))
	p.SetBalance("a1", 42)

	p2 := New(fp)
	st, ok := p2.Status("a1")
	if !ok {
		t.Fatal("account not restored")
	}
	if st.Balance == nil || *st.Balance != 42 {
		t.Errorf("restored balance = %v, want 42", st.Balance)
	}
}

func TestPickExcluding_SkipsTried(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	p.Add(mkAuth("a2", "k2"))

	got := p.PickExcluding(map[string]bool{"a1": true})
	if got == nil || got.UID != "a2" {
		t.Errorf("pick = %v, want a2", got)
	}
}

func TestCooldown_MakesUnhealthy(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	p.Cooldown("a1", CoolSoft, time.Hour, "test")

	if got := p.Pick(); got != nil {
		t.Errorf("pick after cooldown = %v, want nil", got)
	}
}

func TestNoteError_Threshold(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	const threshold = 3
	p.NoteError("a1", threshold, time.Hour)
	p.NoteError("a1", threshold, time.Hour)
	if got := p.Pick(); got == nil {
		t.Error("pick before threshold hit should succeed")
	}
	p.NoteError("a1", threshold, time.Hour)
	if got := p.Pick(); got != nil {
		t.Error("pick after threshold should be cooling")
	}
	st, _ := p.Status("a1")
	if st.Reason != "consecutive errors" {
		t.Errorf("reason = %q", st.Reason)
	}
}

func TestNoteSuccess_ResetsErrors(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	const threshold = 3
	p.NoteError("a1", threshold, time.Hour)
	p.NoteError("a1", threshold, time.Hour)
	p.NoteSuccess("a1")
	p.NoteError("a1", threshold, time.Hour)
	if got := p.Pick(); got == nil {
		t.Error("error count should have reset after success")
	}
}

func TestDisable(t *testing.T) {
	p := New("")
	p.Add(mkAuth("a1", "k1"))
	p.Disable("a1", "session dead")
	if got := p.Pick(); got != nil {
		t.Error("disabled account should not be picked")
	}
}

func TestStatePersistence(t *testing.T) {
	dir := t.TempDir()
	fp := dir + "/state.json"

	p := New(fp)
	p.Add(mkAuth("a1", "k1"))
	p.Disable("a1", "bad")

	// 新池从同一 state 文件恢复
	p2 := New(fp)
	st, ok := p2.Status("a1")
	if !ok {
		t.Fatal("account not restored")
	}
	if !st.Disabled || st.Reason != "bad" {
		t.Errorf("restored state = %+v", st)
	}
}
