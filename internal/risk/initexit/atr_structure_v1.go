package initexit

import (
	"context"
	"fmt"
	"math"

	"brale-core/internal/pkg/numutil"
)

const atrStructureV1Name = "atr_structure_v1"

type atrStructureV1Policy struct{}

func init() {
	Register(atrStructureV1Policy{})
}

func (atrStructureV1Policy) Name() string {
	return atrStructureV1Name
}

func (atrStructureV1Policy) ValidateParams(params map[string]any) error {
	if err := validateBaseStopParams(params); err != nil {
		return err
	}
	if err := validateProvidedPositiveFloatSliceParam(params, "take_profit_ratios"); err != nil {
		return err
	}
	rr := resolveTPRR(params, 1.5, 3.0)
	if len(rr) == 0 {
		return fmt.Errorf("take profit RR is required")
	}
	if err := validateTakeProfitRatios(paramFloatSlice(params, "take_profit_ratios")); err != nil {
		return err
	}
	return nil
}

func (atrStructureV1Policy) Build(_ context.Context, in BuildInput) (BuildOutput, error) {
	stopDist, err := resolveATRStructureStopDistance(in, true)
	if err != nil {
		return BuildOutput{}, err
	}
	stop := stopFromDistance(in.Direction, in.Entry, stopDist)
	rr := resolveTPRR(in.Params, 1.5, 3.0)
	tps := buildTakeProfitsFromRR(in.Entry, stopDist, in.Direction, rr)
	return BuildOutput{
		StopLoss:         stop,
		TakeProfits:      tps,
		TakeProfitRatios: resolveTPExecutionRatios(in.Params, len(tps)),
		StopSource:       "atr_structure",
		StopReason:       "atr_structure",
	}, nil
}

func resolveATRStructureStopDistance(in BuildInput, useStructure bool) (float64, error) {
	if in.Entry <= 0 {
		return 0, fmt.Errorf("entry is required")
	}
	stopATRMultiplier := paramFloat(in.Params, "stop_atr_multiplier", 2.0)
	stopMinDistancePct := paramFloat(in.Params, "stop_min_distance_pct", 0.005)
	atrDist := 0.0
	if stopATRMultiplier > 0 && in.ATR > 0 {
		atrDist = in.ATR * stopATRMultiplier
	}
	structureDist := 0.0
	if useStructure {
		if candidate := pickStructureCandidatePrice(in.Direction, in.Entry, in.Trend.StructureCandidates); candidate > 0 {
			structureDist = math.Abs(in.Entry - candidate)
		}
	}
	minStopDist := 0.0
	if stopMinDistancePct > 0 {
		minStopDist = in.Entry * stopMinDistancePct
	}
	stopDist := numutil.MaxFloat(atrDist, structureDist)
	stopDist = numutil.MaxFloat(stopDist, minStopDist)
	if stopDist <= 0 {
		return 0, fmt.Errorf("stop distance invalid")
	}
	return stopDist, nil
}

func pickStructureCandidatePrice(direction string, entry float64, candidates []StructureCandidate) float64 {
	if entry <= 0 || len(candidates) == 0 {
		return 0
	}
	candidate := 0.0
	for _, item := range candidates {
		price := item.Price
		if price <= 0 {
			continue
		}
		if isShort(direction) {
			if price > entry && (candidate == 0 || price < candidate) {
				candidate = price
			}
			continue
		}
		if price < entry && price > candidate {
			candidate = price
		}
	}
	return candidate
}

func resolveTPRR(params map[string]any, def1, def2 float64) []float64 {
	rr := paramFloatSlice(params, "take_profit_rr")
	if len(rr) > 0 {
		return rr
	}
	return []float64{def1, def2}
}

func resolveTPExecutionRatios(params map[string]any, count int) []float64 {
	ratios := paramFloatSlice(params, "take_profit_ratios")
	if len(ratios) == count {
		return normalizeRatios(ratios)
	}
	return equalRatios(count)
}
