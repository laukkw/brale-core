// 本文件主要内容：解析配置中的 K 线周期并计算最小周期。

package interval

import (
	"strconv"
	"strings"
	"time"

	"brale-core/internal/pkg/errclass"
)

func ParseInterval(interval string) (time.Duration, error) {
	raw := strings.ToLower(strings.TrimSpace(interval))
	if len(raw) < 2 {
		return 0, intervalValidationErrorf("interval is required")
	}
	unit := raw[len(raw)-1]
	numPart := raw[:len(raw)-1]
	value, err := strconv.Atoi(numPart)
	if err != nil || value <= 0 {
		return 0, intervalValidationErrorf("invalid interval=%s", interval)
	}
	switch unit {
	case 's':
		return time.Duration(value) * time.Second, nil
	case 'm':
		return time.Duration(value) * time.Minute, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, intervalValidationErrorf("unsupported interval=%s", interval)
	}
}

func ShortestInterval(intervals []string) (time.Duration, error) {
	if len(intervals) == 0 {
		return 0, intervalValidationErrorf("intervals is required")
	}
	var min time.Duration
	for _, iv := range intervals {
		dur, err := ParseInterval(iv)
		if err != nil {
			return 0, err
		}
		if min == 0 || dur < min {
			min = dur
		}
	}
	if min == 0 {
		return 0, intervalValidationErrorf("intervals invalid")
	}
	return min, nil
}

const validationScope errclass.Scope = "interval"
const validationReason = "invalid_interval"

func intervalValidationErrorf(format string, args ...any) error {
	return errclass.ValidationErrorf(validationScope, validationReason, format, args...)
}
