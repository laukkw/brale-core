package runtimeapi

import (
	"strings"

	"brale-core/internal/pkg/parseutil"
)

func buildMonitorRiskPlan(bundle ConfigBundle) MonitorRiskPlan {
	riskMgmt := bundle.Strategy.RiskManagement
	mode := normalizeMonitorRiskPlanMode(riskMgmt.RiskStrategy.Mode)
	entryMode := strings.TrimSpace(riskMgmt.EntryMode)
	if entryMode == "" {
		entryMode = "signal"
	}
	plan := MonitorRiskPlan{
		Mode:  mode,
		Label: monitorRiskPlanModeLabel(mode),
		EntryPricing: MonitorEntryPricing{
			Mode:  entryMode,
			Label: entryMode,
		},
	}
	if mode == "llm" {
		plan.Initial = MonitorRiskPlanSection{Source: "llm", Label: "LLM生成"}
		plan.Tighten = MonitorRiskPlanSection{Source: "llm", Label: "LLM生成"}
		return plan
	}
	plan.Initial = MonitorRiskPlanSection{Source: "go", Label: "Go规则", Params: buildNativeInitialRiskPlanParams(bundle)}
	plan.Tighten = MonitorRiskPlanSection{Source: "go", Label: "Go规则", Params: buildNativeTightenRiskPlanParams(bundle)}
	return plan
}

func normalizeMonitorRiskPlanMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "llm") {
		return "llm"
	}
	return "native"
}

func monitorRiskPlanModeLabel(mode string) string {
	if mode == "llm" {
		return "LLM生成"
	}
	return "Go规则"
}

func buildNativeInitialRiskPlanParams(bundle ConfigBundle) map[string]any {
	riskMgmt := bundle.Strategy.RiskManagement
	params := map[string]any{
		"policy":           strings.TrimSpace(riskMgmt.InitialExit.Policy),
		"risk_pct":         riskMgmt.RiskPerTradePct,
		"max_leverage":     riskMgmt.MaxLeverage,
		"entry_offset_atr": riskMgmt.EntryOffsetATR,
	}
	if structureInterval := strings.TrimSpace(riskMgmt.InitialExit.StructureInterval); structureInterval != "" {
		params["structure_interval"] = structureInterval
	}
	if value, ok := parseutil.FloatOK(riskMgmt.InitialExit.Params["stop_atr_multiplier"]); ok && value > 0 {
		params["stop_atr_multiplier"] = value
	}
	if value, ok := parseutil.FloatOK(riskMgmt.InitialExit.Params["stop_min_distance_pct"]); ok && value > 0 {
		params["stop_min_distance_pct"] = value
	}
	if values := monitorRiskPlanFloatSlice(riskMgmt.InitialExit.Params["take_profit_rr"]); len(values) > 0 {
		params["take_profit_rr"] = values
	}
	if values := monitorRiskPlanFloatSlice(riskMgmt.InitialExit.Params["take_profit_ratios"]); len(values) > 0 {
		params["take_profit_ratios"] = values
	}
	return params
}

func buildNativeTightenRiskPlanParams(bundle ConfigBundle) map[string]any {
	riskMgmt := bundle.Strategy.RiskManagement
	return map[string]any{
		"breakeven_fee_pct":             riskMgmt.BreakevenFeePct,
		"min_update_interval_sec":       riskMgmt.TightenATR.MinUpdateIntervalSec,
		"structure_threatened_atr_mult": riskMgmt.TightenATR.StructureThreatened,
		"tp1_atr":                       riskMgmt.TightenATR.TP1ATR,
		"tp2_atr":                       riskMgmt.TightenATR.TP2ATR,
		"min_tp_distance_pct":           riskMgmt.TightenATR.MinTPDistancePct,
		"min_tp_gap_pct":                riskMgmt.TightenATR.MinTPGapPct,
	}
}

func monitorRiskPlanFloatSlice(raw any) []float64 {
	switch values := raw.(type) {
	case []float64:
		return append([]float64(nil), values...)
	case []any:
		out := make([]float64, 0, len(values))
		for _, item := range values {
			if value, ok := parseutil.FloatOK(item); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}
