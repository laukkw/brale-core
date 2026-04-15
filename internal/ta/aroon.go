package ta

import "math"

// Aroon computes the Aroon Up and Down indicator.
// AroonUp   = (period - bars_since_highest_high) / period * 100
// AroonDown = (period - bars_since_lowest_low)   / period * 100
func Aroon(highs, lows []float64, period int) (up, down []float64, err error) {
	if err = ValidatePairSeries("highs", highs, "lows", lows); err != nil {
		return nil, nil, err
	}
	if period <= 0 {
		return nil, nil, ErrPeriodPositive()
	}
	n := len(highs)
	up = make([]float64, n)
	down = make([]float64, n)
	for i := range up {
		up[i] = math.NaN()
		down[i] = math.NaN()
	}
	if n < period+1 {
		return up, down, nil
	}
	for i := period; i < n; i++ {
		start := i - period
		highIdx := start
		lowIdx := start
		for j := start + 1; j <= i; j++ {
			if highs[j] >= highs[highIdx] {
				highIdx = j
			}
			if lows[j] <= lows[lowIdx] {
				lowIdx = j
			}
		}
		p := float64(period)
		up[i] = (p - float64(i-highIdx)) / p * 100
		down[i] = (p - float64(i-lowIdx)) / p * 100
	}
	return up, down, nil
}
