package decision

import (
	"encoding/json"

	"brale-core/internal/decision/features"
)

// BuildProviderDataContext extracts Provider grounding anchors directly from
// the exact payloads used by the Agent stage, keeping Agent and Provider in
// sync without rebuilding inputs from CompressionResult.
func BuildProviderDataContext(inputs AgentInputSet) ProviderDataContext {
	return ProviderDataContext{
		IndicatorCrossTF:   buildIndicatorCrossTFFromInputs(inputs.Indicator),
		StructureAnchorCtx: buildStructureAnchorFromInputs(inputs.Structure),
		MechanicsCtx:       buildMechanicsDataFromInputs(inputs.Mechanics),
	}
}

func buildIndicatorCrossTFFromInputs(ind features.IndicatorJSON) *IndicatorCrossTFContext {
	if len(ind.RawJSON) == 0 {
		return nil
	}
	var parsed struct {
		CrossTFSummary struct {
			DecisionTFBias    string `json:"decision_tf_bias"`
			Alignment         string `json:"alignment"`
			ConflictCount     int    `json:"conflict_count"`
			LowerTFAgreement  bool   `json:"lower_tf_agreement"`
			HigherTFAgreement bool   `json:"higher_tf_agreement"`
		} `json:"cross_tf_summary"`
	}
	if err := json.Unmarshal(ind.RawJSON, &parsed); err != nil {
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

func buildStructureAnchorFromInputs(trend features.TrendJSON) *StructureAnchorContext {
	if len(trend.RawJSON) == 0 {
		return nil
	}
	var parsed struct {
		Blocks []struct {
			SuperTrend *struct {
				State    string `json:"state"`
				Interval string `json:"interval"`
			} `json:"supertrend"`
		} `json:"blocks"`
		LatestBreakAcrossBlocks *struct {
			Type string `json:"type"`
			Age  int    `json:"age"`
		} `json:"latest_break_across_blocks"`
	}
	if err := json.Unmarshal(trend.RawJSON, &parsed); err != nil {
		return nil
	}
	var ctx StructureAnchorContext
	found := false
	if parsed.LatestBreakAcrossBlocks != nil && parsed.LatestBreakAcrossBlocks.Type != "" && parsed.LatestBreakAcrossBlocks.Type != "none" {
		ctx.LatestBreakType = parsed.LatestBreakAcrossBlocks.Type
		ctx.LatestBreakBarAge = parsed.LatestBreakAcrossBlocks.Age
		found = true
	}
	for _, block := range parsed.Blocks {
		if block.SuperTrend == nil || block.SuperTrend.State == "" {
			continue
		}
		ctx.SupertrendState = block.SuperTrend.State
		ctx.SupertrendInterval = block.SuperTrend.Interval
		found = true
		break
	}
	if !found {
		return nil
	}
	return &ctx
}

func buildMechanicsDataFromInputs(mech features.MechanicsSnapshot) *MechanicsDataContext {
	if len(mech.RawJSON) == 0 {
		return nil
	}
	var state struct {
		MechanicsConflict []string `json:"mechanics_conflict"`
		CrowdingState     *struct {
			ReversalRisk string `json:"reversal_risk"`
		} `json:"crowding_state"`
		LiquidationState *struct {
			Stress   string `json:"stress"`
			Status   string `json:"status"`
			Window   string `json:"window"`
			Complete bool   `json:"complete"`
		} `json:"liquidation_state"`
		LiquidationSource *struct {
			Source          string `json:"source"`
			Status          string `json:"status"`
			StreamConnected bool   `json:"stream_connected"`
			CoverageSec     int64  `json:"coverage_sec"`
			SampleCount     int    `json:"sample_count"`
			LastEventAgeSec int64  `json:"last_event_age_sec"`
			Complete        bool   `json:"complete"`
		} `json:"liquidation_source"`
	}
	if err := json.Unmarshal(mech.RawJSON, &state); err != nil {
		return nil
	}
	if len(state.MechanicsConflict) == 0 &&
		(state.CrowdingState == nil || state.CrowdingState.ReversalRisk == "") &&
		state.LiquidationState == nil &&
		state.LiquidationSource == nil {
		return nil
	}
	ctx := &MechanicsDataContext{
		Conflicts: state.MechanicsConflict,
	}
	if state.CrowdingState != nil {
		ctx.ReversalRisk = state.CrowdingState.ReversalRisk
	}
	if state.LiquidationState != nil {
		ctx.LiquidationState = &MechanicsLiquidationContext{
			Stress:   state.LiquidationState.Stress,
			Status:   state.LiquidationState.Status,
			Window:   state.LiquidationState.Window,
			Complete: state.LiquidationState.Complete,
		}
	}
	if state.LiquidationSource != nil {
		ctx.LiquidationSource = &MechanicsLiquidationSourceContext{
			Source:          state.LiquidationSource.Source,
			Status:          state.LiquidationSource.Status,
			StreamConnected: state.LiquidationSource.StreamConnected,
			CoverageSec:     state.LiquidationSource.CoverageSec,
			SampleCount:     state.LiquidationSource.SampleCount,
			LastEventAgeSec: state.LiquidationSource.LastEventAgeSec,
			Complete:        state.LiquidationSource.Complete,
		}
	}
	return ctx
}
