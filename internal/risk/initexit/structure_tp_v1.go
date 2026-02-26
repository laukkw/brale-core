package initexit

import (
	"context"
	"fmt"
	"sort"
)

const structureTPV1Name = "structure_tp_v1"

type structureTPV1Policy struct{}

func init() {
	Register(structureTPV1Policy{})
}

func (structureTPV1Policy) Name() string {
	return structureTPV1Name
}

func (structureTPV1Policy) ValidateParams(params map[string]any) error {
	if err := (atrStructureV1Policy{}).ValidateParams(params); err != nil {
		return err
	}
	bufferPct := paramFloat(params, "structure_buffer_pct", 0.001)
	if bufferPct < 0 {
		return fmt.Errorf("structure_buffer_pct must be >= 0")
	}
	return nil
}

func (structureTPV1Policy) Build(_ context.Context, in BuildInput) (BuildOutput, error) {
	stopDist, err := resolveATRStructureStopDistance(in, true)
	if err != nil {
		return BuildOutput{}, err
	}
	stop := stopFromDistance(in.Direction, in.Entry, stopDist)
	rr := resolveTPRR(in.Params, 1.0, 2.0)
	baseTPs := buildTakeProfitsFromRR(in.Entry, stopDist, in.Direction, rr)
	if len(baseTPs) == 0 {
		return BuildOutput{}, fmt.Errorf("take profits invalid")
	}
	bufferPct := paramFloat(in.Params, "structure_buffer_pct", 0.001)
	tps := adjustTPByStructure(in.Direction, in.Entry, baseTPs, in.Trend.StructureCandidates, bufferPct)
	return BuildOutput{
		StopLoss:         stop,
		TakeProfits:      tps,
		TakeProfitRatios: resolveTPExecutionRatios(in.Params, len(tps)),
		StopSource:       "structure_tp",
		StopReason:       "structure_tp",
	}, nil
}

func adjustTPByStructure(direction string, entry float64, baseTPs []float64, candidates []StructureCandidate, bufferPct float64) []float64 {
	if len(baseTPs) == 0 || len(candidates) == 0 {
		return baseTPs
	}
	levels := make([]float64, 0, len(candidates))
	for _, item := range candidates {
		if item.Price > 0 {
			levels = append(levels, item.Price)
		}
	}
	if len(levels) == 0 {
		return baseTPs
	}
	sort.Float64s(levels)
	out := make([]float64, len(baseTPs))
	copy(out, baseTPs)
	if isShort(direction) {
		for i, fixed := range baseTPs {
			level := 0.0
			for idx := len(levels) - 1; idx >= 0; idx-- {
				price := levels[idx]
				if price < entry && price > fixed {
					level = price
					break
				}
			}
			if level <= 0 {
				continue
			}
			next := level
			if bufferPct > 0 {
				next = level * (1 + bufferPct)
			}
			if next < entry && next > fixed {
				out[i] = next
			}
		}
		return out
	}
	for i, fixed := range baseTPs {
		level := 0.0
		for _, price := range levels {
			if price > entry && price < fixed {
				level = price
				break
			}
		}
		if level <= 0 {
			continue
		}
		next := level
		if bufferPct > 0 {
			next = level * (1 - bufferPct)
		}
		if next > entry && next < fixed {
			out[i] = next
		}
	}
	return out
}
