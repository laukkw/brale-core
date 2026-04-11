package decision

import (
	"encoding/json"
	"strings"
	"time"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	"brale-core/internal/interval"
)

// BuildProviderDataContext extracts code-computed quantitative summaries
// from CompressionResult for Provider grounding. It reads the multi-TF
// indicator state, trend structure data, and mechanics state.
func BuildProviderDataContext(comp features.CompressionResult, symbol string, decisionInterval string) ProviderDataContext {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	ctx := ProviderDataContext{}
	ctx.IndicatorCrossTF = buildIndicatorCrossTFFromComp(comp, symbol, decisionInterval)
	ctx.StructureAnchorCtx = buildStructureAnchorFromComp(comp, symbol)
	ctx.MechanicsCtx = buildMechanicsDataFromComp(comp, symbol)
	return ctx
}

// buildIndicatorCrossTFFromComp builds the cross-TF summary by calling
// BuildIndicatorStateJSON and extracting the cross_tf_summary field.
func buildIndicatorCrossTFFromComp(comp features.CompressionResult, symbol string, decisionInterval string) *IndicatorCrossTFContext {
	byInterval, ok := comp.Indicators[symbol]
	if !ok || len(byInterval) == 0 {
		return nil
	}
	multiJSON, err := features.BuildIndicatorStateJSON(symbol, byInterval, decisionInterval)
	if err != nil {
		return nil
	}
	return extractCrossTFFromMultiJSON(multiJSON.RawJSON)
}

func extractCrossTFFromMultiJSON(raw []byte) *IndicatorCrossTFContext {
	var parsed struct {
		CrossTFSummary struct {
			DecisionTFBias    string `json:"decision_tf_bias"`
			Alignment         string `json:"alignment"`
			ConflictCount     int    `json:"conflict_count"`
			LowerTFAgreement  bool   `json:"lower_tf_agreement"`
			HigherTFAgreement bool   `json:"higher_tf_agreement"`
		} `json:"cross_tf_summary"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	if parsed.CrossTFSummary.DecisionTFBias == "" {
		return nil
	}
	return &IndicatorCrossTFContext{
		DecisionTFBias:    parsed.CrossTFSummary.DecisionTFBias,
		Alignment:         parsed.CrossTFSummary.Alignment,
		ConflictCount:     parsed.CrossTFSummary.ConflictCount,
		LowerTFAgreement:  parsed.CrossTFSummary.LowerTFAgreement,
		HigherTFAgreement: parsed.CrossTFSummary.HigherTFAgreement,
	}
}

func buildStructureAnchorFromComp(comp features.CompressionResult, symbol string) *StructureAnchorContext {
	byInterval, ok := comp.Trends[symbol]
	if !ok || len(byInterval) == 0 {
		return nil
	}
	keys := decisionutil.SortedTrendKeys(byInterval)
	if len(keys) == 0 {
		return nil
	}
	var ctx StructureAnchorContext
	found := false
	for _, key := range keys {
		raw := byInterval[key].RawJSON
		var block struct {
			BreakSummary *struct {
				LatestEventType *string `json:"latest_event_type"`
				LatestEventAge  *int    `json:"latest_event_age"`
			} `json:"break_summary"`
			SuperTrend *struct {
				State    string `json:"state"`
				Interval string `json:"interval"`
			} `json:"supertrend"`
		}
		if err := json.Unmarshal(raw, &block); err != nil {
			continue
		}
		if block.BreakSummary != nil && block.BreakSummary.LatestEventType != nil && *block.BreakSummary.LatestEventType != "" && *block.BreakSummary.LatestEventType != "none" {
			ctx.LatestBreakType = *block.BreakSummary.LatestEventType
			if block.BreakSummary.LatestEventAge != nil {
				ctx.LatestBreakBarAge = *block.BreakSummary.LatestEventAge
			}
			found = true
		}
		if block.SuperTrend != nil && block.SuperTrend.State != "" && ctx.SupertrendState == "" {
			ctx.SupertrendState = block.SuperTrend.State
			ctx.SupertrendInterval = block.SuperTrend.Interval
			found = true
		}
	}
	if !found {
		return nil
	}
	return &ctx
}

func buildMechanicsDataFromComp(comp features.CompressionResult, symbol string) *MechanicsDataContext {
	mech, ok := comp.Mechanics[symbol]
	if !ok || len(mech.RawJSON) == 0 {
		return nil
	}
	var state struct {
		MechanicsConflict []string `json:"mechanics_conflict"`
		CrowdingState     *struct {
			ReversalRisk string `json:"reversal_risk"`
		} `json:"crowding_state"`
	}
	if err := json.Unmarshal(mech.RawJSON, &state); err != nil {
		return nil
	}
	if len(state.MechanicsConflict) == 0 && (state.CrowdingState == nil || state.CrowdingState.ReversalRisk == "") {
		return nil
	}
	ctx := &MechanicsDataContext{
		Conflicts: state.MechanicsConflict,
	}
	if state.CrowdingState != nil {
		ctx.ReversalRisk = state.CrowdingState.ReversalRisk
	}
	return ctx
}

func selectDecisionInterval(intervals []string) string {
	shortest := ""
	var shortestDur time.Duration
	for _, candidate := range intervals {
		dur, err := interval.ParseInterval(candidate)
		if err != nil {
			continue
		}
		if shortest == "" || dur < shortestDur {
			shortest = candidate
			shortestDur = dur
		}
	}
	if shortest != "" {
		return shortest
	}
	if len(intervals) == 0 {
		return ""
	}
	return intervals[0]
}
