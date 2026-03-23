package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/store"
)

func summarizePlanReason(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		return strings.TrimSpace(tighten.Reason)
	}
	if label := planSourceLabel(resolvePlanSource(gate, tighten)); label != "" {
		if direction := strings.TrimSpace(gate.Direction); direction != "" {
			return "direction=" + direction + " | " + label
		}
		return label
	}
	if direction := strings.TrimSpace(gate.Direction); direction != "" {
		return "direction=" + direction
	}
	return "plan_ready"
}

func summarizeTerminalStatus(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil && !tighten.Triggered {
		return "blocked"
	}
	if !gate.GlobalTradeable {
		return "blocked"
	}
	return "ok"
}

func summarizeTerminalReason(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		return strings.TrimSpace(tighten.Reason)
	}
	if !gate.GlobalTradeable {
		reason := strings.TrimSpace(gate.GateReason)
		if reason == "" {
			return "gate_blocked"
		}
		return reason
	}
	if shouldRenderPlanNode(gate, tighten) {
		return "plan_emitted"
	}
	return "gate_pass_no_plan"
}

func summarizeProviderOutcomeFromMeta(tradeable bool, meta dashboardFlowOutputMeta) string {
	parts := []string{fmt.Sprintf("tradeable=%t", tradeable)}
	if meta.MonitorTag != "" {
		parts = append(parts, "monitor_tag="+meta.MonitorTag)
	}
	if meta.Reason != "" {
		parts = append(parts, "reason="+meta.Reason)
	}
	return strings.Join(parts, " | ")
}

func summarizeAgentOutcomeFromMeta(meta dashboardFlowOutputMeta) string {
	if meta.MonitorTag != "" && meta.Reason != "" {
		return "monitor_tag=" + meta.MonitorTag + " | reason=" + meta.Reason
	}
	if meta.MonitorTag != "" {
		return "monitor_tag=" + meta.MonitorTag
	}
	if meta.Reason != "" {
		return "reason=" + meta.Reason
	}
	return "agent_output_available"
}

type dashboardFlowOutputMeta struct {
	MonitorTag string
	Reason     string
	Fields     []DashboardFlowValueField
}

func analyzeFlowOutput(raw json.RawMessage) dashboardFlowOutputMeta {
	obj := decodeJSONObject(raw)
	if len(obj) == 0 {
		return dashboardFlowOutputMeta{}
	}
	monitorTag, _ := obj["monitor_tag"].(string)
	reason, _ := obj["reason"].(string)
	return dashboardFlowOutputMeta{
		MonitorTag: strings.TrimSpace(monitorTag),
		Reason:     strings.TrimSpace(reason),
		Fields:     extractTraceFieldsFromObject(obj),
	}
}

func summarizeGateOutcome(gate store.GateEventRecord) string {
	decisionText := strings.TrimSpace(decisionfmt.GateDecisionText(gate.DecisionAction, gate.GateReason))
	parts := []string{
		"action=" + strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
		fmt.Sprintf("tradeable=%t", gate.GlobalTradeable),
	}
	if strings.TrimSpace(gate.GateReason) != "" {
		parts = append(parts, "reason="+strings.TrimSpace(gate.GateReason))
	}
	if decisionText != "" {
		parts = append(parts, "text="+decisionText)
	}
	return strings.Join(parts, " | ")
}

func shouldRenderPlanNode(gate store.GateEventRecord, tighten *DashboardTightenInfo) bool {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" {
		return tighten != nil
	}
	if !gate.GlobalTradeable {
		return false
	}
	switch action {
	case "ALLOW", "OPEN", "ENTRY", "LONG", "SHORT", "BUY", "SELL":
		return true
	default:
		return false
	}
}

func summarizePlanOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		return appendPlanSourceOutcome("tighten risk plan prepared", resolvePlanSource(gate, tighten))
	}
	if strings.TrimSpace(gate.Direction) != "" {
		return appendPlanSourceOutcome("open plan ready | direction="+strings.TrimSpace(gate.Direction), resolvePlanSource(gate, tighten))
	}
	return appendPlanSourceOutcome("open plan ready", resolvePlanSource(gate, tighten))
}

func summarizeTerminalOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		if tighten.Triggered {
			return "tighten executed"
		}
		return "tighten blocked"
	}
	if !gate.GlobalTradeable {
		return "blocked"
	}
	if shouldRenderPlanNode(gate, tighten) {
		return appendPlanSourceOutcome("plan emitted", resolvePlanSource(gate, tighten))
	}
	return "gate passed"
}

func appendPlanSourceOutcome(base, source string) string {
	label := planSourceLabel(source)
	if label == "" {
		return base
	}
	return base + " | " + label
}

func planSourceLabel(source string) string {
	switch normalizePlanSource(source) {
	case "llm":
		return "llm自动生成"
	case "go":
		return "go计算得到"
	default:
		return ""
	}
}

func resolvePlanSource(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	derived := decodeJSONObject(json.RawMessage(gate.DerivedJSON))
	if len(derived) == 0 {
		return ""
	}
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		execRaw, ok := derived["execution"].(map[string]any)
		if !ok {
			return ""
		}
		return normalizePlanSource(fmt.Sprint(execRaw["plan_source"]))
	}
	planRaw, ok := derived["plan"].(map[string]any)
	if !ok {
		return ""
	}
	return normalizePlanSource(fmt.Sprint(planRaw["plan_source"]))
}

func normalizePlanSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "llm":
		return "llm"
	case "go":
		return "go"
	default:
		return ""
	}
}

func summarizeResultFields(gate store.GateEventRecord, tighten *DashboardTightenInfo) []DashboardFlowValueField {
	derived := decodeJSONObject(json.RawMessage(gate.DerivedJSON))
	if len(derived) == 0 {
		return nil
	}
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" {
		return summarizeTightenResultFields(derived, tighten)
	}
	return summarizePlanResultFields(derived)
}

func summarizePlanResultFields(derived map[string]any) []DashboardFlowValueField {
	plan, ok := derived["plan"].(map[string]any)
	if !ok || len(plan) == 0 {
		return nil
	}
	fields := make([]DashboardFlowValueField, 0, 8)
	appendFlowField := func(key string, value any) {
		if field, ok := traceFieldFromValue(key, value); ok {
			fields = append(fields, field)
		}
	}
	appendFlowField("direction", plan["direction"])
	appendFlowField("entry", plan["entry"])
	appendFlowField("stop_loss", plan["stop_loss"])
	appendFlowField("risk_pct", plan["risk_pct"])
	appendFlowField("position_size", plan["position_size"])
	appendFlowField("leverage", plan["leverage"])
	appendFlowField("plan_source", plan["plan_source"])
	if takeProfits, ok := plan["take_profits"].([]any); ok && len(takeProfits) > 0 {
		appendFlowField("tp1", takeProfits[0])
	}
	return fields
}

func summarizeTightenResultFields(derived map[string]any, tighten *DashboardTightenInfo) []DashboardFlowValueField {
	execRaw, ok := derived["execution"].(map[string]any)
	if !ok || len(execRaw) == 0 {
		if tighten == nil {
			return nil
		}
		return []DashboardFlowValueField{{Key: "tighten", Value: strings.TrimSpace(tighten.Reason)}}
	}
	fields := make([]DashboardFlowValueField, 0, 6)
	appendFlowField := func(key string, value any) {
		if field, ok := traceFieldFromValue(key, value); ok {
			fields = append(fields, field)
		}
	}
	appendFlowField("executed", execRaw["executed"])
	appendFlowField("eligible", execRaw["eligible"])
	appendFlowField("plan_source", execRaw["plan_source"])
	appendFlowField("stop_loss", execRaw["stop_loss"])
	appendFlowField("tp_tightened", execRaw["tp_tightened"])
	if tps, ok := execRaw["take_profits"].([]any); ok && len(tps) > 0 {
		appendFlowField("tp1", tps[0])
	}
	if blockedBy, ok := execRaw["blocked_by"].([]any); ok && len(blockedBy) > 0 {
		appendFlowField("blocked_by", blockedBy[0])
	}
	if score, ok := execRaw["score"].(map[string]any); ok {
		appendFlowField("score", score["total"])
		appendFlowField("threshold", score["threshold"])
	}
	return fields
}

func resolveDashboardTightenInfo(gate *store.GateEventRecord) *DashboardTightenInfo {
	if gate == nil || len(gate.DerivedJSON) == 0 {
		return nil
	}
	var derived map[string]any
	if err := json.Unmarshal(gate.DerivedJSON, &derived); err != nil {
		return nil
	}
	executionRaw, ok := derived["execution"].(map[string]any)
	if !ok {
		return nil
	}
	action, _ := executionRaw["action"].(string)
	if !strings.EqualFold(strings.TrimSpace(action), "tighten") {
		return nil
	}
	executed := false
	if value, ok := executionRaw["executed"].(bool); ok {
		executed = value
	}
	reason := "tighten_metadata_present"
	if blockedBy, ok := executionRaw["blocked_by"].([]any); ok && len(blockedBy) > 0 {
		if first, ok := blockedBy[0].(string); ok && strings.TrimSpace(first) != "" {
			reason = strings.TrimSpace(first)
		}
	}
	if executed {
		reason = "executed"
	}
	return &DashboardTightenInfo{Triggered: executed, Reason: reason}
}

func resolveDashboardTightenFromRiskHistory(ctx context.Context, st store.RiskPlanQueryStore, pos store.PositionRecord) *DashboardTightenInfo {
	if st == nil || strings.TrimSpace(pos.PositionID) == "" {
		return nil
	}
	timeline := loadDashboardRiskPlanTimeline(ctx, st, pos, dashboardRiskPlanTimelineLimit)
	if len(timeline) == 0 {
		return nil
	}
	for _, item := range timeline {
		if isDashboardTightenSource(item.Source) {
			return &DashboardTightenInfo{Triggered: true, Reason: strings.ToLower(strings.TrimSpace(item.Source))}
		}
	}
	return nil
}
