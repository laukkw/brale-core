package runtimeapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/asyncjob"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

const (
	observeReportStatusEmpty = "empty"
	observeHistoryScanLimit  = 50
)

type observeUsecase struct {
	server *Server
}

func newObserveUsecase(s *Server) observeUsecase {
	return observeUsecase{server: s}
}

func (u observeUsecase) submit(ctx context.Context, symbol string) (observeResponse, *usecaseError) {
	if u.server == nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	logger := logging.FromContext(ctx).Named("runtime-api")
	resolved, err := u.server.Resolver.Resolve(symbol)
	if err != nil {
		logger.Error("resolve symbol failed", zap.Error(err), zap.String("symbol", symbol))
		return observeResponse{}, &usecaseError{Status: 400, Code: "symbol_invalid", Message: "symbol 配置不可用", Details: err.Error()}
	}
	if resolved.Pipeline == nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "pipeline_missing", Message: "运行链路不可用"}
	}
	key, err := buildObserveJobKey(resolved)
	if err != nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "job_key_failed", Message: "任务 Key 生成失败", Details: err.Error()}
	}
	requestID := key
	jobCtx := context.WithoutCancel(ctx)
	snap, _, err := u.server.ObserveJobs.Enqueue(jobCtx, key, func(jobCtx context.Context, _ string) (observeResponse, error) {
		return u.runObserve(jobCtx, resolved, requestID)
	})
	if err != nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "observe_enqueue_failed", Message: "观察任务提交失败", Details: err.Error()}
	}
	if snap.Status == asyncjob.StatusDone {
		resp := snap.Value
		resp.RequestID = requestID
		resp.TraceID = requestIDFromContext(ctx)
		return resp, nil
	}
	if snap.Status == asyncjob.StatusFailed {
		errText := "观察执行失败"
		if snap.Err != nil {
			errText = snap.Err.Error()
		}
		return observeResponse{}, &usecaseError{Status: 500, Code: "observe_failed", Message: "观察执行失败", Details: errText}
	}
	return observeResponse{
		Symbol:      resolved.Symbol,
		Status:      string(snap.Status),
		Summary:     "观察任务已提交",
		RequestID:   requestID,
		SkippedExec: true,
		TraceID:     requestIDFromContext(ctx),
	}, nil
}

func buildObserveJobKey(resolved ResolvedSymbol) (string, error) {
	payload := observeJobKey{Symbol: resolved.Symbol, Intervals: resolved.Intervals, KlineLimit: resolved.KlineLimit}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:]), nil
}

func (u observeUsecase) runObserve(ctx context.Context, resolved ResolvedSymbol, requestID string) (observeResponse, error) {
	logger := logging.FromContext(ctx).Named("runtime-api")
	if u.server != nil && u.server.LiquidationStream != nil {
		if err := u.server.LiquidationStream.Start(context.WithoutCancel(ctx)); err != nil {
			logger.Warn("liquidation stream start failed for observe", zap.Error(err), zap.String("symbol", resolved.Symbol))
		}
	}
	ctx = llm.WithSessionRequestID(ctx, requestID)
	acct := u.buildObserveAccountState(ctx, resolved.Symbol, logger)
	risk := execution.RiskParams{RiskPerTradePct: resolved.RiskPercent}
	results, err := resolved.Pipeline.RunOnceObserveAsFlat(ctx, []string{resolved.Symbol}, resolved.Intervals, resolved.KlineLimit, acct, risk)
	if err != nil {
		logger.Error("observe run failed", zap.Error(err), zap.String("symbol", resolved.Symbol))
		return observeResponse{}, err
	}
	res, ok := findSymbolResult(results, resolved.Symbol)
	if !ok {
		return observeResponse{}, fmt.Errorf("observe result missing")
	}
	resp := buildObserveResponse(res, requestID, requestIDFromContext(ctx))
	u.storeLast(resolved.Symbol, resp)
	return resp, nil
}

func (u observeUsecase) buildObserveAccountState(ctx context.Context, symbol string, logger *zap.Logger) execution.AccountState {
	acct := execution.AccountState{}
	if u.server == nil || u.server.ExecClient == nil {
		return acct
	}
	acct, err := newPortfolioUsecase(u.server).buildObserveAccountState(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("observe account fetch failed", zap.Error(err), zap.String("symbol", symbol))
		}
		return acct
	}
	return acct
}

func (u observeUsecase) storeLast(symbol string, resp observeResponse) {
	if u.server == nil {
		return
	}
	u.server.lastMu.Lock()
	defer u.server.lastMu.Unlock()
	if u.server.lastRun == nil {
		u.server.lastRun = make(map[string]lastObserve)
	}
	u.server.lastRun[symbol] = lastObserve{resp: resp, at: time.Now()}
}

func (u observeUsecase) report(ctx context.Context, symbol string) (observeResponse, *usecaseError) {
	if u.server == nil {
		return observeResponse{}, &usecaseError{Status: 500, Code: "server_missing", Message: "运行服务未初始化"}
	}
	u.server.lastMu.RLock()
	if u.server.lastRun != nil {
		if rec, ok := u.server.lastRun[symbol]; ok {
			u.server.lastMu.RUnlock()
			resp := rec.resp
			resp.TraceID = requestIDFromContext(ctx)
			return resp, nil
		}
	}
	u.server.lastMu.RUnlock()

	resp, ok, ucErr := u.reportFromHistory(ctx, symbol)
	if ucErr != nil || ok {
		return resp, ucErr
	}
	return emptyObserveReport(symbol, requestIDFromContext(ctx)), nil
}

func emptyObserveReport(symbol, requestID string) observeResponse {
	return observeResponse{
		Symbol:      symbol,
		Status:      observeReportStatusEmpty,
		Summary:     "查询不存在",
		RequestID:   requestID,
		SkippedExec: true,
	}
}

func (u observeUsecase) reportFromHistory(ctx context.Context, symbol string) (observeResponse, bool, *usecaseError) {
	if u.server == nil || u.server.Store == nil {
		return observeResponse{}, false, nil
	}
	rounds, err := listObserveRounds(ctx, u.server.Store, symbol, observeHistoryScanLimit)
	if err != nil {
		return observeResponse{}, false, &usecaseError{Status: 500, Code: "observe_rounds_failed", Message: "observe 历史读取失败", Details: err.Error()}
	}
	for _, round := range rounds {
		if !isUsableObserveRound(round) {
			continue
		}
		resp, ok, ucErr := u.reportFromObserveRound(ctx, symbol, round)
		if ucErr != nil || ok {
			return resp, ok, ucErr
		}
	}
	return observeResponse{}, false, nil
}

type llmRoundTypeLister interface {
	ListLLMRoundsByType(ctx context.Context, symbol string, roundType string, limit int) ([]store.LLMRoundRecord, error)
}

func listObserveRounds(ctx context.Context, st store.Store, symbol string, limit int) ([]store.LLMRoundRecord, error) {
	if typed, ok := st.(llmRoundTypeLister); ok {
		return typed.ListLLMRoundsByType(ctx, symbol, "observe", limit)
	}
	return st.ListLLMRounds(ctx, symbol, limit)
}

func isUsableObserveRound(round store.LLMRoundRecord) bool {
	if !strings.EqualFold(strings.TrimSpace(round.RoundType), "observe") || round.SnapshotID == 0 {
		return false
	}
	if strings.TrimSpace(round.Error) != "" {
		return false
	}
	outcome := strings.TrimSpace(round.Outcome)
	return outcome == "" || strings.EqualFold(outcome, "ok")
}

func (u observeUsecase) reportFromObserveRound(ctx context.Context, symbol string, round store.LLMRoundRecord) (observeResponse, bool, *usecaseError) {
	gate, ok, err := u.server.Store.FindGateEventBySnapshot(ctx, symbol, round.SnapshotID)
	if err != nil {
		return observeResponse{}, false, &usecaseError{Status: 500, Code: "gate_event_failed", Message: "gate 事件读取失败", Details: err.Error()}
	}
	if !ok {
		return observeResponse{}, false, nil
	}
	providers, err := u.server.Store.ListProviderEventsBySnapshot(ctx, symbol, gate.SnapshotID)
	if err != nil {
		return observeResponse{}, false, &usecaseError{Status: 500, Code: "provider_events_failed", Message: "provider 事件读取失败", Details: err.Error()}
	}
	agents, err := u.server.Store.ListAgentEventsBySnapshot(ctx, symbol, gate.SnapshotID)
	if err != nil {
		return observeResponse{}, false, &usecaseError{Status: 500, Code: "agent_events_failed", Message: "agent 事件读取失败", Details: err.Error()}
	}
	input := decisionInputFromHistory(symbol, gate, providers, agents)
	formatter := decisionfmt.New()
	report, err := formatter.BuildDecisionReport(input)
	if err != nil {
		return observeResponse{}, false, &usecaseError{Status: 500, Code: "observe_report_build_failed", Message: "observe 报告解析失败", Details: err.Error()}
	}
	markdown := prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionMarkdown(report))
	html := prependDecisionHeader("🚦 决策报告", formatter.RenderDecisionHTML(report))
	resp := observeResponse{
		Symbol:         symbol,
		Status:         "ok",
		Agent:          observeAgentPayloadFromHistory(agents),
		Provider:       observeProviderPayloadFromHistory(providers),
		Gate:           observeGatePayloadFromReport(report),
		Report:         markdown,
		ReportMarkdown: markdown,
		ReportHTML:     html,
		Summary:        buildDecisionLatestSummary(report),
		RequestID:      firstNonEmpty(round.RequestID, round.ID, gate.RoundID),
		SkippedExec:    true,
		TraceID:        requestIDFromContext(ctx),
	}
	return resp, true, nil
}

func decisionInputFromHistory(symbol string, gate store.GateEventRecord, providers []store.ProviderEventRecord, agents []store.AgentEventRecord) decisionfmt.DecisionInput {
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
	return input
}

func observeAgentPayloadFromHistory(records []store.AgentEventRecord) ObserveAgentPayload {
	var out ObserveAgentPayload
	for _, rec := range records {
		payload := decodeHistoryPayload(rec.OutputJSON)
		switch strings.TrimSpace(rec.Stage) {
		case "indicator":
			out.Indicator = payload
		case "structure":
			out.Structure = payload
		case "mechanics":
			out.Mechanics = payload
		}
	}
	return out
}

func observeProviderPayloadFromHistory(records []store.ProviderEventRecord) *ObserveProviderPayload {
	var out ObserveProviderPayload
	for _, rec := range records {
		payload := decodeHistoryPayload(rec.OutputJSON)
		switch strings.TrimSpace(rec.Role) {
		case "indicator":
			out.Indicator = payload
		case "structure":
			out.Structure = payload
		case "mechanics":
			out.Mechanics = payload
		}
	}
	if out.Indicator == nil && out.Structure == nil && out.Mechanics == nil {
		return nil
	}
	return &out
}

func decodeHistoryPayload(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}

func observeGatePayloadFromReport(report decisionfmt.DecisionReport) ObserveGatePayload {
	gate := report.Gate.Overall
	return ObserveGatePayload{
		Tradeable:      gate.Tradeable,
		DecisionAction: gate.DecisionAction,
		DecisionText:   gate.DecisionText,
		Grade:          gate.Grade,
		Reason:         gate.Reason,
		ReasonCode:     gate.ReasonCode,
		Direction:      gate.Direction,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func findSymbolResult(results []ObserveSymbolResult, symbol string) (ObserveSymbolResult, bool) {
	for _, item := range results {
		if strings.EqualFold(item.Symbol, symbol) {
			return item, true
		}
	}
	return ObserveSymbolResult{}, false
}
