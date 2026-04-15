package decisionflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/pkg/parseutil"
	"brale-core/internal/position"
	"brale-core/internal/readmodel/dashboard"
	"brale-core/internal/store"
)

const GateScanLimit = 200

var ErrSnapshotNotFound = errors.New("snapshot not found")

type FlowStore interface {
	store.TimelineQueryStore
	store.PositionQueryStore
	store.RiskPlanQueryStore
}

type HistoryStore interface {
	store.TimelineQueryStore
	store.PositionQueryStore
}

func BuildFlow(ctx context.Context, st FlowStore, symbol string, snapshotID uint, hasSnapshot bool, cfg SymbolConfig) (FlowResult, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	gates := []store.GateEventRecord{}
	var err error
	if !hasSnapshot {
		gates, err = st.ListGateEvents(ctx, symbol, GateScanLimit)
		if err != nil {
			return FlowResult{}, err
		}
	}
	pos, isOpen, err := st.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil {
		return FlowResult{}, err
	}

	var selection dashboard.FlowSelection
	var gate *store.GateEventRecord
	if hasSnapshot {
		selected, ok, findErr := st.FindGateEventBySnapshot(ctx, symbol, snapshotID)
		if findErr != nil {
			return FlowResult{}, findErr
		}
		if !ok {
			return FlowResult{}, ErrSnapshotNotFound
		}
		selection = dashboard.FlowSelection{
			Anchor: dashboard.FlowAnchor{Type: "selected_round", SnapshotID: snapshotID, Confidence: "high", Reason: "selected_by_snapshot_id"},
		}
		gate = &selected
	} else {
		var ok bool
		selection, gate, ok = dashboard.SelectFlowSelection(snapshotID, false, pos, isOpen, gates)
		if !ok {
			return FlowResult{}, ErrSnapshotNotFound
		}
	}

	var providers []store.ProviderEventRecord
	var agents []store.AgentEventRecord
	if gate != nil && gate.SnapshotID > 0 {
		providers, err = st.ListProviderEventsBySnapshot(ctx, symbol, gate.SnapshotID)
		if err != nil {
			return FlowResult{}, err
		}
		agents, err = st.ListAgentEventsBySnapshot(ctx, symbol, gate.SnapshotID)
		if err != nil {
			return FlowResult{}, err
		}
	}

	tighten := dashboard.ResolveTightenInfo(gate)
	if tighten == nil && isOpen && !hasSnapshot {
		tighten = dashboard.ResolveTightenFromRiskHistory(ctx, st, pos, 6)
	}
	stages := dashboard.AssembleFlowStageSet(providers, agents, dashboard.ShouldPreferInPositionProvider(isOpen, gate), cfg.AgentModels)
	return FlowResult{
		Anchor:    selection.Anchor,
		Nodes:     dashboard.BuildFlowNodes(stages, gate, tighten),
		Intervals: append([]string(nil), cfg.Intervals...),
		Trace:     dashboard.BuildFlowTrace(stages, gate, strings.TrimSpace(pos.Side), isOpen),
		Tighten:   tighten,
	}, nil
}

func MapHistoryItems(gates []store.GateEventRecord) []HistoryItem {
	if len(gates) == 0 {
		return []HistoryItem{}
	}
	out := make([]HistoryItem, 0, len(gates))
	for _, gate := range gates {
		consensus := dashboard.ExtractConsensusMetrics(json.RawMessage(gate.DerivedJSON))
		tighten := dashboard.BuildDecisionTightenDetail(json.RawMessage(gate.DerivedJSON))
		out = append(out, HistoryItem{
			SnapshotID:          gate.SnapshotID,
			Action:              strings.ToUpper(strings.TrimSpace(gate.DecisionAction)),
			Reason:              DisplayReason(strings.ToUpper(strings.TrimSpace(gate.DecisionAction)), strings.TrimSpace(gate.GateReason), tighten),
			At:                  formatHistoryAt(gate),
			ConsensusScore:      consensus.Score,
			ConsensusConfidence: consensus.Confidence,
		})
	}
	return out
}

func formatHistoryAt(gate store.GateEventRecord) string {
	if !gate.CreatedAt.IsZero() {
		return gate.CreatedAt.UTC().Format(time.RFC3339)
	}
	if gate.Timestamp > 0 {
		return time.Unix(gate.Timestamp, 0).UTC().Format(time.RFC3339)
	}
	return ""
}

func BuildDetail(ctx context.Context, st HistoryStore, cfg SymbolConfig, symbol string, snapshotID uint) (*Detail, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	selected, ok, err := st.FindGateEventBySnapshot(ctx, symbol, snapshotID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrSnapshotNotFound
	}
	providers, err := st.ListProviderEventsBySnapshot(ctx, symbol, snapshotID)
	if err != nil {
		return nil, err
	}
	agents, err := st.ListAgentEventsBySnapshot(ctx, symbol, snapshotID)
	if err != nil {
		return nil, err
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
		return nil, reportErr
	}

	providerSummaries := make([]string, 0, len(report.Providers))
	for _, stage := range report.Providers {
		providerSummaries = append(providerSummaries, strings.TrimSpace(stage.Role+": "+stage.Summary))
	}
	agentSummaries := make([]string, 0, len(report.Agents))
	for _, stage := range report.Agents {
		agentSummaries = append(agentSummaries, strings.TrimSpace(stage.Role+": "+stage.Summary))
	}

	consensus := dashboard.ExtractConsensusMetrics(json.RawMessage(selected.DerivedJSON))
	tighten := dashboard.BuildDecisionTightenDetail(json.RawMessage(selected.DerivedJSON))
	planSource := normalizePlanSource(resolvePlanSource(selected, tighten))

	detail := &Detail{
		SnapshotID:                   selected.SnapshotID,
		Action:                       strings.ToUpper(strings.TrimSpace(selected.DecisionAction)),
		Reason:                       DisplayReason(strings.ToUpper(strings.TrimSpace(selected.DecisionAction)), strings.TrimSpace(selected.GateReason), tighten),
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
		PlanContext:                  BuildPlanContext(cfg, planSource),
		Plan:                         BuildPlanSummary(ctx, st, symbol),
		Sieve:                        BuildSieveDetail(json.RawMessage(selected.DerivedJSON), cfg),
		ReportMarkdown:               prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionMarkdown(report)),
		DecisionViewURL:              fmt.Sprintf("/decision-view/?symbol=%s&snapshot_id=%d", symbol, snapshotID),
	}
	applyDerivedDecisionMetrics(detail, json.RawMessage(selected.DerivedJSON))
	return detail, nil
}

func applyDerivedDecisionMetrics(detail *Detail, raw json.RawMessage) {
	if detail == nil || len(raw) == 0 {
		return
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return
	}
	if sq, ok := parseutil.FloatOK(derived["setup_quality"]); ok {
		detail.SetupQuality = &sq
	}
	if rp, ok := parseutil.FloatOK(derived["risk_penalty"]); ok {
		detail.RiskPenalty = &rp
	}
	if ee, ok := parseutil.FloatOK(derived["entry_edge"]); ok {
		detail.EntryEdge = &ee
	}
	detail.GateCategory = strings.TrimSpace(fmt.Sprint(derived["gate_reason_category"]))
}

func BuildPlanContext(cfg SymbolConfig, planSource string) *PlanContext {
	return &PlanContext{
		RiskPerTradePct: cfg.RiskManagement.RiskPerTradePct,
		MaxInvestPct:    cfg.RiskManagement.MaxInvestPct,
		MaxLeverage:     cfg.RiskManagement.MaxLeverage,
		EntryOffsetATR:  cfg.RiskManagement.EntryOffsetATR,
		EntryMode:       strings.TrimSpace(cfg.RiskManagement.EntryMode),
		PlanSource:      normalizePlanSource(planSource),
		InitialExit:     strings.TrimSpace(cfg.RiskManagement.InitialExit),
	}
}

func BuildPlanSummary(ctx context.Context, st store.PositionQueryStore, symbol string) *PlanSummary {
	if st == nil {
		return nil
	}
	pos, ok, err := st.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil || !ok {
		return nil
	}
	plan := &PlanSummary{
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
	plan.TakeProfitLevels = make([]PlanTPLevel, 0, len(decoded.TPLevels))
	plan.TakeProfits = make([]float64, 0, len(decoded.TPLevels))
	for _, level := range decoded.TPLevels {
		plan.TakeProfits = append(plan.TakeProfits, level.Price)
		plan.TakeProfitLevels = append(plan.TakeProfitLevels, PlanTPLevel{
			LevelID: level.LevelID,
			Price:   level.Price,
			QtyPct:  level.QtyPct,
			Hit:     level.Hit,
		})
	}
	return plan
}

func BuildSieveDetail(raw json.RawMessage, cfg SymbolConfig) *SieveDetail {
	if len(raw) == 0 {
		return nil
	}
	var derived map[string]any
	if err := json.Unmarshal(raw, &derived); err != nil {
		return nil
	}
	if !hasSieveSignal(derived) {
		return nil
	}
	sieveCfg := cfg.RiskManagement.Sieve
	detail := &SieveDetail{
		Action:            strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["sieve_action"]))),
		ReasonCode:        strings.TrimSpace(fmt.Sprint(derived["sieve_reason"])),
		Hit:               parseDecisionBool(derived["sieve_hit"]),
		SizeFactor:        parseDecisionFloat(derived["sieve_size_factor"]),
		MinSizeFactor:     firstPositive(parseDecisionFloat(derived["sieve_min_size_factor"]), sieveCfg.MinSizeFactor),
		DefaultAction:     firstNonEmpty(strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["sieve_default_action"]))), strings.ToUpper(strings.TrimSpace(sieveCfg.DefaultGateAction))),
		DefaultSizeFactor: firstPositive(parseDecisionFloat(derived["sieve_default_size_factor"]), sieveCfg.DefaultSizeFactor),
		ActionBefore:      strings.ToUpper(strings.TrimSpace(fmt.Sprint(derived["gate_action_before_sieve"]))),
		PolicyHash:        firstNonEmpty(strings.TrimSpace(fmt.Sprint(derived["sieve_policy_hash"])), cfg.StrategyHash),
	}
	if detail.Action == "" && detail.ReasonCode == "" && !detail.Hit {
		return nil
	}
	rows := make([]SieveRow, 0, len(sieveCfg.Rows))
	for _, row := range sieveCfg.Rows {
		matched := detail.ReasonCode != "" && strings.EqualFold(strings.TrimSpace(row.ReasonCode), detail.ReasonCode)
		if !matched {
			continue
		}
		rows = append(rows, SieveRow{
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

func DisplayReason(action string, fallback string, tighten *dashboard.DecisionTightenDetail) string {
	if strings.EqualFold(strings.TrimSpace(action), "TIGHTEN") && tighten != nil && strings.TrimSpace(tighten.DisplayReason) != "" {
		return strings.TrimSpace(tighten.DisplayReason)
	}
	return strings.TrimSpace(fallback)
}

func prependDecisionHeader(title, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return strings.TrimSpace(title)
	}
	return strings.TrimSpace(title) + "\n\n" + body
}

func resolvePlanSource(gate store.GateEventRecord, tighten *dashboard.DecisionTightenDetail) string {
	var rmTighten *dashboard.TightenInfo
	if tighten != nil {
		rmTighten = &dashboard.TightenInfo{Triggered: tighten.Executed, Reason: tighten.DisplayReason}
	}
	nodes := dashboard.BuildFlowNodes(dashboard.FlowStageSet{}, &gate, rmTighten)
	if len(nodes) == 0 {
		return ""
	}
	idx := len(nodes) - 1
	if len(nodes) >= 2 && nodes[len(nodes)-2].Stage == "plan" {
		idx = len(nodes) - 2
	}
	for _, field := range nodes[idx].Values {
		if field.Key == "plan_source" {
			return field.Value
		}
	}
	return ""
}

func normalizePlanSource(raw string) string {
	switch raw {
	case "llm", "go":
		return raw
	default:
		return ""
	}
}

func hasSieveSignal(derived map[string]any) bool {
	if len(derived) == 0 {
		return false
	}
	for _, key := range []string{"sieve_action", "sieve_reason", "sieve_hit", "sieve_size_factor", "gate_action_before_sieve"} {
		if _, ok := derived[key]; ok {
			return true
		}
	}
	return false
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
