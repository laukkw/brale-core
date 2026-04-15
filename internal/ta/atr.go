package ta

import "math"

// ATR computes the Average True Range with the given period.
// Uses Wilder smoothing (same as RSI averaging).
func ATR(highs, lows, closes []float64, period int) ([]float64, error) {
	if err := ValidateOHLC(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 0 {
		return nil, ErrPeriodPositive()
	}
	out := make([]float64, len(closes))
	for i := range out {
		out[i] = math.NaN()
	}
	if len(closes) < period+1 {
		return out, nil
	}
	tr := TrueRange(highs, lows, closes)
	sum := 0.0
	for i := 1; i <= period; i++ {
		sum += tr[i]
	}
	atr := sum / float64(period)
	out[period] = atr
	for i := period + 1; i < len(tr); i++ {
		atr = ((atr * float64(period-1)) + tr[i]) / float64(period)
		out[i] = atr
	}
	return out, nil
}
