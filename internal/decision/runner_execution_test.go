package decision

import (
	"context"
	"errors"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/prompt/positionprompt"
	"brale-core/internal/strategy"
)

type countingRuleflowEvaluator struct {
	result ruleflow.Result
	err    error
	calls  int
	input  ruleflow.Input
}

func (c *countingRuleflowEvaluator) Evaluate(_ context.Context, _ string, input ruleflow.Input) (ruleflow.Result, error) {
	c.calls++
	c.input = input
	return c.result, c.err
}

type countingProviderService struct {
	judgeCalls int
}

type failingAgentService struct {
	failSymbol string
	calls      int
	err        error
}

func (f *failingAgentService) Analyze(_ context.Context, symbol string, _ features.CompressionResult, _ AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentPromptSet, AgentInputSet, error) {
	f.calls++
	if symbol == f.failSymbol {
		return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, AgentPromptSet{}, AgentInputSet{}, f.err
	}
	return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, AgentPromptSet{}, AgentInputSet{}, nil
}

func (c *countingProviderService) Judge(_ context.Context, _ string, _ agent.IndicatorSummary, _ agent.StructureSummary, _ agent.MechanicsSummary, _ AgentEnabled, _ ProviderDataContext) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, ProviderPromptSet, error) {
	c.judgeCalls++
	return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, ProviderPromptSet{}, nil
}

func (c *countingProviderService) JudgeInPosition(_ context.Context, _ string, _ agent.IndicatorSummary, _ agent.StructureSummary, _ agent.MechanicsSummary, _ positionprompt.Summary, _ AgentEnabled, _ ProviderDataContext) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, error) {
	return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, nil
}

func TestRunnerRunSymbolInPositionStopsBeforeProviderAndRuleflow(t *testing.T) {
	symbol := "BTCUSDT"
	ruleflowCounter := &countingRuleflowEvaluator{}
	providerCounter := &countingProviderService{}
	runner := &Runner{
		Agent:    staticAgentService{},
		Provider: providerCounter,
		Ruleflow: ruleflowCounter,
		Bindings: map[string]strategy.StrategyBinding{symbol: {Symbol: symbol}},
		Configs:  map[string]config.SymbolConfig{symbol: {Symbol: symbol}},
		Enabled:  map[string]AgentEnabled{symbol: {Indicator: true, Structure: true, Mechanics: true}},
	}

	res := runner.runSymbol(context.Background(), symbol, features.CompressionResult{}, execution.AccountState{}, execution.RiskParams{}, RunOptions{BuildPlan: true, ModeBySymbol: map[string]decisionmode.Mode{symbol: decisionmode.ModeInPosition}})
	if res.Symbol != symbol {
		t.Fatalf("symbol=%s want=%s", res.Symbol, symbol)
	}
	if providerCounter.judgeCalls != 0 {
		t.Fatalf("provider judge calls=%d want=0", providerCounter.judgeCalls)
	}
	if ruleflowCounter.calls != 0 {
		t.Fatalf("ruleflow calls=%d want=0", ruleflowCounter.calls)
	}
	if res.RuleflowResult != nil {
		t.Fatalf("expected no ruleflow result for in-position skip")
	}
}

func TestEvaluateRuleflowHoldGateUsesSymbolConsensusThresholds(t *testing.T) {
	symbol := "BTCUSDT"
	ruleflowCounter := &countingRuleflowEvaluator{result: ruleflow.Result{
		Gate: fund.GateDecision{DecisionAction: "HOLD"},
	}}
	p := &Pipeline{
		Runner: &Runner{
			Ruleflow: ruleflowCounter,
			Configs: map[string]config.SymbolConfig{
				symbol: {
					Symbol: symbol,
					Consensus: config.ConsensusConfig{
						ScoreThreshold:      0.77,
						ConfidenceThreshold: 0.66,
					},
				},
			},
		},
		Bindings: map[string]strategy.StrategyBinding{
			symbol: {
				Symbol:        symbol,
				RuleChainPath: "configs/rules/default.json",
				RiskManagement: config.RiskManagementConfig{
					RiskPerTradePct: 0.01,
				},
			},
		},
	}
	_, err := p.evaluateRuleflowHoldGate(context.Background(), symbol, SymbolResult{Symbol: symbol}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, features.CompressionResult{}, "pos-1", true, ruleflow.HardGuardPosition{})
	if err != nil {
		t.Fatalf("evaluateRuleflowHoldGate: %v", err)
	}
	if ruleflowCounter.calls != 1 {
		t.Fatalf("ruleflow calls=%d want 1", ruleflowCounter.calls)
	}
	if ruleflowCounter.input.ScoreThreshold != 0.77 {
		t.Fatalf("score threshold=%v want 0.77", ruleflowCounter.input.ScoreThreshold)
	}
	if ruleflowCounter.input.ConfidenceThreshold != 0.66 {
		t.Fatalf("confidence threshold=%v want 0.66", ruleflowCounter.input.ConfidenceThreshold)
	}
}

func TestApplyRuleflowResultPromotesPlanSourceToLLM(t *testing.T) {
	res := SymbolResult{}
	applyRuleflowResult(&res, ruleflow.Result{
		Gate:       fund.GateDecision{GlobalTradeable: true, DecisionAction: "ALLOW", Derived: map[string]any{}},
		Plan:       &execution.ExecutionPlan{Valid: true, Direction: "long", PlanSource: execution.PlanSourceGo, RiskAnnotations: execution.RiskAnnotations{LiqPrice: 88}},
		FSMNext:    fsm.StateFlat,
		FSMActions: []fsm.Action{{Type: fsm.ActionOpen}},
		FSMRuleHit: fsm.RuleHit{Name: "ALLOW_RULE"},
	}, features.CompressionResult{}, "BTCUSDT", AgentEnabled{Indicator: true, Structure: true, Mechanics: true}, 1.0, 0.1, true)
	if res.Plan == nil {
		t.Fatalf("expected plan")
	}
	if res.Plan.PlanSource != execution.PlanSourceLLM {
		t.Fatalf("plan source=%s want=%s", res.Plan.PlanSource, execution.PlanSourceLLM)
	}
	appendPlanDerived(&res.Gate, res.Plan)
	planDerived, ok := res.Gate.Derived["plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected plan derived map")
	}
	if got := planDerived["plan_source"]; got != execution.PlanSourceLLM {
		t.Fatalf("derived plan_source=%v want=%s", got, execution.PlanSourceLLM)
	}
	if got := planDerived["liquidation_price"]; got != 88.0 {
		t.Fatalf("derived liquidation_price=%v want 88", got)
	}
}

func TestRunnerRunSymbolsStopsAfterFirstError(t *testing.T) {
	t.Helper()
	service := &failingAgentService{failSymbol: "ETHUSDT", err: errors.New("boom")}
	runner := &Runner{
		Agent:    service,
		Provider: staticProviderService{},
		Ruleflow: &countingRuleflowEvaluator{result: ruleflow.Result{Gate: fund.GateDecision{DecisionAction: "WAIT", GateReason: "CONSENSUS_NOT_PASSED"}}},
		Bindings: map[string]strategy.StrategyBinding{
			"BTCUSDT": {Symbol: "BTCUSDT"},
			"ETHUSDT": {Symbol: "ETHUSDT"},
			"SOLUSDT": {Symbol: "SOLUSDT"},
		},
		Configs: map[string]config.SymbolConfig{
			"BTCUSDT": {Symbol: "BTCUSDT"},
			"ETHUSDT": {Symbol: "ETHUSDT"},
			"SOLUSDT": {Symbol: "SOLUSDT"},
		},
		Enabled: map[string]AgentEnabled{
			"BTCUSDT": {Indicator: true, Structure: true, Mechanics: true},
			"ETHUSDT": {Indicator: true, Structure: true, Mechanics: true},
			"SOLUSDT": {Indicator: true, Structure: true, Mechanics: true},
		},
	}

	results := runner.runSymbols(context.Background(), []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}, features.CompressionResult{}, execution.AccountState{}, execution.RiskParams{}, RunOptions{BuildPlan: true})
	if len(results) != 2 {
		t.Fatalf("results len=%d want=2", len(results))
	}
	if results[1].Err == nil {
		t.Fatal("expected second symbol error")
	}
	if service.calls != 2 {
		t.Fatalf("analyze calls=%d want=2", service.calls)
	}
}
