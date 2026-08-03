package tools

import (
	"encoding/json"
	"testing"
)

func TestDefaultArgInt(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal int
		want       int
	}{
		{"absent uses default", map[string]any{}, "limit", 50, 50},
		{"present positive", map[string]any{"limit": float64(100)}, "limit", 50, 100},
		{"present zero uses default", map[string]any{"limit": float64(0)}, "limit", 50, 50},
		{"present negative uses default", map[string]any{"limit": float64(-1)}, "limit", 50, 50},
		{"wrong type uses default", map[string]any{"limit": "abc"}, "limit", 50, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultArgInt(tt.args, tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("defaultArgInt() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAnnotateJSON(t *testing.T) {
	body := []byte(`{"foo":"bar","count":42}`)
	result := annotateJSON(body, map[string]any{"truncated": true, "hint": "pass limit=N"})

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if obj["foo"] != "bar" {
		t.Errorf("existing field 'foo' changed: %v", obj["foo"])
	}
	if obj["count"] != float64(42) {
		t.Errorf("existing field 'count' changed: %v", obj["count"])
	}
	if obj["truncated"] != true {
		t.Errorf("expected truncated=true, got %v", obj["truncated"])
	}
	if obj["hint"] != "pass limit=N" {
		t.Errorf("expected hint='pass limit=N', got %v", obj["hint"])
	}
}

func TestAnnotateJSON_InvalidBody(t *testing.T) {
	body := []byte(`not json`)
	result := annotateJSON(body, map[string]any{"truncated": true})
	if string(result) != string(body) {
		t.Errorf("expected original body on parse failure, got %s", string(result))
	}
}

func TestAnnotatePageMeta(t *testing.T) {
	body := []byte(`{"items":[{"id":"a"}],"total":100,"page":1,"pageSize":50,"totalPages":2}`)
	result := annotatePageMeta(body, "more available")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if obj["truncated"] != true {
		t.Errorf("expected truncated=true (100 > 1*50), got %v", obj["truncated"])
	}
	if obj["hint"] != "more available" {
		t.Errorf("expected hint, got %v", obj["hint"])
	}
	// Existing fields preserved.
	if obj["total"] != float64(100) {
		t.Errorf("total changed: %v", obj["total"])
	}
}

func TestAnnotatePageMeta_NotTruncated(t *testing.T) {
	body := []byte(`{"items":[{"id":"a"}],"total":1,"page":1,"pageSize":50,"totalPages":1}`)
	result := annotatePageMeta(body, "more available")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if obj["truncated"] != false {
		t.Errorf("expected truncated=false (1 <= 1*50), got %v", obj["truncated"])
	}
	if _, hasHint := obj["hint"]; hasHint {
		t.Errorf("hint should not be present when not truncated")
	}
}

func TestAnnotateListWithTotal(t *testing.T) {
	body := []byte(`{"invocations":[{"id":"1"},{"id":"2"}],"total":10}`)
	result := annotateListWithTotal(body, "invocations", "pass limit=N")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if obj["truncated"] != true {
		t.Errorf("expected truncated=true (10 > 2), got %v", obj["truncated"])
	}
	if obj["hint"] != "pass limit=N" {
		t.Errorf("expected hint, got %v", obj["hint"])
	}
}

func TestAnnotateListWithTotal_NotTruncated(t *testing.T) {
	body := []byte(`{"invocations":[{"id":"1"}],"total":1}`)
	result := annotateListWithTotal(body, "invocations", "pass limit=N")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if obj["truncated"] != false {
		t.Errorf("expected truncated=false, got %v", obj["truncated"])
	}
}

func TestCapAndAnnotateArray(t *testing.T) {
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"id": i}
	}
	body, _ := json.Marshal(map[string]any{"invocations": items, "total": 100})

	result := capAndAnnotateArray(body, "invocations", 50, "pass limit=N")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	arr := obj["invocations"].([]any)
	if len(arr) != 50 {
		t.Errorf("expected 50 items, got %d", len(arr))
	}
	if obj["truncated"] != true {
		t.Errorf("expected truncated=true, got %v", obj["truncated"])
	}
	if obj["returned"] != float64(50) {
		t.Errorf("expected returned=50, got %v", obj["returned"])
	}
}

func TestCapBareArray(t *testing.T) {
	items := make([]any, 80)
	for i := range items {
		items[i] = map[string]any{"id": i}
	}
	body, _ := json.Marshal(items)

	result := capBareArray(body, 50, "pass limit=N")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	arr := obj["items"].([]any)
	if len(arr) != 50 {
		t.Errorf("expected 50 items, got %d", len(arr))
	}
	if obj["truncated"] != true {
		t.Errorf("expected truncated=true, got %v", obj["truncated"])
	}
	if obj["total"] != float64(80) {
		t.Errorf("expected total=80, got %v", obj["total"])
	}
}

func TestCapBareArray_NotTruncated(t *testing.T) {
	items := []any{map[string]any{"id": 1}, map[string]any{"id": 2}}
	body, _ := json.Marshal(items)

	result := capBareArray(body, 50, "pass limit=N")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	arr := obj["items"].([]any)
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
	if obj["truncated"] != false {
		t.Errorf("expected truncated=false, got %v", obj["truncated"])
	}
}

func TestCapBareArray_InvalidBody(t *testing.T) {
	body := []byte(`{"not":"an array"}`)
	result := capBareArray(body, 50, "hint")
	// Should return original body since it's not a bare array.
	if string(result) != string(body) {
		t.Errorf("expected original body for non-array, got %s", string(result))
	}
}

func TestCountEvents(t *testing.T) {
	// Bare array of 3 events.
	body := []byte(`[{"id":"1"},{"id":"2"},{"id":"3"}]`)
	if got := countEvents(body); got != 3 {
		t.Errorf("countEvents() = %d, want 3", got)
	}
	// Empty array.
	if got := countEvents([]byte(`[]`)); got != 0 {
		t.Errorf("countEvents(empty) = %d, want 0", got)
	}
	// Not an array.
	if got := countEvents([]byte(`{"foo":"bar"}`)); got != -1 {
		t.Errorf("countEvents(object) = %d, want -1", got)
	}
	// Invalid JSON.
	if got := countEvents([]byte(`not json`)); got != -1 {
		t.Errorf("countEvents(invalid) = %d, want -1", got)
	}
}

func TestCountLogLines(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"bare array", `[{"ts":"..."},{"ts":"..."}]`, 2},
		{"results key", `{"results":["a","b","c"]}`, 3},
		{"lines key", `{"lines":["a"]}`, 1},
		{"logs key", `{"logs":[]}`, 0},
		{"loki-style", `{"data":{"result":[{"values":[[1,"a"],[2,"b"]]},{"values":[[3,"c"]]}]}}`, 3},
		{"loki-style no values", `{"data":{"result":[{"metric":"x"},{"metric":"y"}]}}`, 2},
		{"no recognizable shape", `{"foo":"bar"}`, -1},
		{"invalid json", `not json`, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countLogLines([]byte(tt.body))
			if got != tt.want {
				t.Errorf("countLogLines() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAnnotatePageMeta_FallbackWithItems(t *testing.T) {
	// When page info is missing but items array is present, compare total
	// against len(items) instead of just total > 0.
	body := []byte(`{"items":[{"id":"a"},{"id":"b"}],"total":2}`)
	result := annotatePageMeta(body, "more available")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	// total=2, len(items)=2, so not truncated.
	if obj["truncated"] != false {
		t.Errorf("expected truncated=false (total=2 == len(items)=2), got %v", obj["truncated"])
	}
	if _, hasHint := obj["hint"]; hasHint {
		t.Errorf("hint should not be present when not truncated")
	}
}

func TestAnnotatePageMeta_FallbackWithItemsTruncated(t *testing.T) {
	// total=10 but only 2 items returned — should be truncated.
	body := []byte(`{"items":[{"id":"a"},{"id":"b"}],"total":10}`)
	result := annotatePageMeta(body, "more available")

	var obj map[string]any
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if obj["truncated"] != true {
		t.Errorf("expected truncated=true (total=10 > len(items)=2), got %v", obj["truncated"])
	}
	if obj["hint"] != "more available" {
		t.Errorf("expected hint, got %v", obj["hint"])
	}
}
