package scheduler

import "testing"

func TestParseBalance_Variants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int64
		ok   bool
	}{
		{"top-level credit", `{"credit":123}`, 123, true},
		{"top-level balance", `{"balance":4500}`, 4500, true},
		{"top-level remaining", `{"remaining":99}`, 99, true},
		{"data.credit", `{"data":{"credit":7}}`, 7, true},
		{"data.balance", `{"data":{"balance":888}}`, 888, true},
		{"nested data.remaining_credits", `{"data":{"remaining_credits":"120"}}`, 120, true},
		{"float", `{"credit":12.9}`, 12, true},
		{"string number", `{"quota":"55"}`, 55, true},
		{"no balance key", `{"models":["a"]}`, 0, false},
		{"object data no balance", `{"data":{"models":{}}}`, 0, false},
		{"unknown shape", `[1,2,3]`, 0, false},
		{"junk", `not json`, 0, false},
	}
	for _, c := range cases {
		got, ok := ParseBalance([]byte(c.raw))
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: ParseBalance(%s) = (%d,%v), want (%d,%v)",
				c.name, c.raw, got, ok, c.want, c.ok)
		}
	}
}
