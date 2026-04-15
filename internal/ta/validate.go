package ta

import "fmt"

// ValidateSeries checks that series is non-empty and period is positive.
func ValidateSeries(name string, series []float64, period int) error {
	if len(series) == 0 {
		return fmt.Errorf("%s series must not be empty", name)
	}
	if period <= 0 {
		return fmt.Errorf("period must be > 0")
	}
	return nil
}

// ValidatePairSeries checks that two series are non-empty and equal length.
func ValidatePairSeries(leftName string, left []float64, rightName string, right []float64) error {
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

// ValidateOHLC checks that highs, lows, closes are non-empty and equal length.
func ValidateOHLC(highs, lows, closes []float64) error {
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

// ErrPeriodPositive is the canonical error for non-positive period arguments.
func ErrPeriodPositive() error {
	return fmt.Errorf("period must be > 0")
}
