package runtimeapi

import (
	"context"
	"sort"
	"strings"

	"brale-core/internal/execution"
)

type dashboardAccountUsecase struct {
	execClient runtimeExecClient
	bundles    map[string]ConfigBundle
}

func newDashboardAccountUsecase(s *Server) dashboardAccountUsecase {
	if s == nil {
		return dashboardAccountUsecase{}
	}
	return dashboardAccountUsecase{execClient: s.ExecClient, bundles: s.SymbolConfigs}
}

func (u dashboardAccountUsecase) build(ctx context.Context) (DashboardAccountSummaryResponse, *usecaseError) {
	if u.execClient == nil {
		return DashboardAccountSummaryResponse{}, &usecaseError{Status: 502, Code: "dashboard_account_failed", Message: "dashboard 账户摘要获取失败", Details: "exec client missing"}
	}
	quote, err := u.execClient.Balance(ctx)
	if err != nil {
		return DashboardAccountSummaryResponse{}, &usecaseError{Status: 502, Code: "dashboard_account_failed", Message: "dashboard 账户摘要获取失败", Details: err.Error()}
	}
	profitAll, _ := u.execClient.ProfitAll(ctx)
	balance := buildDashboardBalanceCard(quote, u.bundles)
	profit := buildDashboardProfitCard(profitAll)
	return DashboardAccountSummaryResponse{
		Status:  "ok",
		Balance: balance,
		Profit:  profit,
		Summary: dashboardContractSummary,
	}, nil
}

func buildDashboardBalanceCard(quote map[string]any, bundles map[string]ConfigBundle) DashboardBalanceCard {
	currency := execution.ResolveStakeCurrency(quote)
	total, _ := execution.ExtractUSDTBalance(quote)
	available, ok := execution.ExtractUSDTAvailable(quote)
	if !ok {
		available = total
	}
	used, ok := execution.ExtractUSDTUsed(quote)
	if !ok {
		used = total - available
	}
	if used < 0 {
		used = 0
	}
	monitored := make([]string, 0, len(bundles))
	for symbol := range bundles {
		trimmed := strings.ToUpper(strings.TrimSpace(symbol))
		if trimmed == "" {
			continue
		}
		monitored = append(monitored, trimmed)
	}
	sort.Strings(monitored)
	return DashboardBalanceCard{
		Currency:       currency,
		Total:          total,
		Available:      available,
		Used:           used,
		Monitored:      monitored,
		MonitoredCount: len(monitored),
	}
}

func buildDashboardProfitCard(payload map[string]any) DashboardProfitAllCard {
	if payload == nil {
		return DashboardProfitAllCard{}
	}
	all, ok := payload["all"].(map[string]any)
	if !ok {
		return DashboardProfitAllCard{}
	}
	closedProfit, _ := execution.AsFloat(all["profit_closed_coin"])
	allProfit, _ := execution.AsFloat(all["profit_all_coin"])
	tradeCount, _ := execution.AsFloat(all["trade_count"])
	closedTradeCount, _ := execution.AsFloat(all["closed_trade_count"])
	return DashboardProfitAllCard{
		ClosedProfit:     closedProfit,
		AllProfit:        allProfit,
		TradeCount:       int(tradeCount),
		ClosedTradeCount: int(closedTradeCount),
	}
}
