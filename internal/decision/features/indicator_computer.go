package features

import (
	"fmt"

	"brale-core/internal/config"
)

type IndicatorComputer interface {
	ComputeEMA(closes []float64, period int) ([]float64, error)
	ComputeRSI(closes []float64, period int) ([]float64, error)
	ComputeATR(highs, lows, closes []float64, period int) ([]float64, error)
	ComputeBB(closes []float64, period int, upMultiplier, downMultiplier float64) (upper, middle, lower []float64, err error)
	ComputeOBV(closes, volumes []float64) ([]float64, error)
	ComputeSTC(closes []float64, fast, slow, kPeriod, dPeriod int) ([]float64, error)
	ComputeCHOP(highs, lows, closes []float64, period int) ([]float64, error)
	ComputeAroon(highs, lows []float64, period int) (up, down []float64, err error)
	ComputeStochRSI(rsiSeries []float64, period int) ([]float64, error)
}

func defaultIndicatorComputer(computer IndicatorComputer) IndicatorComputer {
	if computer != nil {
		return computer
	}
	return TAComputer{}
}

func IndicatorComputerForEngine(engine string) (IndicatorComputer, error) {
	switch config.NormalizeIndicatorEngine(engine) {
	case config.IndicatorEngineTA:
		return TAComputer{}, nil
	case config.IndicatorEngineTalib:
		return TalibComputer{}, nil
	case config.IndicatorEngineReference:
		return ReferenceComputer{}, nil
	default:
		return nil, fmt.Errorf("unsupported indicator engine %q", engine)
	}
}
