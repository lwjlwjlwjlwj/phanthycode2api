package pool

import (
	"path/filepath"
	"testing"
	"time"

	"phanthycode2api/internal/credential"
)

func cred(uid string) *credential.Credential {
	return &credential.Credential{UID: uid, AccessToken: "token-" + uid}
}

func TestPickLRU(t *testing.T) {
	base := time.Unix(10000, 0)
	creds := []*credential.Credential{cred("c"), cred("a"), cred("b")}
	p, err := New(creds, "")
	if err != nil {
		t.Fatal(err)
	}
	p.MarkUsed("a", base.Add(-3*time.Minute))
	p.MarkUsed("b", base.Add(-2*time.Minute))
	p.MarkUsed("c", base.Add(-time.Minute))

	if got := p.Pick(base); got == nil || got.UID != "a" {
		t.Fatalf("LRU should pick least recently used account, got %+v", got)
	}
}

func TestBalanceZeroIsDemoted(t *testing.T) {
	base := time.Unix(10000, 0)
	p, _ := New([]*credential.Credential{cred("a"), cred("b")}, "")
	p.MarkUsed("a", base.Add(-time.Hour))
	p.MarkUsed("b", base.Add(-time.Minute))

	p.SetBalance("a", int64Ptr(0))
	if got := p.Pick(base); got == nil || got.UID != "b" {
		t.Fatalf("zero-balance account should be demoted, got %+v", got)
	}
	p.SetBalance("a", int64Ptr(10))
	if got := p.Pick(base); got == nil || got.UID != "a" {
		t.Fatalf("restored balance should make account eligible again, got %+v", got)
	}
}

func TestCooldownExcludesAccount(t *testing.T) {
	now := time.Unix(10000, 0)
	p, _ := New([]*credential.Credential{cred("a")}, "")

	p.Cooldown("a", CooldownSoft, time.Minute, "rate", now)
	if got := p.Pick(now); got != nil {
		t.Fatalf("cooling account should not be picked, got %+v", got)
	}
	if got := p.Pick(now.Add(61 * time.Second)); got == nil {
		t.Fatal("account should be eligible after cooldown expires")
	}
}

func TestDisableIsPermanent(t *testing.T) {
	now := time.Unix(10000, 0)
	p, _ := New([]*credential.Credential{cred("a")}, "")
	p.Disable("a", "session_dead")
	if got := p.Pick(now.Add(1000 * time.Hour)); got != nil {
		t.Fatal("disabled account must never be picked")
	}
}

func TestNoteErrorThreshold(t *testing.T) {
	now := time.Unix(10000, 0)
	p, _ := New([]*credential.Credential{cred("a")}, "")

	if p.NoteError("a", 3, time.Minute, now) {
		t.Fatal("first error should not trigger cooldown")
	}
	p.NoteSuccess("a")
	if p.NoteError("a", 3, time.Minute, now) {
		t.Fatal("counter should have been reset")
	}
	p.NoteError("a", 3, time.Minute, now)
	if cooled := p.NoteError("a", 3, time.Minute, now); !cooled {
		t.Fatal("third consecutive error should trigger cooldown")
	}
	if got := p.Pick(now); got != nil {
		t.Fatal("account should be cooling after threshold")
	}
}

func TestListSortedAndSanitised(t *testing.T) {
	now := time.Unix(10000, 0)
	p, _ := New([]*credential.Credential{cred("z"), cred("a")}, "")
	p.Cooldown("a", CooldownSoft, time.Hour, "test", now)

	list := p.List(now)
	if len(list) != 2 || list[0].UID != "a" || list[1].UID != "z" {
		t.Fatalf("list should be sorted by uid: %+v", list)
	}
	if !list[0].Cooling || list[0].Until == nil || list[0].Reason != "test" {
		t.Fatalf("cooling fields missing: %+v", list[0])
	}
	if list[0].HasAPIKey {
		t.Fatal("account without api key reported has_api_key")
	}
}

func TestStatePersistence(t *testing.T) {
	now := time.Unix(10000, 0)
	stateFile := filepath.Join(t.TempDir(), "state.json")
	creds := []*credential.Credential{cred("a"), cred("b")}

	p, err := New(creds, stateFile)
	if err != nil {
		t.Fatal(err)
	}
	p.Cooldown("a", CooldownHard, 12*time.Hour, "no credit", now)
	p.SetBalance("a", int64Ptr(0))
	p.MarkUsed("b", now)
	p.Disable("b", "session_dead")

	// 新实例只加载凭证，状态应从 state_file 恢复。
	p2, err := New(creds, stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := p2.Pick(now); got != nil {
		t.Fatalf("both accounts should be unavailable after reload, got %+v", got)
	}
	list := p2.List(now)
	if list[0].UID != "a" || !list[0].Cooling || list[0].Balance == nil || *list[0].Balance != 0 {
		t.Fatalf("cooldown/balance not persisted: %+v", list[0])
	}
	if !list[1].Disabled || list[1].Reason != "session_dead" {
		t.Fatalf("disabled state not persisted: %+v", list[1])
	}
}

func TestEmptyPoolReturnsNil(t *testing.T) {
	p, err := New(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Len() != 0 || p.Pick(time.Now()) != nil {
		t.Fatal("empty pool should return nil")
	}
}

func int64Ptr(v int64) *int64 { return &v }
