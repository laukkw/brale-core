package config

import "math"

const (
	DefaultSTCKPeriod = 10
	DefaultSTCDPeriod = 3
)

// EMARequiredBars returns the minimum number of bars needed before EMA can
// produce its first usable value.
func EMARequiredBars(period int) int {
	if period <= 0 {
		return 0
	}
	return period
}

// RSIRequiredBars returns the minimum number of bars needed before RSI can
// produce its first usable value.
func RSIRequiredBars(period int) int {
	if period <= 0 {
		return 0
	}
	if period <= 1 {
		return 2
	}
	return period + 1
}

// ATRRequiredBars returns the minimum number of bars needed before ATR can
// produce its first usable value.
func ATRRequiredBars(period int) int {
	if period <= 0 {
		return 0
	}
	if period <= 1 {
		return 2
	}
	return period + 1
}

// BBRequiredBars returns the minimum number of bars needed before Bollinger
// Bands can produce their first usable value.
func BBRequiredBars(period int) int {
	if period <= 0 {
		return 0
	}
	return period
}

// CHOPRequiredBars returns the minimum number of bars needed before CHOP can
// produce its first usable value.
func CHOPRequiredBars(period int) int {
	if period <= 1 {
		return 0
	}
	return period + 1
}

// StochRSIRequiredBars returns the minimum number of bars needed before
// Stochastic RSI can produce its first usable value.
func StochRSIRequiredBars(rsiPeriod, stochPeriod int) int {
	if rsiPeriod <= 0 || stochPeriod <= 0 {
		return 0
	}
	return RSIRequiredBars(rsiPeriod) + stochPeriod - 1
}

// AroonRequiredBars returns the minimum number of bars needed before Aroon can
// produce its first usable value.
func AroonRequiredBars(period int) int {
	if period <= 0 {
		return 0
	}
	return period + 1
}

// SuperTrendRequiredBars returns the minimum number of bars needed before
// SuperTrend can produce its first usable value.
func SuperTrendRequiredBars(period int, multiplier float64) int {
	if period <= 0 {
		return 0
	}
	return period + int(math.Round(math.Sqrt(float64(period))))
}

// STCRequiredBars returns the minimum number of bars needed before STC can
// produce its first usable value.
func STCRequiredBars(fast, slow int) int {
	if fast <= 0 || slow <= 0 {
		return 0
	}
	return slow + DefaultSTCKPeriod + DefaultSTCDPeriod - 2
}
