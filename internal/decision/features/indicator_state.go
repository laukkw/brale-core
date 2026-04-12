package features

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"brale-core/internal/interval"
)

type indicatorStateInput struct {
	DecisionInterval string                  `json:"decision_interval"`
	MultiTF          []indicatorTFState      `json:"multi_tf"`
	CrossTFSummary   indicatorCrossTFSummary `json:"cross_tf_summary"`
	Missing          []string                `json:"missing,omitempty"`
}

type indicatorTFState struct {
	Interval     string                   `json:"interval"`
	FreshnessSec int64                    `json:"freshness_sec,omitempty"`
	Missing      []string                 `json:"missing,omitempty"`
	Trend        indicatorTrendState      `json:"trend"`
	Momentum     indicatorMomentumState   `json:"momentum"`
	Volatility   indicatorVolatilityState `json:"volatility"`
	Bias         string                   `json:"bias"`
	Events       []string                 `json:"events,omitempty"`
}

type indicatorTrendState struct {
	PriceVsEMAFast     string  `json:"price_vs_ema_fast"`
	PriceVsEMAMid      string  `json:"price_vs_ema_mid"`
	PriceVsEMASlow     string  `json:"price_vs_ema_slow"`
	EMAStack           string  `json:"ema_stack"`
	EMADistanceFastATR float64 `json:"ema_distance_fast_atr"`
	EMADistanceMidATR  float64 `json:"ema_distance_mid_atr"`
	EMADistanceSlowATR float64 `json:"ema_distance_slow_atr"`
}

type indicatorMomentumState struct {
	RSIZone       string `json:"rsi_zone"`
	RSISlopeState string `json:"rsi_slope_state"`
	STCState      string `json:"stc_state"`
	OBVSlopeState string `json:"obv_slope_state"`
	StochRSIZone  string `json:"stoch_rsi_zone,omitempty"`
}

type indicatorVolatilityState struct {
	ATRExpandState string  `json:"atr_expand_state"`
	ATRChangePct   float64 `json:"atr_change_pct,omitempty"`
	BBZone         string  `json:"bb_zone,omitempty"`
	BBWidthState   string  `json:"bb_width_state,omitempty"`
	CHOPRegime     string  `json:"chop_regime,omitempty"`
}

type indicatorCrossTFSummary struct {
	DecisionTFBias    string `json:"decision_tf_bias"`
	LowerTFAgreement  bool   `json:"lower_tf_agreement"`
	HigherTFAgreement bool   `json:"higher_tf_agreement"`
	Alignment         string `json:"alignment"`
	ConflictCount     int    `json:"conflict_count"`
}

const (
	priceVsEMANearATRRatio = 0.25

	rsiZoneLowThreshold      = 35.0
	rsiZoneWeakLowThreshold  = 45.0
	rsiZoneWeakHighThreshold = 55.0
	rsiZoneHighThreshold     = 65.0

	rsiSlopeTrendThreshold = 0.15
	obvSlopeTrendThreshold = 0.02

	atrExpansionThresholdPct   = 5.0
	atrContractionThresholdPct = -5.0

	bbNearLowerPercentBThreshold  = 0.2
	bbNearUpperPercentBThreshold  = 0.8
	bbAboveUpperPercentBThreshold = 1.0
	bbWidthSqueezeThresholdPct    = 2.0
	bbWidthWideThresholdPct       = 6.0

	chopTrendingThreshold = 38.2
	chopChoppyThreshold   = 61.8

	stochRSIOversoldThreshold   = 0.2
	stochRSIOverboughtThreshold = 0.8

	aroonStrongThreshold = 70.0
	aroonWeakThreshold   = 30.0
)

func BuildIndicatorStateJSON(symbol string, byInterval map[string]IndicatorJSON, decisionInterval string) (IndicatorJSON, error) {
	if len(byInterval) == 0 {
		return IndicatorJSON{}, fmt.Errorf("indicator inputs missing for symbol=%s", symbol)
	}
	available := make([]string, 0, len(byInterval))
	for key := range byInterval {
		available = append(available, key)
	}
	selected := selectIndicatorIntervals(decisionInterval, available)
	if len(selected) == 0 {
		return IndicatorJSON{}, fmt.Errorf("indicator intervals unavailable for symbol=%s", symbol)
	}
	if strings.TrimSpace(decisionInterval) == "" {
		decisionInterval = selected[minInt(1, len(selected)-1)]
	}

	results := make([]indicatorTFState, 0, len(selected))
	missing := make([]string, 0, len(selected))
	for _, iv := range selected {
		raw, ok := byInterval[iv]
		if !ok {
			missing = append(missing, iv)
			continue
		}
		state, err := summarizeIndicatorJSON(raw)
		if err != nil {
			return IndicatorJSON{}, fmt.Errorf("summarize indicator state %s/%s: %w", symbol, iv, err)
		}
		results = append(results, state)
	}
	payload := indicatorStateInput{
		DecisionInterval: strings.ToLower(strings.TrimSpace(decisionInterval)),
		MultiTF:          results,
		CrossTFSummary:   buildIndicatorCrossTFSummary(results, decisionInterval),
		Missing:          missing,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return IndicatorJSON{}, err
	}
	return IndicatorJSON{Symbol: symbol, Interval: "multi", RawJSON: raw}, nil
}

func selectIndicatorIntervals(decisionInterval string, available []string) []string {
	if len(available) == 0 {
		return nil
	}
	keys := sortedIndicatorIntervals(available)
	target := strings.ToLower(strings.TrimSpace(decisionInterval))
	if target == "" {
		target = keys[0]
	}
	targetDur, err := interval.ParseInterval(target)
	if err != nil {
		return []string{keys[0]}
	}

	var shorter string
	var longer string
	for _, key := range keys {
		dur, parseErr := interval.ParseInterval(key)
		if parseErr != nil {
			continue
		}
		if dur < targetDur {
			shorter = key
			continue
		}
		if dur > targetDur && longer == "" {
			longer = key
			break
		}
	}

	selected := make([]string, 0, 3)
	appendUnique := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(selected, value) {
			return
		}
		selected = append(selected, value)
	}
	appendUnique(shorter)
	appendUnique(target)
	appendUnique(longer)
	return selected
}

func summarizeIndicatorJSON(raw IndicatorJSON) (indicatorTFState, error) {
	var payload IndicatorCompressedInput
	if err := json.Unmarshal(raw.RawJSON, &payload); err != nil {
		return indicatorTFState{}, err
	}
	return summarizeIndicatorPayload(payload), nil
}

func summarizeIndicatorPayload(payload IndicatorCompressedInput) indicatorTFState {
	atr := 0.0
	if payload.Data.ATR != nil {
		atr = payload.Data.ATR.Latest
	}
	state := indicatorTFState{
		Interval:     payload.Market.Interval,
		FreshnessSec: payload.Meta.DataAgeSec["indicator"],
		Trend: indicatorTrendState{
			PriceVsEMAFast:     classifyPriceVsEMA(payload.Market.CurrentPrice, emaLatest(payload.Data.EMAFast), atr),
			PriceVsEMAMid:      classifyPriceVsEMA(payload.Market.CurrentPrice, emaLatest(payload.Data.EMAMid), atr),
			PriceVsEMASlow:     classifyPriceVsEMA(payload.Market.CurrentPrice, emaLatest(payload.Data.EMASlow), atr),
			EMAStack:           classifyEMAStack(emaLatest(payload.Data.EMAFast), emaLatest(payload.Data.EMAMid), emaLatest(payload.Data.EMASlow)),
			EMADistanceFastATR: computeEMADistanceATR(payload.Market.CurrentPrice, emaLatest(payload.Data.EMAFast), atr),
			EMADistanceMidATR:  computeEMADistanceATR(payload.Market.CurrentPrice, emaLatest(payload.Data.EMAMid), atr),
			EMADistanceSlowATR: computeEMADistanceATR(payload.Market.CurrentPrice, emaLatest(payload.Data.EMASlow), atr),
		},
		Momentum: indicatorMomentumState{
			RSIZone:       classifyRSIZone(rsiCurrent(payload.Data.RSI)),
			RSISlopeState: classifyRSISlope(payload.Data.RSI),
			STCState:      classifySTCState(payload.Data.STC),
			OBVSlopeState: classifyOBVSlope(payload.Data.OBV),
			StochRSIZone:  classifyStochRSIZone(payload.Data.StochRSI),
		},
		Volatility: indicatorVolatilityState{
			ATRExpandState: classifyATRExpansion(payload.Data.ATR),
			ATRChangePct:   atrChangePct(payload.Data.ATR),
			BBZone:         classifyBBZone(payload.Data.BB),
			BBWidthState:   classifyBBWidthState(payload.Data.BB),
			CHOPRegime:     classifyCHOPRegime(payload.Data.CHOP),
		},
	}
	state.Bias = computeIndicatorBias(state)
	state.Events = detectIndicatorEvents(payload)
	return state
}

func buildIndicatorCrossTFSummary(results []indicatorTFState, decisionInterval string) indicatorCrossTFSummary {
	out := indicatorCrossTFSummary{DecisionTFBias: "mixed", Alignment: "mixed"}
	if len(results) == 0 {
		return out
	}
	decisionDur, err := interval.ParseInterval(decisionInterval)
	if err != nil {
		decisionDur = 0
	}
	var lower *indicatorTFState
	var decision *indicatorTFState
	var higher *indicatorTFState
	for i := range results {
		current := &results[i]
		dur, parseErr := interval.ParseInterval(current.Interval)
		if parseErr != nil {
			continue
		}
		switch {
		case current.Interval == decisionInterval:
			decision = current
		case decisionDur > 0 && dur < decisionDur:
			lower = current
		case decisionDur > 0 && dur > decisionDur && higher == nil:
			higher = current
		}
	}
	if decision == nil {
		for i := range results {
			if results[i].Interval == decisionInterval {
				decision = &results[i]
				break
			}
		}
	}
	if decision == nil {
		return out
	}
	out.DecisionTFBias = decision.Bias
	if lower != nil && decision.Bias != "mixed" {
		out.LowerTFAgreement = lower.Bias == decision.Bias
		if lower.Bias != "mixed" && lower.Bias != decision.Bias {
			out.ConflictCount++
		}
	}
	if higher != nil && decision.Bias != "mixed" {
		out.HigherTFAgreement = higher.Bias == decision.Bias
		if higher.Bias != "mixed" && higher.Bias != decision.Bias {
			out.ConflictCount++
		}
	}
	switch {
	case decision.Bias == "mixed":
		out.Alignment = "mixed"
	case out.ConflictCount > 0:
		out.Alignment = "conflict"
	default:
		out.Alignment = "aligned"
	}
	return out
}

func classifyPriceVsEMA(price, ema, atr float64) string {
	if price == 0 || ema == 0 {
		return "near"
	}
	diff := price - ema
	if atr > 0 && absFloat(diff)/atr <= priceVsEMANearATRRatio {
		return "near"
	}
	if diff > 0 {
		return "above"
	}
	return "below"
}

func classifyEMAStack(fast, mid, slow float64) string {
	switch {
	case fast > mid && mid > slow:
		return "bull"
	case fast < mid && mid < slow:
		return "bear"
	default:
		return "mixed"
	}
}

func classifyRSIZone(value float64) string {
	switch {
	case value < rsiZoneLowThreshold:
		return "<35"
	case value < rsiZoneWeakLowThreshold:
		return "35_45"
	case value < rsiZoneWeakHighThreshold:
		return "45_55"
	case value < rsiZoneHighThreshold:
		return "55_65"
	default:
		return ">65"
	}
}

func classifyRSISlope(rsi *rsiSnapshot) string {
	if rsi == nil || rsi.NormalizedSlope == nil {
		return "flat"
	}
	switch {
	case *rsi.NormalizedSlope >= rsiSlopeTrendThreshold:
		return "rising"
	case *rsi.NormalizedSlope <= -rsiSlopeTrendThreshold:
		return "falling"
	default:
		return "flat"
	}
}

func classifySTCState(stc *stcSnapshot) string {
	if stc == nil || strings.TrimSpace(stc.State) == "" {
		return "flat"
	}
	return strings.ToLower(strings.TrimSpace(stc.State))
}

func classifyOBVSlope(obv *obvSnapshot) string {
	if obv == nil || obv.ChangeRate == nil {
		return "flat"
	}
	switch {
	case *obv.ChangeRate >= obvSlopeTrendThreshold:
		return "up"
	case *obv.ChangeRate <= -obvSlopeTrendThreshold:
		return "down"
	default:
		return "flat"
	}
}

func classifyATRExpansion(atr *atrSnapshot) string {
	if atr == nil || atr.ChangePct == nil {
		return "stable"
	}
	switch {
	case *atr.ChangePct >= atrExpansionThresholdPct:
		return "expanding"
	case *atr.ChangePct <= atrContractionThresholdPct:
		return "contracting"
	default:
		return "stable"
	}
}

func computeIndicatorBias(state indicatorTFState) string {
	up := 0
	down := 0
	apply := func(direction string) {
		switch direction {
		case "up":
			up++
		case "down":
			down++
		}
	}
	switch state.Trend.PriceVsEMAMid {
	case "above":
		apply("up")
	case "below":
		apply("down")
	}
	switch state.Trend.EMAStack {
	case "bull":
		apply("up")
	case "bear":
		apply("down")
	}
	switch state.Momentum.RSISlopeState {
	case "rising":
		apply("up")
	case "falling":
		apply("down")
	}
	switch state.Momentum.STCState {
	case "rising":
		apply("up")
	case "falling":
		apply("down")
	}
	switch state.Momentum.OBVSlopeState {
	case "up":
		apply("up")
	case "down":
		apply("down")
	}
	switch {
	case up >= 3 && down <= 1:
		return "up"
	case down >= 3 && up <= 1:
		return "down"
	default:
		return "mixed"
	}
}

func detectIndicatorEvents(payload IndicatorCompressedInput) []string {
	events := make([]string, 0, 3)
	currentPrice := payload.Market.CurrentPrice
	prevPrice := payload.Market.PreviousPrice

	appendEvent := func(name string, ok bool) {
		if ok {
			events = append(events, name)
		}
	}

	appendEvent("price_cross_ema_fast_up", crossedAbove(prevPrice, previousEMA(payload.Data.EMAFast), currentPrice, emaLatest(payload.Data.EMAFast)))
	appendEvent("price_cross_ema_fast_down", crossedBelow(prevPrice, previousEMA(payload.Data.EMAFast), currentPrice, emaLatest(payload.Data.EMAFast)))
	appendEvent("price_cross_ema_mid_up", crossedAbove(prevPrice, previousEMA(payload.Data.EMAMid), currentPrice, emaLatest(payload.Data.EMAMid)))
	appendEvent("price_cross_ema_mid_down", crossedBelow(prevPrice, previousEMA(payload.Data.EMAMid), currentPrice, emaLatest(payload.Data.EMAMid)))

	prevStack := classifyEMAStack(previousEMA(payload.Data.EMAFast), previousEMA(payload.Data.EMAMid), previousEMA(payload.Data.EMASlow))
	currentStack := classifyEMAStack(emaLatest(payload.Data.EMAFast), emaLatest(payload.Data.EMAMid), emaLatest(payload.Data.EMASlow))
	appendEvent("ema_stack_bull_flip", prevStack != "bull" && currentStack == "bull")
	appendEvent("ema_stack_bear_flip", prevStack != "bear" && currentStack == "bear")
	switch classifyAroonSignal(payload.Data.Aroon) {
	case "strong_up":
		events = append(events, "aroon_strong_bullish")
	case "strong_down":
		events = append(events, "aroon_strong_bearish")
	}
	if td := payload.Data.TDSequential; td != nil {
		if td.BuySetup >= 8 {
			events = append(events, fmt.Sprintf("td_buy_setup_%d", td.BuySetup))
		}
		if td.SellSetup >= 8 {
			events = append(events, fmt.Sprintf("td_sell_setup_%d", td.SellSetup))
		}
	}

	return events
}

func crossedAbove(prevPrice, prevEMA, currentPrice, currentEMA float64) bool {
	return prevPrice > 0 && prevEMA > 0 && currentPrice > 0 && currentEMA > 0 &&
		prevPrice <= prevEMA && currentPrice > currentEMA
}

func crossedBelow(prevPrice, prevEMA, currentPrice, currentEMA float64) bool {
	return prevPrice > 0 && prevEMA > 0 && currentPrice > 0 && currentEMA > 0 &&
		prevPrice >= prevEMA && currentPrice < currentEMA
}

func previousEMA(ema *emaSnapshot) float64 {
	if ema == nil || len(ema.LastN) < 2 {
		return 0
	}
	return ema.LastN[len(ema.LastN)-2]
}

func emaLatest(ema *emaSnapshot) float64 {
	if ema == nil {
		return 0
	}
	return ema.Latest
}

func rsiCurrent(rsi *rsiSnapshot) float64 {
	if rsi == nil {
		return 50
	}
	return rsi.Current
}

func computeEMADistanceATR(price, ema, atr float64) float64 {
	if price == 0 || ema == 0 || atr <= 0 {
		return 0
	}
	return roundFloat((price-ema)/atr, 4)
}

func atrChangePct(atr *atrSnapshot) float64 {
	if atr == nil || atr.ChangePct == nil {
		return 0
	}
	return *atr.ChangePct
}

// classifyBBZone classifies the price position within Bollinger Bands.
// Uses %B (percent_b): <0 = below_lower, >1 = above_upper, 0-0.2 = near_lower,
// 0.8-1.0 = near_upper, 0.2-0.8 = mid.
func classifyBBZone(bb *bbSnapshot) string {
	if bb == nil {
		return ""
	}
	switch {
	case bb.PercentB < 0:
		return "below_lower"
	case bb.PercentB <= bbNearLowerPercentBThreshold:
		return "near_lower"
	case bb.PercentB >= bbAboveUpperPercentBThreshold:
		return "above_upper"
	case bb.PercentB >= bbNearUpperPercentBThreshold:
		return "near_upper"
	default:
		return "mid"
	}
}

// classifyBBWidthState classifies Bollinger Band width as squeeze/normal/wide.
// Width < 2% = squeeze, > 6% = wide, else normal.
// These thresholds are tuned for BTC/ETH on 15m-4h timeframes.
func classifyBBWidthState(bb *bbSnapshot) string {
	if bb == nil {
		return ""
	}
	switch {
	case bb.Width < bbWidthSqueezeThresholdPct:
		return "squeeze"
	case bb.Width > bbWidthWideThresholdPct:
		return "wide"
	default:
		return "normal"
	}
}

// classifyCHOPRegime classifies Choppiness Index into trending/choppy.
// CHOP < 38.2 = trending, > 61.8 = choppy, else transition.
// Standard Fibonacci levels used in quant practice for BTC/ETH.
func classifyCHOPRegime(chop *chopSnapshot) string {
	if chop == nil {
		return ""
	}
	switch {
	case chop.Value < chopTrendingThreshold:
		return "trending"
	case chop.Value > chopChoppyThreshold:
		return "choppy"
	default:
		return "transition"
	}
}

// classifyStochRSIZone classifies Stochastic RSI value (0-1 range).
func classifyStochRSIZone(sr *stochRSISnapshot) string {
	if sr == nil {
		return ""
	}
	switch {
	case sr.Value <= stochRSIOversoldThreshold:
		return "oversold"
	case sr.Value >= stochRSIOverboughtThreshold:
		return "overbought"
	default:
		return "neutral"
	}
}

// classifyAroonSignal classifies Aroon indicator state.
// AroonUp > 70 && AroonDown < 30 = strong_up, inverse = strong_down,
// both > 70 = crossover, else neutral.
func classifyAroonSignal(aroon *aroonSnapshot) string {
	if aroon == nil {
		return ""
	}
	switch {
	case aroon.Up > aroonStrongThreshold && aroon.Down < aroonWeakThreshold:
		return "strong_up"
	case aroon.Down > aroonStrongThreshold && aroon.Up < aroonWeakThreshold:
		return "strong_down"
	case aroon.Up > aroonStrongThreshold && aroon.Down > aroonStrongThreshold:
		return "crossover"
	default:
		return "neutral"
	}
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sortedIndicatorIntervals(items []string) []string {
	keys := append([]string(nil), items...)
	sort.Slice(keys, func(i, j int) bool {
		left, errLeft := interval.ParseInterval(keys[i])
		right, errRight := interval.ParseInterval(keys[j])
		if errLeft != nil || errRight != nil {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}
