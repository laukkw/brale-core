package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

type dashboardHistoryUsecase struct {
	store       store.Store
	allowSymbol func(string) bool
}

func newDashboardHistoryUsecase(s *Server) dashboardHistoryUsecase {
	if s == nil {
		return dashboardHistoryUsecase{}
	}
	return dashboardHistoryUsecase{store: s.Store, allowSymbol: s.AllowSymbol}
}

func (u dashboardHistoryUsecase) build(ctx context.Context, rawSymbol string, limit int, snapshotQuery string) (DashboardDecisionHistoryResponse, *usecaseError) {
	if u.store == nil {
		return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 500, Code: "store_missing", Message: "Store 未配置"}
	}

	normalizedSymbol := runtime.NormalizeSymbol(strings.TrimSpace(rawSymbol))
	if normalizedSymbol == "" || !isValidDashboardSymbol(normalizedSymbol) {
		return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 400, Code: "invalid_symbol", Message: "symbol 非法", Details: rawSymbol}
	}
	if u.allowSymbol != nil && !u.allowSymbol(normalizedSymbol) {
		return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: normalizedSymbol}
	}

	gates, err := u.store.ListGateEvents(ctx, normalizedSymbol, limit)
	if err != nil {
		return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 500, Code: "gate_events_failed", Message: "gate 事件读取失败", Details: err.Error()}
	}

	items := mapHistoryItems(gates)
	response := DashboardDecisionHistoryResponse{
		Status:  "ok",
		Symbol:  normalizedSymbol,
		Limit:   limit,
		Items:   items,
		Summary: dashboardContractSummary,
	}
	if len(items) == 0 {
		response.Message = "no_history_available"
	} else {
		response.Message = fmt.Sprintf("history_rows=%d", len(items))
	}

	detailSnapshotID, hasDetail, parseErr := parseDetailSnapshotQuery(snapshotQuery)
	if parseErr != nil {
		return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 400, Code: "invalid_snapshot_id", Message: "snapshot_id 非法", Details: parseErr.Error()}
	}
	if hasDetail {
		detail, detailErr := buildDecisionDetail(ctx, u.store, normalizedSymbol, detailSnapshotID)
		if detailErr != nil {
			return DashboardDecisionHistoryResponse{}, detailErr
		}
		response.Detail = detail
	}

	return response, nil
}

func parseDetailSnapshotQuery(raw string) (uint, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, false, fmt.Errorf("snapshot_id must be positive integer")
	}
	return uint(parsed), true, nil
}

func mapHistoryItems(gates []store.GateEventRecord) []DashboardDecisionHistoryItem {
	if len(gates) == 0 {
		return []DashboardDecisionHistoryItem{}
	}
	out := make([]DashboardDecisionHistoryItem, 0, len(gates))
	for _, gate := range gates {
		at := ""
		if gate.Timestamp > 0 {
			at = time.Unix(gate.Timestamp, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, DashboardDecisionHistoryItem{
			SnapshotID: gate.SnapshotID,
			Action:     strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
			Reason:     strings.TrimSpace(gate.GateReason),
			At:         at,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].At > out[j].At
	})
	return out
}

func buildDecisionDetail(ctx context.Context, st store.Store, symbol string, snapshotID uint) (*DashboardDecisionDetail, *usecaseError) {
	if snapshotID == 0 {
		return nil, &usecaseError{Status: 400, Code: "invalid_snapshot_id", Message: "snapshot_id 非法"}
	}
	gates, err := st.ListGateEvents(ctx, symbol, dashboardDecisionFlowGateScanLimit)
	if err != nil {
		return nil, &usecaseError{Status: 500, Code: "gate_events_failed", Message: "gate 事件读取失败", Details: err.Error()}
	}
	var selected *store.GateEventRecord
	for idx := range gates {
		if gates[idx].SnapshotID == snapshotID {
			selected = &gates[idx]
			break
		}
	}
	if selected == nil {
		return nil, &usecaseError{Status: 404, Code: "snapshot_not_found", Message: "snapshot_id 对应决策不存在", Details: snapshotID}
	}

	providers, err := st.ListProviderEventsBySnapshot(ctx, symbol, snapshotID)
	if err != nil {
		return nil, &usecaseError{Status: 500, Code: "provider_events_failed", Message: "provider 事件读取失败", Details: err.Error()}
	}
	agents, err := st.ListAgentEventsBySnapshot(ctx, symbol, snapshotID)
	if err != nil {
		return nil, &usecaseError{Status: 500, Code: "agent_events_failed", Message: "agent 事件读取失败", Details: err.Error()}
	}

	formatter := decisionfmt.New()
	report, reportErr := formatter.BuildDecisionReport(decisionfmt.DecisionInput{
		Symbol:     symbol,
		SnapshotID: snapshotID,
		Gate: decisionfmt.GateEvent{
			ID:               selected.ID,
			SnapshotID:       selected.SnapshotID,
			GlobalTradeable:  selected.GlobalTradeable,
			DecisionAction:   selected.DecisionAction,
			Grade:            selected.Grade,
			GateReason:       selected.GateReason,
			Direction:        selected.Direction,
			ProviderRefsJSON: json.RawMessage(selected.ProviderRefsJSON),
			RuleHitJSON:      json.RawMessage(selected.RuleHitJSON),
			DerivedJSON:      json.RawMessage(selected.DerivedJSON),
		},
		Providers: mapDecisionProviders(providers),
		Agents:    mapDecisionAgents(agents),
	})
	if reportErr != nil {
		return nil, &usecaseError{Status: 500, Code: "decision_build_failed", Message: "决策详情解析失败", Details: reportErr.Error()}
	}

	providerSummaries := make([]string, 0, len(report.Providers))
	for _, stage := range report.Providers {
		providerSummaries = append(providerSummaries, strings.TrimSpace(stage.Role+": "+stage.Summary))
	}
	agentSummaries := make([]string, 0, len(report.Agents))
	for _, stage := range report.Agents {
		agentSummaries = append(agentSummaries, strings.TrimSpace(stage.Role+": "+stage.Summary))
	}

	detail := &DashboardDecisionDetail{
		SnapshotID:      selected.SnapshotID,
		Action:          strings.ToUpper(strings.TrimSpace(selected.DecisionAction)),
		Reason:          strings.TrimSpace(selected.GateReason),
		Tradeable:       selected.GlobalTradeable,
		Providers:       providerSummaries,
		Agents:          agentSummaries,
		ReportMarkdown:  prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionMarkdown(report)),
		DecisionViewURL: fmt.Sprintf("/decision-view/?symbol=%s&snapshot_id=%d", symbol, snapshotID),
	}
	return detail, nil
}

func mapDecisionProviders(records []store.ProviderEventRecord) []decisionfmt.ProviderEvent {
	out := make([]decisionfmt.ProviderEvent, 0, len(records))
	for _, rec := range records {
		out = append(out, decisionfmt.ProviderEvent{
			SnapshotID: rec.SnapshotID,
			OutputJSON: json.RawMessage(rec.OutputJSON),
			Role:       rec.Role,
		})
	}
	return out
}

func mapDecisionAgents(records []store.AgentEventRecord) []decisionfmt.AgentEvent {
	out := make([]decisionfmt.AgentEvent, 0, len(records))
	for _, rec := range records {
		out = append(out, decisionfmt.AgentEvent{
			SnapshotID: rec.SnapshotID,
			OutputJSON: json.RawMessage(rec.OutputJSON),
			Stage:      rec.Stage,
		})
	}
	return out
}
