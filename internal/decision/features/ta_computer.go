package features

import "brale-core/internal/ta"

// TAComputer implements IndicatorComputer using the pure-Go ta package.
// It is the default production engine, requiring no CGO dependencies.
type TAComputer struct{}

func (TAComputer) ComputeEMA(closes []float64, period int) ([]float64, error) {
	return ta.EMA(closes, period)
}

func (TAComputer) ComputeRSI(closes []float64, period int) ([]float64, error) {
	return ta.RSI(closes, period)
}

func (TAComputer) ComputeATR(highs, lows, closes []float64, period int) ([]float64, error) {
	return ta.ATR(highs, lows, closes, period)
}

func (TAComputer) ComputeBB(closes []float64, period int, upMult, downMult float64) ([]float64, []float64, []float64, error) {
	return ta.BollingerBands(closes, period, upMult, downMult)
}

func (TAComputer) ComputeOBV(closes, volumes []float64) ([]float64, error) {
	return ta.OBV(closes, volumes)
}

func (TAComputer) ComputeSTC(closes []float64, fast, slow, kPeriod, dPeriod int) ([]float64, error) {
	return ta.STC(closes, fast, slow, kPeriod, dPeriod)
}

func (TAComputer) ComputeCHOP(highs, lows, closes []float64, period int) ([]float64, error) {
	return ta.CHOP(highs, lows, closes, period)
}

func (TAComputer) ComputeAroon(highs, lows []float64, period int) ([]float64, []float64, error) {
	return ta.Aroon(highs, lows, period)
}

func (TAComputer) ComputeStochRSI(rsiSeries []float64, period int) ([]float64, error) {
	return ta.StochRSI(rsiSeries, period)
}
