package runtimeapi

import (
	"strings"

	readmodel "brale-core/internal/readmodel/dashboard"
	"brale-core/internal/store"
)

type dashboardFlowStageSet = readmodel.FlowStageSet

func assembleDashboardFlowStageSet(providers []store.ProviderEventRecord, agents []store.AgentEventRecord, preferInPosition bool, agentModels map[string]string) dashboardFlowStageSet {
	return readmodel.AssembleFlowStageSet(providers, agents, preferInPosition, agentModels)
}

func buildDashboardFlowTrace(stages dashboardFlowStageSet, gate *store.GateEventRecord, pos store.PositionRecord, isOpen bool) DashboardFlowTrace {
	rm := readmodel.BuildFlowTrace(stages, gate, strings.TrimSpace(pos.Side), isOpen)
	return mapDashboardFlowTrace(rm)
}

func buildDashboardFlowNodes(stages dashboardFlowStageSet, gate *store.GateEventRecord, tighten *DashboardTightenInfo) []DashboardFlowNode {
	var rmTighten *readmodel.TightenInfo
	if tighten != nil {
		rmTighten = &readmodel.TightenInfo{Triggered: tighten.Triggered, Reason: tighten.Reason}
	}
	return mapDashboardFlowNodes(readmodel.BuildFlowNodes(stages, gate, rmTighten))
}

func shouldPreferInPositionProvider(isOpen bool, gate *store.GateEventRecord) bool {
	return readmodel.ShouldPreferInPositionProvider(isOpen, gate)
}

func mapByProviderRoleWithMode(providers []store.ProviderEventRecord, preferInPosition bool) (map[string]store.ProviderEventRecord, bool) {
	return readmodel.MapByProviderRoleWithMode(providers, preferInPosition)
}

func mapDashboardFlowTrace(trace readmodel.FlowTrace) DashboardFlowTrace {
	out := DashboardFlowTrace{
		Agents:    mapDashboardFlowStageValues(trace.Agents),
		Providers: mapDashboardFlowStageValues(trace.Providers),
	}
	if trace.InPosition != nil {
		out.InPosition = &DashboardFlowInPosition{
			Active: trace.InPosition.Active,
			Side:   trace.InPosition.Side,
			Status: trace.InPosition.Status,
			Reason: trace.InPosition.Reason,
		}
	}
	if trace.Gate != nil {
		out.Gate = &DashboardFlowGateTrace{
			Action:    trace.Gate.Action,
			Tradeable: trace.Gate.Tradeable,
			Status:    trace.Gate.Status,
			Reason:    trace.Gate.Reason,
			Summary:   trace.Gate.Summary,
			Rules:     mapDashboardFlowFields(trace.Gate.Rules),
		}
	}
	return out
}

func mapDashboardFlowStageValues(items []readmodel.FlowStageData) []DashboardFlowStageValues {
	if len(items) == 0 {
		return nil
	}
	out := make([]DashboardFlowStageValues, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardFlowStageValues{
			Stage:   item.Stage,
			Mode:    item.Mode,
			Source:  item.Source,
			Model:   item.Model,
			Status:  item.Status,
			Reason:  item.Reason,
			Summary: item.Summary,
			Values:  mapDashboardFlowFields(item.Values),
		})
	}
	return out
}

func mapDashboardFlowNodes(items []readmodel.FlowNode) []DashboardFlowNode {
	if len(items) == 0 {
		return nil
	}
	out := make([]DashboardFlowNode, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardFlowNode{
			Stage:   item.Stage,
			Title:   item.Title,
			Outcome: item.Outcome,
			Status:  item.Status,
			Reason:  item.Reason,
			Values:  mapDashboardFlowFields(item.Values),
		})
	}
	return out
}

func mapDashboardFlowFields(items []readmodel.FlowValueField) []DashboardFlowValueField {
	if len(items) == 0 {
		return nil
	}
	out := make([]DashboardFlowValueField, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardFlowValueField{
			Key:   item.Key,
			Value: item.Value,
			State: item.State,
		})
	}
	return out
}
