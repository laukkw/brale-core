package features

import "math"

type ReferenceComputer struct{}

func (ReferenceComputer) ComputeEMA(closes []float64, period int) ([]float64, error) {
	if err := validateSeries("closes", closes, period); err != nil {
		return nil, err
	}
	return computeEMAReference(closes, period), nil
}

func (ReferenceComputer) ComputeRSI(closes []float64, period int) ([]float64, error) {
	if err := validateSeries("closes", closes, period); err != nil {
		return nil, err
	}
	return computeRSIReference(closes, period), nil
}

func (ReferenceComputer) ComputeATR(highs, lows, closes []float64, period int) ([]float64, error) {
	if err := validateThreeSeries(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 0 {
		return nil, errPeriodPositive()
	}
	return computeATRReference(highs, lows, closes, period), nil
}

func (ReferenceComputer) ComputeBB(closes []float64, period int, upMultiplier, downMultiplier float64) ([]float64, []float64, []float64, error) {
	if err := validateSeries("closes", closes, period); err != nil {
		return nil, nil, nil, err
	}
	if upMultiplier <= 0 || downMultiplier <= 0 {
		return nil, nil, nil, errPeriodPositive()
	}
	if upMultiplier != downMultiplier {
		upper, middle, lower := computeBollingerBands(closes, period, upMultiplier)
		if downMultiplier != upMultiplier {
			lower = rescaleBand(middle, lower, downMultiplier/upMultiplier)
		}
		return upper, middle, lower, nil
	}
	upper, middle, lower := computeBollingerBands(closes, period, upMultiplier)
	return upper, middle, lower, nil
}

func (ReferenceComputer) ComputeOBV(closes, volumes []float64) ([]float64, error) {
	if err := validatePairSeries("closes", closes, "volumes", volumes); err != nil {
		return nil, err
	}
	return computeOBVSeries(closes, volumes), nil
}

func (ReferenceComputer) ComputeSTC(closes []float64, fast, slow, kPeriod, dPeriod int) ([]float64, error) {
	if err := validateSeries("closes", closes, fast); err != nil {
		return nil, err
	}
	if slow <= 0 || kPeriod <= 0 || dPeriod <= 0 {
		return nil, errPeriodPositive()
	}
	return computeSTCSeriesWithEMA(closes, fast, slow, kPeriod, dPeriod, computeEMAReference), nil
}

func (ReferenceComputer) ComputeCHOP(highs, lows, closes []float64, period int) ([]float64, error) {
	if err := validateThreeSeries(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 1 {
		return nil, errPeriodPositive()
	}
	return computeCHOP(highs, lows, closes, period), nil
}

func (ReferenceComputer) ComputeAroon(highs, lows []float64, period int) ([]float64, []float64, error) {
	if err := validatePairSeries("highs", highs, "lows", lows); err != nil {
		return nil, nil, err
	}
	if period <= 0 {
		return nil, nil, errPeriodPositive()
	}
	up, down := computeAroon(highs, lows, period)
	return up, down, nil
}

func (ReferenceComputer) ComputeStochRSI(rsiSeries []float64, period int) ([]float64, error) {
	if err := validateSeries("rsi", rsiSeries, period); err != nil {
		return nil, err
	}
	return computeStochRSI(rsiSeries, period), nil
}

func computeEMAReference(values []float64, period int) []float64 {
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
	ema := sum / float64(period)
	out[period-1] = ema
	alpha := 2.0 / float64(period+1)
	for i := period; i < len(values); i++ {
		ema += alpha * (values[i] - ema)
		out[i] = ema
	}
	return out
}

func computeRSIReference(closes []float64, period int) []float64 {
	out := make([]float64, len(closes))
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || len(closes) < period+1 {
		return out
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
	return out
}

func computeATRReference(highs, lows, closes []float64, period int) []float64 {
	out := make([]float64, len(closes))
	for i := range out {
		out[i] = math.NaN()
	}
	if period <= 0 || len(closes) < period+1 {
		return out
	}
	tr := computeTrueRangeSeries(highs, lows, closes)
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
	return out
}

func computeTrueRangeSeries(highs, lows, closes []float64) []float64 {
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

func computeSTCSeriesWithEMA(closes []float64, fast, slow, kPeriod, dPeriod int, ema func([]float64, int) []float64) []float64 {
	if len(closes) == 0 {
		return nil
	}
	fastEMA := ema(closes, fast)
	slowEMA := ema(closes, slow)
	macd := make([]float64, len(closes))
	for i := range closes {
		macd[i] = fastEMA[i] - slowEMA[i]
	}

	kValues := rollingStochastic(macd, kPeriod)
	dValues := rollingSMA(kValues, dPeriod)
	stc := make([]float64, len(closes))
	for i := range closes {
		switch {
		case math.IsNaN(kValues[i]), math.IsInf(kValues[i], 0):
			stc[i] = math.NaN()
		case math.IsNaN(dValues[i]), math.IsInf(dValues[i], 0):
			stc[i] = math.NaN()
		default:
			denom := dValues[i] - kValues[i]
			if math.Abs(denom) <= 1e-12 {
				stc[i] = math.NaN()
				continue
			}
			stc[i] = clampFloat64(100*(macd[i]-kValues[i])/denom, 0, 100)
		}
	}
	return stc
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
