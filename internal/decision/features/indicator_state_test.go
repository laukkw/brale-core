package features

import (
	"encoding/json"
	"testing"
)

func TestSelectIndicatorIntervalsReturnsShortDecisionLong(t *testing.T) {
	got := selectIndicatorIntervals("15m", []string{"4h", "1h", "15m", "5m"})
	want := []string{"5m", "15m", "1h"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("interval[%d]=%q want %q (%v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildIndicatorStateJSONBuildsMultiTFSummary(t *testing.T) {
	byInterval := map[string]IndicatorJSON{
		"5m":  buildIndicatorJSONForStateTest(t, "BTCUSDT", "5m", 105, 97, 12, indicatorDataForStateTest(102, []float64{98, 102}, 101, []float64{101, 101}, 100, []float64{101, 100}, 62, 0.2, 4, 6, 0.03, "rising")),
		"15m": buildIndicatorJSONForStateTest(t, "BTCUSDT", "15m", 104, 102, 20, indicatorDataForStateTest(101, []float64{100, 101}, 100, []float64{99, 100}, 99, []float64{98, 99}, 58, 0.18, 5, 4, 0.02, "rising")),
		"1h":  buildIndicatorJSONForStateTest(t, "BTCUSDT", "1h", 96, 98, 30, indicatorDataForStateTest(98, []float64{99, 98}, 99, []float64{100, 99}, 100, []float64{101, 100}, 42, -0.2, 6, -3, -0.03, "falling")),
	}

	got, err := BuildIndicatorStateJSON("BTCUSDT", byInterval, "15m")
	if err != nil {
		t.Fatalf("BuildIndicatorStateJSON() error = %v", err)
	}

	var payload indicatorStateInput
	if err := json.Unmarshal(got.RawJSON, &payload); err != nil {
		t.Fatalf("unmarshal state input: %v", err)
	}
	if payload.DecisionInterval != "15m" {
		t.Fatalf("decision_interval=%q want 15m", payload.DecisionInterval)
	}
	if len(payload.MultiTF) != 3 {
		t.Fatalf("multi_tf len=%d want 3", len(payload.MultiTF))
	}
	if payload.MultiTF[0].Interval != "5m" {
		t.Fatalf("first interval=%q want 5m", payload.MultiTF[0].Interval)
	}
	if payload.MultiTF[0].Trend.PriceVsEMAFast != "above" {
		t.Fatalf("price_vs_ema_fast=%q want above", payload.MultiTF[0].Trend.PriceVsEMAFast)
	}
	if payload.MultiTF[0].Volatility.ATRExpandState != "expanding" {
		t.Fatalf("atr_expand_state=%q want expanding", payload.MultiTF[0].Volatility.ATRExpandState)
	}
	if payload.MultiTF[0].Bias != "up" {
		t.Fatalf("bias=%q want up", payload.MultiTF[0].Bias)
	}
	if !containsString(payload.MultiTF[0].Events, "price_cross_ema_fast_up") {
		t.Fatalf("events=%v should contain price_cross_ema_fast_up", payload.MultiTF[0].Events)
	}
	if !containsString(payload.MultiTF[0].Events, "ema_stack_bull_flip") {
		t.Fatalf("events=%v should contain ema_stack_bull_flip", payload.MultiTF[0].Events)
	}
	if payload.CrossTFSummary.Alignment != "conflict" {
		t.Fatalf("alignment=%q want conflict", payload.CrossTFSummary.Alignment)
	}
	if payload.CrossTFSummary.DecisionTFBias != "up" {
		t.Fatalf("decision_tf_bias=%q want up", payload.CrossTFSummary.DecisionTFBias)
	}
	if payload.CrossTFSummary.HigherTFAgreement {
		t.Fatalf("higher_tf_agreement=%v want false", payload.CrossTFSummary.HigherTFAgreement)
	}
}

func TestBuildIndicatorStateJSONIncludesExtendedIndicatorFieldsAndEvents(t *testing.T) {
	data := indicatorDataForStateTest(101, []float64{99, 101}, 100, []float64{99, 100}, 98, []float64{99, 98}, 61, 0.2, 8, 7, 0.04, "rising")
	data.BB = &bbSnapshot{PercentB: 0.91, Width: 1.5}
	data.CHOP = &chopSnapshot{Value: 65}
	data.StochRSI = &stochRSISnapshot{Value: 0.1}
	data.Aroon = &aroonSnapshot{Up: 80, Down: 20}
	data.TDSequential = &tdSequentialSnapshot{BuySetup: 8}

	byInterval := map[string]IndicatorJSON{
		"15m": buildIndicatorJSONForStateTest(t, "BTCUSDT", "15m", 103, 99, 12, data),
	}

	got, err := BuildIndicatorStateJSON("BTCUSDT", byInterval, "15m")
	if err != nil {
		t.Fatalf("BuildIndicatorStateJSON() error = %v", err)
	}

	var payload indicatorStateInput
	if err := json.Unmarshal(got.RawJSON, &payload); err != nil {
		t.Fatalf("unmarshal state input: %v", err)
	}
	if len(payload.MultiTF) != 1 {
		t.Fatalf("multi_tf len=%d want 1", len(payload.MultiTF))
	}
	state := payload.MultiTF[0]
	if state.Momentum.StochRSIZone != "oversold" {
		t.Fatalf("stoch_rsi_zone=%q want oversold", state.Momentum.StochRSIZone)
	}
	if state.Volatility.BBZone != "near_upper" {
		t.Fatalf("bb_zone=%q want near_upper", state.Volatility.BBZone)
	}
	if state.Volatility.BBWidthState != "squeeze" {
		t.Fatalf("bb_width_state=%q want squeeze", state.Volatility.BBWidthState)
	}
	if state.Volatility.CHOPRegime != "choppy" {
		t.Fatalf("chop_regime=%q want choppy", state.Volatility.CHOPRegime)
	}
	if !containsString(state.Events, "aroon_strong_bullish") {
		t.Fatalf("events=%v should contain aroon_strong_bullish", state.Events)
	}
	if !containsString(state.Events, "td_buy_setup_8") {
		t.Fatalf("events=%v should contain td_buy_setup_8", state.Events)
	}
}

func buildIndicatorJSONForStateTest(t *testing.T, symbol, interval string, currentPrice, previousPrice float64, age int64, data indicatorData) IndicatorJSON {
	t.Helper()

	raw, err := json.Marshal(IndicatorCompressedInput{
		Meta: indicatorMeta{
			SeriesOrder: "oldest_to_latest",
			SampledAt:   "2026-04-11T00:00:00Z",
			Version:     indicatorCompressVersion,
			DataAgeSec:  map[string]int64{"indicator": age},
		},
		Market: indicatorMarket{
			Symbol:       symbol,
			Interval:     interval,
			CurrentPrice: currentPrice,
			PreviousPrice: previousPrice,
		},
		Data: data,
	})
	if err != nil {
		t.Fatalf("marshal indicator input: %v", err)
	}
	return IndicatorJSON{Symbol: symbol, Interval: interval, RawJSON: raw}
}

func indicatorDataForStateTest(fast float64, fastLastN []float64, mid float64, midLastN []float64, slow float64, slowLastN []float64, rsi float64, rsiNorm float64, atr float64, atrChange float64, obvChange float64, stcState string) indicatorData {
	return indicatorData{
		EMAFast: &emaSnapshot{Latest: fast, LastN: fastLastN},
		EMAMid:  &emaSnapshot{Latest: mid, LastN: midLastN},
		EMASlow: &emaSnapshot{Latest: slow, LastN: slowLastN},
		RSI: &rsiSnapshot{
			Current:         rsi,
			NormalizedSlope: floatPtr(rsiNorm),
		},
		ATR: &atrSnapshot{
			Latest:    atr,
			ChangePct: floatPtr(atrChange),
		},
		OBV: &obvSnapshot{
			ChangeRate: floatPtr(obvChange),
		},
		STC: &stcSnapshot{
			State: stcState,
		},
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
