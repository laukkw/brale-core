package decision

import (
	"context"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/prompt/positionprompt"
	"brale-core/internal/risk/initexit"
	"brale-core/internal/strategy"
)

func TestRunnerFlatRiskInitFailClosedWhenCallbackMissing(t *testing.T) {
	symbol := "BTCUSDT"
	runner := newRunnerForFlatRiskTests(symbol, nil)
	result := runner.runSymbol(
		context.Background(),
		symbol,
		features.CompressionResult{},
		execution.AccountState{Equity: 10000, Available: 10000},
		execution.RiskParams{},
		RunOptions{BuildPlan: true, RiskStrategyModeBySymbol: map[string]string{symbol: execution.PlanSourceLLM}},
	)
	if result.Err == nil {
		t.Fatalf("expected llm risk init failure when callback missing")
	}
	if result.Gate.GateReason != llmRiskReasonTransportFailure {
		t.Fatalf("gate reason=%q, want %q", result.Gate.GateReason, llmRiskReasonTransportFailure)
	}
	if result.Gate.GlobalTradeable {
		t.Fatalf("gate should fail-close and become non-tradeable")
	}
}

func TestRunnerFlatRiskInitSetsPlanSourceLLM(t *testing.T) {
	symbol := "BTCUSDT"
	entry := 100.6
	stop := 99.0
	runner := newRunnerForFlatRiskTests(symbol, func(_ context.Context, input FlatRiskInitInput) (*initexit.BuildPatch, error) {
		if input.Symbol != symbol {
			t.Fatalf("symbol=%q, want %q", input.Symbol, symbol)
		}
		if input.Plan.StopLoss != 0 {
			t.Fatalf("llm mode should not precompute stop_loss, got %v", input.Plan.StopLoss)
		}
		if len(input.Plan.TakeProfits) != 0 {
			t.Fatalf("llm mode should not precompute take_profits, got %v", input.Plan.TakeProfits)
		}
		if len(input.Plan.TakeProfitRatios) != 0 {
			t.Fatalf("llm mode should not precompute take_profit_ratios, got %v", input.Plan.TakeProfitRatios)
		}
		return &initexit.BuildPatch{
			Entry:            &entry,
			StopLoss:         &stop,
			TakeProfits:      []float64{101, 102},
			TakeProfitRatios: []float64{0.5, 0.5},
		}, nil
	})

	result := runner.runSymbol(
		context.Background(),
		symbol,
		features.CompressionResult{},
		execution.AccountState{Equity: 10000, Available: 10000},
		execution.RiskParams{},
		RunOptions{BuildPlan: true, RiskStrategyModeBySymbol: map[string]string{symbol: execution.PlanSourceLLM}},
	)
	if result.Err != nil {
		t.Fatalf("run symbol: %v", result.Err)
	}
	if result.Plan == nil {
		t.Fatalf("expected plan")
	}
	if result.Plan.PlanSource != execution.PlanSourceLLM {
		t.Fatalf("plan source=%q, want %q", result.Plan.PlanSource, execution.PlanSourceLLM)
	}
	if result.Plan.Entry != entry {
		t.Fatalf("entry=%v, want %v", result.Plan.Entry, entry)
	}
	if result.Plan.StopLoss != stop {
		t.Fatalf("stop_loss=%v, want %v", result.Plan.StopLoss, stop)
	}
	if len(result.Plan.TakeProfits) != 2 || len(result.Plan.TakeProfitRatios) != 2 {
		t.Fatalf("expected patched tp/tp ratios, got tps=%v ratios=%v", result.Plan.TakeProfits, result.Plan.TakeProfitRatios)
	}
	planDerived, ok := result.Gate.Derived["plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected gate.derived.plan map")
	}
	if got := planDerived["plan_source"]; got != execution.PlanSourceLLM {
		t.Fatalf("gate derived plan_source=%v, want %q", got, execution.PlanSourceLLM)
	}
	if got := planDerived["entry"]; got != entry {
		t.Fatalf("gate derived entry=%v, want %v", got, entry)
	}
}

func TestRunnerFlatRiskInitFailClosedWhenLLMEntryMissing(t *testing.T) {
	symbol := "BTCUSDT"
	stop := 99.0
	runner := newRunnerForFlatRiskTests(symbol, func(context.Context, FlatRiskInitInput) (*initexit.BuildPatch, error) {
		return &initexit.BuildPatch{
			StopLoss:         &stop,
			TakeProfits:      []float64{101, 102},
			TakeProfitRatios: []float64{0.5, 0.5},
		}, nil
	})

	result := runner.runSymbol(
		context.Background(),
		symbol,
		features.CompressionResult{},
		execution.AccountState{Equity: 10000, Available: 10000},
		execution.RiskParams{},
		RunOptions{BuildPlan: true, RiskStrategyModeBySymbol: map[string]string{symbol: execution.PlanSourceLLM}},
	)
	if result.Err == nil {
		t.Fatalf("expected llm risk init failure when entry missing")
	}
	if result.Gate.GateReason != llmRiskReasonSchemaFailure {
		t.Fatalf("gate reason=%q, want %q", result.Gate.GateReason, llmRiskReasonSchemaFailure)
	}
	if result.Gate.GlobalTradeable {
		t.Fatalf("gate should fail-close and become non-tradeable")
	}
}

func TestRunnerFlatRiskInitFailClosedWhenLLMEntryInvalid(t *testing.T) {
	symbol := "BTCUSDT"
	entry := 0.0
	stop := 99.0
	runner := newRunnerForFlatRiskTests(symbol, func(context.Context, FlatRiskInitInput) (*initexit.BuildPatch, error) {
		return &initexit.BuildPatch{
			Entry:            &entry,
			StopLoss:         &stop,
			TakeProfits:      []float64{101, 102},
			TakeProfitRatios: []float64{0.5, 0.5},
		}, nil
	})

	result := runner.runSymbol(
		context.Background(),
		symbol,
		features.CompressionResult{},
		execution.AccountState{Equity: 10000, Available: 10000},
		execution.RiskParams{},
		RunOptions{BuildPlan: true, RiskStrategyModeBySymbol: map[string]string{symbol: execution.PlanSourceLLM}},
	)
	if result.Err == nil {
		t.Fatalf("expected llm risk init failure when entry invalid")
	}
	if result.Gate.GateReason != llmRiskReasonSchemaFailure {
		t.Fatalf("gate reason=%q, want %q", result.Gate.GateReason, llmRiskReasonSchemaFailure)
	}
}

func newRunnerForFlatRiskTests(symbol string, cb FlatRiskInitLLM) *Runner {
	return &Runner{
		Agent:           staticAgentService{},
		Provider:        staticProviderService{},
		FlatRiskInitLLM: cb,
		Ruleflow: staticRuleflowEvaluator{result: ruleflow.Result{
			Gate: fund.GateDecision{GlobalTradeable: true, DecisionAction: "ALLOW", GateReason: "PASS", Direction: "long", Grade: 1},
			Plan: &execution.ExecutionPlan{
				Valid:            true,
				Direction:        "long",
				Entry:            100,
				RiskPct:          0.01,
				PositionSize:     0,
				Leverage:         0,
				PlanSource:       execution.PlanSourceLLM,
				TakeProfits:      nil,
				TakeProfitRatios: nil,
				RiskAnnotations: execution.RiskAnnotations{
					ATR:          2,
					MaxInvestPct: 1,
					MaxInvestAmt: 10000,
					MaxLeverage:  20,
				},
			},
			FSMActions: []fsm.Action{{Type: fsm.ActionOpen, Reason: "GATE_ALLOW"}},
		}},
		Bindings: map[string]strategy.StrategyBinding{
			symbol: {
				Symbol: symbol,
				RiskManagement: config.RiskManagementConfig{
					RiskPerTradePct: 0.01,
					MaxInvestPct:    1.0,
					MaxLeverage:     20,
					Grade1Factor:    1.0,
					Grade2Factor:    1.0,
					Grade3Factor:    1.0,
				},
			},
		},
		Configs: map[string]config.SymbolConfig{symbol: {Symbol: symbol}},
		Enabled: map[string]AgentEnabled{symbol: {Indicator: true, Structure: true, Mechanics: true}},
	}
}

type staticAgentService struct{}

func (staticAgentService) Analyze(context.Context, string, features.CompressionResult, AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentPromptSet, error) {
	return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, AgentPromptSet{}, nil
}

type staticProviderService struct{}

func (staticProviderService) Judge(context.Context, string, agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, AgentEnabled) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, ProviderPromptSet, error) {
	return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, ProviderPromptSet{}, nil
}

func (staticProviderService) JudgeInPosition(context.Context, string, agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, positionprompt.Summary, AgentEnabled) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, error) {
	return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, nil
}

type staticRuleflowEvaluator struct {
	result ruleflow.Result
	err    error
}

func (s staticRuleflowEvaluator) Evaluate(context.Context, string, ruleflow.Input) (ruleflow.Result, error) {
	return s.result, s.err
}
