package ta

import "math"

// RSI computes the Relative Strength Index with the given period.
// Uses the Wilder smoothing method (exponential moving average of gains/losses).
func RSI(closes []float64, period int) ([]float64, error) {
	if err := ValidateSeries("closes", closes, period); err != nil {
		return nil, err
	}
	out := make([]float64, len(closes))
	for i := range out {
		out[i] = math.NaN()
	}
	if len(closes) < period+1 {
		return out, nil
	}
	avgGain := 0.0
	avgLoss := 0.0
	for i := 1; i <= period; i++ {
		delta := closes[i] - closes[i-1]
		if delta > 0 {
			avgGain += delta
		} else {
			avgLoss -= delta
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)
	out[period] = rsiFromAverages(avgGain, avgLoss)
	for i := period + 1; i < len(closes); i++ {
		delta := closes[i] - closes[i-1]
		gain := 0.0
		loss := 0.0
		if delta > 0 {
			gain = delta
		} else {
			loss = -delta
		}
		avgGain = ((avgGain * float64(period-1)) + gain) / float64(period)
		avgLoss = ((avgLoss * float64(period-1)) + loss) / float64(period)
		out[i] = rsiFromAverages(avgGain, avgLoss)
	}
	return out, nil
}

func rsiFromAverages(avgGain, avgLoss float64) float64 {
	switch {
	case avgLoss == 0 && avgGain == 0:
		return 50
	case avgLoss == 0:
		return 100
	default:
		rs := avgGain / avgLoss
		return 100 - (100 / (1 + rs))
	}
}
