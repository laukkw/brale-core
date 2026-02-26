package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/transport"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func buildAgentPayload(res ObserveSymbolResult) map[string]any {
	return map[string]any{
		"indicator": res.AgentIndicator,
		"structure": res.AgentStructure,
		"mechanics": res.AgentMechanics,
	}
}

func buildProviderPayload(res ObserveSymbolResult) map[string]any {
	return buildProviderPayloadFrom(res.Providers.Indicator, res.Providers.Structure, res.Providers.Mechanics)
}

func buildInPositionProviderPayload(res ObserveSymbolResult) map[string]any {
	return buildProviderPayloadFrom(res.InPositionIndicator, res.InPositionStructure, res.InPositionMechanics)
}

func buildProviderPayloadFrom(ind any, st any, mech any) map[string]any {
	return map[string]any{
		"indicator": ind,
		"structure": st,
		"mechanics": mech,
	}
}

func buildInPositionPayload(res ObserveSymbolResult) map[string]any {
	return map[string]any{
		"indicator": res.InPositionIndicator,
		"structure": res.InPositionStructure,
		"mechanics": res.InPositionMechanics,
		"summary":   buildInPositionSummary(res.InPositionIndicator, res.InPositionStructure, res.InPositionMechanics),
	}
}

func buildGatePayload(gate fund.GateDecision) map[string]any {
	return map[string]any{
		"tradeable":       gate.GlobalTradeable,
		"decision_action": gate.DecisionAction,
		"decision_text":   decisionfmt.GateDecisionText(gate.DecisionAction, gate.GateReason),
		"grade":           gate.Grade,
		"reason":          gate.GateReason,
		"reason_code":     gate.GateReason,
		"direction":       gate.Direction,
	}
}

func buildObserveSummary(gate fund.GateDecision) string {
	text := strings.TrimSpace(decisionfmt.GateDecisionText(gate.DecisionAction, gate.GateReason))
	if text != "" {
		return fmt.Sprintf("观察完成：%s", text)
	}
	if gate.GateReason != "" {
		return fmt.Sprintf("观察完成：gate=%s", gate.GateReason)
	}
	return "观察完成"
}

func buildObserveReport(res ObserveSymbolResult) (string, string, string) {
	input, err := buildDecisionInputFromResult(res)
	if err != nil {
		return "", "", ""
	}
	formatter := decisionfmt.New()
	report, err := formatter.BuildDecisionReport(input)
	if err != nil {
		return "", "", ""
	}
	markdown := prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionMarkdown(report))
	html := prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionHTML(report))
	return markdown, markdown, html
}

func buildDecisionInputFromResult(res ObserveSymbolResult) (decisionfmt.DecisionInput, error) {
	input := decisionfmt.DecisionInput{
		Symbol:     res.Symbol,
		SnapshotID: 0,
		Gate: decisionfmt.GateEvent{
			GlobalTradeable: res.Gate.GlobalTradeable,
			DecisionAction:  res.Gate.DecisionAction,
			GateReason:      res.Gate.GateReason,
			Direction:       res.Gate.Direction,
			Grade:           res.Gate.Grade,
		},
	}
	if raw, ok := buildGateProviderRefs(res); ok {
		input.Gate.ProviderRefsJSON = raw
	}
	if res.Gate.RuleHit != nil {
		converted := decisionfmt.GateRuleHit{
			Name:      res.Gate.RuleHit.Name,
			Priority:  res.Gate.RuleHit.Priority,
			Action:    res.Gate.RuleHit.Action,
			Reason:    res.Gate.RuleHit.Reason,
			Grade:     res.Gate.RuleHit.Grade,
			Direction: res.Gate.RuleHit.Direction,
			Default:   res.Gate.RuleHit.Default,
		}
		raw, err := json.Marshal(converted)
		if err == nil {
			input.Gate.RuleHitJSON = raw
		}
	}
	if res.Gate.Derived != nil {
		raw, err := json.Marshal(res.Gate.Derived)
		if err == nil {
			input.Gate.DerivedJSON = raw
		}
	}
	providers := selectProviderOutputs(res)
	input.Providers = appendDecisionProvider(input.Providers, "indicator", providers.Indicator)
	input.Providers = appendDecisionProvider(input.Providers, "structure", providers.Structure)
	input.Providers = appendDecisionProvider(input.Providers, "mechanics", providers.Mechanics)
	input.Agents = appendDecisionAgent(input.Agents, "indicator", res.AgentIndicator)
	input.Agents = appendDecisionAgent(input.Agents, "structure", res.AgentStructure)
	input.Agents = appendDecisionAgent(input.Agents, "mechanics", res.AgentMechanics)
	return input, nil
}

type gateProviderRefsPayload struct {
	Indicator provider.IndicatorProviderOut `json:"indicator"`
	Structure provider.StructureProviderOut `json:"structure"`
	Mechanics provider.MechanicsProviderOut `json:"mechanics"`
}

func buildGateProviderRefs(res ObserveSymbolResult) (json.RawMessage, bool) {
	if res.InPositionEvaluated {
		return nil, false
	}
	refs := gateProviderRefsPayload{
		Indicator: res.Providers.Indicator,
		Structure: res.Providers.Structure,
		Mechanics: res.Providers.Mechanics,
	}
	raw, err := json.Marshal(refs)
	if err != nil {
		return nil, false
	}
	if len(raw) == 0 {
		return nil, false
	}
	return raw, true
}

type observeProviderOutputs struct {
	Indicator any
	Structure any
	Mechanics any
}

func selectProviderOutputs(res ObserveSymbolResult) observeProviderOutputs {
	if res.InPositionEvaluated {
		return observeProviderOutputs{
			Indicator: res.InPositionIndicator,
			Structure: res.InPositionStructure,
			Mechanics: res.InPositionMechanics,
		}
	}
	return observeProviderOutputs{
		Indicator: res.Providers.Indicator,
		Structure: res.Providers.Structure,
		Mechanics: res.Providers.Mechanics,
	}
}

func appendDecisionProvider(list []decisionfmt.ProviderEvent, role string, output any) []decisionfmt.ProviderEvent {
	raw, err := json.Marshal(output)
	if err != nil {
		return list
	}
	return append(list, decisionfmt.ProviderEvent{Role: role, OutputJSON: raw})
}

func appendDecisionAgent(list []decisionfmt.AgentEvent, stage string, output any) []decisionfmt.AgentEvent {
	raw, err := json.Marshal(output)
	if err != nil {
		return list
	}
	return append(list, decisionfmt.AgentEvent{Stage: stage, OutputJSON: raw})
}

func buildInPositionSummary(ind provider.InPositionIndicatorOut, st provider.InPositionStructureOut, mech provider.InPositionMechanicsOut) string {
	formatter := decisionfmt.New()
	parts := make([]string, 0, 3)
	if summary := formatStageSummary(formatter, ind); summary != "" {
		parts = append(parts, fmt.Sprintf("指标:\n%s", summary))
	}
	if summary := formatStageSummary(formatter, st); summary != "" {
		parts = append(parts, fmt.Sprintf("结构:\n%s", summary))
	}
	if summary := formatStageSummary(formatter, mech); summary != "" {
		parts = append(parts, fmt.Sprintf("力学/风险:\n%s", summary))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func formatStageSummary(formatter decisionfmt.Formatter, output any) string {
	raw, err := json.Marshal(output)
	if err != nil {
		return ""
	}
	summary, _, err := formatter.HumanizeLLMOutput(raw)
	if err != nil {
		return ""
	}
	if text, ok := summary.(string); ok {
		return text
	}
	if summary == nil {
		return ""
	}
	return fmt.Sprintf("%v", summary)
}

func modeFromScheduled(enabled bool) string {
	if enabled {
		return "normal"
	}
	return "observer"
}

func writeJSON(w http.ResponseWriter, v any) {
	transport.WriteJSON(w, http.StatusOK, v)
}

func writeError(ctx context.Context, w http.ResponseWriter, status int, code, msg string, details any) {
	resp := errorResponse{
		Code:      code,
		Msg:       msg,
		RequestID: requestIDFromContext(ctx),
		Details:   details,
	}
	transport.WriteJSON(w, status, resp)
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if id == "" {
			id = strings.TrimSpace(r.Header.Get("X-Request-ID"))
		}
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		ctx = logging.With(ctx, zap.String("request_id", id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val := ctx.Value(requestIDKey{}); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func prependDecisionHeader(header string, body string) string {
	header = strings.TrimSpace(header)
	body = strings.TrimSpace(body)
	if header == "" {
		return body
	}
	if body == "" {
		return header
	}
	return header + "\n" + body
}
