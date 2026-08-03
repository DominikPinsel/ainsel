package tools

import (
	"encoding/json"
)

// defaultArgInt returns the integer value of a named argument from the MCP
// request, or defaultVal if the argument is absent or non-positive.
func defaultArgInt(args map[string]any, key string, defaultVal int) int {
	if v, ok := args[key].(float64); ok && v > 0 {
		return int(v)
	}
	return defaultVal
}

// annotateJSON parses body as a JSON object, merges in the additions, and
// re-marshals. If body is not a JSON object the original bytes are returned
// unchanged (graceful degradation — the tool still works, just without
// annotation).
func annotateJSON(body []byte, additions map[string]any) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	for k, v := range additions {
		obj[k] = v
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// annotatePageMeta adds "truncated" and "hint" fields to a hub PageMeta
// envelope ({items:[...], total:N, page:N, pageSize:N, totalPages:N}).
// It reads the existing total/page/pageSize/totalPages to determine whether
// more data is available. If the response is not a PageMeta object the
// original body is returned unchanged.
func annotatePageMeta(body []byte, hint string) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	total, hasTotal := obj["total"].(float64)
	page, hasPage := obj["page"].(float64)
	pageSize, hasPageSize := obj["pageSize"].(float64)

	truncated := false
	if hasTotal && hasPage && hasPageSize && pageSize > 0 {
		truncated = total > page*pageSize
	} else if hasTotal {
		// Fallback: if we have total but not page info, compare total
		// against the items array length if available, otherwise fall
		// back to total > 0.
		if items, ok := obj["items"].([]any); ok {
			truncated = total > float64(len(items))
		} else {
			truncated = total > 0
		}
	}

	obj["truncated"] = truncated
	if truncated {
		obj["hint"] = hint
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// annotateListWithTotal adds "truncated" and "hint" to a response shaped
// like {"keyName": [...], "total": N}. The returned count is len(array).
// truncated is true when total > returned.
func annotateListWithTotal(body []byte, keyName string, hint string) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	total, hasTotal := obj["total"].(float64)
	var returned float64
	if arr, ok := obj[keyName].([]any); ok {
		returned = float64(len(arr))
	}
	truncated := false
	if hasTotal {
		truncated = total > returned
	}
	obj["truncated"] = truncated
	if truncated {
		obj["hint"] = hint
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// capAndAnnotateArray parses body as a JSON object containing an array under
// keyName, slices it to maxItems, and adds "truncated", "returned", and
// "hint" fields. If the body is not the expected shape, the original bytes
// are returned unchanged.
func capAndAnnotateArray(body []byte, keyName string, maxItems int, hint string) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	arr, ok := obj[keyName].([]any)
	if !ok {
		return body
	}
	total := len(arr)
	if total > maxItems {
		obj[keyName] = arr[:maxItems]
		obj["truncated"] = true
		obj["returned"] = maxItems
		obj["total"] = total
		obj["hint"] = hint
	} else {
		obj["truncated"] = false
		obj["returned"] = total
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// countEvents attempts to count the number of events in a JSON response.
// The hub's /api/v1/queue/recent endpoint returns a bare JSON array of event
// objects. If the body is not a JSON array, -1 is returned.
func countEvents(body []byte) int {
	var arr []any
	if err := json.Unmarshal(body, &arr); err != nil {
		return -1
	}
	return len(arr)
}

// countLogLines attempts to count the number of log lines in a JSON response.
// The hub's log endpoint may return various shapes: a bare array, or an object
// with a "results", "lines", "logs", or "data.result" array. Returns -1 if
// the count cannot be determined.
func countLogLines(body []byte) int {
	// Try bare array first.
	var arr []any
	if err := json.Unmarshal(body, &arr); err == nil {
		return len(arr)
	}
	// Try object with common array keys.
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return -1
	}
	for _, key := range []string{"results", "lines", "logs", "entries"} {
		if a, ok := obj[key].([]any); ok {
			return len(a)
		}
	}
	// Try Loki-style {"data": {"result": [...]}} or {"data": {"result": [{"values": [...]}]}}.
	if data, ok := obj["data"].(map[string]any); ok {
		if result, ok := data["result"].([]any); ok {
			// If each result has a "values" array, sum them.
			total := 0
			found := false
			for _, r := range result {
				if rm, ok := r.(map[string]any); ok {
					if vals, ok := rm["values"].([]any); ok {
						total += len(vals)
						found = true
					}
				}
			}
			if found {
				return total
			}
			// Otherwise count the result entries themselves.
			return len(result)
		}
	}
	return -1
}

// capBareArray parses body as a bare JSON array, slices it to maxItems, and
// wraps it as {"items": [...], "truncated": bool, "returned": N, "hint": ...}.
// If the body is not a JSON array, the original bytes are returned unchanged.
func capBareArray(body []byte, maxItems int, hint string) []byte {
	var arr []any
	if err := json.Unmarshal(body, &arr); err != nil {
		return body
	}
	total := len(arr)
	truncated := total > maxItems
	if truncated {
		arr = arr[:maxItems]
	}
	out := map[string]any{
		"items":     arr,
		"truncated": truncated,
		"returned":  len(arr),
	}
	if truncated {
		out["total"] = total
		out["hint"] = hint
	}
	result, err := json.Marshal(out)
	if err != nil {
		return body
	}
	return result
}
