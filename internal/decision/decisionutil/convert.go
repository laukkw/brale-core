package decisionutil

import (
	"strings"

	"brale-core/internal/pkg/parseutil"
)

func ToBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(val, "true")
	case float64:
		return val != 0
	default:
		return false
	}
}

func ToFloat(v any) float64 {
	return parseutil.Float(v)
}
