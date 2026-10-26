package phanthy

import "testing"

func TestExtractBalance(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *int64
	}{
		{"顶层 balance", `{"balance":123}`, ptr(123)},
		{"data 下 balance", `{"data":{"balance":456}}`, ptr(456)},
		{"credit 字符串", `{"credit":"789"}`, ptr(789)},
		{"remaining_credits 浮点", `{"remaining_credits":12.0}`, ptr(12)},
		{"quota 在 data", `{"data":{"quota":"0"}}`, ptr(0)},
		{"无余额字段", `{"data":{"models":[]}}`, nil},
		{"非法 JSON", `not json`, nil},
		{"非数字值", `{"balance":"unknown"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractBalance([]byte(tc.body))
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %d", *got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("want %d, got %v", *tc.want, got)
			}
		})
	}
}

func ptr(v int64) *int64 { return &v }
