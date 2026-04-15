package ta

import (
	"fmt"
	"math"
)

// BollingerBands computes Bollinger Bands with SMA as the middle band.
// Returns (upper, middle, lower) series, each of len(closes).
func BollingerBands(closes []float64, period int, upMultiplier, downMultiplier float64) (upper, middle, lower []float64, err error) {
	if err = ValidateSeries("closes", closes, period); err != nil {
		return nil, nil, nil, err
	}
	if upMultiplier <= 0 || downMultiplier <= 0 {
		return nil, nil, nil, fmt.Errorf("bb multipliers must be > 0")
	}
	upper, middle, lower = bollingerBands(closes, period, upMultiplier)
	if downMultiplier != upMultiplier {
		lower = rescaleBand(middle, lower, downMultiplier/upMultiplier)
	}
	return upper, middle, lower, nil
}

func bollingerBands(closes []float64, period int, multiplier float64) (upper, middle, lower []float64) {
	n := len(closes)
	upper = make([]float64, n)
	middle = make([]float64, n)
	lower = make([]float64, n)
	for i := range closes {
		upper[i] = math.NaN()
		middle[i] = math.NaN()
		lower[i] = math.NaN()
	}
	if period <= 0 || n < period {
		return
	}
	for i := period - 1; i < n; i++ {
		start := i + 1 - period
		sum := 0.0
		for j := start; j <= i; j++ {
			sum += closes[j]
		}
		sma := sum / float64(period)
		variance := 0.0
		for j := start; j <= i; j++ {
			d := closes[j] - sma
			variance += d * d
		}
		stddev := math.Sqrt(variance / float64(period))
		middle[i] = sma
		upper[i] = sma + multiplier*stddev
		lower[i] = sma - multiplier*stddev
	}
	return
}

func rescaleBand(middle, band []float64, factor float64) []float64 {
	out := make([]float64, len(band))
	for i := range band {
		switch {
		case i >= len(middle):
			out[i] = band[i]
		case math.IsNaN(middle[i]), math.IsNaN(band[i]):
			out[i] = band[i]
		default:
			out[i] = middle[i] - ((middle[i] - band[i]) * factor)
		}
	}
	return out
}
