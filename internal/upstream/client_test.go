package upstream

import (
	"net/http"
	"testing"
)

func TestClassify_ModelDenied(t *testing.T) {
	// 上游对套餐未开放的模型返回 403 model_not_allowed：
	// 这是请求侧问题，必须归到 ErrModelDenied，不能当成账号故障。
	body := `{"error":{"code":"model_not_allowed","message":"Model is not allowed for this plan."}}`
	if got := Classify(http.StatusForbidden, body); got != ErrModelDenied {
		t.Errorf("Classify(403, model_not_allowed) = %v, want %v", got, ErrModelDenied)
	}
}

func TestClassify_ForbiddenOtherwiseIsClient(t *testing.T) {
	// 其他 403（不含模型关键词）仍按普通 4xx 处理。
	if got := Classify(http.StatusForbidden, `{"error":{"code":"forbidden"}}`); got != ErrClient {
		t.Errorf("Classify(403, forbidden) = %v, want %v", got, ErrClient)
	}
}

// TestClassify_ModelDeniedOnNonForbiddenStatus 回归：同类拒绝不只出现在 403。
// 上游还用 424 + upstream_permission_denied 承载同一件事，漏判会把健康账号冷却掉。
func TestClassify_ModelDeniedOnNonForbiddenStatus(t *testing.T) {
	body := `{"error":{"code":"upstream_permission_denied","message":"Model service access was denied. Choose another model or contact support."}}`
	if got := Classify(424, body); got != ErrModelDenied {
		t.Errorf("Classify(424, upstream_permission_denied) = %v, want %v", got, ErrModelDenied)
	}
}

// TestClassify_WAFBlockIsNotModelDenied 回归：边缘节点拦截页不能当成「模型未授权」，
// 否则会把「换个号重试就好」误判成「这个模型不可用」，让调用方放弃重试。
func TestClassify_WAFBlockIsNotModelDenied(t *testing.T) {
	body := `<html><head><title>Access denied</title></head><body>Access denied</body></html>`
	if got := Classify(http.StatusForbidden, body); got == ErrModelDenied {
		t.Errorf("Classify(403, WAF page) = %v，不能判成模型未授权", got)
	}
}

func TestClassify_KnownKinds(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   ErrKind
	}{
		{http.StatusPaymentRequired, `{}`, ErrHardCredit},
		{http.StatusOK, `{"error":{"message":"积分不足"}}`, ErrHardCredit},
		{http.StatusTooManyRequests, `{}`, ErrSoftRate},
		{http.StatusUnauthorized, `{}`, ErrSessionDead},
		{http.StatusNotFound, `{}`, ErrNotFound},
		{http.StatusInternalServerError, `{}`, ErrServer},
		{http.StatusBadRequest, `{}`, ErrClient},
		{http.StatusOK, `{}`, ErrNone},
	}
	for _, c := range cases {
		if got := Classify(c.status, c.body); got != c.want {
			t.Errorf("Classify(%d, %q) = %v, want %v", c.status, c.body, got, c.want)
		}
	}
}
