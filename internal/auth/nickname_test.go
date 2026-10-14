package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func writeNicknameFixture(t *testing.T, dir, uid, nickname string) {
	t.Helper()
	doc := `{"auth":{"accessToken":"tok","refreshToken":"ref","expiresAt":1},` +
		`"account":{"uid":"` + uid + `","nickname":"` + nickname + `"}}`
	if err := os.WriteFile(filepath.Join(dir, uid+".json"), []byte(doc), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// TestUniqueNicknameAppendsUIDOnCollision 同一天登录的账号默认昵称相同，重名时补 uid 后四位。
func TestUniqueNicknameAppendsUIDOnCollision(t *testing.T) {
	dir := t.TempDir()
	writeNicknameFixture(t, dir, "phanthy-1790570942", "phanthy-0928")

	got := UniqueNickname(dir, "phanthy-1790570968", "phanthy-0928")
	if got != "phanthy-0928-0968" {
		t.Fatalf("UniqueNickname = %q, want phanthy-0928-0968", got)
	}
}

// TestUniqueNicknameKeepsBaseWhenFree 没重名就保持原样。
func TestUniqueNicknameKeepsBaseWhenFree(t *testing.T) {
	dir := t.TempDir()
	writeNicknameFixture(t, dir, "phanthy-1790570942", "phanthy-0927")

	if got := UniqueNickname(dir, "phanthy-1790570968", "phanthy-0928"); got != "phanthy-0928" {
		t.Fatalf("UniqueNickname = %q, want phanthy-0928", got)
	}
}

// TestUniqueNicknameAvoidsSuffixCollision 连补后缀都撞上时继续加序号。
func TestUniqueNicknameAvoidsSuffixCollision(t *testing.T) {
	dir := t.TempDir()
	writeNicknameFixture(t, dir, "phanthy-1790570942", "phanthy-0928")
	writeNicknameFixture(t, dir, "phanthy-1790570968", "phanthy-0928-0968")

	got := UniqueNickname(dir, "phanthy-1790570968", "phanthy-0928")
	if got != "phanthy-0928-0968(2)" {
		t.Fatalf("UniqueNickname = %q, want phanthy-0928-0968(2)", got)
	}
}
