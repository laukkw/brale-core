package ruleflow

import (
	"context"

	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
	"brale-core/internal/strategy"
)

type Input struct {
	Symbol              string
	Providers           fund.ProviderBundle
	AgentStructure      agent.StructureSummary
	InPosition          InPositionOutputs
	Position            HardGuardPosition
	State               fsm.PositionState
	PositionID          string
	ExitConfirmCount    int
	BuildPlan           bool
	Compression         features.CompressionResult
	Account             execution.AccountState
	Risk                execution.RiskParams
	Binding             strategy.StrategyBinding
	StructureDirection  string
	ConsensusScore      float64
	ConsensusConfidence float64
	ConsensusAgreement  float64
	ConsensusResonance  float64
	ConsensusResonant   bool
	ScoreThreshold      float64
	ConfidenceThreshold float64
}

type InPositionOutputs struct {
	Indicator provider.InPositionIndicatorOut
	Structure provider.InPositionStructureOut
	Mechanics provider.InPositionMechanicsOut
	Ready     bool
}

type HardGuardPosition struct {
	Side        string
	MarkPrice   float64
	MarkPriceOK bool
	StopLoss    float64
	StopLossOK  bool
}

type Result struct {
	Gate             fund.GateDecision
	Plan             *execution.ExecutionPlan
	FSMNext          fsm.PositionState
	FSMActions       []fsm.Action
	FSMRuleHit       fsm.RuleHit
	ExitConfirmCount int
}

type Evaluator interface {
	Evaluate(ctx context.Context, ruleChainPath string, input Input) (Result, error)
}
