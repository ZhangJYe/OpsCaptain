package contextcompression

import (
	"encoding/json"
	"testing"
)

func TestCompressJSON_PreservesHeadAndTail(t *testing.T) {
	items := make([]map[string]string, 10)
	for i := range items {
		items[i] = map[string]string{"id": string(rune('A' + i)), "value": "ok"}
	}
	data, _ := json.Marshal(items)

	result, ok := compressJSON(string(data), "", 3, 2)
	if !ok {
		t.Fatal("expected ok=true")
	}

	var resultItems []map[string]string
	if err := json.Unmarshal([]byte(result), &resultItems); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	// 应保留前3+后2=5项（没有error项被额外保留）
	if len(resultItems) < 5 {
		t.Errorf("expected at least 5 items, got %d", len(resultItems))
	}

	// 首项应保留
	if resultItems[0]["id"] != "A" {
		t.Errorf("first item should be A, got %s", resultItems[0]["id"])
	}
}

func TestCompressJSON_PreservesErrorItems(t *testing.T) {
	items := []map[string]string{
		{"id": "1", "status": "ok"},
		{"id": "2", "status": "ok"},
		{"id": "3", "status": "error", "message": "timeout"},
		{"id": "4", "status": "ok"},
		{"id": "5", "status": "ok"},
		{"id": "6", "status": "ok"},
		{"id": "7", "status": "warning", "message": "high latency"},
		{"id": "8", "status": "ok"},
	}
	data, _ := json.Marshal(items)

	result, ok := compressJSON(string(data), "", 2, 1)
	if !ok {
		t.Fatal("expected ok=true")
	}

	// error 和 warning 项应被保留
	if !containsSubstr(result, `"error"`) {
		t.Error("error item should be preserved")
	}
	if !containsSubstr(result, `"warning"`) {
		t.Error("warning item should be preserved")
	}
}

func TestCompressJSON_PreservesQueryHits(t *testing.T) {
	items := []map[string]string{
		{"id": "1", "data": "normal operation"},
		{"id": "2", "data": "paymentservice 503 error"},
		{"id": "3", "data": "normal operation"},
		{"id": "4", "data": "normal operation"},
		{"id": "5", "data": "normal operation"},
		{"id": "6", "data": "normal operation"},
		{"id": "7", "data": "normal operation"},
		{"id": "8", "data": "normal operation"},
	}
	data, _ := json.Marshal(items)

	result, ok := compressJSON(string(data), "paymentservice 503", 2, 1)
	if !ok {
		t.Fatal("expected ok=true")
	}

	if !containsSubstr(result, "paymentservice") {
		t.Error("query-hit item should be preserved")
	}
}

func TestCompressJSON_InvalidJSON(t *testing.T) {
	result, ok := compressJSON("not json", "", 3, 2)
	if ok {
		t.Error("expected ok=false for invalid JSON")
	}
	if result != "" {
		t.Error("expected empty result for invalid JSON")
	}
}

func TestCompressJSON_SmallArray(t *testing.T) {
	items := []map[string]string{
		{"id": "1"}, {"id": "2"}, {"id": "3"}, {"id": "4"},
	}
	data, _ := json.Marshal(items)

	// 4项 <= 3+2=5，不需要压缩
	result, ok := compressJSON(string(data), "", 3, 2)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if result != string(data) {
		t.Error("small array should not be compressed")
	}
}

func TestCompressJSON_EmptyArray(t *testing.T) {
	result, ok := compressJSON("[]", "", 3, 2)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if result != "[]" {
		t.Errorf("empty array should remain empty, got %s", result)
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
