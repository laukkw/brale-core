package initexit

import (
	"fmt"
	"math"
)

func ValidateAndNormalize(direction string, entry float64, out BuildOutput) (BuildOutput, error) {
	if entry <= 0 {
		return BuildOutput{}, fmt.Errorf("entry is required")
	}
	if out.StopLoss <= 0 {
		return BuildOutput{}, fmt.Errorf("stop_loss is required")
	}
	if isShort(direction) {
		if out.StopLoss <= entry {
			return BuildOutput{}, fmt.Errorf("short stop_loss must be above entry")
		}
	} else {
		if out.StopLoss >= entry {
			return BuildOutput{}, fmt.Errorf("long stop_loss must be below entry")
		}
	}
	if len(out.TakeProfits) == 0 {
		return BuildOutput{}, fmt.Errorf("take_profits is required")
	}
	prev := 0.0
	for i, tp := range out.TakeProfits {
		if tp <= 0 || math.IsNaN(tp) || math.IsInf(tp, 0) {
			return BuildOutput{}, fmt.Errorf("take_profit[%d] invalid", i)
		}
		if isShort(direction) {
			if tp >= entry {
				return BuildOutput{}, fmt.Errorf("short take_profit[%d] must be below entry", i)
			}
			if i > 0 && tp >= prev {
				return BuildOutput{}, fmt.Errorf("short take_profits must be descending")
			}
		} else {
			if tp <= entry {
				return BuildOutput{}, fmt.Errorf("long take_profit[%d] must be above entry", i)
			}
			if i > 0 && tp <= prev {
				return BuildOutput{}, fmt.Errorf("long take_profits must be ascending")
			}
		}
		prev = tp
	}
	ratios := out.TakeProfitRatios
	if len(ratios) == 0 {
		ratios = equalRatios(len(out.TakeProfits))
	}
	if len(ratios) != len(out.TakeProfits) {
		return BuildOutput{}, fmt.Errorf("take_profit_ratios length mismatch")
	}
	sum := 0.0
	for i, ratio := range ratios {
		if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			return BuildOutput{}, fmt.Errorf("take_profit_ratio[%d] invalid", i)
		}
		sum += ratio
	}
	if sum <= 0 {
		return BuildOutput{}, fmt.Errorf("take_profit_ratios sum invalid")
	}
	out.TakeProfitRatios = normalizeRatios(ratios)
	return out, nil
}
