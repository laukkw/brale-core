package runtimeapi

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/execution"
	readmodel "brale-core/internal/readmodel/dashboard"
	portfoliorm "brale-core/internal/readmodel/portfolio"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

type portfolioUsecase struct {
	execClient  runtimeExecClient
	store       portfolioStore
	allowSymbol func(symbol string) bool
}

type portfolioStore interface {
	store.PositionQueryStore
	store.RiskPlanQueryStore
}

const (
	dashboardRiskPlanTimelineLimit = 6

	dashboardPnLRealizedSourceRealizedProfit = readmodel.PNLRealizedSourceRealizedProfit
	dashboardPnLRealizedSourceCloseProfitAbs = readmodel.PNLRealizedSourceCloseProfitAbs
	dashboardPnLUnrealizedSourceProfitAbs    = readmodel.PNLUnrealizedSourceProfitAbs
	dashboardPnLTotalSourceTotalProfitAbs    = readmodel.PNLTotalSourceTotalProfitAbs
	dashboardPnLTotalSourceComponents        = readmodel.PNLTotalSourceComponents
	dashboardPnLDriftThreshold               = readmodel.PNLDriftThreshold
)

type dashboardPnLProvenance struct {
	RealizedSource   string
	UnrealizedSource string
	TotalSource      string
}

func newPortfolioUsecase(s *Server) portfolioUsecase {
	if s == nil {
		return portfolioUsecase{}
	}
	return portfolioUsecase{execClient: s.ExecClient, store: s.Store, allowSymbol: s.AllowSymbol}
}

func (u portfolioUsecase) balanceUSDT(ctx context.Context) float64 {
	if u.execClient == nil {
		return 0
	}
	quote, err := u.execClient.Balance(ctx)
	if err != nil {
		return 0
	}
	return execution.BalanceEquity(quote)
}

func (u portfolioUsecase) buildObserveAccountState(ctx context.Context) (execution.AccountState, error) {
	if u.execClient == nil {
		return execution.AccountState{}, fmt.Errorf("exec client missing")
	}
	quote, err := u.execClient.Balance(ctx)
	if err != nil {
		return execution.AccountState{}, err
	}
	return execution.AccountStateFromBalance(quote)
}

func (u portfolioUsecase) buildPositionStatus(ctx context.Context) ([]PositionStatusItem, error) {
	if u.execClient == nil {
		return nil, fmt.Errorf("exec client missing")
	}
	items, err := portfoliorm.BuildPositionStatus(ctx, u.execClient, u.store, u.allowSymbol, dashboardRiskPlanTimelineLimit)
	if err != nil {
		return nil, err
	}
	return mapPositionStatusItems(items), nil
}

func resolveDashboardPnLFromTrade(tr execution.Trade) (DashboardPnLCard, dashboardPnLProvenance) {
	card, provenance := readmodel.ResolvePnLFromTrade(tr)
	return DashboardPnLCard{Realized: card.Realized, Unrealized: card.Unrealized, Total: card.Total}, dashboardPnLProvenance{
		RealizedSource:   provenance.RealizedSource,
		UnrealizedSource: provenance.UnrealizedSource,
		TotalSource:      provenance.TotalSource,
	}
}

func reconcileDashboardPnL(pnl DashboardPnLCard) DashboardReconciliation {
	rc := readmodel.ReconcilePnL(readmodel.PnLCard{Realized: pnl.Realized, Unrealized: pnl.Unrealized, Total: pnl.Total})
	return DashboardReconciliation{Status: rc.Status, DriftAbs: rc.DriftAbs, DriftPct: rc.DriftPct, DriftThreshold: rc.DriftThreshold}
}

func resolveDashboardLeverage(tr execution.Trade) float64 {
	return readmodel.ResolveLeverage(tr)
}

func (u portfolioUsecase) buildDashboardAccountPnL(ctx context.Context) (DashboardPnLCard, bool) {
	if u.execClient == nil {
		return DashboardPnLCard{}, false
	}
	card, ok := portfoliorm.BuildAccountPnL(ctx, u.execClient, u.allowSymbol)
	if !ok {
		return DashboardPnLCard{}, false
	}
	return DashboardPnLCard{Realized: card.Realized, Unrealized: card.Unrealized, Total: card.Total}, true
}

func extractDashboardAccountTotalProfit(quote map[string]any) (float64, bool) {
	return readmodel.ExtractAccountTotalProfit(quote)
}

func (u portfolioUsecase) buildDashboardOverview(ctx context.Context, rawSymbol string) (string, []DashboardOverviewSymbol, *usecaseError) {
	trimmedRaw := strings.TrimSpace(rawSymbol)
	normalizedSymbol := runtime.NormalizeSymbol(trimmedRaw)
	if trimmedRaw != "" && (normalizedSymbol == "" || !isValidDashboardSymbol(normalizedSymbol)) {
		return "", nil, &usecaseError{Status: 400, Code: "invalid_symbol", Message: "symbol 非法", Details: rawSymbol}
	}
	if normalizedSymbol != "" && u.allowSymbol != nil && !u.allowSymbol(normalizedSymbol) {
		return "", nil, &usecaseError{Status: 400, Code: "symbol_not_allowed", Message: "symbol 不在允许列表", Details: normalizedSymbol}
	}
	if u.execClient == nil {
		return "", nil, &usecaseError{Status: 502, Code: "dashboard_overview_failed", Message: "dashboard 概览获取失败", Details: "exec client missing"}
	}
	normalizedSymbol, cards, err := portfoliorm.BuildOverview(ctx, u.execClient, u.store, rawSymbol, u.allowSymbol, dashboardRiskPlanTimelineLimit)
	if err != nil {
		return "", nil, &usecaseError{Status: 502, Code: "dashboard_overview_failed", Message: "dashboard 概览获取失败", Details: err.Error()}
	}
	return normalizedSymbol, mapDashboardOverviewSymbols(cards), nil
}

func isValidDashboardSymbol(symbol string) bool {
	if symbol == "" {
		return false
	}
	for _, r := range symbol {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (u portfolioUsecase) buildTradeHistory(ctx context.Context, limit, offset int, symbolFilter string) ([]TradeHistoryItem, error) {
	if u.execClient == nil {
		return nil, fmt.Errorf("exec client missing")
	}
	items, err := portfoliorm.BuildTradeHistory(ctx, u.execClient, u.store, limit, offset, symbolFilter, u.allowSymbol, dashboardRiskPlanTimelineLimit)
	if err != nil {
		return nil, err
	}
	return mapTradeHistoryItems(items), nil
}
