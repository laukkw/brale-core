package features

import (
	"math"
	"time"

	"brale-core/internal/snapshot"
)

func roundFloat(v float64, digits int) float64 {
	if digits <= 0 {
		return math.Round(v)
	}
	factor := math.Pow10(digits)
	return math.Round(v*factor) / factor
}

func candleTimestamp(c snapshot.Candle) string {
	ts := c.OpenTime
	if ts == 0 {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return time.UnixMilli(ts).UTC().Format(time.RFC3339)
}

func maxFloat(values []float64) float64 {
	m := -math.MaxFloat64
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if v > m {
			m = v
		}
	}
	if m == -math.MaxFloat64 {
		return 0
	}
	return m
}

func minFloat(values []float64) float64 {
	m := math.MaxFloat64
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if v < m {
			m = v
		}
	}
	if m == math.MaxFloat64 {
		return 0
	}
	return m
}
