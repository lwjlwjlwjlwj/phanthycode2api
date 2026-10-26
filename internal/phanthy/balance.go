package phanthy

import (
	"encoding/json"
	"strconv"
	"strings"
)

// balanceKeys 是余额字段的候选键。成功 schema 属于 BLOCKED-CRED，
// 因此按逆向报告的防御性策略遍历多种命名。
var balanceKeys = []string{
	"credit",
	"credits",
	"balance",
	"points",
	"quota",
	"remaining",
	"remaining_credits",
	"remaining_credit",
	"credit_balance",
}

// ExtractBalance 从模型统计响应中提取余额；无法确定时返回 nil。
// 先查顶层，再查 data 对象（两种布局在逆向报告中都被列为可能）。
func ExtractBalance(body []byte) *int64 {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil
	}
	if v := findBalance(root); v != nil {
		return v
	}
	if raw, ok := root["data"]; ok {
		var data map[string]json.RawMessage
		if err := json.Unmarshal(raw, &data); err == nil {
			if v := findBalance(data); v != nil {
				return v
			}
		}
	}
	return nil
}

func findBalance(obj map[string]json.RawMessage) *int64 {
	for _, key := range balanceKeys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		if v, ok := numericValue(raw); ok {
			return &v
		}
	}
	return nil
}

// numericValue 接受整数、浮点数与数字字符串；非数字返回 false。
func numericValue(raw json.RawMessage) (int64, bool) {
	var num json.Number
	if err := json.Unmarshal(raw, &num); err == nil {
		if v, err := num.Int64(); err == nil {
			return v, true
		}
		if f, err := num.Float64(); err == nil {
			return int64(f), true
		}
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		str = strings.TrimSpace(str)
		if str == "" {
			return 0, false
		}
		if v, err := strconv.ParseInt(str, 10, 64); err == nil {
			return v, true
		}
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}
