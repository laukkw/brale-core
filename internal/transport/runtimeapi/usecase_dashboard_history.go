package runtimeapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	readmodel "brale-core/internal/readmodel/decisionflow"
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

	items := mapDashboardDecisionHistoryItems(readmodel.MapHistoryItems(gates))
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
		detail, detailErr := readmodel.BuildDetail(ctx, u.store, u.resolveSymbolConfig(normalizedSymbol), normalizedSymbol, detailSnapshotID)
		if detailErr != nil {
			if detailErr == readmodel.ErrSnapshotNotFound {
				return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 404, Code: "snapshot_not_found", Message: "snapshot_id 对应决策不存在", Details: detailSnapshotID}
			}
			return DashboardDecisionHistoryResponse{}, &usecaseError{Status: 500, Code: "decision_build_failed", Message: "决策详情解析失败", Details: detailErr.Error()}
		}
		response.Detail = mapDashboardDecisionDetail(detail)
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

func (u dashboardHistoryUsecase) resolveSymbolConfig(symbol string) readmodel.SymbolConfig {
	cfg := readmodel.SymbolConfig{}
	if bundle, ok := u.configs[symbol]; ok {
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

func buildDecisionDetail(ctx context.Context, st dashboardHistoryStore, configs map[string]ConfigBundle, symbol string, snapshotID uint) (*DashboardDecisionDetail, *usecaseError) {
	if snapshotID == 0 {
		return nil, &usecaseError{Status: 400, Code: "invalid_snapshot_id", Message: "snapshot_id 非法"}
	}
	u := dashboardHistoryUsecase{store: st, configs: configs}
	detail, err := readmodel.BuildDetail(ctx, st, u.resolveSymbolConfig(symbol), symbol, snapshotID)
	if err != nil {
		if err == readmodel.ErrSnapshotNotFound {
			return nil, &usecaseError{Status: 404, Code: "snapshot_not_found", Message: "snapshot_id 对应决策不存在", Details: snapshotID}
		}
		return nil, &usecaseError{Status: 500, Code: "decision_build_failed", Message: "决策详情解析失败", Details: err.Error()}
	}
	return mapDashboardDecisionDetail(detail), nil
}
