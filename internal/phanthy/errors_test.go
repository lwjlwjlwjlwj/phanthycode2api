package phanthy

import (
	"net/http"
	"testing"
)

func TestClassifyPriority(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   Kind
	}{
		{"402", http.StatusPaymentRequired, `{}`, KindHardCredit},
		{"401 但余额文案优先", http.StatusUnauthorized, `{"error":{"message":"insufficient credit"}}`, KindHardCredit},
		{"中文余额文案", http.StatusForbidden, `余额不足，请充值`, KindHardCredit},
		{"401", http.StatusUnauthorized, `{"error":{"code":"missing_login"}}`, KindSessionDead},
		{"403 登录态文案", http.StatusForbidden, `{"error":{"message":"OAuth token has been revoked"}}`, KindSessionDead},
		{"429", http.StatusTooManyRequests, ``, KindSoftRate},
		{"404", http.StatusNotFound, ``, KindNotFound},
		{"503", http.StatusServiceUnavailable, ``, KindServer},
		{"400", http.StatusBadRequest, `{"error":{"code":"invalid_request"}}`, KindClient},
		{"422", http.StatusUnprocessableEntity, ``, KindClient},
		{"200", http.StatusOK, `{}`, KindNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.status, []byte(tc.body)); got != tc.want {
				t.Fatalf("Classify(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestParseEnvelope(t *testing.T) {
	code, message := ParseEnvelope([]byte(`{"error":{"code":"missing_login","message":"need login"}}`))
	if code != "missing_login" || message != "need login" {
		t.Fatalf("unexpected envelope: %q %q", code, message)
	}
	if code, message := ParseEnvelope([]byte(`not json`)); code != "" || message != "" {
		t.Fatalf("invalid body should yield empty fields, got %q %q", code, message)
	}
}
