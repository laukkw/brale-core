package initexit

import (
	"context"
	"fmt"
)

const fixedRRV1Name = "fixed_rr_v1"

type fixedRRV1Policy struct{}

func init() {
	Register(fixedRRV1Policy{})
}

func (fixedRRV1Policy) Name() string {
	return fixedRRV1Name
}

func (fixedRRV1Policy) ValidateParams(params map[string]any) error {
	if err := validateBaseStopParams(params); err != nil {
		return err
	}
	if err := validateProvidedPositiveFloatSliceParam(params, "take_profit_ratios"); err != nil {
		return err
	}
	if err := validateTakeProfitRatios(paramFloatSlice(params, "take_profit_ratios")); err != nil {
		return err
	}
	rr := resolveTPRR(params, 1.0, 2.0)
	if len(rr) == 0 {
		return fmt.Errorf("take profit RR is required")
	}
	return nil
}

func (fixedRRV1Policy) Build(_ context.Context, in BuildInput) (BuildOutput, error) {
	stopDist, err := resolveATRStructureStopDistance(in, false)
	if err != nil {
		return BuildOutput{}, err
	}
	stop := stopFromDistance(in.Direction, in.Entry, stopDist)
	rr := resolveTPRR(in.Params, 1.0, 2.0)
	tps := buildTakeProfitsFromRR(in.Entry, stopDist, in.Direction, rr)
	return BuildOutput{
		StopLoss:         stop,
		TakeProfits:      tps,
		TakeProfitRatios: resolveTPExecutionRatios(in.Params, len(tps)),
		StopSource:       "fixed_rr",
		StopReason:       "fixed_rr",
	}, nil
}
