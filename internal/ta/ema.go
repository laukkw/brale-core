package ta

import "math"

// EMA computes an Exponential Moving Average with the given period.
// The first (period-1) values are NaN. The seed is the SMA of the first period values.
func EMA(values []float64, period int) ([]float64, error) {
	if err := ValidateSeries("values", values, period); err != nil {
		return nil, err
	}
	return ema(values, period), nil
}

func ema(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || len(values) < period {
		return out
	}
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	e := sum / float64(period)
	out[period-1] = e
	alpha := 2.0 / float64(period+1)
	for i := period; i < len(values); i++ {
		e += alpha * (values[i] - e)
		out[i] = e
	}
	return out
}
