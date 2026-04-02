package dashboard

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/store"
)

var OrderedRoles = []string{"indicator", "structure", "mechanics"}

type FlowValueField struct {
	Key   string
	Value string
	State string
}

type FlowStageData struct {
	Stage   string
	Mode    string
	Source  string
	Model   string
	Status  string
	Reason  string
	Summary string
	Values  []FlowValueField
}

type FlowStageSet struct {
	ProviderByRole     map[string]store.ProviderEventRecord
	ProviderInPosition bool
	ProviderStages     []FlowStageData
	AgentStages        []FlowStageData
	providerStageByKey map[string]FlowStageData
	agentStageByKey    map[string]FlowStageData
}

type FlowInPosition struct {
	Active bool
	Side   string
	Status string
	Reason string
}

type FlowGateTrace struct {
	Action    string
	Tradeable bool
	Status    string
	Reason    string
	Summary   string
	Rules     []FlowValueField
}

type FlowTrace struct {
	Agents     []FlowStageData
	Providers  []FlowStageData
	InPosition *FlowInPosition
	Gate       *FlowGateTrace
}

type FlowNode struct {
	Stage   string
	Title   string
	Outcome string
	Status  string
	Reason  string
	Values  []FlowValueField
}

type flowOutputMeta struct {
	MonitorTag string
	Reason     string
	Fields     []FlowValueField
}

func AssembleFlowStageSet(providers []store.ProviderEventRecord, agents []store.AgentEventRecord, preferInPosition bool, agentModels map[string]string) FlowStageSet {
	providerByRole, providerInPosition := mapByProviderRoleWithMode(providers, preferInPosition)
	providerStages := buildProviderStageData(providerByRole, providerInPosition)
	agentStages := buildAgentStageData(agents, agentModels)
	return FlowStageSet{
		ProviderByRole:     providerByRole,
		ProviderInPosition: providerInPosition,
		ProviderStages:     providerStages,
		AgentStages:        agentStages,
		providerStageByKey: mapFlowStageData(providerStages),
		agentStageByKey:    mapFlowStageData(agentStages),
	}
}

func MapByProviderRoleWithMode(providers []store.ProviderEventRecord, preferInPosition bool) (map[string]store.ProviderEventRecord, bool) {
	return mapByProviderRoleWithMode(providers, preferInPosition)
}

func BuildFlowTrace(stages FlowStageSet, gate *store.GateEventRecord, side string, isOpen bool) FlowTrace {
	trace := FlowTrace{
		Agents:    append([]FlowStageData(nil), stages.AgentStages...),
		Providers: append([]FlowStageData(nil), stages.ProviderStages...),
	}
	if isOpen {
		trace.InPosition = &FlowInPosition{
			Active: true,
			Side:   strings.ToLower(strings.TrimSpace(side)),
			Status: "ok",
			Reason: "position_open_active",
		}
	}
	if gate != nil {
		trace.Gate = &FlowGateTrace{
			Action:    strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
			Tradeable: gate.GlobalTradeable,
			Status:    statusFromTradeable(gate.GlobalTradeable),
			Reason:    strings.TrimSpace(gate.GateReason),
			Summary:   summarizeGateOutcome(*gate),
			Rules:     ExtractGateRules([]byte(gate.RuleHitJSON)),
		}
	}
	return trace
}

func BuildFlowNodes(stages FlowStageSet, gate *store.GateEventRecord, tighten *TightenInfo) []FlowNode {
	nodes := make([]FlowNode, 0, 10)
	providerTitlePrefix := "Provider"
	if stages.ProviderInPosition {
		providerTitlePrefix = "InPositionProvider"
	}
	for _, role := range OrderedRoles {
		_, ok := stages.ProviderByRole[role]
		if !ok {
			nodes = append(nodes, FlowNode{Stage: "gap", Title: fmt.Sprintf("%s/%s", providerTitlePrefix, role), Outcome: "missing_provider_stage", Status: "blocked", Reason: "missing_provider_stage"})
			continue
		}
		meta := stages.providerStageByKey[role]
		nodes = append(nodes, FlowNode{Stage: "provider", Title: fmt.Sprintf("%s/%s", providerTitlePrefix, role), Outcome: meta.Summary, Status: meta.Status, Reason: meta.Reason})
	}
	for _, stage := range OrderedRoles {
		meta, ok := stages.agentStageByKey[stage]
		if !ok {
			nodes = append(nodes, FlowNode{Stage: "gap", Title: fmt.Sprintf("Agent/%s", stage), Outcome: "missing_agent_stage", Status: "blocked", Reason: "missing_agent_stage"})
			continue
		}
		nodes = append(nodes, FlowNode{Stage: "agent", Title: fmt.Sprintf("Agent/%s", stage), Outcome: meta.Summary, Status: meta.Status, Reason: meta.Reason})
	}
	if gate == nil {
		nodes = append(nodes,
			FlowNode{Stage: "gap", Title: "Gate", Outcome: "missing_gate_stage", Status: "blocked", Reason: "missing_gate_stage"},
			FlowNode{Stage: "result", Title: "Terminal Outcome", Outcome: "missing_gate_event", Status: "blocked", Reason: "missing_gate_event"},
		)
		return nodes
	}
	nodes = append(nodes, FlowNode{Stage: "gate", Title: "Gate", Outcome: summarizeGateOutcome(*gate), Status: statusFromTradeable(gate.GlobalTradeable), Reason: strings.TrimSpace(gate.GateReason)})
	if shouldRenderPlanNode(*gate, tighten) {
		nodes = append(nodes, FlowNode{
			Stage:   "plan",
			Title:   "Plan",
			Outcome: summarizePlanOutcome(*gate, tighten),
			Status:  "ok",
			Reason:  summarizePlanReason(*gate, tighten),
			Values:  summarizeResultFields(*gate, tighten),
		})
	}
	nodes = append(nodes, FlowNode{
		Stage:   "result",
		Title:   "Terminal Outcome",
		Outcome: summarizeTerminalOutcome(*gate, tighten),
		Status:  summarizeTerminalStatus(*gate, tighten),
		Reason:  summarizeTerminalReason(*gate, tighten),
		Values:  summarizeResultFields(*gate, tighten),
	})
	return nodes
}

func SummarizeTightenResultFields(derived map[string]any, tighten *TightenInfo) []FlowValueField {
	return summarizeTightenResultFields(derived, tighten)
}

func ShouldPreferInPositionProvider(isOpen bool, gate *store.GateEventRecord) bool {
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

func ExtractTraceFieldsFromObject(obj map[string]any) []FlowValueField {
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make([]FlowValueField, 0, len(keys))
	for _, key := range keys {
		lk := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lk, "detail") || strings.Contains(lk, "reason") || strings.Contains(lk, "prompt") || strings.Contains(lk, "markdown") || strings.Contains(lk, "report") || strings.Contains(lk, "text") {
			continue
		}
		if field, ok := TraceFieldFromValue(key, obj[key]); ok {
			ordered = append(ordered, field)
		}
	}
	return ordered
}

func TraceFieldFromValue(key string, value any) (FlowValueField, bool) {
	field := FlowValueField{Key: key}
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
			return FlowValueField{}, false
		}
		field.Value = text
		return field, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return FlowValueField{}, false
		}
		field.Value = strconv.FormatFloat(v, 'f', -1, 64)
		return field, true
	case int, int64, uint, uint64, int32, uint32, float32:
		field.Value = fmt.Sprintf("%v", v)
		return field, true
	default:
		return FlowValueField{}, false
	}
}

func DecodeJSONObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

func ExtractGateRules(raw json.RawMessage) []FlowValueField {
	obj := DecodeJSONObject(raw)
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]FlowValueField, 0, len(keys))
	for _, key := range keys {
		field, ok := TraceFieldFromValue(key, obj[key])
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

func FirstBlockingFieldReason(fields []FlowValueField) string {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.State), "block") {
			return strings.TrimSpace(field.Key) + "=" + strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func analyzeFlowOutput(raw json.RawMessage) flowOutputMeta {
	obj := DecodeJSONObject(raw)
	if len(obj) == 0 {
		return flowOutputMeta{}
	}
	monitorTag, _ := obj["monitor_tag"].(string)
	reason, _ := obj["reason"].(string)
	return flowOutputMeta{
		MonitorTag: strings.TrimSpace(monitorTag),
		Reason:     strings.TrimSpace(reason),
		Fields:     ExtractTraceFieldsFromObject(obj),
	}
}

func buildProviderStageData(providerByRole map[string]store.ProviderEventRecord, inPositionMode bool) []FlowStageData {
	out := make([]FlowStageData, 0, len(OrderedRoles))
	mode := "standard"
	if inPositionMode {
		mode = "in_position"
	}
	for _, role := range OrderedRoles {
		rec, ok := providerByRole[role]
		if !ok {
			continue
		}
		meta := analyzeFlowOutput(json.RawMessage(rec.OutputJSON))
		out = append(out, FlowStageData{
			Stage:   role,
			Mode:    mode,
			Source:  strings.TrimSpace(rec.ProviderID),
			Status:  statusFromTradeable(rec.Tradeable),
			Reason:  firstNonEmpty(meta.Reason, FirstBlockingFieldReason(meta.Fields)),
			Summary: summarizeProviderOutcomeFromMeta(rec.Tradeable, meta),
			Values:  meta.Fields,
		})
	}
	return out
}

func buildAgentStageData(records []store.AgentEventRecord, stageModels map[string]string) []FlowStageData {
	latest := mapByAgentStage(records)
	out := make([]FlowStageData, 0, len(OrderedRoles))
	for _, stage := range OrderedRoles {
		rec, ok := latest[stage]
		if !ok {
			continue
		}
		meta := analyzeFlowOutput(json.RawMessage(rec.OutputJSON))
		out = append(out, FlowStageData{
			Stage:   stage,
			Source:  strings.TrimSpace(rec.Stage),
			Model:   strings.TrimSpace(stageModels[stage]),
			Status:  "ok",
			Reason:  meta.Reason,
			Summary: summarizeAgentOutcomeFromMeta(meta),
			Values:  meta.Fields,
		})
	}
	return out
}

func mapFlowStageData(items []FlowStageData) map[string]FlowStageData {
	out := make(map[string]FlowStageData, len(items))
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

func statusFromTradeable(tradeable bool) string {
	if tradeable {
		return "ok"
	}
	return "blocked"
}

func summarizeProviderOutcomeFromMeta(tradeable bool, meta flowOutputMeta) string {
	parts := []string{fmt.Sprintf("tradeable=%t", tradeable)}
	if meta.MonitorTag != "" {
		parts = append(parts, "monitor_tag="+meta.MonitorTag)
	}
	if meta.Reason != "" {
		parts = append(parts, "reason="+meta.Reason)
	}
	return strings.Join(parts, " | ")
}

func summarizeAgentOutcomeFromMeta(meta flowOutputMeta) string {
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

func shouldRenderPlanNode(gate store.GateEventRecord, tighten *TightenInfo) bool {
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

func summarizePlanReason(gate store.GateEventRecord, tighten *TightenInfo) string {
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

func summarizePlanOutcome(gate store.GateEventRecord, tighten *TightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil {
		return appendPlanSourceOutcome("tighten risk plan prepared", resolvePlanSource(gate, tighten))
	}
	if strings.TrimSpace(gate.Direction) != "" {
		return appendPlanSourceOutcome("open plan ready | direction="+strings.TrimSpace(gate.Direction), resolvePlanSource(gate, tighten))
	}
	return appendPlanSourceOutcome("open plan ready", resolvePlanSource(gate, tighten))
}

func summarizeTerminalOutcome(gate store.GateEventRecord, tighten *TightenInfo) string {
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

func summarizeTerminalStatus(gate store.GateEventRecord, tighten *TightenInfo) string {
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" && tighten != nil && !tighten.Triggered {
		return "blocked"
	}
	if !gate.GlobalTradeable {
		return "blocked"
	}
	return "ok"
}

func summarizeTerminalReason(gate store.GateEventRecord, tighten *TightenInfo) string {
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

func summarizeResultFields(gate store.GateEventRecord, tighten *TightenInfo) []FlowValueField {
	derived := DecodeJSONObject(json.RawMessage(gate.DerivedJSON))
	if len(derived) == 0 {
		return nil
	}
	action := strings.ToUpper(strings.TrimSpace(gate.DecisionAction))
	if action == "TIGHTEN" {
		return summarizeTightenResultFields(derived, tighten)
	}
	return summarizePlanResultFields(derived)
}

func summarizePlanResultFields(derived map[string]any) []FlowValueField {
	plan, ok := derived["plan"].(map[string]any)
	if !ok || len(plan) == 0 {
		return nil
	}
	fields := make([]FlowValueField, 0, 8)
	appendFlowField := func(key string, value any) {
		if field, ok := TraceFieldFromValue(key, value); ok {
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

func summarizeTightenResultFields(derived map[string]any, tighten *TightenInfo) []FlowValueField {
	execRaw, ok := derived["execution"].(map[string]any)
	if !ok || len(execRaw) == 0 {
		if tighten == nil {
			return nil
		}
		return []FlowValueField{{Key: "tighten", Value: strings.TrimSpace(tighten.Reason)}}
	}
	fields := make([]FlowValueField, 0, 6)
	appendFlowField := func(key string, value any) {
		if field, ok := TraceFieldFromValue(key, value); ok {
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

func resolvePlanSource(gate store.GateEventRecord, tighten *TightenInfo) string {
	derived := DecodeJSONObject(json.RawMessage(gate.DerivedJSON))
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

func appendPlanSourceOutcome(base, source string) string {
	label := planSourceLabel(source)
	if label == "" {
		return base
	}
	return base + " | " + label
}

func firstNonEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return val
		}
	}
	return ""
}
