package runtimeapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"brale-core/internal/decision/decisionfmt"
)

func (s *Server) handleDecisionLatest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !ensureMethod(ctx, w, r, http.MethodGet) {
		return
	}
	if s.Store == nil {
		writeError(ctx, w, http.StatusInternalServerError, "store_missing", "Store 未配置", nil)
		return
	}
	symbol := strings.TrimSpace(r.URL.Query().Get("symbol"))
	if symbol == "" {
		writeError(ctx, w, http.StatusBadRequest, "symbol_required", "symbol 不能为空", nil)
		return
	}
	gates, err := s.Store.ListGateEvents(ctx, symbol, 1)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "gate_events_failed", "gate 事件读取失败", err)
		return
	}
	if len(gates) == 0 {
		writeJSON(w, DecisionLatestResponse{
			Status:    "ok",
			Symbol:    symbol,
			Summary:   "查询不存在",
			RequestID: requestIDFromContext(ctx),
		})
		return
	}
	gate := gates[0]
	providers, err := s.Store.ListProviderEventsBySnapshot(ctx, symbol, gate.SnapshotID)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "provider_events_failed", "provider 事件读取失败", err)
		return
	}
	agents, err := s.Store.ListAgentEventsBySnapshot(ctx, symbol, gate.SnapshotID)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "agent_events_failed", "agent 事件读取失败", err)
		return
	}
	input := decisionfmt.DecisionInput{
		Symbol:     symbol,
		SnapshotID: gate.SnapshotID,
		Gate: decisionfmt.GateEvent{
			ID:               gate.ID,
			SnapshotID:       gate.SnapshotID,
			GlobalTradeable:  gate.GlobalTradeable,
			DecisionAction:   gate.DecisionAction,
			Grade:            gate.Grade,
			GateReason:       gate.GateReason,
			Direction:        gate.Direction,
			ProviderRefsJSON: json.RawMessage(gate.ProviderRefsJSON),
			RuleHitJSON:      json.RawMessage(gate.RuleHitJSON),
			DerivedJSON:      json.RawMessage(gate.DerivedJSON),
		},
		Providers: make([]decisionfmt.ProviderEvent, 0, len(providers)),
		Agents:    make([]decisionfmt.AgentEvent, 0, len(agents)),
	}
	for _, rec := range providers {
		input.Providers = append(input.Providers, decisionfmt.ProviderEvent{
			SnapshotID: rec.SnapshotID,
			OutputJSON: json.RawMessage(rec.OutputJSON),
			Role:       rec.Role,
		})
	}
	for _, rec := range agents {
		input.Agents = append(input.Agents, decisionfmt.AgentEvent{
			SnapshotID: rec.SnapshotID,
			OutputJSON: json.RawMessage(rec.OutputJSON),
			Stage:      rec.Stage,
		})
	}
	formatter := decisionfmt.New()
	report, err := formatter.BuildDecisionReport(input)
	if err != nil {
		writeError(ctx, w, http.StatusInternalServerError, "decision_build_failed", "决策解析失败", err)
		return
	}
	markdown := prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionMarkdown(report))
	html := prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionHTML(report))
	writeJSON(w, DecisionLatestResponse{
		Status:         "ok",
		Symbol:         symbol,
		SnapshotID:     gate.SnapshotID,
		Report:         markdown,
		ReportMarkdown: markdown,
		ReportHTML:     html,
		Summary:        "",
		RequestID:      requestIDFromContext(ctx),
	})
}
