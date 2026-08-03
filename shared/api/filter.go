package ainselapishared

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter represents a single field-level filter condition applied to an event's
// Data payload (parsed as map[string]any).
type Filter struct {
	Field  string   `json:"field" yaml:"field"`
	Op     string   `json:"op" yaml:"op"`
	Value  string   `json:"value,omitempty" yaml:"value,omitempty"`
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// Match evaluates the filter against a JSON payload (map[string]any).
func (f *Filter) Match(payload map[string]any) bool {
	val, ok := resolveField(payload, f.Field)
	if !ok {
		return false
	}

	// Handle null values explicitly
	if val == nil {
		switch f.Op {
		case "eq":
			return f.Value == "" || f.Value == "null"
		case "neq":
			return f.Value != "" && f.Value != "null"
		case "contains":
			// null doesn't contain anything
			return false
		case "not-contains", "contains not":
			// null doesn't contain the value, so not-contains is true
			return true
		case "in":
			// null is not in any list
			return false
		case "not-in", "not in":
			// null is not in the list, so not-in is true
			return true
		default:
			return false
		}
	}

	// Handle array fields specially for contains/in/not-contains operators
	if arr, isArray := val.([]interface{}); isArray {
		switch f.Op {
		case "contains":
			// Check if the value exists in the array
			for _, item := range arr {
				if stringifyEqual(item, f.Value) {
					return true
				}
			}
			return false
		case "not-contains", "contains not":
			// Check that the value does NOT exist in the array
			for _, item := range arr {
				if stringifyEqual(item, f.Value) {
					return false
				}
			}
			return true
		case "in":
			// Check if any element in the array is in the Values list
			for _, item := range arr {
				itemStr := stringify(item)
				for _, v := range f.Values {
					if itemStr == v {
						return true
					}
				}
			}
			return false
		case "not-in", "not in":
			// Check that no element in the array is in the Values list
			for _, item := range arr {
				itemStr := stringify(item)
				for _, v := range f.Values {
					if itemStr == v {
						return false
					}
				}
			}
			return true
		case "eq":
			// For eq on arrays, check if the array has exactly one element matching
			if len(arr) == 1 {
				return stringifyEqual(arr[0], f.Value)
			}
			return false
		default:
			return false
		}
	}

	// Handle scalar fields
	var s string
	switch v := val.(type) {
	case string:
		s = v
	case bool:
		s = fmt.Sprintf("%t", v)
	case float64:
		s = fmt.Sprintf("%g", v)
	default:
		return false
	}

	switch f.Op {
	case "eq":
		return s == f.Value
	case "neq":
		return s != f.Value
	case "prefix":
		return strings.HasPrefix(s, f.Value)
	case "suffix":
		return strings.HasSuffix(s, f.Value)
	case "contains":
		return strings.Contains(s, f.Value)
	case "not-contains", "contains not":
		return !strings.Contains(s, f.Value)
	case "in":
		for _, v := range f.Values {
			if s == v {
				return true
			}
		}
		return false
	case "not-in", "not in":
		for _, v := range f.Values {
			if s == v {
				return false
			}
		}
		return true
	case "regex":
		matched, err := regexp.MatchString(f.Value, s)
		return err == nil && matched
	default:
		return false
	}
}

// MatchFilters evaluates all filters with AND logic. Returns true if all match
// or if filters is empty.
func MatchFilters(filters []Filter, payload map[string]any) bool {
	for _, f := range filters {
		if !f.Match(payload) {
			return false
		}
	}
	return true
}

// stringify converts any value to its string representation for comparison.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return fmt.Sprintf("%t", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}

// stringifyEqual checks if a value stringifies to the target string.
func stringifyEqual(v any, target string) bool {
	return stringify(v) == target
}

// resolveField walks a dotted field path into a nested map.
func resolveField(m map[string]any, field string) (any, bool) {
	parts := strings.Split(field, ".")
	var current any = m
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}
