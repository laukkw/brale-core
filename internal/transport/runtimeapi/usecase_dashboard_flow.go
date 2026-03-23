package runtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"brale-core/internal/position"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

const dashboardDecisionFlowGateScanLimit = 200

var dashboardFlowOrderedRoles = []string{"indicator", "structure", "mechanics"}

type dashboardFlowUsecase struct {
	resolver    SymbolResolver
	store       dashboardFlowStore
	allowSymbol func(string) bool
}

type dashboardFlowStore interface {
	store.TimelineQueryStore
	store.PositionQueryStore
	store.RiskPlanQueryStore
}

type dashboardFlowStageData struct {
	Stage   string
	Mode    string
	Source  string
	Status  string
	Reason  string
	Summary string
	Values  []DashboardFlowValueField
}

func newDashboardFlowUsecase(s *Server) dashboardFlowUsecase {
	if s == nil {
		return dashboardFlowUsecase{}
	}
	return dashboardFlowUsecase{resolver: s.Resolver, store: s.Store, allowSymbol: s.AllowSymbol}
}

func (u dashboardFlowUsecase) build(ctx context.Context, rawSymbol string, snapshotQuery string) (DashboardDecisionFlowResponse, *usecaseError) {
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

	selectedSnapshotID, hasSelectedSnapshot, parseErr := parseDetailSnapshotQuery(snapshotQuery)
	if parseErr != nil {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 400, Code: "invalid_snapshot_id", Message: "snapshot_id 非法", Details: parseErr.Error()}
	}

	var err error
	gates := []store.GateEventRecord{}
	if !hasSelectedSnapshot {
		gates, err = u.store.ListGateEvents(ctx, normalizedSymbol, dashboardDecisionFlowGateScanLimit)
		if err != nil {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "gate_events_failed", Message: "gate 事件读取失败", Details: err.Error()}
		}
	}

	pos, isOpen, err := u.store.FindPositionBySymbol(ctx, normalizedSymbol, position.OpenPositionStatuses)
	if err != nil {
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "position_lookup_failed", Message: "持仓查询失败", Details: err.Error()}
	}

	selection := dashboardFlowSelection{}
	if hasSelectedSnapshot {
		selectedGate, found, gateErr := u.store.FindGateEventBySnapshot(ctx, normalizedSymbol, selectedSnapshotID)
		if gateErr != nil {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "gate_events_failed", Message: "gate 事件读取失败", Details: gateErr.Error()}
		}
		if !found {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 404, Code: "snapshot_not_found", Message: "snapshot_id 对应决策不存在", Details: selectedSnapshotID}
		}
		selection = dashboardFlowSelection{
			Anchor: DashboardFlowAnchor{Type: "selected_round", SnapshotID: selectedSnapshotID, Confidence: "high", Reason: "selected_by_snapshot_id"},
			Gate:   &selectedGate,
		}
	} else {
		var ok bool
		selection, ok = selectDashboardFlowSelection(selectedSnapshotID, false, pos, isOpen, gates)
		if !ok {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 404, Code: "snapshot_not_found", Message: "snapshot_id 对应决策不存在", Details: selectedSnapshotID}
		}
	}
	anchor := selection.Anchor
	gateway := selection.Gate

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
	if tighten == nil && isOpen && !hasSelectedSnapshot {
		tighten = resolveDashboardTightenFromRiskHistory(ctx, u.store, pos)
	}

	preferInPositionProvider := shouldPreferInPositionProvider(isOpen, gateway)
	stages := assembleDashboardFlowStageSet(providers, agents, preferInPositionProvider)
	nodes := buildDashboardFlowNodes(stages, gateway, tighten)
	intervals := u.resolveSymbolIntervals(normalizedSymbol)
	trace := buildDashboardFlowTrace(stages, gateway, pos, isOpen)

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

func dashboardStatusFromTradeable(tradeable bool) string {
	if tradeable {
		return "ok"
	}
	return "blocked"
}

func firstBlockingFieldReason(fields []DashboardFlowValueField) string {
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field.State), "block") {
			return strings.TrimSpace(field.Key) + "=" + strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func extractTraceFieldsFromObject(obj map[string]any) []DashboardFlowValueField {
	if len(obj) == 0 {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
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
	sort.Strings(keys)
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
