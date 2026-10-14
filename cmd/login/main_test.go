package main

import "testing"

// TestExtractCode 覆盖用户实际会粘贴的各种形态。
// 回归背景：本函数曾定义了却从未被调用，导致带 #p2a-login 的输入被原样
// 发给 /oauth/token，稳定返回 invalid_grant。
func TestExtractCode(t *testing.T) {
	const code = "LaKU15K2Qj2HoBysON7cPsfGtHvh38Egvm95KR6qwYU"

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"裸 code", code, code},
		{"裸 code 带 fragment", code + "#p2a-login", code},
		{"裸 code 带空白", "  " + code + "  ", code},
		{"带 fragment 与空白", " " + code + "#p2a-login ", code},
		{"code= 前缀", "code=" + code, code},
		{"?code= 前缀", "?code=" + code, code},
		{"前缀带 fragment", "code=" + code + "#p2a-login", code},
		{
			"完整回调 URL",
			"https://code.phanthy.com/oauth/code/success?code=" + code + "#p2a-login",
			code,
		},
		{
			"完整 URL 带额外参数",
			"https://code.phanthy.com/oauth/code/success?state=p2a-login&code=" + code,
			code,
		},
		{"空输入", "", ""},
		{"仅 fragment", "#p2a-login", ""},
		{"前后换行", "\n" + code + "\n", code},
	}

	for _, c := range cases {
		if got := extractCode(c.in); got != c.want {
			t.Errorf("%s: extractCode(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestMaskCode 保证打印的是脱敏值，不会把完整 code 写进日志。
func TestMaskCode(t *testing.T) {
	const code = "LaKU15K2Qj2HoBysON7cPsfGtHvh38Egvm95KR6qwYU"
	got := maskCode(code)
	if got == code {
		t.Errorf("maskCode 未脱敏: %q", got)
	}
	if got != "LaKU…qwYU" {
		t.Errorf("maskCode(%q) = %q, want %q", code, got, "LaKU…qwYU")
	}
	// 短字符串原样返回，避免越界。
	if s := maskCode("short"); s != "short" {
		t.Errorf("maskCode(short) = %q, want %q", s, "short")
	}
}
