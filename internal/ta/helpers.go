package ta

import "math"

// SMA computes a Simple Moving Average of the given period.
// Output length equals input length; indices before the first full window are NaN.
func SMA(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || len(values) < period {
		return out
	}
	for i := period - 1; i < len(values); i++ {
		sum := 0.0
		valid := true
		start := i + 1 - period
		for j := start; j <= i; j++ {
			v := values[j]
			if math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
				break
			}
			sum += v
		}
		if !valid {
			continue
		}
		out[i] = sum / float64(period)
	}
	return out
}

// TrueRange computes the True Range series for OHLC data.
// TR[0] = High[0] - Low[0]; for i > 0, TR[i] = max(H-L, |H-PrevC|, |L-PrevC|).
func TrueRange(highs, lows, closes []float64) []float64 {
	tr := make([]float64, len(closes))
	for i := range closes {
		if i == 0 {
			tr[i] = highs[i] - lows[i]
			continue
		}
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}
	return tr
}

// RollingStochastic computes (value - min) / (max - min) * 100 over a rolling window.
func RollingStochastic(series []float64, period int) []float64 {
	out := make([]float64, len(series))
	for i := range series {
		out[i] = math.NaN()
		if period <= 0 || i+1 < period {
			continue
		}
		start := i + 1 - period
		lo := series[start]
		hi := series[start]
		for j := start + 1; j <= i; j++ {
			if series[j] < lo {
				lo = series[j]
			}
			if series[j] > hi {
				hi = series[j]
			}
		}
		if math.Abs(hi-lo) <= 1e-12 {
			continue
		}
		out[i] = 100 * (series[i] - lo) / (hi - lo)
	}
	return out
}

// ClampFloat64 clamps v to the range [lo, hi].
func ClampFloat64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// WMA computes a Weighted Moving Average series.
func WMA(values []float64, period int) []float64 {
	if len(values) < period || period <= 0 {
		return nil
	}
	divisor := float64(period*(period+1)) / 2
	out := make([]float64, 0, len(values)-period+1)
	for end := period - 1; end < len(values); end++ {
		sum := 0.0
		weight := float64(period)
		for idx := end - period + 1; idx <= end; idx++ {
			sum += values[idx] * weight
			weight--
		}
		out = append(out, sum/divisor)
	}
	return out
}

// HMA computes a Hull Moving Average series.
func HMA(values []float64, period int) []float64 {
	if len(values) == 0 || period <= 0 {
		return nil
	}
	halfPeriod := int(math.Round(float64(period) / 2))
	sqrtPeriod := int(math.Round(math.Sqrt(float64(period))))
	if halfPeriod <= 0 || sqrtPeriod <= 0 {
		return nil
	}

	wma1 := WMA(values, halfPeriod)
	wma2 := WMA(values, period)
	if len(wma2) == 0 {
		return nil
	}
	skip := period - halfPeriod
	if skip < 0 || skip > len(wma1) {
		return nil
	}
	wma1 = wma1[skip:]
	if len(wma1) != len(wma2) {
		return nil
	}

	diff := make([]float64, len(wma2))
	for i := range wma2 {
		diff[i] = 2*wma1[i] - wma2[i]
	}
	return WMA(diff, sqrtPeriod)
}
