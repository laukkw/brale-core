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
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/position"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

type dashboardHistoryUsecase struct {
	store       dashboardHistoryStore
	allowSymbol func(string) bool
	configs     map[string]ConfigBundle
}

type dashboardHistoryStore interface {
	store.TimelineQueryStore
	store.PositionQueryStore
}

func newDashboardHistoryUsecase(s *Server) dashboardHistoryUsecase {
	if s == nil {
		return dashboardHistoryUsecase{}
	}
	return dashboardHistoryUsecase{store: s.Store, allowSymbol: s.AllowSymbol, configs: s.SymbolConfigs}
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
		detail, detailErr := buildDecisionDetail(ctx, u.store, u.configs, normalizedSymbol, detailSnapshotID)
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
		consensus := extractConsensusMetrics(json.RawMessage(gate.DerivedJSON))
		tighten := buildDecisionTightenDetail(json.RawMessage(gate.DerivedJSON))
		out = append(out, DashboardDecisionHistoryItem{
			SnapshotID:          gate.SnapshotID,
			Action:              strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
			Reason:              decisionDisplayReason(strings.ToUpper(strings.TrimSpace(gate.DecisionAction)), strings.TrimSpace(gate.GateReason), tighten),
			At:                  at,
			ConsensusScore:      consensus.Score,
			ConsensusConfidence: consensus.Confidence,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].At > out[j].At
	})
	return out
}

func buildDecisionDetail(ctx context.Context, st dashboardHistoryStore, configs map[string]ConfigBundle, symbol string, snapshotID uint) (*DashboardDecisionDetail, *usecaseError) {
	if snapshotID == 0 {
		return nil, &usecaseError{Status: 400, Code: "invalid_snapshot_id", Message: "snapshot_id 非法"}
	}
	selected, ok, err := st.FindGateEventBySnapshot(ctx, symbol, snapshotID)
	if err != nil {
		return nil, &usecaseError{Status: 500, Code: "gate_events_failed", Message: "gate 事件读取失败", Details: err.Error()}
	}
	if !ok {
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
	consensus := extractConsensusMetrics(json.RawMessage(selected.DerivedJSON))
	tighten := buildDecisionTightenDetail(json.RawMessage(selected.DerivedJSON))
	planSource := resolvePlanSource(selected, resolveDashboardTightenInfo(&selected))

	detail := &DashboardDecisionDetail{
		SnapshotID:                   selected.SnapshotID,
		Action:                       strings.ToUpper(strings.TrimSpace(selected.DecisionAction)),
		Reason:                       decisionDisplayReason(strings.ToUpper(strings.TrimSpace(selected.DecisionAction)), strings.TrimSpace(selected.GateReason), tighten),
		Tradeable:                    selected.GlobalTradeable,
		ConsensusScore:               consensus.Score,
		ConsensusConfidence:          consensus.Confidence,
		ConsensusScoreThreshold:      consensus.ScoreThreshold,
		ConsensusConfidenceThreshold: consensus.ConfidenceThreshold,
		ConsensusScorePassed:         consensus.ScorePassed,
		ConsensusConfidencePassed:    consensus.ConfidencePassed,
		ConsensusPassed:              consensus.Passed,
		Providers:                    providerSummaries,
		Agents:                       agentSummaries,
		Tighten:                      tighten,
		PlanContext:                  buildDecisionPlanContext(configs, symbol, planSource),
		Plan:                         buildDecisionPlanSummary(ctx, st, symbol),
		Sieve:                        buildDecisionSieveDetail(json.RawMessage(selected.DerivedJSON), configs, symbol),
		ReportMarkdown:               prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionMarkdown(report)),
		DecisionViewURL:              fmt.Sprintf("/decision-view/?symbol=%s&snapshot_id=%d", symbol, snapshotID),
	}
	return detail, nil
}

func decisionDisplayReason(action string, fallback string, tighten *DashboardDecisionTightenDetail) string {
	if strings.EqualFold(strings.TrimSpace(action), "TIGHTEN") && tighten != nil && strings.TrimSpace(tighten.DisplayReason) != "" {
		return strings.TrimSpace(tighten.DisplayReason)
	}
	return strings.TrimSpace(fallback)
}

func buildDecisionTightenDetail(raw json.RawMessage) *DashboardDecisionTightenDetail {
	if len(raw) == 0 {
		return nil
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return nil
	}
	execRaw, ok := derived["execution"].(map[string]any)
	if !ok || len(execRaw) == 0 {
		return nil
	}
	action := strings.TrimSpace(fmt.Sprint(execRaw["action"]))
	if !strings.EqualFold(action, "tighten") {
		return nil
	}
	detail := &DashboardDecisionTightenDetail{
		Action:      strings.ToUpper(action),
		Evaluated:   parseDecisionBool(execRaw["evaluated"]),
		Eligible:    parseDecisionBool(execRaw["eligible"]),
		Executed:    parseDecisionBool(execRaw["executed"]),
		TPTightened: parseDecisionBool(execRaw["tp_tightened"]),
	}
	if blockedBy, ok := execRaw["blocked_by"].([]any); ok {
		list := make([]string, 0, len(blockedBy))
		for _, item := range blockedBy {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				list = append(list, text)
			}
		}
		detail.BlockedBy = list
	}
	if score, ok := execRaw["score"].(map[string]any); ok {
		detail.Score = parseDecisionFloat(score["total"])
		detail.ScoreThreshold = parseDecisionFloat(score["threshold"])
		detail.ScoreParseOK = parseDecisionBool(score["parse_ok"])
	}
	detail.DisplayReason = tightenDisplayReason(detail)
	return detail
}

func tightenDisplayReason(detail *DashboardDecisionTightenDetail) string {
	if detail == nil {
		return ""
	}
	if detail.Executed {
		if detail.TPTightened {
			return "已执行收紧，并同步收紧止盈"
		}
		return "已执行持仓收紧"
	}
	if len(detail.BlockedBy) > 0 {
		return "收紧未执行: " + detail.BlockedBy[0]
	}
	if detail.Eligible {
		return "满足收紧条件，等待执行"
	}
	if detail.Evaluated {
		return "已评估持仓收紧，但未触发"
	}
	return "持仓收紧未评估"
}

func buildDecisionPlanContext(configs map[string]ConfigBundle, symbol string, planSource string) *DashboardDecisionPlanContext {
	bundle, ok := configs[runtime.NormalizeSymbol(symbol)]
	if !ok {
		return nil
	}
	riskMgmt := bundle.Strategy.RiskManagement
	return &DashboardDecisionPlanContext{
		RiskPerTradePct: riskMgmt.RiskPerTradePct,
		MaxInvestPct:    riskMgmt.MaxInvestPct,
		MaxLeverage:     riskMgmt.MaxLeverage,
		EntryOffsetATR:  riskMgmt.EntryOffsetATR,
		EntryMode:       strings.TrimSpace(riskMgmt.EntryMode),
		PlanSource:      normalizePlanSource(planSource),
		InitialExit:     strings.TrimSpace(riskMgmt.InitialExit.Policy),
	}
}

func buildDecisionPlanSummary(ctx context.Context, st store.PositionQueryStore, symbol string) *DashboardDecisionPlanSummary {
	if st == nil {
		return nil
	}
	pos, ok, err := st.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil || !ok {
		return nil
	}
	plan := &DashboardDecisionPlanSummary{
		Status:       strings.TrimSpace(pos.Status),
		Direction:    strings.ToLower(strings.TrimSpace(pos.Side)),
		EntryPrice:   pos.AvgEntry,
		PositionSize: pos.Qty,
		RiskPct:      pos.RiskPct,
		Leverage:     pos.Leverage,
	}
	if !pos.CreatedAt.IsZero() {
		plan.OpenedAt = pos.CreatedAt.UTC().Format(time.RFC3339)
	}
	decoded, err := position.DecodeRiskPlan(pos.RiskJSON)
	if err != nil {
		return plan
	}
	plan.StopLoss = decoded.StopPrice
	plan.InitialQty = decoded.InitialQty
	plan.TakeProfitLevels = make([]DashboardDecisionPlanTPLevel, 0, len(decoded.TPLevels))
	plan.TakeProfits = make([]float64, 0, len(decoded.TPLevels))
	for _, level := range decoded.TPLevels {
		plan.TakeProfits = append(plan.TakeProfits, level.Price)
		plan.TakeProfitLevels = append(plan.TakeProfitLevels, DashboardDecisionPlanTPLevel{
			LevelID: level.LevelID,
			Price:   level.Price,
			QtyPct:  level.QtyPct,
			Hit:     level.Hit,
		})
	}
	return plan
}

func buildDecisionSieveDetail(raw json.RawMessage, configs map[string]ConfigBundle, symbol string) *DashboardDecisionSieveDetail {
	if len(raw) == 0 {
		return nil
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return nil
	}
	if !hasDecisionSieveSignal(derived) {
		return nil
	}
	return buildDecisionSieveDetailFromConfig(configs, symbol, derived)
}

func hasDecisionSieveSignal(derived map[string]any) bool {
	if len(derived) == 0 {
		return false
	}
	keys := []string{"sieve_action", "sieve_reason", "sieve_hit", "sieve_size_factor", "gate_action_before_sieve"}
	for _, key := range keys {
		if _, ok := derived[key]; ok {
			return true
		}
	}
	return false
}

func buildDecisionSieveDetailFromConfig(configs map[string]ConfigBundle, symbol string, derived map[string]any) *DashboardDecisionSieveDetail {
	bundle, ok := configs[runtime.NormalizeSymbol(symbol)]
	if !ok {
		if len(derived) == 0 {
			return nil
		}
		return &DashboardDecisionSieveDetail{
			Action:            strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["sieve_action"]))),
			ReasonCode:        strings.TrimSpace(fmt.Sprint(derived["sieve_reason"])),
			Hit:               parseDecisionBool(derived["sieve_hit"]),
			SizeFactor:        parseDecisionFloat(derived["sieve_size_factor"]),
			MinSizeFactor:     parseDecisionFloat(derived["sieve_min_size_factor"]),
			DefaultAction:     strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["sieve_default_action"]))),
			DefaultSizeFactor: parseDecisionFloat(derived["sieve_default_size_factor"]),
			ActionBefore:      strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["gate_action_before_sieve"]))),
			PolicyHash:        strings.TrimSpace(fmt.Sprint(derived["sieve_policy_hash"])),
		}
	}
	sieveCfg := bundle.Strategy.RiskManagement.Sieve
	detail := &DashboardDecisionSieveDetail{
		Action:            strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["sieve_action"]))),
		ReasonCode:        strings.TrimSpace(fmt.Sprint(derived["sieve_reason"])),
		Hit:               parseDecisionBool(derived["sieve_hit"]),
		SizeFactor:        parseDecisionFloat(derived["sieve_size_factor"]),
		MinSizeFactor:     firstPositive(parseDecisionFloat(derived["sieve_min_size_factor"]), sieveCfg.MinSizeFactor),
		DefaultAction:     firstNonEmpty(strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["sieve_default_action"]))), strings.ToUpper(strings.TrimSpace(sieveCfg.DefaultGateAction))),
		DefaultSizeFactor: firstPositive(parseDecisionFloat(derived["sieve_default_size_factor"]), sieveCfg.DefaultSizeFactor),
		ActionBefore:      strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["gate_action_before_sieve"]))),
		PolicyHash:        firstNonEmpty(strings.TrimSpace(fmt.Sprint(derived["sieve_policy_hash"])), bundle.Strategy.Hash),
	}
	if detail.Action == "" && detail.ReasonCode == "" && !detail.Hit {
		return nil
	}
	rows := make([]DashboardDecisionSieveRow, 0, len(sieveCfg.Rows))
	for _, row := range sieveCfg.Rows {
		matched := detail.ReasonCode != "" && strings.EqualFold(strings.TrimSpace(row.ReasonCode), detail.ReasonCode)
		if !matched {
			continue
		}
		rows = append(rows, DashboardDecisionSieveRow{
			MechanicsTag:  strings.TrimSpace(row.MechanicsTag),
			LiqConfidence: strings.TrimSpace(row.LiqConfidence),
			CrowdingAlign: row.CrowdingAlign,
			GateAction:    strings.ToUpper(strings.TrimSpace(row.GateAction)),
			SizeFactor:    row.SizeFactor,
			ReasonCode:    strings.TrimSpace(row.ReasonCode),
			Matched:       matched,
		})
	}
	detail.Rows = rows
	return detail
}

func firstPositive(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseDecisionFloat(value any) float64 {
	if parsed, ok := parseutil.FloatOK(value); ok {
		return parsed
	}
	return 0
}

func parseDecisionBool(value any) bool {
	parsed, ok := parseConsensusBool(value)
	return ok && parsed
}

type dashboardConsensusMetrics struct {
	Score               *float64
	Confidence          *float64
	ScoreThreshold      *float64
	ConfidenceThreshold *float64
	ScorePassed         *bool
	ConfidencePassed    *bool
	Passed              *bool
}

func extractConsensusMetrics(raw json.RawMessage) dashboardConsensusMetrics {
	if len(raw) == 0 {
		return dashboardConsensusMetrics{}
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return dashboardConsensusMetrics{}
	}
	consensusRaw, ok := derived["direction_consensus"]
	if !ok {
		return dashboardConsensusMetrics{}
	}
	consensus, ok := consensusRaw.(map[string]any)
	if !ok || len(consensus) == 0 {
		return dashboardConsensusMetrics{}
	}
	out := dashboardConsensusMetrics{}
	if score, ok := parseutil.FloatOK(consensus["score"]); ok {
		out.Score = &score
	}
	if confidence, ok := parseutil.FloatOK(consensus["confidence"]); ok {
		out.Confidence = &confidence
	}
	if scoreThreshold, ok := parseutil.FloatOK(consensus["score_threshold"]); ok {
		out.ScoreThreshold = &scoreThreshold
	}
	if confidenceThreshold, ok := parseutil.FloatOK(consensus["confidence_threshold"]); ok {
		out.ConfidenceThreshold = &confidenceThreshold
	}
	if scorePassed, ok := parseConsensusBool(consensus["score_passed"]); ok {
		out.ScorePassed = boolPtr(scorePassed)
	} else if out.Score != nil && out.ScoreThreshold != nil {
		out.ScorePassed = boolPtr(absFloat(*out.Score) >= *out.ScoreThreshold)
	}
	if confidencePassed, ok := parseConsensusBool(consensus["confidence_passed"]); ok {
		out.ConfidencePassed = boolPtr(confidencePassed)
	} else if out.Confidence != nil && out.ConfidenceThreshold != nil {
		out.ConfidencePassed = boolPtr(*out.Confidence >= *out.ConfidenceThreshold)
	}
	if passed, ok := parseConsensusBool(consensus["passed"]); ok {
		out.Passed = boolPtr(passed)
	} else if out.ScorePassed != nil && out.ConfidencePassed != nil {
		out.Passed = boolPtr(*out.ScorePassed && *out.ConfidencePassed)
	}
	return out
}

func parseConsensusBool(value any) (bool, bool) {
	switch raw := value.(type) {
	case bool:
		return raw, true
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(raw))
		if trimmed == "true" {
			return true, true
		}
		if trimmed == "false" {
			return false, true
		}
		return false, false
	case float64:
		return raw != 0, true
	case float32:
		return raw != 0, true
	case int:
		return raw != 0, true
	case int64:
		return raw != 0, true
	case uint64:
		return raw != 0, true
	default:
		return false, false
	}
}

func boolPtr(value bool) *bool {
	out := value
	return &out
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
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
