package initexit

import "context"

type StructureCandidate struct {
	Price float64
	Type  string
}

type TrendInput struct {
	StructureCandidates []StructureCandidate
}

type BuildInput struct {
	Symbol    string
	Direction string
	Entry     float64
	ATR       float64
	Trend     TrendInput
	Params    map[string]any
}

type BuildOutput struct {
	StopLoss         float64
	TakeProfits      []float64
	TakeProfitRatios []float64
	StopSource       string
	StopReason       string
}

type Policy interface {
	Name() string
	Build(ctx context.Context, in BuildInput) (BuildOutput, error)
	ValidateParams(params map[string]any) error
}
