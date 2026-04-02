// 本文件主要内容：定义决策流程的接口与输出结构。

package decision

import (
	"context"

	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/prompt/positionprompt"
	"brale-core/internal/snapshot"
)

type Snapshotter interface {
	Fetch(ctx context.Context, symbols, intervals []string, limit int) (snapshot.MarketSnapshot, error)
}

type Compressor interface {
	Compress(ctx context.Context, snap snapshot.MarketSnapshot) (features.CompressionResult, []features.FeatureError, error)
}

type LLMStagePrompt struct {
	System string
	User   string
	Error  string
}

type AgentPromptSet struct {
	Indicator LLMStagePrompt
	Structure LLMStagePrompt
	Mechanics LLMStagePrompt
}

type ProviderPromptSet struct {
	Indicator LLMStagePrompt
	Structure LLMStagePrompt
	Mechanics LLMStagePrompt
}

type AgentService interface {
	Analyze(ctx context.Context, symbol string, data features.CompressionResult, enabled AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentPromptSet, error)
}

type ProviderService interface {
	Judge(ctx context.Context, symbol string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, enabled AgentEnabled) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, ProviderPromptSet, error)
	JudgeInPosition(ctx context.Context, symbol string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, summary positionprompt.Summary, enabled AgentEnabled) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, error)
}

type StateProvider interface {
	Load(ctx context.Context, symbol string) (fsm.PositionState, string, error)
}

type SymbolResult struct {
	Symbol              string
	Gate                fund.GateDecision
	Plan                *execution.ExecutionPlan
	FSMNext             fsm.PositionState
	FSMActions          []fsm.Action
	FSMRuleHit          fsm.RuleHit
	RuleflowResult      *ruleflow.Result
	Providers           fund.ProviderBundle
	EnabledAgents       AgentEnabled
	AgentIndicator      agent.IndicatorSummary
	AgentStructure      agent.StructureSummary
	AgentMechanics      agent.MechanicsSummary
	ConsensusDirection  string
	ConsensusScore      float64
	ConsensusConfidence float64
	ConsensusAgreement  float64
	ConsensusResonance  float64
	ConsensusResonant   bool
	AgentPrompts        AgentPromptSet
	ProviderPrompts     ProviderPromptSet
	InPositionIndicator provider.InPositionIndicatorOut
	InPositionStructure provider.InPositionStructureOut
	InPositionMechanics provider.InPositionMechanicsOut
	InPositionPrompts   ProviderPromptSet
	InPositionEvaluated bool
	Err                 error
}
