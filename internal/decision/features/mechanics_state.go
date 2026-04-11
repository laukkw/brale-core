package features

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"brale-core/internal/interval"
)

type MechanicsStateSummary struct {
	FreshnessSec      int64                      `json:"freshness_sec,omitempty"`
	OIState           *mechanicsOIState          `json:"oi_state,omitempty"`
	FundingState      *mechanicsFundingState     `json:"funding_state,omitempty"`
	CrowdingState     *mechanicsCrowdingState    `json:"crowding_state,omitempty"`
	LiquidationState  *mechanicsLiquidationState `json:"liquidation_state,omitempty"`
	SentimentState    *mechanicsSentimentState   `json:"sentiment_state,omitempty"`
	MechanicsConflict []string                   `json:"mechanics_conflict,omitempty"`
	Missing           []string                   `json:"missing,omitempty"`
}

type mechanicsOIState struct {
	ChangeState     string  `json:"change_state"`
	OIChangePct     float64 `json:"oi_change_pct"`
	PriceChangePct  float64 `json:"price_change_pct"`
	OIPriceRelation string  `json:"oi_price_relation"`
}

type mechanicsFundingState struct {
	Bias string  `json:"bias"`
	Heat string  `json:"heat"`
	Rate float64 `json:"rate"`
}

type mechanicsCrowdingState struct {
	Bias         string  `json:"bias"`
	LSRatio      float64 `json:"ls_ratio"`
	TakerRatio   float64 `json:"taker_ratio"`
	ReversalRisk string  `json:"reversal_risk"`
}

type mechanicsLiquidationState struct {
	Stress    string  `json:"stress"`
	Window    string  `json:"window,omitempty"`
	ZScore    float64 `json:"zscore,omitempty"`
	VolOverOI float64 `json:"vol_over_oi,omitempty"`
	Spike     bool    `json:"spike,omitempty"`
	Imbalance float64 `json:"imbalance,omitempty"`
}

type mechanicsSentimentState struct {
	FearGreed     string `json:"fear_greed"`
	TopTraderBias string `json:"top_trader_bias"`
}

func BuildMechanicsStateSummary(input MechanicsCompressedInput) (MechanicsStateSummary, error) {
	summary := MechanicsStateSummary{
		FreshnessSec: computeMechanicsFreshness(input),
	}
	summary.OIState = classifyOIState(input)
	summary.FundingState = classifyFundingState(input)
	summary.LiquidationState = classifyLiquidationState(input)
	summary.CrowdingState = classifyCrowdingState(input, summary.FundingState, summary.LiquidationState)
	summary.SentimentState = classifySentimentState(input)
	summary.MechanicsConflict = detectMechanicsConflicts(summary)
	summary.Missing = detectMechanicsMissing(summary)
	if !hasMechanicsStateSummary(summary) {
		return MechanicsStateSummary{}, fmt.Errorf("mechanics: no usable state summary fields")
	}
	return summary, nil
}

func buildMechanicsStateRaw(input MechanicsCompressedInput, pretty bool) ([]byte, error) {
	summary, err := BuildMechanicsStateSummary(input)
	if err != nil {
		return nil, err
	}
	if pretty {
		return json.MarshalIndent(summary, "", "  ")
	}
	return json.Marshal(summary)
}

func hasMechanicsStateSummary(summary MechanicsStateSummary) bool {
	return summary.OIState != nil ||
		summary.FundingState != nil ||
		summary.CrowdingState != nil ||
		summary.LiquidationState != nil ||
		summary.SentimentState != nil
}

func classifyOIState(input MechanicsCompressedInput) *mechanicsOIState {
	entry, ok := pickPrimaryOIHistory(input.OIHistory)
	if !ok {
		return nil
	}
	return &mechanicsOIState{
		ChangeState:     classifyOIChangeState(entry.ChangePct),
		OIChangePct:     roundFloat(entry.ChangePct, 2),
		PriceChangePct:  roundFloat(entry.PriceChangePct, 2),
		OIPriceRelation: classifyOIPriceRelation(entry.PriceChangePct, entry.ChangePct),
	}
}

func classifyFundingState(input MechanicsCompressedInput) *mechanicsFundingState {
	if input.Funding == nil {
		return nil
	}
	rate := roundFloat(input.Funding.Rate, 6)
	bias := "neutral"
	switch {
	case rate > 0.0001:
		bias = "long"
	case rate < -0.0001:
		bias = "short"
	}
	heat := "neutral"
	if absFloat(rate) >= 0.0005 {
		heat = "hot"
	}
	return &mechanicsFundingState{
		Bias: bias,
		Heat: heat,
		Rate: rate,
	}
}

func classifyCrowdingState(input MechanicsCompressedInput, funding *mechanicsFundingState, liq *mechanicsLiquidationState) *mechanicsCrowdingState {
	lsRatio, takerRatio, ok := crowdingAnchors(input)
	if !ok {
		return nil
	}
	bias := "balanced"
	switch {
	case lsRatio > 1.2 && takerRatio > 1.1:
		bias = "long_crowded"
	case lsRatio < 0.8 && takerRatio < 0.9:
		bias = "short_crowded"
	}
	reversalRisk := "low"
	if bias != "balanced" {
		hotFunding := funding != nil && funding.Heat == "hot"
		highLiq := liq != nil && liq.Stress == "high"
		switch {
		case hotFunding && highLiq:
			reversalRisk = "high"
		case hotFunding || highLiq:
			reversalRisk = "medium"
		}
	}
	return &mechanicsCrowdingState{
		Bias:         bias,
		LSRatio:      roundFloat(lsRatio, 4),
		TakerRatio:   roundFloat(takerRatio, 4),
		ReversalRisk: reversalRisk,
	}
}

func classifyLiquidationState(input MechanicsCompressedInput) *mechanicsLiquidationState {
	if len(input.LiquidationsByWindow) == 0 {
		return nil
	}
	keys := sortedMechanicsIntervals(mapKeys(input.LiquidationsByWindow))
	var best *mechanicsLiquidationState
	bestScore := -1
	for _, key := range keys {
		window := input.LiquidationsByWindow[key]
		current := summarizeLiquidationWindow(key, window)
		score := liquidationStressScore(current.Stress)
		if best == nil || score > bestScore || (score == bestScore && liquidationTieBreaker(current) > liquidationTieBreaker(*best)) {
			copy := current
			best = &copy
			bestScore = score
		}
	}
	return best
}

func classifySentimentState(input MechanicsCompressedInput) *mechanicsSentimentState {
	var (
		hasFearGreed bool
		hasTopTrader bool
		fearValue    int
		topTraderLSR float64
	)
	if input.FearGreed != nil {
		hasFearGreed = true
		fearValue = int(roundFloat(input.FearGreed.Value, 0))
	}
	if input.FuturesSentiment != nil {
		topTraderLSR = input.FuturesSentiment.TopTraderLSR
		if topTraderLSR == 0 {
			topTraderLSR = input.FuturesSentiment.LSRatio
		}
		hasTopTrader = topTraderLSR != 0
	}
	if !hasFearGreed && !hasTopTrader {
		return nil
	}
	fearGreed := "unknown"
	if hasFearGreed {
		switch {
		case fearValue <= 25:
			fearGreed = "fear"
		case fearValue <= 55:
			fearGreed = "neutral"
		case fearValue <= 75:
			fearGreed = "greed"
		default:
			fearGreed = "extreme_greed"
		}
	}
	topTraderBias := "unknown"
	if hasTopTrader {
		switch {
		case topTraderLSR > 1.1:
			topTraderBias = "long"
		case topTraderLSR < 0.9:
			topTraderBias = "short"
		default:
			topTraderBias = "neutral"
		}
	}
	return &mechanicsSentimentState{
		FearGreed:     fearGreed,
		TopTraderBias: topTraderBias,
	}
}

func detectMechanicsConflicts(summary MechanicsStateSummary) []string {
	conflicts := make([]string, 0, 6)
	if summary.OIState != nil {
		switch summary.OIState.OIPriceRelation {
		case "price_up_oi_down", "price_down_oi_down":
			conflicts = append(conflicts, summary.OIState.OIPriceRelation)
		}
	}
	if summary.CrowdingState != nil && summary.LiquidationState != nil && summary.LiquidationState.Stress == "high" {
		switch summary.CrowdingState.Bias {
		case "long_crowded":
			conflicts = append(conflicts, "crowding_long_but_liq_stress_high")
		case "short_crowded":
			conflicts = append(conflicts, "crowding_short_but_liq_stress_high")
		}
	}
	if summary.FundingState != nil && summary.OIState != nil {
		if summary.FundingState.Bias == "long" && summary.OIState.ChangeState == "falling" {
			conflicts = append(conflicts, "funding_long_but_oi_falling")
		}
		if summary.FundingState.Bias == "short" && summary.OIState.ChangeState == "rising" {
			conflicts = append(conflicts, "funding_short_but_oi_rising")
		}
	}
	return conflicts
}

func detectMechanicsMissing(summary MechanicsStateSummary) []string {
	missing := make([]string, 0, 5)
	if summary.OIState == nil {
		missing = append(missing, "oi_state")
	}
	if summary.FundingState == nil {
		missing = append(missing, "funding_state")
	}
	if summary.CrowdingState == nil {
		missing = append(missing, "crowding_state")
	}
	if summary.LiquidationState == nil {
		missing = append(missing, "liquidation_state")
	}
	if summary.SentimentState == nil {
		missing = append(missing, "sentiment_state")
	}
	return missing
}

func computeMechanicsFreshness(input MechanicsCompressedInput) int64 {
	ref, err := time.Parse(time.RFC3339, strings.TrimSpace(input.Timestamp))
	if err != nil || ref.IsZero() {
		return 0
	}
	best := int64(-1)
	visit := func(ts string) {
		parsed, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(ts))
		if parseErr != nil || parsed.IsZero() {
			return
		}
		age := int64(ref.Sub(parsed).Seconds())
		if age < 0 {
			age = 0
		}
		if best < 0 || age < best {
			best = age
		}
	}
	if input.OI != nil {
		visit(input.OI.Timestamp)
		visit(input.OI.PriceTimestamp)
	}
	if input.Funding != nil {
		visit(input.Funding.Timestamp)
	}
	for _, item := range input.LongShortByInterval {
		visit(item.Timestamp)
	}
	for _, item := range input.CVDByInterval {
		visit(item.Timestamp)
	}
	if input.FearGreed != nil {
		visit(input.FearGreed.Timestamp)
	}
	if input.Liquidations != nil {
		visit(input.Liquidations.Timestamp)
	}
	if input.FuturesSentiment != nil {
		visit(input.FuturesSentiment.Timestamp)
	}
	if best < 0 {
		return 0
	}
	return best
}

func pickPrimaryOIHistory(items map[string]oiHistoryPayload) (oiHistoryPayload, bool) {
	if len(items) == 0 {
		return oiHistoryPayload{}, false
	}
	keys := sortedMechanicsIntervals(mapKeys(items))
	for _, key := range keys {
		return items[key], true
	}
	return oiHistoryPayload{}, false
}

func pickPrimaryCVD(items map[string]cvdPayload) (cvdPayload, bool) {
	if len(items) == 0 {
		return cvdPayload{}, false
	}
	keys := sortedMechanicsIntervals(mapKeys(items))
	for _, key := range keys {
		return items[key], true
	}
	return cvdPayload{}, false
}

func crowdingAnchors(input MechanicsCompressedInput) (float64, float64, bool) {
	if input.FuturesSentiment != nil {
		return input.FuturesSentiment.LSRatio, input.FuturesSentiment.TakerLongShortVolRatio, true
	}
	if len(input.LongShortByInterval) == 0 {
		return 0, 0, false
	}
	keys := sortedMechanicsIntervals(mapKeys(input.LongShortByInterval))
	if len(keys) == 0 {
		return 0, 0, false
	}
	lsRatio := input.LongShortByInterval[keys[0]].Ratio
	return lsRatio, 0, true
}

func summarizeLiquidationWindow(name string, payload liqWindowPayload) mechanicsLiquidationState {
	stress := "low"
	zscore := 0.0
	volOverOI := 0.0
	spike := false
	if payload.Rel != nil {
		zscore = payload.Rel.ZScore
		volOverOI = payload.Rel.VolOverOI
		spike = payload.Rel.Spike
	}
	switch {
	case spike || zscore >= 2.5 || volOverOI >= 0.08:
		stress = "high"
	case zscore >= 1.5 || volOverOI >= 0.04:
		stress = "elevated"
	}
	return mechanicsLiquidationState{
		Stress:    stress,
		Window:    name,
		ZScore:    roundFloat(zscore, 4),
		VolOverOI: roundFloat(volOverOI, 4),
		Spike:     spike,
		Imbalance: roundFloat(payload.Imbalance, 4),
	}
}

func liquidationStressScore(stress string) int {
	switch stress {
	case "high":
		return 3
	case "elevated":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func liquidationTieBreaker(state mechanicsLiquidationState) float64 {
	return absFloat(state.ZScore)*100 + absFloat(state.VolOverOI)*10 + absFloat(state.Imbalance)
}

func classifyOIChangeState(changePct float64) string {
	switch {
	case changePct >= 2:
		return "rising"
	case changePct <= -2:
		return "falling"
	default:
		return "flat"
	}
}

func classifyOIPriceRelation(priceChangePct, oiChangePct float64) string {
	priceDir := signLabel(priceChangePct)
	oiDir := signLabel(oiChangePct)
	switch {
	case priceDir == "up" && oiDir == "up":
		return "price_up_oi_up"
	case priceDir == "up" && oiDir == "down":
		return "price_up_oi_down"
	case priceDir == "down" && oiDir == "up":
		return "price_down_oi_up"
	case priceDir == "down" && oiDir == "down":
		return "price_down_oi_down"
	case priceDir == "flat" && oiDir == "flat":
		return "mixed"
	case priceDir == "unknown" || oiDir == "unknown":
		return "unknown"
	default:
		return "mixed"
	}
}

func signLabel(value float64) string {
	switch {
	case value > 0:
		return "up"
	case value < 0:
		return "down"
	case value == 0:
		return "flat"
	default:
		return "unknown"
	}
}

func normalizedMechanicsToken(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

func sortedMechanicsIntervals(keys []string) []string {
	sort.Slice(keys, func(i, j int) bool {
		di, errI := interval.ParseInterval(keys[i])
		dj, errJ := interval.ParseInterval(keys[j])
		if errI != nil || errJ != nil {
			return keys[i] < keys[j]
		}
		return di < dj
	})
	return keys
}

func mapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}
