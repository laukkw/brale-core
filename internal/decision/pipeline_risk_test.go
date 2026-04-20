package decision

import (
	"context"
	"strings"
	"testing"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/risk"
	"brale-core/internal/store"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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

func (n *captureRiskPlanNotifier) SendError(ctx context.Context, notice ErrorNotice) error {
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
			if input.UnrealizedPnlRatio != 0.05 {
				t.Fatalf("unrealized_pnl_ratio=%v, want 0.05", input.UnrealizedPnlRatio)
			}
			if input.PositionAgeMin < 30 || input.PositionAgeMin > 31 {
				t.Fatalf("position_age_min=%d, want [30,31]", input.PositionAgeMin)
			}
			if !input.TP1Hit {
				t.Fatalf("tp1_hit=%v, want true", input.TP1Hit)
			}
			if input.DistanceToLiqPct != 0 {
				t.Fatalf("distance_to_liq_pct=%v, want 0", input.DistanceToLiqPct)
			}
			if input.CurrentStopLoss != 99 {
				t.Fatalf("current_stop_loss=%v, want tightened baseline 99", input.CurrentStopLoss)
			}
			if len(input.CurrentTakeProfits) != 1 || input.CurrentTakeProfits[0] != 120 {
				t.Fatalf("current_take_profits=%v, want [120]", input.CurrentTakeProfits)
			}
			if len(input.HitTakeProfits) != 1 || input.HitTakeProfits[0] != 110 {
				t.Fatalf("hit_take_profits=%v, want [110]", input.HitTakeProfits)
			}
			if input.RemainingQty != 1.75 {
				t.Fatalf("remaining_qty=%v, want 1.75", input.RemainingQty)
			}
			if input.RemainingNotional != 183.75 {
				t.Fatalf("remaining_notional=%v, want 183.75", input.RemainingNotional)
			}
			return &TightenRiskUpdatePatch{
				Action:      "adjust",
				StopLoss:    &stop,
				TakeProfits: []float64{109.5},
				Reason:      ptrString("trail under structure"),
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
			{LevelID: "tp-1", Price: 110, QtyPct: 0.5, Hit: true},
			{LevelID: "tp-2", Price: 120, QtyPct: 0.5},
		},
	}
	pos := store.PositionRecord{Symbol: "BTCUSDT", Side: "long", AvgEntry: 100, Qty: 1.75, CreatedAt: time.Now().Add(-30 * time.Minute)}
	updateCtx := tightenContext{
		Binding:   strategy.StrategyBinding{RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "llm"}, TightenATR: config.TightenATRConfig{TP1ATR: 0.5, TP2ATR: 1.0}}},
		MarkPrice: 105,
		ATR:       2,
	}

	got, tpTightened, trace, reason, err := p.buildTightenPlan(context.Background(), pos, plan, updateCtx, 99)
	if err != nil {
		t.Fatalf("build tighten plan: %v", err)
	}
	if !called {
		t.Fatalf("llm tighten callback should be called")
	}
	if got.StopPrice != stop {
		t.Fatalf("stop_loss=%v, want %v", got.StopPrice, stop)
	}
	if len(got.TPLevels) < 2 || got.TPLevels[0].Price != 110 || got.TPLevels[1].Price != 109.5 {
		t.Fatalf("tp levels=%+v", got.TPLevels)
	}
	if !tpTightened {
		t.Fatalf("tp_tightened should be true when llm patch updates tp")
	}
	if trace == nil || trace.Stage != "risk_tighten" {
		t.Fatalf("trace=%#v", trace)
	}
	if reason != "trail under structure" {
		t.Fatalf("reason=%q, want llm patch reason", reason)
	}
}

func TestApplyTightenRiskPatchStopNotImprovedIncludesBaselineAndLLMStops(t *testing.T) {
	stop := 98.5
	plan := risk.RiskPlan{
		StopPrice: 99,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 108, QtyPct: 0.5},
			{LevelID: "tp-2", Price: 112, QtyPct: 0.5},
		},
	}

	_, _, err := applyTightenRiskPatch(plan, "long", 100, 105, &TightenRiskUpdatePatch{
		Action:      "adjust",
		StopLoss:    &stop,
		TakeProfits: []float64{109, 113},
	})
	if err == nil {
		t.Fatalf("expected stop not improved error")
	}
	reject, ok := asTightenPatchRejectError(err)
	if !ok {
		t.Fatalf("expected tightenPatchRejectError, got %T", err)
	}
	if reject.BaselineStop != 99 {
		t.Fatalf("baseline_stop=%v want 99", reject.BaselineStop)
	}
	if reject.LLMStop != stop {
		t.Fatalf("llm_stop=%v want %v", reject.LLMStop, stop)
	}
	if !strings.Contains(err.Error(), "baseline_stop=99.0000") || !strings.Contains(err.Error(), "llm_stop=98.5000") {
		t.Fatalf("error=%q", err.Error())
	}
}

func TestApplyTightenRiskPatchHoldKeepsPlan(t *testing.T) {
	plan := risk.RiskPlan{
		StopPrice: 99,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 108, QtyPct: 0.5},
			{LevelID: "tp-2", Price: 112, QtyPct: 0.5},
		},
	}

	got, tpTightened, err := applyTightenRiskPatch(plan, "long", 100, 105, &TightenRiskUpdatePatch{
		Action: "hold",
		Reason: ptrString("no new risk signal"),
	})
	if err != nil {
		t.Fatalf("apply tighten risk patch hold: %v", err)
	}
	if tpTightened {
		t.Fatalf("tp_tightened=%v, want false", tpTightened)
	}
	if got.StopPrice != plan.StopPrice {
		t.Fatalf("stop_price=%v want %v", got.StopPrice, plan.StopPrice)
	}
	if len(got.TPLevels) != len(plan.TPLevels) || got.TPLevels[0].Price != plan.TPLevels[0].Price || got.TPLevels[1].Price != plan.TPLevels[1].Price {
		t.Fatalf("tp_levels=%+v want %+v", got.TPLevels, plan.TPLevels)
	}
}

func TestApplyTightenRiskPatchOnlyUpdatesRemainingTakeProfits(t *testing.T) {
	stop := 100.2
	plan := risk.RiskPlan{
		StopPrice: 99,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 108, QtyPct: 0.5, Hit: true},
			{LevelID: "tp-2", Price: 112, QtyPct: 0.5},
		},
	}

	got, tpTightened, err := applyTightenRiskPatch(plan, "long", 100, 105, &TightenRiskUpdatePatch{
		Action:      "adjust",
		StopLoss:    &stop,
		TakeProfits: []float64{109.5},
		Reason:      ptrString("trail under structure"),
	})
	if err != nil {
		t.Fatalf("apply tighten risk patch: %v", err)
	}
	if !tpTightened {
		t.Fatalf("tp_tightened=%v, want true", tpTightened)
	}
	if got.TPLevels[0].Price != 108 || !got.TPLevels[0].Hit {
		t.Fatalf("hit tp level changed unexpectedly: %+v", got.TPLevels[0])
	}
	if got.TPLevels[1].Price != 109.5 {
		t.Fatalf("remaining tp level=%+v", got.TPLevels[1])
	}
}

func TestApplyTightenRiskPatchRejectsMismatchedRemainingTakeProfits(t *testing.T) {
	stop := 100.2
	plan := risk.RiskPlan{
		StopPrice: 99,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 108, QtyPct: 0.5, Hit: true},
			{LevelID: "tp-2", Price: 112, QtyPct: 0.5},
		},
	}

	_, _, err := applyTightenRiskPatch(plan, "long", 100, 105, &TightenRiskUpdatePatch{
		Action:      "adjust",
		StopLoss:    &stop,
		TakeProfits: []float64{109.5, 111.5},
		Reason:      ptrString("trail under structure"),
	})
	if err == nil || !strings.Contains(err.Error(), "must match remaining take_profits") {
		t.Fatalf("err=%v, want mismatch error", err)
	}
}

func TestApplyPostTP1StopFloorUsesBreakevenAsMinimum(t *testing.T) {
	plan := risk.RiskPlan{StopPrice: 95}
	got, updated := applyPostTP1StopFloor(plan, "long", 100, 0.001, 105)
	if !updated {
		t.Fatalf("updated=%v, want true", updated)
	}
	if got.StopPrice != 100.1 {
		t.Fatalf("stop_price=%v, want 100.1", got.StopPrice)
	}
}

func TestApplyPostTP1StopFloorDoesNotLoosenExistingStop(t *testing.T) {
	plan := risk.RiskPlan{StopPrice: 101}
	got, updated := applyPostTP1StopFloor(plan, "long", 100, 0.001, 105)
	if updated {
		t.Fatalf("updated=%v, want false", updated)
	}
	if got.StopPrice != 101 {
		t.Fatalf("stop_price=%v, want 101", got.StopPrice)
	}
}

func TestRiskPlanUpdateLogFieldsIncludeTightenPatchStops(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	err := &tightenPatchRejectError{
		Reason:       "tighten llm patch stop_loss is not improved",
		BaselineStop: 99,
		LLMStop:      98.5,
	}

	logger.Error("risk plan update failed", riskPlanUpdateLogFields(err)...)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("log entries=%d want 1", len(entries))
	}
	ctxMap := entries[0].ContextMap()
	if ctxMap["baseline_stop"] != 99.0 {
		t.Fatalf("baseline_stop=%v want 99", ctxMap["baseline_stop"])
	}
	if ctxMap["llm_stop"] != 98.5 {
		t.Fatalf("llm_stop=%v want 98.5", ctxMap["llm_stop"])
	}
}

func TestComputeDistanceToLiqPctValidatesLiquidationSide(t *testing.T) {
	gateWithLiq := func(price float64) fund.GateDecision {
		return fund.GateDecision{
			Derived: map[string]any{
				"plan": map[string]any{"liquidation_price": price},
			},
		}
	}

	tests := []struct {
		name      string
		side      string
		liqPrice  float64
		markPrice float64
		want      float64
	}{
		{name: "long valid below mark", side: "long", liqPrice: 80, markPrice: 100, want: 0.2},
		{name: "long invalid above mark", side: "long", liqPrice: 120, markPrice: 100, want: 0},
		{name: "short valid above mark", side: "short", liqPrice: 120, markPrice: 100, want: 0.2},
		{name: "short invalid below mark", side: "short", liqPrice: 80, markPrice: 100, want: 0},
		{name: "unknown side", side: "", liqPrice: 80, markPrice: 100, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDistanceToLiqPct(tt.side, gateWithLiq(tt.liqPrice), tt.markPrice)
			if got != tt.want {
				t.Fatalf("computeDistanceToLiqPct()=%v want %v", got, tt.want)
			}
		})
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

	got, tpTightened, trace, reason, err := p.buildTightenPlan(context.Background(), pos, plan, updateCtx, 99)
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
	if reason != "" {
		t.Fatalf("reason=%q, want empty for native path", reason)
	}
}

func TestBuildTightenPlanNativeModeSkipsHitTakeProfits(t *testing.T) {
	p := &Pipeline{}
	plan := risk.RiskPlan{
		StopPrice: 95,
		TPLevels: []risk.TPLevel{
			{LevelID: "tp-1", Price: 110, QtyPct: 0.5, Hit: true},
			{LevelID: "tp-2", Price: 120, QtyPct: 0.5},
		},
	}
	pos := store.PositionRecord{Symbol: "BTCUSDT", Side: "long", AvgEntry: 100}
	updateCtx := tightenContext{
		Binding:   strategy.StrategyBinding{RiskManagement: config.RiskManagementConfig{RiskStrategy: config.RiskStrategyConfig{Mode: "native"}, TightenATR: config.TightenATRConfig{TP1ATR: 0.5, TP2ATR: 1.0}}},
		MarkPrice: 105,
		ATR:       2,
	}

	got, tpTightened, trace, reason, err := p.buildTightenPlan(context.Background(), pos, plan, updateCtx, 99)
	if err != nil {
		t.Fatalf("build tighten plan: %v", err)
	}
	if got.TPLevels[0].Price != 110 || !got.TPLevels[0].Hit {
		t.Fatalf("hit tp level changed unexpectedly: %+v", got.TPLevels[0])
	}
	if got.TPLevels[1].Price == 120 && tpTightened {
		t.Fatalf("tp_tightened=%v with unchanged remaining tp", tpTightened)
	}
	if trace != nil {
		t.Fatalf("trace=%#v, want nil for native path", trace)
	}
	if reason != "" {
		t.Fatalf("reason=%q, want empty for native path", reason)
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

func ptrString(value string) *string {
	return &value
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

	if err := p.logRiskPlanUpdate(
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
	); err != nil {
		t.Fatalf("log risk plan update: %v", err)
	}

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

func TestLogRiskPlanUpdateCarriesLLMTightenReason(t *testing.T) {
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

	if err := p.logRiskPlanUpdate(
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
		"protect open profit",
		false,
	); err != nil {
		t.Fatalf("log risk plan update: %v", err)
	}

	if !notifier.called {
		t.Fatalf("expected SendRiskPlanUpdate to be called")
	}
	if notifier.notice.Source != "monitor-tighten" {
		t.Fatalf("source=%q, want monitor-tighten", notifier.notice.Source)
	}
	if notifier.notice.StopReason != "protect open profit" {
		t.Fatalf("stop_reason=%q, want llm tighten reason", notifier.notice.StopReason)
	}
	if notifier.notice.TightenReason != "protect open profit" {
		t.Fatalf("tighten_reason=%q, want llm tighten reason", notifier.notice.TightenReason)
	}
	if notifier.notice.Reason != "初始止损基于ATR与结构低点" {
		t.Fatalf("reason=%q, want original plan stop reason", notifier.notice.Reason)
	}
}
