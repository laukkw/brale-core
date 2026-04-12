package features

import (
	"encoding/json"
	"testing"
)

func TestClassifyPriceVsEMA(t *testing.T) {
	tests := []struct {
		name  string
		price float64
		ema   float64
		atr   float64
		want  string
	}{
		{name: "zero price treated as near", price: 0, ema: 100, atr: 10, want: "near"},
		{name: "within atr threshold treated as near", price: 102.5, ema: 100, atr: 10, want: "near"},
		{name: "above beyond threshold", price: 103, ema: 100, atr: 10, want: "above"},
		{name: "below beyond threshold", price: 97, ema: 100, atr: 10, want: "below"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPriceVsEMA(tt.price, tt.ema, tt.atr); got != tt.want {
				t.Fatalf("classifyPriceVsEMA() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyEMAStack(t *testing.T) {
	tests := []struct {
		name string
		fast float64
		mid  float64
		slow float64
		want string
	}{
		{name: "bull stack", fast: 105, mid: 100, slow: 95, want: "bull"},
		{name: "bear stack", fast: 95, mid: 100, slow: 105, want: "bear"},
		{name: "mixed stack", fast: 105, mid: 95, slow: 100, want: "mixed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyEMAStack(tt.fast, tt.mid, tt.slow); got != tt.want {
				t.Fatalf("classifyEMAStack() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyRSIZone(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 34.9, want: "<35"},
		{value: 35, want: "35_45"},
		{value: 45, want: "45_55"},
		{value: 55, want: "55_65"},
		{value: 65, want: ">65"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := classifyRSIZone(tt.value); got != tt.want {
				t.Fatalf("classifyRSIZone(%v) = %q want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestClassifyRSISlope(t *testing.T) {
	tests := []struct {
		name string
		rsi  *rsiSnapshot
		want string
	}{
		{name: "nil snapshot", rsi: nil, want: "flat"},
		{name: "nil norm slope", rsi: &rsiSnapshot{}, want: "flat"},
		{name: "rising boundary", rsi: &rsiSnapshot{NormalizedSlope: floatPtr(0.15)}, want: "rising"},
		{name: "falling boundary", rsi: &rsiSnapshot{NormalizedSlope: floatPtr(-0.15)}, want: "falling"},
		{name: "inside flat band", rsi: &rsiSnapshot{NormalizedSlope: floatPtr(0.1499)}, want: "flat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyRSISlope(tt.rsi); got != tt.want {
				t.Fatalf("classifyRSISlope() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifySTCState(t *testing.T) {
	tests := []struct {
		name string
		stc  *stcSnapshot
		want string
	}{
		{name: "nil snapshot", stc: nil, want: "flat"},
		{name: "blank state", stc: &stcSnapshot{}, want: "flat"},
		{name: "normalizes casing and spaces", stc: &stcSnapshot{State: " Rising "}, want: "rising"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifySTCState(tt.stc); got != tt.want {
				t.Fatalf("classifySTCState() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyOBVSlope(t *testing.T) {
	tests := []struct {
		name string
		obv  *obvSnapshot
		want string
	}{
		{name: "nil snapshot", obv: nil, want: "flat"},
		{name: "up boundary", obv: &obvSnapshot{ChangeRate: floatPtr(0.02)}, want: "up"},
		{name: "down boundary", obv: &obvSnapshot{ChangeRate: floatPtr(-0.02)}, want: "down"},
		{name: "inside flat band", obv: &obvSnapshot{ChangeRate: floatPtr(0.0199)}, want: "flat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyOBVSlope(tt.obv); got != tt.want {
				t.Fatalf("classifyOBVSlope() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyATRExpansion(t *testing.T) {
	tests := []struct {
		name string
		atr  *atrSnapshot
		want string
	}{
		{name: "nil snapshot", atr: nil, want: "stable"},
		{name: "expanding boundary", atr: &atrSnapshot{ChangePct: floatPtr(5)}, want: "expanding"},
		{name: "contracting boundary", atr: &atrSnapshot{ChangePct: floatPtr(-5)}, want: "contracting"},
		{name: "inside stable band", atr: &atrSnapshot{ChangePct: floatPtr(4.99)}, want: "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyATRExpansion(tt.atr); got != tt.want {
				t.Fatalf("classifyATRExpansion() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyBBZone(t *testing.T) {
	tests := []struct {
		name string
		bb   *bbSnapshot
		want string
	}{
		{name: "nil snapshot", bb: nil, want: ""},
		{name: "below lower", bb: &bbSnapshot{PercentB: -0.01}, want: "below_lower"},
		{name: "near lower boundary", bb: &bbSnapshot{PercentB: 0.2}, want: "near_lower"},
		{name: "mid band", bb: &bbSnapshot{PercentB: 0.5}, want: "mid"},
		{name: "near upper boundary", bb: &bbSnapshot{PercentB: 0.8}, want: "near_upper"},
		{name: "above upper boundary", bb: &bbSnapshot{PercentB: 1.0}, want: "above_upper"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyBBZone(tt.bb); got != tt.want {
				t.Fatalf("classifyBBZone() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyBBWidthState(t *testing.T) {
	tests := []struct {
		name string
		bb   *bbSnapshot
		want string
	}{
		{name: "nil snapshot", bb: nil, want: ""},
		{name: "squeeze below threshold", bb: &bbSnapshot{Width: 1.99}, want: "squeeze"},
		{name: "normal at lower boundary", bb: &bbSnapshot{Width: 2.0}, want: "normal"},
		{name: "normal at upper boundary", bb: &bbSnapshot{Width: 6.0}, want: "normal"},
		{name: "wide above threshold", bb: &bbSnapshot{Width: 6.01}, want: "wide"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyBBWidthState(tt.bb); got != tt.want {
				t.Fatalf("classifyBBWidthState() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyCHOPRegime(t *testing.T) {
	tests := []struct {
		name string
		chop *chopSnapshot
		want string
	}{
		{name: "nil snapshot", chop: nil, want: ""},
		{name: "trending below boundary", chop: &chopSnapshot{Value: 38.19}, want: "trending"},
		{name: "transition at lower boundary", chop: &chopSnapshot{Value: 38.2}, want: "transition"},
		{name: "transition at upper boundary", chop: &chopSnapshot{Value: 61.8}, want: "transition"},
		{name: "choppy above boundary", chop: &chopSnapshot{Value: 61.81}, want: "choppy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCHOPRegime(tt.chop); got != tt.want {
				t.Fatalf("classifyCHOPRegime() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyStochRSIZone(t *testing.T) {
	tests := []struct {
		name string
		sr   *stochRSISnapshot
		want string
	}{
		{name: "nil snapshot", sr: nil, want: ""},
		{name: "oversold boundary", sr: &stochRSISnapshot{Value: 0.2}, want: "oversold"},
		{name: "neutral mid range", sr: &stochRSISnapshot{Value: 0.5}, want: "neutral"},
		{name: "overbought boundary", sr: &stochRSISnapshot{Value: 0.8}, want: "overbought"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyStochRSIZone(tt.sr); got != tt.want {
				t.Fatalf("classifyStochRSIZone() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyAroonSignal(t *testing.T) {
	tests := []struct {
		name  string
		aroon *aroonSnapshot
		want  string
	}{
		{name: "nil snapshot", aroon: nil, want: ""},
		{name: "strong up", aroon: &aroonSnapshot{Up: 71, Down: 29}, want: "strong_up"},
		{name: "strong down", aroon: &aroonSnapshot{Up: 29, Down: 71}, want: "strong_down"},
		{name: "crossover", aroon: &aroonSnapshot{Up: 71, Down: 71}, want: "crossover"},
		{name: "neutral at threshold", aroon: &aroonSnapshot{Up: 70, Down: 30}, want: "neutral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyAroonSignal(tt.aroon); got != tt.want {
				t.Fatalf("classifyAroonSignal() = %q want %q", got, tt.want)
			}
		})
	}
}

func TestComputeIndicatorBias(t *testing.T) {
	tests := []struct {
		name  string
		state indicatorTFState
		want  string
	}{
		{
			name: "up bias with three aligned signals",
			state: indicatorTFState{
				Trend:    indicatorTrendState{PriceVsEMAMid: "above", EMAStack: "bull"},
				Momentum: indicatorMomentumState{RSISlopeState: "rising", STCState: "flat", OBVSlopeState: "flat"},
			},
			want: "up",
		},
		{
			name: "down bias with three aligned signals",
			state: indicatorTFState{
				Trend:    indicatorTrendState{PriceVsEMAMid: "below", EMAStack: "bear"},
				Momentum: indicatorMomentumState{RSISlopeState: "falling", STCState: "flat", OBVSlopeState: "flat"},
			},
			want: "down",
		},
		{
			name: "mixed when votes conflict",
			state: indicatorTFState{
				Trend:    indicatorTrendState{PriceVsEMAMid: "above", EMAStack: "bear"},
				Momentum: indicatorMomentumState{RSISlopeState: "rising", STCState: "falling", OBVSlopeState: "flat"},
			},
			want: "mixed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeIndicatorBias(tt.state); got != tt.want {
				t.Fatalf("computeIndicatorBias() = %q want %q", got, tt.want)
			}
		})
	}
}

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
			Symbol:        symbol,
			Interval:      interval,
			CurrentPrice:  currentPrice,
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
