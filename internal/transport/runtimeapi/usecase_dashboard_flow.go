package runtimeapi

import (
	"context"
	"strings"

	readmodel "brale-core/internal/readmodel/decisionflow"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

type dashboardFlowUsecase struct {
	resolver    SymbolResolver
	store       dashboardFlowStore
	allowSymbol func(string) bool
	symbolCfgs  map[string]ConfigBundle
}

type dashboardFlowStore interface {
	store.TimelineQueryStore
	store.PositionQueryStore
	store.RiskPlanQueryStore
}

func newDashboardFlowUsecase(s *Server) dashboardFlowUsecase {
	if s == nil {
		return dashboardFlowUsecase{}
	}
	return dashboardFlowUsecase{
		resolver:    s.Resolver,
		store:       s.Store,
		allowSymbol: s.AllowSymbol,
		symbolCfgs:  s.SymbolConfigs,
	}
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
	result, err := readmodel.BuildFlow(ctx, u.store, normalizedSymbol, selectedSnapshotID, hasSelectedSnapshot, u.resolveSymbolConfig(normalizedSymbol))
	if err != nil {
		if err == readmodel.ErrSnapshotNotFound {
			return DashboardDecisionFlowResponse{}, &usecaseError{Status: 404, Code: "snapshot_not_found", Message: "snapshot_id 对应决策不存在", Details: selectedSnapshotID}
		}
		return DashboardDecisionFlowResponse{}, &usecaseError{Status: 500, Code: "decision_flow_failed", Message: "decision flow 读取失败", Details: err.Error()}
	}

	return DashboardDecisionFlowResponse{
		Status: "ok",
		Symbol: normalizedSymbol,
		Flow: DashboardDecisionFlow{
			Anchor:    DashboardFlowAnchor(result.Anchor),
			Nodes:     mapDashboardFlowNodes(result.Nodes),
			Intervals: append([]string(nil), result.Intervals...),
			Trace:     mapDashboardFlowTrace(result.Trace),
			Tighten:   mapDashboardTightenInfo(result.Tighten),
		},
		Summary: dashboardContractSummary,
	}, nil
}

func (u dashboardFlowUsecase) resolveAgentStageModels(symbol string) map[string]string {
	if len(u.symbolCfgs) == 0 {
		return nil
	}
	bundle, ok := u.symbolCfgs[symbol]
	if !ok {
		return nil
	}
	models := map[string]string{
		"indicator": strings.TrimSpace(bundle.Symbol.LLM.Agent.Indicator.Model),
		"structure": strings.TrimSpace(bundle.Symbol.LLM.Agent.Structure.Model),
		"mechanics": strings.TrimSpace(bundle.Symbol.LLM.Agent.Mechanics.Model),
	}
	for stage, model := range models {
		if model == "" {
			delete(models, stage)
		}
	}
	if len(models) == 0 {
		return nil
	}
	return models
}

func (u dashboardFlowUsecase) resolveSymbolConfig(symbol string) readmodel.SymbolConfig {
	cfg := readmodel.SymbolConfig{AgentModels: u.resolveAgentStageModels(symbol)}
	if u.resolver != nil {
		resolved, err := u.resolver.Resolve(symbol)
		if err == nil {
			cfg.Intervals = normalizedIntervals(resolved.Intervals)
		}
	}
	if bundle, ok := u.symbolCfgs[symbol]; ok {
		cfg.StrategyHash = bundle.Strategy.Hash
		cfg.RiskManagement = readmodel.RiskManagementConfig{
			RiskPerTradePct: bundle.Strategy.RiskManagement.RiskPerTradePct,
			MaxInvestPct:    bundle.Strategy.RiskManagement.MaxInvestPct,
			MaxLeverage:     bundle.Strategy.RiskManagement.MaxLeverage,
			EntryOffsetATR:  bundle.Strategy.RiskManagement.EntryOffsetATR,
			EntryMode:       bundle.Strategy.RiskManagement.EntryMode,
			InitialExit:     bundle.Strategy.RiskManagement.InitialExit.Policy,
			Sieve:           bundle.Strategy.RiskManagement.Sieve,
		}
	}
	return cfg
}
