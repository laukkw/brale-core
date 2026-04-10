package decision

import (
	"testing"

	"brale-core/internal/decision/features"
)

func TestBuildStructureAnchorSummary_ExposesSuperTrendWithoutUsingItAsNearestAnchor(t *testing.T) {
	raw := []byte(`{
		"meta":{"symbol":"BTCUSDT","interval":"1h","timestamp":"2026-04-10T00:00:00Z"},
		"structure_points":[],
		"structure_candidates":[
			{"price":99,"type":"support","source":"range_low","age_candles":1},
			{"price":105,"type":"resistance","source":"range_high","age_candles":1}
		],
		"recent_candles":[],
		"global_context":{"trend_slope":0,"normalized_slope":0,"vol_ratio":1,"window":100},
		"smc":{"order_block":{"type":"none"},"fvg":{"type":"none"}},
		"supertrend":{"state":"UP","level":101,"distance_pct":0.8}
	}`)

	comp := features.CompressionResult{
		Trends: map[string]map[string]features.TrendJSON{
			"BTCUSDT": {
				"1h": {Symbol: "BTCUSDT", Interval: "1h", RawJSON: raw},
			},
		},
	}

	summary, err := buildStructureAnchorSummary(comp, "BTCUSDT", 102, 2)
	if err != nil {
		t.Fatalf("buildStructureAnchorSummary() error = %v", err)
	}

	superTrend, ok := summary["supertrend"].(map[string]any)
	if !ok {
		t.Fatalf("supertrend summary missing")
	}
	if superTrend["level"] != 101.0 {
		t.Fatalf("supertrend.level=%v want 101", superTrend["level"])
	}

	below, ok := summary["nearest_below_entry"].(map[string]any)
	if !ok {
		t.Fatalf("nearest_below_entry missing")
	}
	if below["source"] == "supertrend" {
		t.Fatalf("supertrend must not participate in nearest anchor competition")
	}

	above, ok := summary["nearest_above_entry"].(map[string]any)
	if !ok {
		t.Fatalf("nearest_above_entry missing")
	}
	if above["source"] == "supertrend" {
		t.Fatalf("supertrend must not participate in nearest anchor competition")
	}
}
