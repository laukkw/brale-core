package runtimeapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/store"
)

type dashboardFlowStageSet struct {
	ProviderByRole      map[string]store.ProviderEventRecord
	ProviderInPosition  bool
	ProviderStages      []dashboardFlowStageData
	AgentStages         []dashboardFlowStageData
	providerStageByName map[string]dashboardFlowStageData
	agentStageByName    map[string]dashboardFlowStageData
}

func assembleDashboardFlowStageSet(providers []store.ProviderEventRecord, agents []store.AgentEventRecord, preferInPosition bool) dashboardFlowStageSet {
	providerByRole, providerInPosition := mapByProviderRoleWithMode(providers, preferInPosition)
	providerStages := buildProviderStageData(providerByRole, providerInPosition)
	agentStages := buildAgentStageData(agents)
	return dashboardFlowStageSet{
		ProviderByRole:      providerByRole,
		ProviderInPosition:  providerInPosition,
		ProviderStages:      providerStages,
		AgentStages:         agentStages,
		providerStageByName: mapFlowStageData(providerStages),
		agentStageByName:    mapFlowStageData(agentStages),
	}
}

func buildDashboardFlowTrace(stages dashboardFlowStageSet, gate *store.GateEventRecord, pos store.PositionRecord, isOpen bool) DashboardFlowTrace {
	trace := DashboardFlowTrace{}
	trace.Agents = buildStageTrace(stages.AgentStages)
	trace.Providers = buildStageTrace(stages.ProviderStages)
	if isOpen {
		trace.InPosition = &DashboardFlowInPosition{
			Active: true,
			Side:   strings.ToLower(strings.TrimSpace(pos.Side)),
			Status: "ok",
			Reason: "position_open_active",
		}
	}
	if gate != nil {
		trace.Gate = &DashboardFlowGateTrace{
			Action:    strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
			Tradeable: gate.GlobalTradeable,
			Status:    dashboardStatusFromTradeable(gate.GlobalTradeable),
			Reason:    strings.TrimSpace(gate.GateReason),
			Summary:   summarizeGateOutcome(*gate),
			Rules:     extractGateRules([]byte(gate.RuleHitJSON)),
		}
	}
	return trace
}

func buildProviderStageData(providerByRole map[string]store.ProviderEventRecord, inPositionMode bool) []dashboardFlowStageData {
	out := make([]dashboardFlowStageData, 0, len(dashboardFlowOrderedRoles))
	mode := "standard"
	if inPositionMode {
		mode = "in_position"
	}
	for _, role := range dashboardFlowOrderedRoles {
		rec, ok := providerByRole[role]
		if !ok {
			continue
		}
		meta := analyzeFlowOutput(json.RawMessage(rec.OutputJSON))
		out = append(out, dashboardFlowStageData{
			Stage:   role,
			Mode:    mode,
			Source:  strings.TrimSpace(rec.ProviderID),
			Status:  dashboardStatusFromTradeable(rec.Tradeable),
			Reason:  firstNonEmpty(meta.Reason, firstBlockingFieldReason(meta.Fields)),
			Summary: summarizeProviderOutcomeFromMeta(rec.Tradeable, meta),
			Values:  meta.Fields,
		})
	}
	return out
}

func buildAgentStageData(records []store.AgentEventRecord) []dashboardFlowStageData {
	latest := mapByAgentStage(records)
	out := make([]dashboardFlowStageData, 0, len(dashboardFlowOrderedRoles))
	for _, stage := range dashboardFlowOrderedRoles {
		rec, ok := latest[stage]
		if !ok {
			continue
		}
		meta := analyzeFlowOutput(json.RawMessage(rec.OutputJSON))
		out = append(out, dashboardFlowStageData{
			Stage:   stage,
			Source:  strings.TrimSpace(rec.Stage),
			Status:  "ok",
			Reason:  meta.Reason,
			Summary: summarizeAgentOutcomeFromMeta(meta),
			Values:  meta.Fields,
		})
	}
	return out
}

func buildStageTrace(items []dashboardFlowStageData) []DashboardFlowStageValues {
	out := make([]DashboardFlowStageValues, 0, len(items))
	for _, item := range items {
		out = append(out, DashboardFlowStageValues(item))
	}
	return out
}

func buildDashboardFlowNodes(stages dashboardFlowStageSet, gate *store.GateEventRecord, tighten *DashboardTightenInfo) []DashboardFlowNode {
	nodes := make([]DashboardFlowNode, 0, 10)
	providerTitlePrefix := "Provider"
	if stages.ProviderInPosition {
		providerTitlePrefix = "InPositionProvider"
	}
	for _, role := range dashboardFlowOrderedRoles {
		_, ok := stages.ProviderByRole[role]
		if !ok {
			nodes = append(nodes, DashboardFlowNode{Stage: "gap", Title: fmt.Sprintf("%s/%s", providerTitlePrefix, role), Outcome: "missing_provider_stage", Status: "blocked", Reason: "missing_provider_stage"})
			continue
		}
		meta := stages.providerStageByName[role]
		nodes = append(nodes, DashboardFlowNode{Stage: "provider", Title: fmt.Sprintf("%s/%s", providerTitlePrefix, role), Outcome: meta.Summary, Status: meta.Status, Reason: meta.Reason})
	}
	for _, stage := range dashboardFlowOrderedRoles {
		meta, ok := stages.agentStageByName[stage]
		if !ok {
			nodes = append(nodes, DashboardFlowNode{Stage: "gap", Title: fmt.Sprintf("Agent/%s", stage), Outcome: "missing_agent_stage", Status: "blocked", Reason: "missing_agent_stage"})
			continue
		}
		nodes = append(nodes, DashboardFlowNode{Stage: "agent", Title: fmt.Sprintf("Agent/%s", stage), Outcome: meta.Summary, Status: meta.Status, Reason: meta.Reason})
	}
	if gate == nil {
		nodes = append(nodes,
			DashboardFlowNode{Stage: "gap", Title: "Gate", Outcome: "missing_gate_stage", Status: "blocked", Reason: "missing_gate_stage"},
			DashboardFlowNode{Stage: "result", Title: "Terminal Outcome", Outcome: "missing_gate_event", Status: "blocked", Reason: "missing_gate_event"},
		)
		return nodes
	}
	nodes = append(nodes, DashboardFlowNode{Stage: "gate", Title: "Gate", Outcome: summarizeGateOutcome(*gate), Status: dashboardStatusFromTradeable(gate.GlobalTradeable), Reason: strings.TrimSpace(gate.GateReason)})
	if shouldRenderPlanNode(*gate, tighten) {
		nodes = append(nodes, DashboardFlowNode{Stage: "plan", Title: "Plan", Outcome: summarizePlanOutcome(*gate, tighten), Status: "ok", Reason: summarizePlanReason(*gate, tighten), Values: summarizeResultFields(*gate, tighten)})
	}
	nodes = append(nodes, DashboardFlowNode{Stage: "result", Title: "Terminal Outcome", Outcome: summarizeTerminalOutcome(*gate, tighten), Status: summarizeTerminalStatus(*gate, tighten), Reason: summarizeTerminalReason(*gate, tighten), Values: summarizeResultFields(*gate, tighten)})
	return nodes
}

func mapFlowStageData(items []dashboardFlowStageData) map[string]dashboardFlowStageData {
	out := make(map[string]dashboardFlowStageData, len(items))
	for _, item := range items {
		out[item.Stage] = item
	}
	return out
}

func mapByProviderRoleWithMode(providers []store.ProviderEventRecord, preferInPosition bool) (map[string]store.ProviderEventRecord, bool) {
	standard := make(map[string]store.ProviderEventRecord, len(providers))
	inPosition := make(map[string]store.ProviderEventRecord, len(providers))
	for _, rec := range providers {
		key, inPositionRole := parseProviderRole(rec.Role)
		if key == "" {
			continue
		}
		target := standard
		if inPositionRole {
			target = inPosition
		}
		existing, ok := target[key]
		if !ok || rec.Timestamp > existing.Timestamp {
			target[key] = rec
		}
	}
	if preferInPosition {
		if len(inPosition) > 0 {
			return inPosition, true
		}
		return standard, false
	}
	if len(standard) > 0 {
		return standard, false
	}
	if len(inPosition) > 0 {
		return inPosition, true
	}
	return map[string]store.ProviderEventRecord{}, false
}

func parseProviderRole(raw string) (stage string, inPosition bool) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", false
	}
	inPosition = strings.Contains(trimmed, "in_position") || strings.Contains(trimmed, "inposition")
	return normalizeFlowStageKey(trimmed), inPosition
}

func shouldPreferInPositionProvider(isOpen bool, gate *store.GateEventRecord) bool {
	if !isOpen {
		return false
	}
	if gate == nil {
		return true
	}
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	switch action {
	case "ALLOW", "OPEN", "ENTRY", "LONG", "SHORT", "BUY", "SELL":
		return false
	case "TIGHTEN", "HOLD", "MANAGE", "KEEP", "WAIT", "SKIP":
		return true
	default:
		return true
	}
}

func mapByAgentStage(agents []store.AgentEventRecord) map[string]store.AgentEventRecord {
	out := make(map[string]store.AgentEventRecord, len(agents))
	for _, rec := range agents {
		key := normalizeFlowStageKey(rec.Stage)
		if key == "" {
			continue
		}
		existing, ok := out[key]
		if !ok || rec.Timestamp > existing.Timestamp {
			out[key] = rec
		}
	}
	return out
}

func normalizeFlowStageKey(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.Contains(s, "indicator"):
		return "indicator"
	case strings.Contains(s, "structure"):
		return "structure"
	case strings.Contains(s, "mechanics"):
		return "mechanics"
	default:
		return ""
	}
}
