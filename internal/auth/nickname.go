package auth

import "fmt"

// UniqueNickname 返回一个不与 dir 下既有账号重名的昵称。
// 默认昵称是按日期生成的（phanthy-0928），同一天登录多个账号就会撞名，
// 列表里根本分不清；重名时补上 uid 后四位，仍然冲突再依次加序号。
func UniqueNickname(dir, uid, base string) string {
	taken := map[string]bool{}
	if existing, err := LoadDir(dir); err == nil {
		for _, a := range existing {
			if a.Nickname != "" {
				taken[a.Nickname] = true
			}
		}
	}
	if !taken[base] {
		return base
	}
	suffix := uid
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	name := base + "-" + suffix
	for i := 2; taken[name]; i++ {
		name = fmt.Sprintf("%s-%s(%d)", base, suffix, i)
	}
	return name
}
