package translate

import (
	"reflect"
	"testing"
)

func TestParseResp(t *testing.T) {
	text := `

<English -> 中文 (简体)>
English result
中文结果。

	`
	result := parseResp(text)
	if result == nil {
		t.Errorf("result is nil")
		return
	}
	expect := &TranslateResp{
		From:   "English",
		To:     "中文 (简体)",
		Result: []string{"English result\n中文结果。"},
	}
	if !reflect.DeepEqual(result, expect) {
		t.Errorf("bad result, expected: %+v, actual: %+v", expect, result)
	}
}
