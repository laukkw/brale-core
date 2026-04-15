package ta

import "math"

// STC computes the Schaff Trend Cycle oscillator.
// It applies double-smoothed stochastic of MACD (fastEMA - slowEMA).
func STC(closes []float64, fast, slow, kPeriod, dPeriod int) ([]float64, error) {
	if err := ValidateSeries("closes", closes, fast); err != nil {
		return nil, err
	}
	if slow <= 0 || kPeriod <= 0 || dPeriod <= 0 {
		return nil, ErrPeriodPositive()
	}
	return stc(closes, fast, slow, kPeriod, dPeriod), nil
}

func stc(closes []float64, fast, slow, kPeriod, dPeriod int) []float64 {
	if len(closes) == 0 {
		return nil
	}
	fastEMA := ema(closes, fast)
	slowEMA := ema(closes, slow)
	macd := make([]float64, len(closes))
	for i := range closes {
		macd[i] = fastEMA[i] - slowEMA[i]
	}

	kValues := RollingStochastic(macd, kPeriod)
	dValues := SMA(kValues, dPeriod)
	out := make([]float64, len(closes))
	for i := range closes {
		switch {
		case math.IsNaN(kValues[i]), math.IsInf(kValues[i], 0):
			out[i] = math.NaN()
		case math.IsNaN(dValues[i]), math.IsInf(dValues[i], 0):
			out[i] = math.NaN()
		default:
			denom := dValues[i] - kValues[i]
			if math.Abs(denom) <= 1e-12 {
				out[i] = math.NaN()
				continue
			}
			out[i] = ClampFloat64(100*(macd[i]-kValues[i])/denom, 0, 100)
		}
	}
	return out
}
