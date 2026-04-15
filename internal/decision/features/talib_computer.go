package features

import (
	"fmt"

	talib "github.com/markcheno/go-talib"
)

type TalibComputer struct{}

func (TalibComputer) ComputeEMA(closes []float64, period int) ([]float64, error) {
	if err := validateSeries("closes", closes, period); err != nil {
		return nil, err
	}
	return talib.Ema(closes, period), nil
}

func (TalibComputer) ComputeRSI(closes []float64, period int) ([]float64, error) {
	if err := validateSeries("closes", closes, period); err != nil {
		return nil, err
	}
	return talib.Rsi(closes, period), nil
}

func (TalibComputer) ComputeATR(highs, lows, closes []float64, period int) ([]float64, error) {
	if err := validateThreeSeries(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 0 {
		return nil, fmt.Errorf("period must be > 0")
	}
	return talib.Atr(highs, lows, closes, period), nil
}

func (TalibComputer) ComputeBB(closes []float64, period int, upMultiplier, downMultiplier float64) ([]float64, []float64, []float64, error) {
	if err := validateSeries("closes", closes, period); err != nil {
		return nil, nil, nil, err
	}
	if upMultiplier <= 0 || downMultiplier <= 0 {
		return nil, nil, nil, fmt.Errorf("bb multipliers must be > 0")
	}
	upper, middle, lower := talib.BBands(closes, period, upMultiplier, downMultiplier, talib.SMA)
	return upper, middle, lower, nil
}

func (TalibComputer) ComputeOBV(closes, volumes []float64) ([]float64, error) {
	if err := validatePairSeries("closes", closes, "volumes", volumes); err != nil {
		return nil, err
	}
	return computeOBVSeries(closes, volumes), nil
}

func (TalibComputer) ComputeSTC(closes []float64, fast, slow, kPeriod, dPeriod int) ([]float64, error) {
	if err := validateSeries("closes", closes, fast); err != nil {
		return nil, err
	}
	if slow <= 0 || kPeriod <= 0 || dPeriod <= 0 {
		return nil, errPeriodPositive()
	}
	return computeSTCSeries(closes, fast, slow, kPeriod, dPeriod), nil
}

func (TalibComputer) ComputeCHOP(highs, lows, closes []float64, period int) ([]float64, error) {
	if err := validateThreeSeries(highs, lows, closes); err != nil {
		return nil, err
	}
	if period <= 1 {
		return nil, errPeriodPositive()
	}
	return computeCHOP(highs, lows, closes, period), nil
}

func (TalibComputer) ComputeAroon(highs, lows []float64, period int) ([]float64, []float64, error) {
	if err := validatePairSeries("highs", highs, "lows", lows); err != nil {
		return nil, nil, err
	}
	if period <= 0 {
		return nil, nil, errPeriodPositive()
	}
	up, down := talib.Aroon(highs, lows, period)
	return up, down, nil
}

func (TalibComputer) ComputeStochRSI(rsiSeries []float64, period int) ([]float64, error) {
	if err := validateSeries("rsi", rsiSeries, period); err != nil {
		return nil, err
	}
	return computeStochRSI(rsiSeries, period), nil
}

func validateSeries(name string, series []float64, period int) error {
	if len(series) == 0 {
		return fmt.Errorf("%s series must not be empty", name)
	}
	if period <= 0 {
		return fmt.Errorf("period must be > 0")
	}
	return nil
}

func errPeriodPositive() error {
	return fmt.Errorf("period must be > 0")
}

func validatePairSeries(leftName string, left []float64, rightName string, right []float64) error {
	if len(left) == 0 {
		return fmt.Errorf("%s series must not be empty", leftName)
	}
	if len(right) == 0 {
		return fmt.Errorf("%s series must not be empty", rightName)
	}
	if len(left) != len(right) {
		return fmt.Errorf("%s and %s must have same length", leftName, rightName)
	}
	return nil
}

func validateThreeSeries(highs, lows, closes []float64) error {
	if len(highs) == 0 {
		return fmt.Errorf("highs series must not be empty")
	}
	if len(lows) == 0 {
		return fmt.Errorf("lows series must not be empty")
	}
	if len(closes) == 0 {
		return fmt.Errorf("closes series must not be empty")
	}
	if len(highs) != len(lows) || len(lows) != len(closes) {
		return fmt.Errorf("highs, lows, and closes must have same length")
	}
	return nil
}

func computeSTCSeries(closes []float64, fast, slow, kPeriod, dPeriod int) []float64 {
	return computeSTCSeriesWithEMA(closes, fast, slow, kPeriod, dPeriod, talibEma)
}

func talibEma(series []float64, period int) []float64 {
	return talib.Ema(series, period)
}
