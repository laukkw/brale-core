package ta

import "math"

// SuperTrend computes the SuperTrend indicator using HMA-smoothed ATR.
// Returns the SuperTrend line series. Positive values indicate support (uptrend);
// when price crosses below the upper band, the trend flips.
func SuperTrend(highs, lows, closes []float64, period int, multiplier float64) ([]float64, error) {
	if err := ValidateOHLC(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 0 {
		return nil, ErrPeriodPositive()
	}
	if multiplier <= 0 {
		return nil, ErrPeriodPositive()
	}
	if len(closes) < 2 {
		return nil, nil
	}
	return superTrend(highs, lows, closes, period, multiplier), nil
}

func superTrend(highs, lows, closes []float64, period int, multiplier float64) []float64 {
	tr := make([]float64, 0, len(closes)-1)
	for i := 1; i < len(closes); i++ {
		tr = append(tr, math.Max(highs[i]-lows[i],
			math.Max(highs[i]-closes[i-1], closes[i-1]-lows[i])))
	}

	atr := HMA(tr, period)
	if len(atr) == 0 {
		return nil
	}

	atrIdle := superTrendATRIdlePeriod(period)
	medians := make([]float64, 0, len(closes)-atrIdle)
	closings := make([]float64, 0, len(closes)-atrIdle)
	for i := atrIdle; i < len(closes); i++ {
		medians = append(medians, (highs[i]+lows[i])/2)
		closings = append(closings, closes[i])
	}
	if len(medians) != len(atr) || len(closings) != len(atr) {
		return nil
	}

	first := true
	upTrend := false
	previousClosing := 0.0
	finalUpperBand := 0.0
	finalLowerBand := 0.0
	st := make([]float64, len(atr))

	for i := range atr {
		median := medians[i]
		atrMultiple := atr[i] * multiplier
		closing := closings[i]
		basicUpperBand := median + atrMultiple
		basicLowerBand := median - atrMultiple

		if first {
			first = false
			finalUpperBand = basicUpperBand
			finalLowerBand = basicLowerBand
			st[i] = finalLowerBand
		} else {
			if basicUpperBand < finalUpperBand || previousClosing > finalUpperBand {
				finalUpperBand = basicUpperBand
			}
			if basicLowerBand > finalLowerBand || previousClosing < finalLowerBand {
				finalLowerBand = basicLowerBand
			}
			if upTrend {
				if closing <= finalUpperBand {
					st[i] = finalUpperBand
				} else {
					st[i] = finalLowerBand
					upTrend = false
				}
			} else {
				if closing >= finalLowerBand {
					st[i] = finalLowerBand
				} else {
					st[i] = finalUpperBand
					upTrend = true
				}
			}
		}

		previousClosing = closing
	}

	return st
}

func superTrendATRIdlePeriod(period int) int {
	if period <= 0 {
		return 0
	}
	sqrtPeriod := int(math.Round(math.Sqrt(float64(period))))
	return period + sqrtPeriod - 1
}
