package decision

import (
	"context"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/risk"
	"brale-core/internal/store"
	"brale-core/internal/strategy"
)

type captureRiskPlanNotifier struct {
	notice RiskPlanUpdateNotice
	called bool
}

func (n *captureRiskPlanNotifier) SendGate(ctx context.Context, input decisionfmt.DecisionInput, report decisionfmt.DecisionReport) error {
	return nil
}

func (n *captureRiskPlanNotifier) SendRiskPlanUpdate(ctx context.Context, notice RiskPlanUpdateNotice) error {
	n.called = true
	n.notice = notice
	return nil
}

func (n *captureRiskPlanNotifier) SendError(ctx context.Context, message string) error {
	return nil
}

func TestBuildTightenPlanUsesLLMWhenRiskModeIsLLM(t *testing.T) {
	called := false
	stop := 100.2
	p := &Pipeline{
		TightenRiskLLM: func(ctx context.Context, input TightenRiskUpdateInput) (*TightenRiskUpdatePatch, error) {
			called = true
			flow, ok := llm.SessionFlowFromContext(ctx)
			if !ok || flow != llm.LLMFlowInPosition {
				t.Fatalf("flow=%q ok=%v, want in_position", flow, ok)
			}
			symbol, ok := llm.SessionSymbolFromContext(ctx)
			if !ok || symbol != "BTCUSDT" {
				t.Fatalf("session symbol=%q ok=%v, want BTCUSDT", symbol, ok)
			}
			if input.Symbol != "BTCUSDT" {
				t.Fatalf("symbol=%q, want BTCUSDT", input.Symbol)
			}
			return &TightenRiskUpdatePatch{
				StopLoss:    &stop,
				TakeProfits: []float64{106.5, 109.5},
				Trace: &execution.LLMRiskTrace{
					Stage:        "risk_tighten",
					Flow:         "in_position",
					SystemPrompt: "risk-tighten-system",
					UserPrompt:   "risk-tighten-user",
					RawOutput:    `{"stop_loss":100.2}`,
				},
			}, nil
		},
	}

	plan := risk.RiskPlan{
		StopPrice: 95,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 110, QtyPct: 0.5},
			{LevelID: "tp-2", Price: 120, QtyPct: 0.5},
		},
	}
	pos := store.PositionRecord{Symbol: "BTCUSDT", Side: "long", AvgEntry: 100}
	updateCtx := tightenContext{
		Binding:   strategy.StrategyBinding{RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "llm"}, TightenATR: config.TightenATRConfig{TP1ATR: 0.5, TP2ATR: 1.0}}},
		MarkPrice: 105,
		ATR:       2,
	}

	got, tpTightened, trace, err := p.buildTightenPlan(context.Background(), pos, plan, updateCtx, 99)
	if err != nil {
		t.Fatalf("build tighten plan: %v", err)
	}
	if !called {
		t.Fatalf("llm tighten callback should be called")
	}
	if got.StopPrice != stop {
		t.Fatalf("stop_loss=%v, want %v", got.StopPrice, stop)
	}
	if len(got.TPLevels) < 2 || got.TPLevels[0].Price != 106.5 || got.TPLevels[1].Price != 109.5 {
		t.Fatalf("tp levels=%+v", got.TPLevels)
	}
	if !tpTightened {
		t.Fatalf("tp_tightened should be true when llm patch updates tp")
	}
	if trace == nil || trace.Stage != "risk_tighten" {
		t.Fatalf("trace=%#v", trace)
	}
}

func TestBuildTightenPlanKeepsGoPathForNativeMode(t *testing.T) {
	p := &Pipeline{}
	plan := risk.RiskPlan{
		StopPrice: 95,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 110, QtyPct: 0.5},
			{LevelID: "tp-2", Price: 120, QtyPct: 0.5},
		},
	}
	pos := store.PositionRecord{Symbol: "BTCUSDT", Side: "long", AvgEntry: 100}
	updateCtx := tightenContext{
		Binding:   strategy.StrategyBinding{RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "native"}, TightenATR: config.TightenATRConfig{TP1ATR: 0.5, TP2ATR: 1.0}}},
		MarkPrice: 105,
		ATR:       2,
	}

	expected := risk.RiskPlan{
		StopPrice: 99,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 110, QtyPct: 0.5},
			{LevelID: "tp-2", Price: 120, QtyPct: 0.5},
		},
	}
	expected, expectedTightened := risk.TightenTPLevels(expected, pos.Side, pos.AvgEntry, updateCtx.ATR, updateCtx.Binding.RiskManagement.TightenATR.TP1ATR, updateCtx.Binding.RiskManagement.TightenATR.TP2ATR, updateCtx.Binding.RiskManagement.TightenATR.MinTPDistancePct, updateCtx.Binding.RiskManagement.TightenATR.MinTPGapPct)

	got, tpTightened, trace, err := p.buildTightenPlan(context.Background(), pos, plan, updateCtx, 99)
	if err != nil {
		t.Fatalf("build tighten plan: %v", err)
	}

	if got.StopPrice != expected.StopPrice {
		t.Fatalf("stop_loss=%v, want %v", got.StopPrice, expected.StopPrice)
	}
	if len(got.TPLevels) < 2 || got.TPLevels[0].Price != expected.TPLevels[0].Price || got.TPLevels[1].Price != expected.TPLevels[1].Price {
		t.Fatalf("tp levels=%+v, want %+v", got.TPLevels, expected.TPLevels)
	}
	if tpTightened != expectedTightened {
		t.Fatalf("tp_tightened=%v, want %v", tpTightened, expectedTightened)
	}
	if trace != nil {
		t.Fatalf("trace=%#v, want nil for native path", trace)
	}
}

func TestTightenExecutionToMapIncludesPlanSourceAndRiskLevels(t *testing.T) {
	exec := tightenExecution{
		Action:      "tighten",
		Executed:    true,
		TPTightened: true,
		PlanSource:  "LLM",
		StopLoss:    101.2,
		TakeProfits: []float64{103.1, 104.6},
		LLMRiskTrace: &execution.LLMRiskTrace{
			Stage:        "risk_tighten",
			SystemPrompt: "risk-tighten-system",
		},
	}

	derived := exec.toMap()
	if got, _ := derived["plan_source"].(string); got != "llm" {
		t.Fatalf("plan_source=%v, want llm", derived["plan_source"])
	}
	if got, _ := derived["stop_loss"].(float64); got != 101.2 {
		t.Fatalf("stop_loss=%v, want 101.2", derived["stop_loss"])
	}
	trace, ok := derived["llm_trace"].(map[string]any)
	if !ok || trace["stage"] != "risk_tighten" {
		t.Fatalf("llm_trace=%#v", derived["llm_trace"])
	}
	tp, ok := derived["take_profits"].([]float64)
	if !ok || len(tp) != 2 || tp[0] != 103.1 || tp[1] != 104.6 {
		t.Fatalf("take_profits=%v", derived["take_profits"])
	}
}

func TestNewTightenUpdateResultCopiesTakeProfits(t *testing.T) {
	source := []float64{101.2, 103.4}
	result := newTightenUpdateResult("llm", 99.8, source)
	source[0] = 0
	if len(result.TakeProfits) != 2 || result.TakeProfits[0] != 101.2 || result.TakeProfits[1] != 103.4 {
		t.Fatalf("take profits copied incorrectly: %+v", result.TakeProfits)
	}
	if result.PlanSource != "llm" {
		t.Fatalf("plan source=%s want llm", result.PlanSource)
	}
	if result.StopLoss != 99.8 {
		t.Fatalf("stop loss=%v want 99.8", result.StopLoss)
	}
}

func TestLogRiskPlanUpdateCarriesOriginalPlanStopReason(t *testing.T) {
	notifier := &captureRiskPlanNotifier{}
	p := &Pipeline{Notifier: notifier}
	pos := store.PositionRecord{
		Symbol:     "ETHUSDT",
		Side:       "long",
		AvgEntry:   2500,
		RiskPct:    0.01,
		Leverage:   3,
		PositionID: "pos-eth-1",
		StopReason: "初始止损基于ATR与结构低点",
	}
	plan := risk.RiskPlan{
		StopPrice: 2440,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 2560, QtyPct: 0.5},
			{LevelID: "tp-2", Price: 2620, QtyPct: 0.5},
		},
	}

	p.logRiskPlanUpdate(
		context.Background(),
		pos,
		plan,
		2400,
		"monitor-tighten",
		2510,
		30,
		0.02,
		true,
		3.5,
		3.0,
		nil,
		true,
		"monitor-tighten",
		false,
	)

	if !notifier.called {
		t.Fatalf("expected SendRiskPlanUpdate to be called")
	}
	if notifier.notice.StopReason != "monitor-tighten" {
		t.Fatalf("stop_reason=%q, want monitor-tighten", notifier.notice.StopReason)
	}
	if notifier.notice.Reason != "初始止损基于ATR与结构低点" {
		t.Fatalf("reason=%q, want original plan stop reason", notifier.notice.Reason)
	}
}
