package ta

import "math"

// StochRSI computes the Stochastic RSI: (RSI - MinRSI) / (MaxRSI - MinRSI)
// over a rolling window. Input should be a pre-computed RSI series.
func StochRSI(rsiSeries []float64, period int) ([]float64, error) {
	if err := ValidateSeries("rsi", rsiSeries, period); err != nil {
		return nil, err
	}
	n := len(rsiSeries)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	for i := period - 1; i < n; i++ {
		start := i + 1 - period
		lo := rsiSeries[start]
		hi := rsiSeries[start]
		allValid := true
		for j := start; j <= i; j++ {
			v := rsiSeries[j]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				allValid = false
				break
			}
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		if !allValid {
			continue
		}
		rng := hi - lo
		if rng <= 1e-12 {
			out[i] = 0.5
			continue
		}
		out[i] = (rsiSeries[i] - lo) / rng
	}
	return out, nil
}
