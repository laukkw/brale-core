package decision

import (
	"encoding/json"
	"testing"

	"brale-core/internal/decision/features"
)

func TestBuildProviderDataContextUsesDecisionIntervalAndParsesAnchors(t *testing.T) {
	symbol := "BTCUSDT"
	comp := features.CompressionResult{
		Indicators: map[string]map[string]features.IndicatorJSON{
			symbol: {
				"5m":  buildIndicatorJSONForProviderCtxTest(t, symbol, "5m", 105, 97, 12, 102, []float64{98, 102}, 101, []float64{101, 101}, 100, []float64{101, 100}, 62, 0.2, 4, 6, 0.03, "rising"),
				"15m": buildIndicatorJSONForProviderCtxTest(t, symbol, "15m", 104, 102, 20, 101, []float64{100, 101}, 100, []float64{99, 100}, 99, []float64{98, 99}, 58, 0.18, 5, 4, 0.02, "rising"),
				"1h":  buildIndicatorJSONForProviderCtxTest(t, symbol, "1h", 96, 98, 30, 98, []float64{99, 98}, 99, []float64{100, 99}, 100, []float64{101, 100}, 42, -0.2, 6, -3, -0.03, "falling"),
			},
		},
		Trends: map[string]map[string]features.TrendJSON{
			symbol: {
				"15m": {Symbol: symbol, Interval: "15m", RawJSON: []byte(`{"break_summary":{"latest_event_type":"bos_up","latest_event_age":2},"supertrend":{"state":"up","interval":"15m"}}`)},
			},
		},
		Mechanics: map[string]features.MechanicsSnapshot{
			symbol: {Symbol: symbol, RawJSON: []byte(`{"crowding_state":{"reversal_risk":"medium"},"mechanics_conflict":["funding_long_but_oi_falling"]}`)},
		},
	}

	got := BuildProviderDataContext(comp, symbol, "15m")
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
