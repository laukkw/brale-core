package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/position"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

const dashboardDecisionFlowGateScanLimit = 200

var dashboardFlowOrderedRoles = []string{"indicator", "structure", "mechanics"}

type dashboardFlowUsecase struct {
	resolver    SymbolResolver
	store       store.Store
	allowSymbol func(string) bool
}

func newDashboardFlowUsecase(s *Server) dashboardFlowUsecase {
	if s == nil {
		return dashboardFlowUsecase{}
	}
	return dashboardFlowUsecase{resolver: s.Resolver, store: s.Store, allowSymbol: s.AllowSymbol}
}

func (u dashboardFlowUsecase) build(ctx context.Context, rawSymbol string) (DashboardDecisionFlowResponse, *usecaseError) {
	if u.store == nil {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "store_missing", Message: "Store 未配置"}
	}

	normalizedSymbol := runtime.NormalizeSymbol(strings.TrimSpace(rawSymbol))
	if normalizedSymbol == "" || !isValidDashboardSymbol(normalizedSymbol) {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 400, Code: "invalid_symbol", Message: "symbol 非法", Details: rawSymbol}
	}
	if u.allowSymbol != nil && !u.allowSymbol(normalizedSymbol) {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: normalizedSymbol}
	}

	gates, err := u.store.ListGateEvents(ctx, normalizedSymbol, dashboardDecisionFlowGateScanLimit)
	if err != nil {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "gate_events_failed", Message: "gate 事件读取失败", Details: err.Error()}
	}

	pos, isOpen, err := u.store.FindPositionBySymbol(ctx, normalizedSymbol, position.OpenPositionStatuses)
	if err != nil {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "position_lookup_failed", Message: "持仓查询失败", Details: err.Error()}
	}

	anchor := resolveDashboardFlowAnchor(pos, isOpen, gates)
	gateway := selectLatestFlowGate(gates)

	providers := []store.ProviderEventRecord{}
	agents := []store.AgentEventRecord{}
	if gateway != nil && gateway.SnapshotID > 0 {
		providers, err = u.store.ListProviderEventsBySnapshot(ctx, normalizedSymbol, gateway.SnapshotID)
		if err != nil {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "provider_events_failed", Message: "provider 事件读取失败", Details: err.Error()}
		}
		agents, err = u.store.ListAgentEventsBySnapshot(ctx, normalizedSymbol, gateway.SnapshotID)
		if err != nil {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "agent_events_failed", Message: "agent 事件读取失败", Details: err.Error()}
		}
	}

	tighten := resolveDashboardTightenInfo(gateway)
	if tighten == nil && isOpen {
		tighten = resolveDashboardTightenFromRiskHistory(ctx, u.store, pos)
	}

	preferInPositionProvider := shouldPreferInPositionProvider(isOpen, gateway)
	providerByRole, providerInPositionMode := mapByProviderRoleWithMode(providers, preferInPositionProvider)
	nodes := buildDashboardFlowNodes(providerByRole, providerInPositionMode, agents, gateway, tighten)
	intervals := u.resolveSymbolIntervals(normalizedSymbol)
	trace := buildDashboardFlowTrace(providerByRole, providerInPositionMode, agents, gateway, pos, isOpen)

	return DashboardDecisionFlowResponse{
		Status: "ok",
		Symbol: normalizedSymbol,
		Flow: DashboardDecisionFlow{
			Anchor:    anchor,
			Nodes:     nodes,
			Intervals: intervals,
			Trace:     trace,
			Tighten:   tighten,
		},
		Summary: dashboardContractSummary,
	}, nil
}

func buildDashboardFlowTrace(providerByRole map[string]store.ProviderEventRecord, providerInPositionMode bool, agents []store.AgentEventRecord, gate *store.GateEventRecord, pos store.PositionRecord, isOpen bool) DashboardFlowTrace {
	trace := DashboardFlowTrace{}
	trace.Agents = buildAgentTrace(agents)
	trace.Providers = buildProviderTrace(providerByRole, providerInPositionMode)
	if isOpen {
		trace.InPosition = &DashboardFlowInPosition{Active: true, Side: strings.ToLower(strings.TrimSpace(pos.Side))}
	}
	if gate != nil {
		trace.Gate = &DashboardFlowGateTrace{
			Action:    strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
			Tradeable: gate.GlobalTradeable,
			Reason:    strings.TrimSpace(gate.GateReason),
			Rules:     extractGateRules([]byte(gate.RuleHitJSON)),
		}
	}
	return trace
}

func buildProviderTrace(providerByRole map[string]store.ProviderEventRecord, inPositionMode bool) []DashboardFlowStageValues {
	ordered := []string{"indicator", "structure", "mechanics"}
	out := make([]DashboardFlowStageValues, 0, len(ordered))
	mode := "standard"
	if inPositionMode {
		mode = "in_position"
	}
	for _, role := range ordered {
		rec, ok := providerByRole[role]
		if !ok {
			continue
		}
		fields := extractTraceFields([]byte(rec.OutputJSON))
		out = append(out, DashboardFlowStageValues{Stage: role, Mode: mode, Source: strings.TrimSpace(rec.ProviderID), Values: fields})
	}
	return out
}

func buildAgentTrace(records []store.AgentEventRecord) []DashboardFlowStageValues {
	ordered := []string{"indicator", "structure", "mechanics"}
	latest := mapByAgentStage(records)
	out := make([]DashboardFlowStageValues, 0, len(ordered))
	for _, stage := range ordered {
		rec, ok := latest[stage]
		if !ok {
			continue
		}
		fields := extractTraceFields([]byte(rec.OutputJSON))
		out = append(out, DashboardFlowStageValues{Stage: stage, Source: strings.TrimSpace(rec.Stage), Values: fields})
	}
	return out
}

func extractTraceFields(raw json.RawMessage) []DashboardFlowValueField {
	obj := decodeJSONObject(raw)
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	ordered := make([]DashboardFlowValueField, 0, len(keys))
	for _, key := range keys {
		lk := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lk, "detail") || strings.Contains(lk, "reason") || strings.Contains(lk, "prompt") || strings.Contains(lk, "markdown") || strings.Contains(lk, "report") || strings.Contains(lk, "text") {
			continue
		}
		if field, ok := traceFieldFromValue(key, obj[key]); ok {
			ordered = append(ordered, field)
		}
	}
	return ordered
}

func traceFieldFromValue(key string, value any) (DashboardFlowValueField, bool) {
	field := DashboardFlowValueField{Key: key}
	switch v := value.(type) {
	case bool:
		if v {
			field.Value = "true"
			field.State = "pass"
		} else {
			field.Value = "false"
			field.State = "block"
		}
		return field, true
	case string:
		text := strings.TrimSpace(v)
		if text == "" || len(text) > 48 {
			return DashboardFlowValueField{}, false
		}
		field.Value = text
		return field, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return DashboardFlowValueField{}, false
		}
		field.Value = strconv.FormatFloat(v, 'f', -1, 64)
		return field, true
	case int, int64, uint, uint64, int32, uint32, float32:
		field.Value = fmt.Sprintf("%v", v)
		return field, true
	default:
		return DashboardFlowValueField{}, false
	}
}

func decodeJSONObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

func extractGateRules(raw json.RawMessage) []DashboardFlowValueField {
	obj := decodeJSONObject(raw)
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	out := make([]DashboardFlowValueField, 0, len(keys))
	for _, key := range keys {
		field, ok := traceFieldFromValue(key, obj[key])
		if !ok {
			continue
		}
		if field.State == "" {
			if strings.EqualFold(field.Value, "true") {
				field.State = "pass"
			} else if strings.EqualFold(field.Value, "false") {
				field.State = "block"
			}
		}
		out = append(out, field)
	}
	return out
}

func (u dashboardFlowUsecase) resolveSymbolIntervals(symbol string) []string {
	if u.resolver == nil {
		return nil
	}
	resolved, err := u.resolver.Resolve(symbol)
	if err != nil {
		return nil
	}
	return normalizedIntervals(resolved.Intervals)
}

func selectAnchorGate(anchorSnapshotID uint, gates []store.GateEventRecord) *store.GateEventRecord {
	if len(gates) == 0 {
		return nil
	}
	if anchorSnapshotID > 0 {
		for idx := range gates {
			if gates[idx].SnapshotID == anchorSnapshotID {
				return &gates[idx]
			}
		}
	}
	for idx := range gates {
		if gates[idx].SnapshotID > 0 {
			return &gates[idx]
		}
	}
	return nil
}

func selectLatestFlowGate(gates []store.GateEventRecord) *store.GateEventRecord {
	if len(gates) == 0 {
		return nil
	}
	for idx := range gates {
		if gates[idx].SnapshotID > 0 {
			return &gates[idx]
		}
	}
	return &gates[0]
}

func buildDashboardFlowNodes(providerByRole map[string]store.ProviderEventRecord, providerInPositionMode bool, agents []store.AgentEventRecord, gate *store.GateEventRecord, tighten *DashboardTightenInfo) []DashboardFlowNode {
	nodes := make([]DashboardFlowNode, 0, 10)
	providerTitlePrefix := "Provider"
	if providerInPositionMode {
		providerTitlePrefix = "InPositionProvider"
	}
	for _, role := range dashboardFlowOrderedRoles {
		rec, ok := providerByRole[role]
		if !ok {
			nodes = append(nodes, DashboardFlowNode{Stage: "gap", Title: fmt.Sprintf("%s/%s", providerTitlePrefix, role), Outcome: "missing_provider_stage"})
			continue
		}
		nodes = append(nodes, DashboardFlowNode{Stage: "provider", Title: fmt.Sprintf("%s/%s", providerTitlePrefix, role), Outcome: summarizeProviderOutcome(rec)})
	}

	agentByStage := mapByAgentStage(agents)
	for _, stage := range dashboardFlowOrderedRoles {
		rec, ok := agentByStage[stage]
		if !ok {
			nodes = append(nodes, DashboardFlowNode{Stage: "gap", Title: fmt.Sprintf("Agent/%s", stage), Outcome: "missing_agent_stage"})
			continue
		}
		nodes = append(nodes, DashboardFlowNode{Stage: "agent", Title: fmt.Sprintf("Agent/%s", stage), Outcome: summarizeAgentOutcome(rec)})
	}

	if gate == nil {
		nodes = append(nodes,
			DashboardFlowNode{Stage: "gap", Title: "Gate", Outcome: "missing_gate_stage"},
			DashboardFlowNode{Stage: "result", Title: "Terminal Outcome", Outcome: "missing_gate_event"},
		)
		return nodes
	}

	nodes = append(nodes, DashboardFlowNode{Stage: "gate", Title: "Gate", Outcome: summarizeGateOutcome(*gate)})

	if shouldRenderPlanNode(*gate, tighten) {
		nodes = append(nodes, DashboardFlowNode{Stage: "plan", Title: "Plan", Outcome: summarizePlanOutcome(*gate, tighten)})
	}

	nodes = append(nodes, DashboardFlowNode{Stage: "result", Title: "Terminal Outcome", Outcome: summarizeTerminalOutcome(*gate, tighten)})
	return nodes
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

func summarizeProviderOutcome(rec store.ProviderEventRecord) string {
	monitorTag, reason := readMonitorTagAndReason(json.RawMessage(rec.OutputJSON))
	parts := []string{fmt.Sprintf("tradeable=%t", rec.Tradeable)}
	if monitorTag != "" {
		parts = append(parts, "monitor_tag="+monitorTag)
	}
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}
	return strings.Join(parts, " | ")
}

func summarizeAgentOutcome(rec store.AgentEventRecord) string {
	monitorTag, reason := readMonitorTagAndReason(json.RawMessage(rec.OutputJSON))
	if monitorTag != "" && reason != "" {
		return "monitor_tag=" + monitorTag + " | reason=" + reason
	}
	if monitorTag != "" {
		return "monitor_tag=" + monitorTag
	}
	if reason != "" {
		return "reason=" + reason
	}
	return "agent_output_available"
}

func readMonitorTagAndReason(raw json.RawMessage) (string, string) {
	if len(raw) == 0 {
		return "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	monitorTag, _ := payload["monitor_tag"].(string)
	reason, _ := payload["reason"].(string)
	return strings.TrimSpace(monitorTag), strings.TrimSpace(reason)
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
		return "risk_plan_update | reason=" + strings.TrimSpace(tighten.Reason)
	}
	if strings.TrimSpace(gate.Direction) != "" {
		return "plan_ready | direction=" + strings.TrimSpace(gate.Direction)
	}
	return "plan_ready"
}

func summarizeTerminalOutcome(gate store.GateEventRecord, tighten *DashboardTightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		if tighten.Triggered {
			return "tighten_executed | reason=" + strings.TrimSpace(tighten.Reason)
		}
		return "tighten_blocked | reason=" + strings.TrimSpace(tighten.Reason)
	}
	if !gate.GlobalTradeable {
		reason := strings.TrimSpace(gate.GateReason)
		if reason == "" {
			reason = "gate_blocked"
		}
		return "blocked | reason=" + reason
	}
	if shouldRenderPlanNode(gate, tighten) {
		return "plan_emitted"
	}
	return "gate_pass_no_plan"
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

func resolveDashboardTightenFromRiskHistory(ctx context.Context, st store.Store, pos store.PositionRecord) *DashboardTightenInfo {
	if st == nil || strings.TrimSpace(pos.PositionID) == "" {
		return nil
	}
	rows, err := st.ListRiskPlanHistory(ctx, pos.PositionID, 10)
	if err != nil || len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		source := strings.ToLower(strings.TrimSpace(row.Source))
		if source == "monitor-tighten" || source == "monitor-breakeven" {
			return &DashboardTightenInfo{Triggered: true, Reason: source}
		}
	}
	return nil
}

func resolveDashboardFlowAnchor(pos store.PositionRecord, isOpen bool, gates []store.GateEventRecord) DashboardFlowAnchor {
	if isOpen {
		if snapID, ok := resolveOpeningSnapshotID(pos, gates); ok {
			confidence := "medium"
			reason := "matched_by_position_timeline"
			if fromOpenIntentID(snapID, pos.OpenIntentID) {
				confidence = "high"
				reason = "matched_by_open_intent_id"
			}
			return DashboardFlowAnchor{
				Type:       "opening_round",
				SnapshotID: snapID,
				Confidence: confidence,
				Reason:     reason,
			}
		}
		if latest, ok := latestGateSnapshotID(gates); ok {
			return DashboardFlowAnchor{
				Type:       "latest_round",
				SnapshotID: latest,
				Confidence: "low",
				Reason:     "opening_round_unresolved_fallback_latest",
			}
		}
		return DashboardFlowAnchor{
			Type:       "latest_round",
			SnapshotID: 0,
			Confidence: "low",
			Reason:     "no_history_for_open_position",
		}
	}

	if latest, ok := latestGateSnapshotID(gates); ok {
		return DashboardFlowAnchor{
			Type:       "latest_round",
			SnapshotID: latest,
			Confidence: "medium",
			Reason:     "flat_use_latest_round",
		}
	}

	return DashboardFlowAnchor{
		Type:       "latest_round",
		SnapshotID: 0,
		Confidence: "low",
		Reason:     "no_history_available",
	}
}

func latestGateSnapshotID(gates []store.GateEventRecord) (uint, bool) {
	for _, gate := range gates {
		if gate.SnapshotID > 0 {
			return gate.SnapshotID, true
		}
	}
	return 0, false
}

func resolveOpeningSnapshotID(pos store.PositionRecord, gates []store.GateEventRecord) (uint, bool) {
	if len(gates) == 0 {
		return 0, false
	}
	if openIntentSnapshot, ok := parseSnapshotIDFromOpenIntentID(pos.OpenIntentID); ok {
		for _, gate := range gates {
			if gate.SnapshotID == openIntentSnapshot {
				return gate.SnapshotID, true
			}
		}
	}

	anchorTimestamp := int64(0)
	if !pos.CreatedAt.IsZero() {
		anchorTimestamp = pos.CreatedAt.Unix()
	}
	if anchorTimestamp <= 0 && !pos.UpdatedAt.IsZero() {
		anchorTimestamp = pos.UpdatedAt.Unix()
	}

	bestSnapshot := uint(0)
	bestTimestamp := int64(0)
	for _, gate := range gates {
		if gate.SnapshotID == 0 || gate.Timestamp <= 0 {
			continue
		}
		if anchorTimestamp > 0 {
			if gate.Timestamp > anchorTimestamp {
				continue
			}
			if gate.Timestamp >= bestTimestamp {
				bestTimestamp = gate.Timestamp
				bestSnapshot = gate.SnapshotID
			}
			continue
		}
		if bestSnapshot == 0 || gate.Timestamp < bestTimestamp {
			bestTimestamp = gate.Timestamp
			bestSnapshot = gate.SnapshotID
		}
	}

	if bestSnapshot > 0 {
		return bestSnapshot, true
	}
	return 0, false
}

func parseSnapshotIDFromOpenIntentID(openIntentID string) (uint, bool) {
	raw := strings.TrimSpace(openIntentID)
	if raw == "" {
		return 0, false
	}
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r < '0' || r > '9'
	})
	for _, token := range tokens {
		if len(token) < 9 {
			continue
		}
		parsed, err := strconv.ParseUint(token, 10, 64)
		if err != nil || parsed == 0 {
			continue
		}
		return uint(parsed), true
	}
	return 0, false
}

func fromOpenIntentID(snapshotID uint, openIntentID string) bool {
	parsed, ok := parseSnapshotIDFromOpenIntentID(openIntentID)
	return ok && parsed == snapshotID
}
