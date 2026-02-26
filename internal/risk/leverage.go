package risk

import (
	"math"
	"strings"
)

const (
	defaultLiquidationFee = 0.0002
	mmrThresholdNotional  = 10000
	mmrLow                = 0.005
	mmrHigh               = 0.015
)

type LeverageLiquidation struct {
	PositionSize     float64
	Leverage         float64
	LiquidationPrice float64
	MMR              float64
	Fee              float64
}

func ResolveLeverageAndLiquidation(entry float64, positionSize float64, maxInvestAmt float64, maxLeverage float64, direction string) LeverageLiquidation {
	result := LeverageLiquidation{
		PositionSize:     positionSize,
		Leverage:         1.0,
		LiquidationPrice: 0.0,
		MMR:              0.0,
		Fee:              defaultLiquidationFee,
	}
	if entry <= 0 || positionSize <= 0 || maxInvestAmt <= 0 {
		return result
	}
	notional := positionSize * entry
	leverage := math.Ceil(notional / maxInvestAmt)
	if leverage < 1 {
		leverage = 1
	}
	if maxLeverage > 0 {
		maxLev := math.Floor(maxLeverage)
		if maxLev < 1 {
			maxLev = 1
		}
		if leverage > maxLev {
			leverage = maxLev
		}
	}
	marginRequired := notional / leverage
	if marginRequired > maxInvestAmt {
		positionSize = (leverage * maxInvestAmt) / entry
		notional = positionSize * entry
		marginRequired = notional / leverage
	}
	result.PositionSize = positionSize
	result.Leverage = leverage
	if marginRequired <= mmrThresholdNotional {
		result.MMR = mmrLow
	} else {
		result.MMR = mmrHigh
	}
	if leverage > 1 {
		switch strings.ToLower(strings.TrimSpace(direction)) {
		case "long":
			result.LiquidationPrice = entry * (1 - result.MMR + result.Fee) / (1 - 1/leverage)
		case "short":
			result.LiquidationPrice = entry * (1 + result.MMR - result.Fee) / (1 + 1/leverage)
		}
	}
	return result
}

func IsStopBeyondLiquidation(direction string, stopLoss float64, liqPrice float64) bool {
	if liqPrice <= 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "long":
		return stopLoss >= liqPrice
	case "short":
		return stopLoss <= liqPrice
	default:
		return false
	}
}
