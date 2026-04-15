package test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"brale-core/internal/decision/features"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
	"brale-core/internal/position"
	"brale-core/internal/risk"
	"brale-core/internal/store"
)

func TestRiskMonitor_TakeProfitHit_VersionSync(t *testing.T) {
	storeImpl := newTestStore(t)
	exec := newFakeExecutor("freqtrade", 1001, time.Now().UnixMilli())
	priceSource := newFakePriceSource()
	riskPlanSvc := &position.RiskPlanService{Store: storeImpl}
	positioner := &position.PositionService{
		Store:     storeImpl,
		Executor:  exec,
		Cache:     position.NewPositionCache(),
		PlanCache: position.NewPlanCache(),
		RiskPlans: riskPlanSvc,
	}

	plan := execution.ExecutionPlan{
		Symbol:       "ETHUSDT",
		Direction:    "long",
		Entry:        100,
		StopLoss:     90,
		TakeProfits:  []float64{110},
		PositionSize: 1,
		PositionID:   "pos-1",
	}
	riskPlan := risk.BuildRiskPlan(risk.RiskPlanInput{
		Entry:            plan.Entry,
		StopLoss:         plan.StopLoss,
		PositionSize:     plan.PositionSize,
		TakeProfits:      plan.TakeProfits,
		TakeProfitRatios: plan.TakeProfitRatios,
	})
	raw, err := json.Marshal(risk.CompactRiskPlan(riskPlan))
	if err != nil {
		t.Fatalf("marshal risk plan: %v", err)
	}
	pos := store.PositionRecord{
		PositionID:         plan.PositionID,
		Symbol:             plan.Symbol,
		Side:               plan.Direction,
		Status:             position.PositionOpenActive,
		Version:            1,
		RiskJSON:           raw,
		Qty:                1,
		AvgEntry:           100,
		ExecutorName:       exec.Name(),
		ExecutorPositionID: "ext-1",
	}
	ctx := context.Background()
	if err := storeImpl.SavePosition(ctx, &pos); err != nil {
		t.Fatalf("save position: %v", err)
	}
	exec.AddExternalPosition(execution.ExternalPosition{
		PositionID:   "ext-1",
		Symbol:       plan.Symbol,
		Side:         plan.Direction,
		Quantity:     1,
		AvgEntry:     100,
		Status:       "open",
		CurrentPrice: 110,
		UpdatedAt:    exec.NowMillis(),
	})
	priceSource.Set(plan.Symbol, 110)

	monitor := &position.RiskMonitor{
		Store:       storeImpl,
		PriceSource: priceSource,
		Positions:   positioner,
		PlanCache:   positioner.PlanCache,
		AccountFetcher: func(context.Context, string) (execution.AccountState, error) {
			return execution.AccountState{Equity: 10000, Available: 10000, Currency: "USDT"}, nil
		},
	}
	if err := monitor.RunOnce(ctx, plan.Symbol); err != nil {
		t.Fatalf("risk monitor: %v", err)
	}

	updated := loadPosition(t, storeImpl, plan.PositionID)
	if updated.Status != position.PositionClosePending {
		t.Fatalf("expected status %s, got %s", position.PositionClosePending, updated.Status)
	}
	if updated.Version != 5 {
		t.Fatalf("expected version 5, got %d", updated.Version)
	}
	decoded, err := position.DecodeRiskPlan(updated.RiskJSON)
	if err != nil {
		t.Fatalf("decode risk plan: %v", err)
	}
	hit := false
	for _, level := range decoded.TPLevels {
		if level.LevelID == "tp-1" {
			hit = level.Hit
		}
	}
	if !hit {
		t.Fatalf("expected tp-1 hit to be true")
	}
}

func TestRiskMonitorTakeProfitUpdate(t *testing.T) {
	symbol := "BTC/USDT"
	positionID := "pos-tighten"
	env := newPipelineEnv(t, pipelineConfig{symbol: symbol, positionID: positionID, bind: defaultBinding(symbol)})
	env.pipeline.Core.PriceSource = env.priceSource
	env.pipeline.BarInterval = time.Minute

	indicator := provider.InPositionIndicatorOut{MomentumSustaining: false, DivergenceDetected: false, Reason: "stall", MonitorTag: "tighten"}
	structure := provider.InPositionStructureOut{Integrity: true, ThreatLevel: provider.ThreatLevelNone, Reason: "broken", MonitorTag: "structure_broken"}
	mechanics := provider.InPositionMechanicsOut{AdverseLiquidation: false, CrowdingReversal: false, Reason: "ok"}
	env.pipeline.Runner.Provider = fakeProvider{
		inPositionIndicator: &indicator,
		inPositionStructure: &structure,
		inPositionMechanics: &mechanics,
	}

	env.pipeline.Runner.Compressor = fakeCompressor{comp: features.CompressionResult{
		Indicators: map[string]map[string]features.IndicatorJSON{
			symbol: {
				"1h": {Symbol: symbol, Interval: "1h", RawJSON: []byte(`{"close":110,"atr":10,"data":{"atr":{"latest":10,"change_pct":0.5}}}`)},
			},
		},
		Trends: map[string]map[string]features.TrendJSON{
			symbol: {"1h": {Symbol: symbol, Interval: "1h", RawJSON: []byte(`{"structure_candidates":[],"structure_points":[]}`)}},
		},
	}}

	plan := execution.ExecutionPlan{
		Symbol:           symbol,
		Direction:        "long",
		Entry:            100,
		StopLoss:         90,
		TakeProfits:      []float64{120, 130},
		TakeProfitRatios: []float64{0.25, 0.75},
		PositionSize:     1,
		PositionID:       positionID,
		RiskAnnotations:  execution.RiskAnnotations{RiskDistance: 10},
	}
	riskPlan := risk.BuildRiskPlan(risk.RiskPlanInput{
		Entry:            plan.Entry,
		StopLoss:         plan.StopLoss,
		PositionSize:     plan.PositionSize,
		TakeProfits:      plan.TakeProfits,
		TakeProfitRatios: plan.TakeProfitRatios,
	})
	raw, err := json.Marshal(risk.CompactRiskPlan(riskPlan))
	if err != nil {
		t.Fatalf("marshal risk plan: %v", err)
	}
	pos := store.PositionRecord{
		PositionID:         plan.PositionID,
		Symbol:             plan.Symbol,
		Side:               plan.Direction,
		Status:             position.PositionOpenActive,
		Version:            1,
		RiskJSON:           raw,
		Qty:                1,
		AvgEntry:           100,
		ExecutorName:       env.executor.Name(),
		ExecutorPositionID: "ext-1",
	}
	ctx := context.Background()
	if err := env.store.SavePosition(ctx, &pos); err != nil {
		t.Fatalf("save position: %v", err)
	}
	env.priceSource.Set(symbol, 110)

	runPipelineOnce(t, env, symbol, 10000, 0.01)

	updated := loadPosition(t, env.store, plan.PositionID)
	decoded, err := position.DecodeRiskPlan(updated.RiskJSON)
	if err != nil {
		t.Fatalf("decode risk plan: %v", err)
	}
	if math.Abs(decoded.StopPrice-90) > 1e-8 {
		t.Fatalf("expected stop to remain 90 under 4.5 threshold, got %v", decoded.StopPrice)
	}
	if len(decoded.TPLevels) != 2 {
		t.Fatalf("expected 2 tp levels, got %d", len(decoded.TPLevels))
	}
	if math.Abs(decoded.TPLevels[0].Price-120) > 1e-8 {
		t.Fatalf("expected tp-1 to remain 120, got %v", decoded.TPLevels[0].Price)
	}
	if math.Abs(decoded.TPLevels[1].Price-130) > 1e-8 {
		t.Fatalf("expected tp-2 to remain 130, got %v", decoded.TPLevels[1].Price)
	}
	if decoded.TPLevels[0].QtyPct != riskPlan.TPLevels[0].QtyPct {
		t.Fatalf("expected tp-1 qty pct to remain %v, got %v", riskPlan.TPLevels[0].QtyPct, decoded.TPLevels[0].QtyPct)
	}
}
