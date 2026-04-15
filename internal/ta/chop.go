package ta

import "math"

// CHOP computes the Choppiness Index.
// CHOP = 100 * log10(SUM(TR, n) / (Highest_High - Lowest_Low)) / log10(n)
func CHOP(highs, lows, closes []float64, period int) ([]float64, error) {
	if err := ValidateOHLC(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 1 {
		return nil, ErrPeriodPositive()
	}
	n := len(closes)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if n < period+1 {
		return out, nil
	}
	tr := TrueRange(highs, lows, closes)
	log10Period := math.Log10(float64(period))
	if log10Period <= 1e-12 {
		return out, nil
	}
	for i := period; i < n; i++ {
		sumTR := 0.0
		highest := lows[i-period+1]
		lowest := highs[i-period+1]
		for j := i - period + 1; j <= i; j++ {
			sumTR += tr[j]
			if highs[j] > highest {
				highest = highs[j]
			}
			if lows[j] < lowest {
				lowest = lows[j]
			}
		}
		rng := highest - lowest
		if rng <= 1e-12 {
			continue
		}
		ratio := sumTR / rng
		if ratio <= 0 {
			continue
		}
		out[i] = 100 * math.Log10(ratio) / log10Period
	}
	return out, nil
}
