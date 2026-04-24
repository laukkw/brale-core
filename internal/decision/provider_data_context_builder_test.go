package decision

import (
	"encoding/json"
	"testing"

	"brale-core/internal/decision/features"
)

func TestBuildProviderDataContextUsesDecisionIntervalAndParsesAnchors(t *testing.T) {
	symbol := "BTCUSDT"
	indicator := buildIndicatorStateJSONForProviderCtxTest(t, symbol)
	inputs := AgentInputSet{
		Indicator: indicator,
		Structure: features.TrendJSON{
			Symbol:   symbol,
			Interval: "multi",
			RawJSON:  []byte(`{"blocks":[{"supertrend":{"state":"up","interval":"15m"}}],"latest_break_across_blocks":{"interval":"15m","type":"bos_up","age":2}}`),
		},
		Mechanics: features.MechanicsSnapshot{
			Symbol:  symbol,
			RawJSON: []byte(`{"crowding_state":{"reversal_risk":"medium"},"mechanics_conflict":["funding_long_but_oi_falling"],"liquidation_state":{"stress":"unknown","status":"warming_up","window":"1h","complete":false},"liquidation_source":{"source":"binance_force_order_snapshot_ws","status":"warming_up","stream_connected":true,"coverage_sec":1800,"sample_count":3,"last_event_age_sec":45,"complete":false}}`),
		},
	}

	got := BuildProviderDataContext(inputs)
	if got.IndicatorCrossTF == nil {
		t.Fatalf("indicator context missing")
	}
	if got.IndicatorCrossTF.DecisionTFBias != "up" {
		t.Fatalf("decision_tf_bias=%q want up", got.IndicatorCrossTF.DecisionTFBias)
	}
	if got.IndicatorCrossTF.Alignment != "conflict" {
		t.Fatalf("alignment=%q want conflict", got.IndicatorCrossTF.Alignment)
	}
	if got.StructureAnchorCtx == nil {
		t.Fatalf("structure context missing")
	}
	if got.StructureAnchorCtx.SupertrendState != "up" || got.StructureAnchorCtx.SupertrendInterval != "15m" {
		t.Fatalf("unexpected supertrend ctx: %+v", got.StructureAnchorCtx)
	}
	if got.StructureAnchorCtx.LatestBreakType != "bos_up" || got.StructureAnchorCtx.LatestBreakBarAge != 2 {
		t.Fatalf("unexpected break ctx: %+v", got.StructureAnchorCtx)
	}
	if got.MechanicsCtx == nil {
		t.Fatalf("mechanics context missing")
	}
	if got.MechanicsCtx.ReversalRisk != "medium" {
		t.Fatalf("reversal_risk=%q want medium", got.MechanicsCtx.ReversalRisk)
	}
	if len(got.MechanicsCtx.Conflicts) != 1 || got.MechanicsCtx.Conflicts[0] != "funding_long_but_oi_falling" {
		t.Fatalf("conflicts=%v", got.MechanicsCtx.Conflicts)
	}
	if got.MechanicsCtx.LiquidationState == nil || got.MechanicsCtx.LiquidationState.Status != "warming_up" {
		t.Fatalf("liquidation_state=%+v want warming_up", got.MechanicsCtx.LiquidationState)
	}
	if got.MechanicsCtx.LiquidationSource == nil || got.MechanicsCtx.LiquidationSource.Source != "binance_force_order_snapshot_ws" {
		t.Fatalf("liquidation_source=%+v", got.MechanicsCtx.LiquidationSource)
	}
}

func buildIndicatorStateJSONForProviderCtxTest(t *testing.T, symbol string) features.IndicatorJSON {
	t.Helper()
	byInterval := map[string]features.IndicatorJSON{
		"5m":  buildIndicatorJSONForProviderCtxTest(t, symbol, "5m", 105, 97, 12, 102, []float64{98, 102}, 101, []float64{101, 101}, 100, []float64{101, 100}, 62, 0.2, 4, 6, 0.03, "rising"),
		"15m": buildIndicatorJSONForProviderCtxTest(t, symbol, "15m", 104, 102, 20, 101, []float64{100, 101}, 100, []float64{99, 100}, 99, []float64{98, 99}, 58, 0.18, 5, 4, 0.02, "rising"),
		"1h":  buildIndicatorJSONForProviderCtxTest(t, symbol, "1h", 96, 98, 30, 98, []float64{99, 98}, 99, []float64{100, 99}, 100, []float64{101, 100}, 42, -0.2, 6, -3, -0.03, "falling"),
	}
	state, err := features.BuildIndicatorStateJSON(symbol, byInterval, "15m")
	if err != nil {
		t.Fatalf("BuildIndicatorStateJSON: %v", err)
	}
	return state
}

func buildIndicatorJSONForProviderCtxTest(t *testing.T, symbol, interval string, currentPrice, previousPrice float64, age int64, fast float64, fastLastN []float64, mid float64, midLastN []float64, slow float64, slowLastN []float64, rsi float64, rsiNorm float64, atr float64, atrChange float64, obvChange float64, stcState string) features.IndicatorJSON {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"_meta": map[string]any{
			"series_order": "oldest_to_latest",
			"sampled_at":   "2026-04-11T00:00:00Z",
			"version":      "indicator_compress_v1",
			"data_age_sec": map[string]int64{"indicator": age},
		},
		"market": map[string]any{
			"symbol":          symbol,
			"interval":        interval,
			"current_price":   currentPrice,
			"previous_price":  previousPrice,
			"price_timestamp": "2026-04-11T00:00:00Z",
		},
		"data": map[string]any{
			"ema_fast": map[string]any{"latest": fast, "last_n": fastLastN},
			"ema_mid":  map[string]any{"latest": mid, "last_n": midLastN},
			"ema_slow": map[string]any{"latest": slow, "last_n": slowLastN},
			"rsi":      map[string]any{"current": rsi, "normalized_slope": rsiNorm},
			"atr":      map[string]any{"latest": atr, "change_pct": atrChange},
			"obv":      map[string]any{"change_rate": obvChange},
			"stc":      map[string]any{"state": stcState},
		},
	})
	if err != nil {
		t.Fatalf("marshal indicator input: %v", err)
	}
	return features.IndicatorJSON{Symbol: symbol, Interval: interval, RawJSON: raw}
}
