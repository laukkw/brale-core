package runtimeapi

import (
	"context"
	"strings"

	readmodel "brale-core/internal/readmodel/dashboard"
	"brale-core/internal/store"
)

func summarizePlanReason(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return readmodelSummarizePlanReason(gate, tighten)
}

func summarizeTerminalStatus(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return readmodelSummarizeTerminalStatus(gate, tighten)
}

func summarizeTerminalReason(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return readmodelSummarizeTerminalReason(gate, tighten)
}

func summarizePlanOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return readmodelSummarizePlanOutcome(gate, tighten)
}

func summarizeTerminalOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return readmodelSummarizeTerminalOutcome(gate, tighten)
}

func summarizeResultFields(gate store.GateEventRecord, tighten *DashboardTightenInfo) []DashboardFlowValueField {
	rm := readmodelSummarizeResultFields(gate, tighten)
	return mapDashboardFlowFields(rm)
}

func summarizeTightenResultFields(derived map[string]any, tighten *DashboardTightenInfo) []DashboardFlowValueField {
	rm := readmodel.SummarizeTightenResultFields(derived, toReadmodelTightenInfo(tighten))
	return mapDashboardFlowFields(rm)
}

func resolveDashboardTightenInfo(gate *store.GateEventRecord) *DashboardTightenInfo {
	rm := readmodel.ResolveTightenInfo(gate)
	if rm == nil {
		return nil
	}
	return &DashboardTightenInfo{Triggered: rm.Triggered, Reason: rm.Reason}
}

func resolveDashboardTightenFromRiskHistory(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord) *DashboardTightenInfo {
	rm := readmodel.ResolveTightenFromRiskHistory(ctx, st, pos, dashboardRiskPlanTimelineLimit)
	if rm == nil {
		return nil
	}
	return &DashboardTightenInfo{Triggered: rm.Triggered, Reason: rm.Reason}
}

func toReadmodelTightenInfo(tighten *DashboardTightenInfo) *readmodel.TightenInfo {
	if tighten == nil {
		return nil
	}
	return &readmodel.TightenInfo{Triggered: tighten.Triggered, Reason: tighten.Reason}
}

func readmodelSummarizePlanReason(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return invokeReadmodelFlowSummary(gate, tighten, func(g store.GateEventRecord, t *readmodel.TightenInfo) string {
		nodes := readmodel.BuildFlowNodes(readmodel.FlowStageSet{}, &g, t)
		if len(nodes) >= 2 && nodes[len(nodes)-2].Stage == "plan" {
			return nodes[len(nodes)-2].Reason
		}
		return ""
	})
}

func readmodelSummarizePlanOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return invokeReadmodelFlowSummary(gate, tighten, func(g store.GateEventRecord, t *readmodel.TightenInfo) string {
		nodes := readmodel.BuildFlowNodes(readmodel.FlowStageSet{}, &g, t)
		if len(nodes) >= 2 && nodes[len(nodes)-2].Stage == "plan" {
			return nodes[len(nodes)-2].Outcome
		}
		return ""
	})
}

func readmodelSummarizeTerminalOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return invokeReadmodelFlowSummary(gate, tighten, func(g store.GateEventRecord, t *readmodel.TightenInfo) string {
		nodes := readmodel.BuildFlowNodes(readmodel.FlowStageSet{}, &g, t)
		if len(nodes) > 0 {
			return nodes[len(nodes)-1].Outcome
		}
		return ""
	})
}

func readmodelSummarizeTerminalStatus(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return invokeReadmodelFlowSummary(gate, tighten, func(g store.GateEventRecord, t *readmodel.TightenInfo) string {
		nodes := readmodel.BuildFlowNodes(readmodel.FlowStageSet{}, &g, t)
		if len(nodes) > 0 {
			return nodes[len(nodes)-1].Status
		}
		return ""
	})
}

func readmodelSummarizeTerminalReason(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return invokeReadmodelFlowSummary(gate, tighten, func(g store.GateEventRecord, t *readmodel.TightenInfo) string {
		nodes := readmodel.BuildFlowNodes(readmodel.FlowStageSet{}, &g, t)
		if len(nodes) > 0 {
			return nodes[len(nodes)-1].Reason
		}
		return ""
	})
}

func readmodelSummarizeResultFields(gate store.GateEventRecord, tighten *DashboardTightenInfo) []readmodel.FlowValueField {
	nodes := readmodel.BuildFlowNodes(readmodel.FlowStageSet{}, &gate, toReadmodelTightenInfo(tighten))
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) >= 2 && nodes[len(nodes)-2].Stage == "plan" {
		return nodes[len(nodes)-2].Values
	}
	return nodes[len(nodes)-1].Values
}

func invokeReadmodelFlowSummary(gate store.GateEventRecord, tighten *DashboardTightenInfo, pick func(store.GateEventRecord, *readmodel.TightenInfo) string) string {
	return pick(gate, toReadmodelTightenInfo(tighten))
}

func resolvePlanSource(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	return readmodelNormalizePlanSourceFromNodes(gate, tighten)
}

func normalizePlanSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "llm", "go":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

func readmodelNormalizePlanSourceFromNodes(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	fields := readmodelSummarizeResultFields(gate, tighten)
	for _, field := range fields {
		if field.Key == "plan_source" {
			return normalizePlanSource(field.Value)
		}
	}
	return ""
}
