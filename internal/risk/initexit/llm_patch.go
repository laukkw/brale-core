package initexit

// BuildPatch is a narrow patch surface reserved for LLM post-processing.
type BuildPatch struct {
	Entry            *float64
	StopLoss         *float64
	TakeProfits      []float64
	TakeProfitRatios []float64
}

func ApplyPatch(base BuildOutput, patch *BuildPatch) BuildOutput {
	if patch == nil {
		return base
	}
	if patch.StopLoss != nil {
		base.StopLoss = *patch.StopLoss
	}
	if len(patch.TakeProfits) > 0 {
		base.TakeProfits = append([]float64(nil), patch.TakeProfits...)
	}
	if len(patch.TakeProfitRatios) > 0 {
		base.TakeProfitRatios = append([]float64(nil), patch.TakeProfitRatios...)
	}
	return base
}
