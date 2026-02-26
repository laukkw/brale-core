package parseutil

import (
	"strconv"
	"strings"
)

func Float(v any) float64 {
	value, _ := FloatOK(v)
	return value
}

func FloatOK(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case int32:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case string:
		return FloatStringOK(val)
	default:
		return 0, false
	}
}

func FloatString(raw string) float64 {
	value, _ := FloatStringOK(raw)
	return value
}

func ParseFloatString(raw string) (float64, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(text, 64)
}

func FloatStringOK(raw string) (float64, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, false
	}
	value, err := ParseFloatString(text)
	if err != nil {
		return 0, false
	}
	return value, true
}

func ParseNullableFloatJSON(data []byte) (float64, error) {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return 0, nil
	}
	if len(text) >= 2 && text[0] == '"' && text[len(text)-1] == '"' {
		text = text[1 : len(text)-1]
	}
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	return ParseFloatString(text)
}

func FirstPositiveFloat(v any) (float64, bool) {
	switch raw := v.(type) {
	case []any:
		for _, item := range raw {
			if val, ok := FloatOK(item); ok && val > 0 {
				return val, true
			}
		}
	case []float64:
		for _, item := range raw {
			if item > 0 {
				return item, true
			}
		}
	case []float32:
		for _, item := range raw {
			if item > 0 {
				return float64(item), true
			}
		}
	case []int:
		for _, item := range raw {
			if item > 0 {
				return float64(item), true
			}
		}
	case []int64:
		for _, item := range raw {
			if item > 0 {
				return float64(item), true
			}
		}
	}
	return 0, false
}
