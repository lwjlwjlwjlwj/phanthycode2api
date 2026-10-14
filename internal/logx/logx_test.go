package logx

import "testing"

func TestParseRecognizesAliases(t *testing.T) {
	cases := map[string]Level{
		"debug":   LevelDebug,
		"verbose": LevelDebug,
		"TRACE":   LevelDebug,
		"info":    LevelInfo,
		"":        LevelInfo,
		"bogus":   LevelInfo,
		"error":   LevelError,
		"warn":    LevelError,
	}
	for name, want := range cases {
		if got := Parse(name); got != want {
			t.Errorf("Parse(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSetControlsEnabled(t *testing.T) {
	defer Set("info")

	if got := Set("debug"); got != LevelDebug || !Enabled(LevelDebug) {
		t.Fatalf("debug level not enabled: got %v", got)
	}
	if got := Set("error"); got != LevelError || Enabled(LevelInfo) || Enabled(LevelDebug) {
		t.Fatalf("error level should suppress info/debug: got %v", got)
	}
	if got := Set("nonsense"); got != LevelInfo || !Enabled(LevelInfo) {
		t.Fatalf("unknown level should fall back to info: got %v", got)
	}
}
