package initexit

import (
	"fmt"
	"math"

	"brale-core/internal/pkg/parseutil"
)

func paramFloat(params map[string]any, key string, fallback float64) float64 {
	if len(params) == 0 {
		return fallback
	}
	raw, ok := params[key]
	if !ok {
		return fallback
	}
	val, ok := parseutil.FloatOK(raw)
	if !ok {
		return fallback
	}
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return fallback
	}
	return val
}

func paramFloatSlice(params map[string]any, key string) []float64 {
	if len(params) == 0 {
		return nil
	}
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil
	}
	appendIfValid := func(out []float64, val float64) []float64 {
		if val <= 0 || math.IsNaN(val) || math.IsInf(val, 0) {
			return out
		}
		return append(out, val)
	}
	switch list := raw.(type) {
	case []any:
		out := make([]float64, 0, len(list))
		for _, item := range list {
			val, ok := parseutil.FloatOK(item)
			if !ok {
				continue
			}
			out = appendIfValid(out, val)
		}
		return out
	case []float64:
		out := make([]float64, 0, len(list))
		for _, item := range list {
			out = appendIfValid(out, item)
		}
		return out
	case []float32:
		out := make([]float64, 0, len(list))
		for _, item := range list {
			out = appendIfValid(out, float64(item))
		}
		return out
	case []int:
		out := make([]float64, 0, len(list))
		for _, item := range list {
			out = appendIfValid(out, float64(item))
		}
		return out
	case []int64:
		out := make([]float64, 0, len(list))
		for _, item := range list {
			out = appendIfValid(out, float64(item))
		}
		return out
	default:
		return nil
	}
}

func stopFromDistance(direction string, entry, stopDist float64) float64 {
	if entry <= 0 || stopDist <= 0 {
		return 0
	}
	if isShort(direction) {
		return entry + stopDist
	}
	return entry - stopDist
}

func buildTakeProfitsFromRR(entry, stopDist float64, direction string, rr []float64) []float64 {
	if entry <= 0 || stopDist <= 0 || len(rr) == 0 {
		return nil
	}
	out := make([]float64, 0, len(rr))
	for _, ratio := range rr {
		if ratio <= 0 {
			continue
		}
		if isShort(direction) {
			out = append(out, entry-stopDist*ratio)
			continue
		}
		out = append(out, entry+stopDist*ratio)
	}
	return out
}

func isShort(direction string) bool {
	return normalizePolicyName(direction) == "short"
}

func normalizeRatios(ratios []float64) []float64 {
	if len(ratios) == 0 {
		return nil
	}
	sum := 0.0
	for _, item := range ratios {
		sum += item
	}
	if sum <= 0 {
		return nil
	}
	out := make([]float64, len(ratios))
	for i, item := range ratios {
		out[i] = item / sum
	}
	return out
}

func equalRatios(count int) []float64 {
	if count <= 0 {
		return nil
	}
	val := 1.0 / float64(count)
	out := make([]float64, count)
	for i := range out {
		out[i] = val
	}
	return out
}

func validateBaseStopParams(params map[string]any) error {
	stopATRMultiplier := paramFloat(params, "stop_atr_multiplier", 2.0)
	stopMinDistancePct := paramFloat(params, "stop_min_distance_pct", 0.005)
	if stopATRMultiplier <= 0 {
		return fmt.Errorf("stop_atr_multiplier must be > 0")
	}
	if stopMinDistancePct <= 0 {
		return fmt.Errorf("stop_min_distance_pct must be > 0")
	}
	return nil
}

func validateTakeProfitRatios(ratios []float64) error {
	if len(ratios) == 0 {
		return nil
	}
	sum := 0.0
	for i, ratio := range ratios {
		if ratio <= 0 {
			return fmt.Errorf("take_profit_ratios[%d] must be > 0", i)
		}
		sum += ratio
	}
	if sum <= 0 {
		return fmt.Errorf("take_profit_ratios sum must be > 0")
	}
	return nil
}

func validateProvidedPositiveFloatSliceParam(params map[string]any, key string) error {
	if len(params) == 0 {
		return nil
	}
	raw, ok := params[key]
	if !ok || raw == nil {
		return nil
	}
	check := func(i int, val float64) error {
		if math.IsNaN(val) || math.IsInf(val, 0) || val <= 0 {
			return fmt.Errorf("%s[%d] must be > 0", key, i)
		}
		return nil
	}
	switch list := raw.(type) {
	case []any:
		for i, item := range list {
			val, ok := parseutil.FloatOK(item)
			if !ok {
				return fmt.Errorf("%s[%d] must be numeric", key, i)
			}
			if err := check(i, val); err != nil {
				return err
			}
		}
		return nil
	case []float64:
		for i, item := range list {
			if err := check(i, item); err != nil {
				return err
			}
		}
		return nil
	case []float32:
		for i, item := range list {
			if err := check(i, float64(item)); err != nil {
				return err
			}
		}
		return nil
	case []int:
		for i, item := range list {
			if err := check(i, float64(item)); err != nil {
				return err
			}
		}
		return nil
	case []int64:
		for i, item := range list {
			if err := check(i, float64(item)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%s must be an array of numbers", key)
	}
}
