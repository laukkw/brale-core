package features

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildMechanicsStateSummaryBuildsStateAndConflicts(t *testing.T) {
	input := MechanicsCompressedInput{
		Timestamp: "2026-04-11T00:00:20Z",
		OIHistory: map[string]oiHistoryPayload{
			"15m": {
				Value:          1200,
				ChangePct:      8.1,
				Price:          103,
				PriceChangePct: 3.2,
			},
		},
		Funding: &fundingPayload{
			Rate:      0.0007,
			Timestamp: "2026-04-11T00:00:15Z",
		},
		FuturesSentiment: &futuresSentimentPayload{
			TopTraderLSR:           1.16,
			LSRatio:                1.42,
			TakerLongShortVolRatio: 1.18,
			Timestamp:              "2026-04-11T00:00:15Z",
		},
		LiquidationsByWindow: map[string]liqWindowPayload{
			"5m": {
				Imbalance: 0.12,
				Rel: &liqRelPayload{
					ZScore:    1.2,
					VolOverOI: 0.03,
				},
			},
			"1h": {
				Imbalance: 0.35,
				Rel: &liqRelPayload{
					ZScore:    1.8,
					VolOverOI: 0.05,
					Spike:     true,
				},
			},
		},
		FearGreed: &fearGreedPayload{
			Value:     68,
			Timestamp: "2026-04-11T00:00:15Z",
		},
	}

	got, err := BuildMechanicsStateSummary(input)
	if err != nil {
		t.Fatalf("BuildMechanicsStateSummary() error = %v", err)
	}
	if got.FreshnessSec != 5 {
		t.Fatalf("freshness_sec=%d want 5", got.FreshnessSec)
	}
	if got.OIState == nil || got.OIState.ChangeState != "rising" {
		t.Fatalf("oi_state=%+v want rising", got.OIState)
	}
	if got.OIState.OIPriceRelation != "price_up_oi_up" {
		t.Fatalf("oi_price_relation=%q want price_up_oi_up", got.OIState.OIPriceRelation)
	}
	if got.FundingState == nil || got.FundingState.Bias != "long" || got.FundingState.Heat != "hot" {
		t.Fatalf("funding_state=%+v want long/hot", got.FundingState)
	}
	if got.CrowdingState == nil || got.CrowdingState.Bias != "long_crowded" || got.CrowdingState.ReversalRisk != "high" {
		t.Fatalf("crowding_state=%+v want long_crowded/high", got.CrowdingState)
	}
	if got.LiquidationState == nil || got.LiquidationState.Window != "1h" || got.LiquidationState.Stress != "high" {
		t.Fatalf("liquidation_state=%+v want 1h/high", got.LiquidationState)
	}
	if got.SentimentState == nil || got.SentimentState.FearGreed != "greed" || got.SentimentState.TopTraderBias != "long" {
		t.Fatalf("sentiment_state=%+v want greed/long", got.SentimentState)
	}
	if !containsString(got.MechanicsConflict, "crowding_long_but_liq_stress_high") {
		t.Fatalf("mechanics_conflict=%v should contain crowding_long_but_liq_stress_high", got.MechanicsConflict)
	}
	if len(got.Missing) != 0 {
		t.Fatalf("missing=%v want empty", got.Missing)
	}
}

func TestBuildMechanicsStateRawDropsLegacyFields(t *testing.T) {
	input := MechanicsCompressedInput{
		Timestamp:              "2026-04-11T00:00:20Z",
		FearGreedNextUpdateSec: 300,
		FearGreedHistory: []fearGreedHistoryPoint{
			{Value: 40, Classification: "neutral", Timestamp: "2026-04-11T00:00:10Z"},
		},
		SentimentByInterval: map[string]sentimentPayload{
			"5m": {Score: 3, Tag: "bullish"},
		},
		OIHistory: map[string]oiHistoryPayload{
			"15m": {ChangePct: -4.2, PriceChangePct: 1.1},
		},
		Funding: &fundingPayload{
			Rate: -0.0006,
		},
		LiquidationsByWindow: map[string]liqWindowPayload{
			"1h": {
				Imbalance: 0.28,
				Bins: []liqPriceBinPayload{
					{Bps: 25, LongVol: 10, ShortVol: 20, TotalVol: 30, Imbalance: -0.33},
				},
				Rel: &liqRelPayload{ZScore: 2.1, VolOverOI: 0.06},
			},
		},
		FearGreed: &fearGreedPayload{Value: 24},
	}

	raw, err := buildMechanicsStateRaw(input, false)
	if err != nil {
		t.Fatalf("buildMechanicsStateRaw() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal state raw: %v", err)
	}

	serialized := string(raw)
	for _, field := range []string{
		`"timestamp"`,
		`"fear_greed_history"`,
		`"fear_greed_next_update_sec"`,
		`"sentiment_by_interval"`,
		`"bins"`,
	} {
		if strings.Contains(serialized, field) {
			t.Fatalf("state raw should not contain %s: %s", field, serialized)
		}
	}
	if _, ok := payload["oi_state"]; !ok {
		t.Fatalf("state raw missing oi_state: %s", serialized)
	}
	if _, ok := payload["liquidation_state"]; !ok {
		t.Fatalf("state raw missing liquidation_state: %s", serialized)
	}
	if _, ok := payload["sentiment_state"]; !ok {
		t.Fatalf("state raw missing sentiment_state: %s", serialized)
	}
}
