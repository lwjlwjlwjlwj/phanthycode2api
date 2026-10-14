package upstream

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

// denialSSE 复刻上游拒绝服务时的真实响应：HTTP 状态是 200，
// 错误只存在于事件流里，正文合计不到 200 字节。
const denialEvent = `event: error
data: {"type":"error","error":{"type":"api_error","code":"upstream_permission_denied","message":"Model service access was denied. Choose another model or contact support."}}`

// TestAggregate_UpstreamErrorEvent 回归：上游用 200 + event: error 拒绝，
// 必须返回错误，而不是被当成一次「正常的空回复」（管理台记成 200 + tokens 0/0）。
func TestAggregate_UpstreamErrorEvent(t *testing.T) {
	_, err := Aggregate(strings.NewReader(denialEvent), "phanthy-pro")
	if err == nil {
		t.Fatal("Aggregate 必须返回错误，而不是空回复")
	}
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T %v, want *StreamError", err, err)
	}
	if se.Kind != ErrModelDenied {
		t.Errorf("Kind = %v, want %v（套餐未授权模型属请求侧问题）", se.Kind, ErrModelDenied)
	}
	if se.Event == nil || se.Event.Code != "upstream_permission_denied" {
		t.Errorf("Event.Code = %v", se.Event)
	}
}

// TestStream_UpstreamErrorEventBeforeContent 拒绝发生在任何 chunk 之前时，
// 调用方还能改写 HTTP 状态码（Committed=false），不能补 finish_reason。
func TestStream_UpstreamErrorEventBeforeContent(t *testing.T) {
	rec := httptest.NewRecorder()
	_, _, err := Stream(rec, strings.NewReader(denialEvent), "phanthy-pro")
	if err == nil {
		t.Fatal("Stream 必须返回错误")
	}
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T %v, want *StreamError", err, err)
	}
	if se.Committed {
		t.Error("Committed = true，但尚未写过任何 chunk")
	}
	body := rec.Body.String()
	if strings.Contains(body, `"finish_reason":"stop"`) {
		t.Errorf("被拒绝时不得补 finish_reason=stop，body=%s", body)
	}
}

// TestStream_UpstreamErrorEventAfterContent 拒绝发生在正文之后时，
// 状态码已定局（Committed=true），必须把错误如实写进流里并让调用方知道失败。
func TestStream_UpstreamErrorEventAfterContent(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"phanthy-pro","usage":{"input_tokens":7,"output_tokens":0}}}

` + denialEvent

	rec := httptest.NewRecorder()
	_, _, err := Stream(rec, strings.NewReader(sse), "phanthy-pro")
	var se *StreamError
	if !errors.As(err, &se) {
		t.Fatalf("err = %T %v, want *StreamError", err, err)
	}
	if !se.Committed {
		t.Error("Committed = false，但 message_start 已经写过 chunk")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "upstream_permission_denied") {
		t.Errorf("流里应带上游错误码，body=%s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Errorf("流应以 DONE 收尾，body=%s", body)
	}
}

// TestAggregate_PartialUsageKeepsInputTokens 回归：message_delta 只带
// output_tokens 时，不得用零值把 message_start 里的 input_tokens 抹掉
// （这正是管理台出现 tokens=0/<out> 的原因）。
func TestAggregate_PartialUsageKeepsInputTokens(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"phanthy-pro","usage":{"input_tokens":32,"output_tokens":0}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":19}}

data: [DONE]`

	resp, err := Aggregate(strings.NewReader(sse), "")
	if err != nil {
		t.Fatalf("Aggregate: %v", err)
	}
	in, out := UsageTotals(resp)
	if in != 32 || out != 19 {
		t.Errorf("usage = %d/%d, want 32/19", in, out)
	}
}

func TestClassifyStreamErr(t *testing.T) {
	cases := []struct {
		code, msg string
		want      ErrKind
	}{
		{"upstream_permission_denied", "Model service access was denied.", ErrModelDenied},
		{"permission_denied", "", ErrModelDenied},
		{"invalid_request", "Model service access was denied.", ErrModelDenied},
		{"", "积分不足", ErrHardCredit},
		{"", "session expired", ErrSessionDead},
		{"", "upstream overloaded, retry later", ErrServer},
		{"invalid_request", "Request body must be JSON.", ErrClient},
	}
	for _, c := range cases {
		if got := ClassifyStreamErr(c.code, c.msg); got != c.want {
			t.Errorf("ClassifyStreamErr(%q, %q) = %v, want %v", c.code, c.msg, got, c.want)
		}
	}
}
