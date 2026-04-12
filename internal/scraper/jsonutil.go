package scraper

import "github.com/bytedance/gg/gconv"

// StringVal extracts a string from a map, trying multiple keys in order.
func StringVal(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// SliceVal extracts a []any from a map key.
func SliceVal(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

// IntVal extracts an int from a map key (handles float64 from JSON).
func IntVal(m map[string]any, key string) int {
	return gconv.To[int, any](m[key])
}

// FloatVal extracts a float64 from a map key.
func FloatVal(m map[string]any, key string) float64 {
	return gconv.To[float64, any](m[key])
}

// YearFromDate extracts a 4-digit year from a date string or number.
func YearFromDate(v any) int {
	s := gconv.To[string, any](v)
	if len(s) >= 4 {
		return gconv.To[int, any](s[:4])
	}
	return 0
}
