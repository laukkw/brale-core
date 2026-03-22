package ruleflow

import (
	"context"
	"math"
	"path/filepath"
	"runtime"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
	"brale-core/internal/strategy"
)

func TestPlanBuilderLLMModeSkipsGoInitialExit(t *testing.T) {
	result := evaluateFlatPlanForRiskMode(t, "llm")
	if result.Plan == nil {
		t.Fatalf("expected plan")
	}
	if !result.Plan.Valid {
		t.Fatalf("expected valid plan")
	}
	if result.Plan.StopLoss != 0 {
		t.Fatalf("llm mode stop_loss=%v, want 0 (no Go precompute)", result.Plan.StopLoss)
	}
	if len(result.Plan.TakeProfits) != 0 {
		t.Fatalf("llm mode take_profits=%v, want empty (no Go precompute)", result.Plan.TakeProfits)
	}
	if len(result.Plan.TakeProfitRatios) != 0 {
		t.Fatalf("llm mode take_profit_ratios=%v, want empty (no Go precompute)", result.Plan.TakeProfitRatios)
	}
	if result.Plan.PlanSource != execution.PlanSourceLLM {
		t.Fatalf("llm mode plan_source=%q, want %q", result.Plan.PlanSource, execution.PlanSourceLLM)
	}
}

func TestPlanBuilderNativeModeKeepsGoInitialExit(t *testing.T) {
	result := evaluateFlatPlanForRiskMode(t, "native")
	if result.Plan == nil {
		t.Fatalf("expected plan")
	}
	if !result.Plan.Valid {
		t.Fatalf("expected valid plan")
	}
	if result.Plan.StopLoss <= 0 {
		t.Fatalf("native mode stop_loss=%v, want > 0", result.Plan.StopLoss)
	}
	if !(result.Plan.StopLoss < result.Plan.Entry) {
		t.Fatalf("native mode stop_loss=%v, entry=%v, want stop_loss < entry", result.Plan.StopLoss, result.Plan.Entry)
	}
	if len(result.Plan.TakeProfits) == 0 {
		t.Fatalf("native mode take_profits empty, want Go-generated levels")
	}
	if len(result.Plan.TakeProfitRatios) == 0 {
		t.Fatalf("native mode take_profit_ratios empty, want Go-generated ratios")
	}
	if len(result.Plan.TakeProfits) != len(result.Plan.TakeProfitRatios) {
		t.Fatalf("native mode tp count=%d ratios count=%d, want equal", len(result.Plan.TakeProfits), len(result.Plan.TakeProfitRatios))
	}
	ratioSum := 0.0
	for _, ratio := range result.Plan.TakeProfitRatios {
		ratioSum += ratio
	}
	if math.Abs(ratioSum-1.0) > 1e-6 {
		t.Fatalf("native mode take_profit_ratios sum=%v, want 1.0", ratioSum)
	}
	if result.Plan.PlanSource != execution.PlanSourceGo {
		t.Fatalf("native mode plan_source=%q, want %q", result.Plan.PlanSource, execution.PlanSourceGo)
	}
}

func TestParseResultIncludesPlanSource(t *testing.T) {
	raw := map[string]any{
		"symbol": "BTCUSDT",
		"gate":   map[string]any{"action": "ALLOW", "reason": "PASS", "tradeable": true, "direction": "long", "grade": 1},
		"plan": map[string]any{
			"valid":              true,
			"direction":          "long",
			"entry":              100.0,
			"stop_loss":          99.0,
			"risk_pct":           0.01,
			"position_size":      100.0,
			"leverage":           1.0,
			"r_multiple":         1.0,
			"template":           "rulego_plan",
			"plan_source":        execution.PlanSourceLLM,
			"take_profits":       []any{101.0, 102.0},
			"take_profit_ratios": []any{0.5, 0.5},
		},
	}
	parsed, err := parseResult(raw)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if parsed.Plan == nil {
		t.Fatalf("expected parsed plan")
	}
	if parsed.Plan.PlanSource != execution.PlanSourceLLM {
		t.Fatalf("parsed plan_source=%q, want %q", parsed.Plan.PlanSource, execution.PlanSourceLLM)
	}
}

func evaluateFlatPlanForRiskMode(t *testing.T, mode string) Result {
	t.Helper()
	engine := NewEngine()
	result, err := engine.Evaluate(context.Background(), defaultRuleChainPath(t), Input{
		Symbol: "BTCUSDT",
		Providers: fund.ProviderBundle{
			Indicator: provider.IndicatorProviderOut{
				MomentumExpansion: true,
				Alignment:         true,
				MeanRevNoise:      false,
				SignalTag:         "trend_surge",
			},
			Structure: provider.StructureProviderOut{
				ClearStructure: true,
				Integrity:      true,
				Reason:         "ok",
				SignalTag:      "breakout_confirmed",
			},
			Mechanics: provider.MechanicsProviderOut{
				LiquidationStress: provider.SemanticSignal{Value: false, Confidence: provider.ConfidenceLow, Reason: "ok"},
				SignalTag:         "neutral",
			},
			Enabled: fund.ProviderEnabled{Indicator: true, Structure: true, Mechanics: true},
		},
		StructureDirection: "long",
		State:              fsm.StateFlat,
		BuildPlan:          true,
		Account:            execution.AccountState{Equity: 10000, Available: 10000},
		Risk:               execution.RiskParams{RiskPerTradePct: 0.01},
		Binding: strategy.StrategyBinding{
			Symbol: "BTCUSDT",
			RiskManagement: config.RiskManagementConfig{
				RiskPerTradePct: 0.01,
				MaxInvestPct:    1.0,
				MaxLeverage:     20,
				Grade1Factor:    1.0,
				Grade2Factor:    1.0,
				Grade3Factor:    1.0,
				EntryOffsetATR:  0,
				EntryMode:       "market",
				RiskStrategy:    config.RiskStrategyConfig{Mode: mode},
				InitialExit: config.InitialExitConfig{
					Policy:            "atr_structure_v1",
					StructureInterval: "1h",
					Params:            map[string]any{},
				},
			},
		},
		Compression: features.CompressionResult{
			Indicators: map[string]map[string]features.IndicatorJSON{
				"BTCUSDT": {
					"1h": {
						Symbol:   "BTCUSDT",
						Interval: "1h",
						RawJSON:  []byte(`{"close":100,"atr":2}`),
					},
				},
			},
			Trends: map[string]map[string]features.TrendJSON{
				"BTCUSDT": {
					"1h": {
						Symbol:   "BTCUSDT",
						Interval: "1h",
						RawJSON:  []byte(`{"recent_candles":[{"h":101,"l":99},{"h":103,"l":98}],"structure_candidates":[{"price":98,"type":"support"}]}`),
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("evaluate ruleflow: %v", err)
	}
	return result
}

func defaultRuleChainPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../configs/rules/default.json"))
}
