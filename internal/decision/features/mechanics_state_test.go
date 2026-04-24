package features

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyFundingState(t *testing.T) {
	tests := []struct {
		name  string
		input MechanicsCompressedInput
		want  *mechanicsFundingState
	}{
		{
			name:  "missing funding returns nil",
			input: MechanicsCompressedInput{},
			want:  nil,
		},
		{
			name:  "neutral at bias threshold",
			input: MechanicsCompressedInput{Funding: &fundingPayload{Rate: 0.0001}},
			want:  &mechanicsFundingState{Bias: "neutral", Heat: "neutral", Rate: 0.0001},
		},
		{
			name:  "long hot boundary",
			input: MechanicsCompressedInput{Funding: &fundingPayload{Rate: 0.0005}},
			want:  &mechanicsFundingState{Bias: "long", Heat: "hot", Rate: 0.0005},
		},
		{
			name:  "short hot boundary",
			input: MechanicsCompressedInput{Funding: &fundingPayload{Rate: -0.0005}},
			want:  &mechanicsFundingState{Bias: "short", Heat: "hot", Rate: -0.0005},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyFundingState(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("classifyFundingState() = %+v want nil", got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("classifyFundingState() = %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestClassifyCrowdingState(t *testing.T) {
	tests := []struct {
		name    string
		input   MechanicsCompressedInput
		funding *mechanicsFundingState
		liq     *mechanicsLiquidationState
		want    *mechanicsCrowdingState
	}{
		{
			name:    "missing anchors returns nil",
			input:   MechanicsCompressedInput{},
			funding: nil,
			liq:     nil,
			want:    nil,
		},
		{
			name: "long crowded with high reversal risk",
			input: MechanicsCompressedInput{
				FuturesSentiment: &futuresSentimentPayload{
					LSRatio:                1.21,
					TakerLongShortVolRatio: 1.11,
				},
			},
			funding: &mechanicsFundingState{Heat: "hot"},
			liq:     &mechanicsLiquidationState{Stress: "high"},
			want:    &mechanicsCrowdingState{Bias: "long_crowded", LSRatio: 1.21, TakerRatio: 1.11, ReversalRisk: "high"},
		},
		{
			name: "short crowded with medium reversal risk",
			input: MechanicsCompressedInput{
				FuturesSentiment: &futuresSentimentPayload{
					LSRatio:                0.79,
					TakerLongShortVolRatio: 0.89,
				},
			},
			funding: &mechanicsFundingState{Heat: "hot"},
			liq:     &mechanicsLiquidationState{Stress: "elevated"},
			want:    &mechanicsCrowdingState{Bias: "short_crowded", LSRatio: 0.79, TakerRatio: 0.89, ReversalRisk: "medium"},
		},
		{
			name: "balanced at thresholds",
			input: MechanicsCompressedInput{
				FuturesSentiment: &futuresSentimentPayload{
					LSRatio:                1.2,
					TakerLongShortVolRatio: 1.1,
				},
			},
			want: &mechanicsCrowdingState{Bias: "balanced", LSRatio: 1.2, TakerRatio: 1.1, ReversalRisk: "low"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCrowdingState(tt.input, tt.funding, tt.liq)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("classifyCrowdingState() = %+v want nil", got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("classifyCrowdingState() = %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestClassifySentimentState(t *testing.T) {
	tests := []struct {
		name  string
		input MechanicsCompressedInput
		want  *mechanicsSentimentState
	}{
		{
			name:  "missing inputs returns nil",
			input: MechanicsCompressedInput{},
			want:  nil,
		},
		{
			name: "fear boundary with long top trader bias",
			input: MechanicsCompressedInput{
				FearGreed:        &fearGreedPayload{Value: 25},
				FuturesSentiment: &futuresSentimentPayload{TopTraderLSR: 1.11},
			},
			want: &mechanicsSentimentState{FearGreed: "fear", TopTraderBias: "long"},
		},
		{
			name: "neutral boundary with neutral top trader bias",
			input: MechanicsCompressedInput{
				FearGreed:        &fearGreedPayload{Value: 55},
				FuturesSentiment: &futuresSentimentPayload{TopTraderLSR: 1.1},
			},
			want: &mechanicsSentimentState{FearGreed: "neutral", TopTraderBias: "neutral"},
		},
		{
			name: "greed boundary with lsr fallback to short",
			input: MechanicsCompressedInput{
				FearGreed:        &fearGreedPayload{Value: 75},
				FuturesSentiment: &futuresSentimentPayload{LSRatio: 0.89},
			},
			want: &mechanicsSentimentState{FearGreed: "greed", TopTraderBias: "short"},
		},
		{
			name: "extreme greed above boundary",
			input: MechanicsCompressedInput{
				FearGreed: &fearGreedPayload{Value: 76},
			},
			want: &mechanicsSentimentState{FearGreed: "extreme_greed", TopTraderBias: "unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifySentimentState(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("classifySentimentState() = %+v want nil", got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("classifySentimentState() = %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestSummarizeLiquidationWindow(t *testing.T) {
	tests := []struct {
		name    string
		payload liqWindowPayload
		want    mechanicsLiquidationState
	}{
		{
			name:    "low below thresholds",
			payload: liqWindowPayload{Imbalance: 0.12, Rel: &liqRelPayload{ZScore: 1.49, VolOverOI: 0.039}},
			want:    mechanicsLiquidationState{Stress: "low", Window: "15m", ZScore: 1.49, VolOverOI: 0.039, Spike: false, Imbalance: 0.12},
		},
		{
			name:    "elevated at thresholds",
			payload: liqWindowPayload{Imbalance: -0.2, Rel: &liqRelPayload{ZScore: 1.5, VolOverOI: 0.04}},
			want:    mechanicsLiquidationState{Stress: "elevated", Window: "15m", ZScore: 1.5, VolOverOI: 0.04, Spike: false, Imbalance: -0.2},
		},
		{
			name:    "high at zscore threshold",
			payload: liqWindowPayload{Imbalance: 0.35, Rel: &liqRelPayload{ZScore: 2.5, VolOverOI: 0.01}},
			want:    mechanicsLiquidationState{Stress: "high", Window: "15m", ZScore: 2.5, VolOverOI: 0.01, Spike: false, Imbalance: 0.35},
		},
		{
			name:    "high on spike",
			payload: liqWindowPayload{Rel: &liqRelPayload{Spike: true}},
			want:    mechanicsLiquidationState{Stress: "high", Window: "15m", ZScore: 0, VolOverOI: 0, Spike: true, Imbalance: 0},
		},
		{
			name:    "warming_up is unknown not low",
			payload: liqWindowPayload{Status: "warming_up", CoverageSec: 1200, Complete: false, Rel: &liqRelPayload{ZScore: 2.9, VolOverOI: 0.2, Spike: true}},
			want:    mechanicsLiquidationState{Stress: "unknown", Status: "warming_up", Complete: false, Window: "15m", ZScore: 2.9, VolOverOI: 0.2, Spike: true, Imbalance: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeLiquidationWindow("15m", tt.payload)
			if got != tt.want {
				t.Fatalf("summarizeLiquidationWindow() = %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestLiquidationStressScore(t *testing.T) {
	tests := []struct {
		stress string
		want   int
	}{
		{stress: "high", want: 3},
		{stress: "elevated", want: 2},
		{stress: "low", want: 1},
		{stress: "unknown", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.stress, func(t *testing.T) {
			if got := liquidationStressScore(tt.stress); got != tt.want {
				t.Fatalf("liquidationStressScore(%q) = %d want %d", tt.stress, got, tt.want)
			}
		})
	}
}

func TestClassifyOIChangeState(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 2, want: "rising"},
		{value: -2, want: "falling"},
		{value: 1.99, want: "flat"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := classifyOIChangeState(tt.value); got != tt.want {
				t.Fatalf("classifyOIChangeState(%v) = %q want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestClassifyOIPriceRelation(t *testing.T) {
	tests := []struct {
		name      string
		priceMove float64
		oiMove    float64
		want      string
	}{
		{name: "price up oi up", priceMove: 1, oiMove: 1, want: "price_up_oi_up"},
		{name: "price up oi down", priceMove: 1, oiMove: -1, want: "price_up_oi_down"},
		{name: "price down oi up", priceMove: -1, oiMove: 1, want: "price_down_oi_up"},
		{name: "price down oi down", priceMove: -1, oiMove: -1, want: "price_down_oi_down"},
		{name: "both flat", priceMove: 0, oiMove: 0, want: "mixed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyOIPriceRelation(tt.priceMove, tt.oiMove); got != tt.want {
				t.Fatalf("classifyOIPriceRelation() = %q want %q", got, tt.want)
			}
		})
	}
}

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

func TestBuildMechanicsStateRawPreservesLiquidationQualityFields(t *testing.T) {
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
				Imbalance:   0.28,
				Status:      "warming_up",
				CoverageSec: 1800,
				SampleCount: 3,
				Complete:    false,
				Bins: []liqPriceBinPayload{
					{Bps: 25, LongVol: 10, ShortVol: 20, TotalVol: 30, Imbalance: -0.33},
				},
				Rel: &liqRelPayload{ZScore: 2.1, VolOverOI: 0.06},
			},
		},
		LiquidationSource: &liqSourcePayload{
			Source:          "binance_force_order_snapshot_ws",
			Coverage:        "largest_order_per_symbol_per_1000ms",
			Status:          "warming_up",
			StreamConnected: true,
			CoverageSec:     1800,
			SampleCount:     3,
			LastEventAgeSec: 45,
			Complete:        false,
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
	if _, ok := payload["liquidation_source"]; !ok {
		t.Fatalf("state raw missing liquidation_source: %s", serialized)
	}
	if _, ok := payload["liquidations_by_window"]; !ok {
		t.Fatalf("state raw missing liquidations_by_window: %s", serialized)
	}
	if _, ok := payload["sentiment_state"]; !ok {
		t.Fatalf("state raw missing sentiment_state: %s", serialized)
	}
}
