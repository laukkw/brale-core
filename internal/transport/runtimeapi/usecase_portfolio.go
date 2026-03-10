package runtimeapi

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"brale-core/internal/execution"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

type portfolioUsecase struct {
	execClient  runtimeExecClient
	store       portfolioStore
	allowSymbol func(symbol string) bool
}

type portfolioStore interface {
	FindPositionBySymbol(ctx context.Context, symbol string, statuses []string) (store.PositionRecord, bool, error)
}

const (
	dashboardPnLRealizedSourceRealizedProfit = "realized_profit"
	dashboardPnLRealizedSourceCloseProfitAbs = "close_profit_abs"
	dashboardPnLUnrealizedSourceProfitAbs    = "profit_abs"
	dashboardPnLTotalSourceTotalProfitAbs    = "total_profit_abs"
	dashboardPnLTotalSourceComponents        = "realized_plus_unrealized"

	dashboardPnLDriftThreshold = 0.01
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
	equity, ok := execution.ExtractUSDTBalance(quote)
	if !ok {
		return 0
	}
	return equity
}

func (u portfolioUsecase) buildObserveAccountState(ctx context.Context) (execution.AccountState, error) {
	if u.execClient == nil {
		return execution.AccountState{}, fmt.Errorf("exec client missing")
	}
	quote, err := u.execClient.Balance(ctx)
	if err != nil {
		return execution.AccountState{}, err
	}
	equity, ok := execution.ExtractUSDTBalance(quote)
	if !ok || equity <= 0 {
		return execution.AccountState{}, fmt.Errorf("balance not available")
	}
	available, ok := execution.ExtractUSDTAvailable(quote)
	if !ok || available <= 0 {
		available = equity
	}
	return execution.AccountState{
		Equity:    equity,
		Available: available,
		Currency:  execution.ResolveStakeCurrency(quote),
	}, nil
}

func (u portfolioUsecase) buildPositionStatus(ctx context.Context) ([]PositionStatusItem, error) {
	if u.execClient == nil {
		return nil, fmt.Errorf("exec client missing")
	}
	trades, err := u.execClient.ListOpenTrades(ctx)
	if err != nil {
		return nil, err
	}
	positions := make([]PositionStatusItem, 0, len(trades))
	for _, tr := range trades {
		symbol := normalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if u.allowSymbol != nil && !u.allowSymbol(symbol) {
			continue
		}
		amount := float64(tr.Amount)
		amountRequested := float64(tr.AmountRequested)
		margin := float64(tr.StakeAmount)
		if margin <= 0 {
			margin = float64(tr.OpenTradeValue)
		}
		entryPrice := float64(tr.OpenRate)
		currentPrice := float64(tr.CurrentRate)
		pnl, _ := resolveDashboardPnLFromTrade(tr)
		openedAt, durationMin, durationSec := positionStatusTiming(int64(tr.OpenFillTimestamp))
		stopLoss, takeProfits, _ := u.lookupRiskLevels(ctx, symbol)
		side := "long"
		if tr.IsShort {
			side = "short"
		}
		positions = append(positions, PositionStatusItem{
			Symbol:           symbol,
			Amount:           amount,
			AmountRequested:  amountRequested,
			MarginAmount:     margin,
			EntryPrice:       entryPrice,
			CurrentPrice:     currentPrice,
			Side:             side,
			ProfitTotal:      pnl.Total,
			ProfitRealized:   pnl.Realized,
			ProfitUnrealized: pnl.Unrealized,
			OpenedAt:         openedAt,
			DurationMin:      durationMin,
			DurationSec:      durationSec,
			TakeProfits:      takeProfits,
			StopLoss:         stopLoss,
		})
	}
	return positions, nil
}

func resolveDashboardPnLFromTrade(tr execution.Trade) (DashboardPnLCard, dashboardPnLProvenance) {
	realized := float64(tr.RealizedProfit)
	realizedSource := dashboardPnLRealizedSourceRealizedProfit
	if realized == 0 {
		fallback := float64(tr.CloseProfitAbs)
		if fallback != 0 {
			realized = fallback
			realizedSource = dashboardPnLRealizedSourceCloseProfitAbs
		}
	}

	unrealized := float64(tr.ProfitAbs)
	total := float64(tr.TotalProfitAbs)
	totalSource := dashboardPnLTotalSourceTotalProfitAbs
	if total == 0 {
		total = realized + unrealized
		totalSource = dashboardPnLTotalSourceComponents
	}

	return DashboardPnLCard{Realized: realized, Unrealized: unrealized, Total: total}, dashboardPnLProvenance{
		RealizedSource:   realizedSource,
		UnrealizedSource: dashboardPnLUnrealizedSourceProfitAbs,
		TotalSource:      totalSource,
	}
}

func reconcileDashboardPnL(pnl DashboardPnLCard) DashboardReconciliation {
	expectedTotal := pnl.Realized + pnl.Unrealized
	driftAbs := math.Abs(pnl.Total - expectedTotal)
	base := math.Max(math.Abs(expectedTotal), math.Abs(pnl.Total))
	driftPct := 0.0
	if base > 0 {
		driftPct = driftAbs / base
	}

	status := "ok"
	if driftAbs > dashboardPnLDriftThreshold {
		status = "warn"
	}
	if driftAbs > dashboardPnLDriftThreshold*5 {
		status = "error"
	}

	return DashboardReconciliation{
		Status:         status,
		DriftAbs:       driftAbs,
		DriftPct:       driftPct,
		DriftThreshold: dashboardPnLDriftThreshold,
	}
}

func (u portfolioUsecase) lookupRiskLevels(ctx context.Context, symbol string) (float64, []float64, bool) {
	if u.store == nil {
		return 0, nil, false
	}
	pos, ok, storeErr := u.store.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if storeErr != nil || !ok {
		return 0, nil, false
	}
	return decodeDashboardRiskLevels(pos.RiskJSON)
}

func decodeDashboardRiskLevels(riskJSON []byte) (float64, []float64, bool) {
	if len(riskJSON) == 0 {
		return 0, nil, false
	}
	plan, err := position.DecodeRiskPlan(riskJSON)
	if err != nil {
		return 0, nil, false
	}
	takeProfits := make([]float64, 0, len(plan.TPLevels))
	for _, level := range plan.TPLevels {
		takeProfits = append(takeProfits, level.Price)
	}
	return plan.StopPrice, takeProfits, true
}

func (u portfolioUsecase) mapDashboardOverviewSymbol(ctx context.Context, tr execution.Trade) DashboardOverviewSymbol {
	symbol := normalizeFreqtradePair(tr.Pair)
	stopLoss, takeProfits, _ := u.lookupRiskLevels(ctx, symbol)
	pnl, _ := resolveDashboardPnLFromTrade(tr)

	side := "long"
	if tr.IsShort {
		side = "short"
	}

	return DashboardOverviewSymbol{
		Symbol: symbol,
		Position: DashboardPositionCard{
			Side:         side,
			Amount:       float64(tr.Amount),
			EntryPrice:   float64(tr.OpenRate),
			CurrentPrice: float64(tr.CurrentRate),
			TakeProfits:  takeProfits,
			StopLoss:     stopLoss,
		},
		PnL:            pnl,
		Reconciliation: reconcileDashboardPnL(pnl),
	}
}

func (u portfolioUsecase) buildDashboardAccountPnL(ctx context.Context) (DashboardPnLCard, bool) {
	if u.execClient == nil {
		return DashboardPnLCard{}, false
	}
	quote, err := u.execClient.Balance(ctx)
	if err != nil {
		return DashboardPnLCard{}, false
	}
	totalProfit, ok := extractDashboardAccountTotalProfit(quote)
	if !ok {
		return DashboardPnLCard{}, false
	}
	unrealized := u.sumOpenUnrealizedPnL(ctx)
	realized := totalProfit - unrealized
	return DashboardPnLCard{Realized: realized, Unrealized: unrealized, Total: totalProfit}, true
}

func (u portfolioUsecase) sumOpenUnrealizedPnL(ctx context.Context) float64 {
	if u.execClient == nil {
		return 0
	}
	trades, err := u.execClient.ListOpenTrades(ctx)
	if err != nil {
		return 0
	}
	sum := 0.0
	for _, tr := range trades {
		symbol := normalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if u.allowSymbol != nil && !u.allowSymbol(symbol) {
			continue
		}
		sum += float64(tr.ProfitAbs)
	}
	return sum
}

func extractDashboardAccountTotalProfit(quote map[string]any) (float64, bool) {
	if quote == nil {
		return 0, false
	}
	startingCapital, hasStartingCapital := execution.AsFloat(quote["starting_capital"])
	startingCapitalRatio, hasStartingCapitalRatio := execution.AsFloat(quote["starting_capital_ratio"])
	if hasStartingCapital && hasStartingCapitalRatio {
		return startingCapital * startingCapitalRatio, true
	}
	if hasStartingCapital {
		if totalBot, ok := execution.AsFloat(quote["total_bot"]); ok {
			return totalBot - startingCapital, true
		}
		if total, ok := execution.AsFloat(quote["total"]); ok {
			return total - startingCapital, true
		}
	}
	return 0, false
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

	trades, err := u.execClient.ListOpenTrades(ctx)
	if err != nil {
		return "", nil, &usecaseError{Status: 502, Code: "dashboard_overview_failed", Message: "dashboard 概览获取失败", Details: err.Error()}
	}

	cards := make([]DashboardOverviewSymbol, 0, len(trades))
	for _, tr := range trades {
		symbol := normalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if u.allowSymbol != nil && !u.allowSymbol(symbol) {
			continue
		}
		if normalizedSymbol != "" && symbol != normalizedSymbol {
			continue
		}
		cards = append(cards, u.mapDashboardOverviewSymbol(ctx, tr))
	}

	sort.Slice(cards, func(i, j int) bool {
		return cards[i].Symbol < cards[j].Symbol
	})

	if len(cards) == 0 {
		return normalizedSymbol, []DashboardOverviewSymbol{}, nil
	}
	return normalizedSymbol, cards, nil
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
	normalizedFilter := runtime.NormalizeSymbol(strings.TrimSpace(symbolFilter))
	resp, err := u.execClient.ListTrades(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	if limit > 0 && offset <= 0 && resp.TotalTrades > limit {
		latestOffset := resp.TotalTrades - limit
		if latestOffset < 0 {
			latestOffset = 0
		}
		resp, err = u.execClient.ListTrades(ctx, limit, latestOffset)
		if err != nil {
			return nil, err
		}
	}
	items := make([]TradeHistoryItem, 0, len(resp.Trades))
	for _, tr := range resp.Trades {
		symbol := normalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if u.allowSymbol != nil && !u.allowSymbol(symbol) {
			continue
		}
		if normalizedFilter != "" && symbol != normalizedFilter {
			continue
		}
		profit := float64(tr.CloseProfitAbs)
		if profit == 0 {
			profit = float64(tr.RealizedProfit)
		}
		durationSec := int64(tr.TradeDuration)
		if tr.TradeDurationSeconds > 0 {
			durationSec = int64(tr.TradeDurationSeconds)
		}
		openedAt := parseMillisTimestamp(int64(tr.OpenFillTimestamp))
		closedAt := parseMillisTimestamp(int64(tr.CloseTimestamp))
		if closedAt.IsZero() && !openedAt.IsZero() && durationSec > 0 {
			closedAt = openedAt.Add(time.Duration(durationSec) * time.Second)
		}
		side := "long"
		if tr.IsShort {
			side = "short"
		}
		items = append(items, TradeHistoryItem{
			Symbol:       symbol,
			Side:         side,
			Amount:       float64(tr.Amount),
			MarginAmount: float64(tr.StakeAmount),
			OpenedAt:     openedAt,
			ClosedAt:     closedAt,
			DurationSec:  durationSec,
			Profit:       profit,
		})
	}
	return items, nil
}

func normalizeFreqtradePair(pair string) string {
	return symbolpkg.FromFreqtradePair(pair)
}

func parseMillisTimestamp(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ts)
}

var shanghaiLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}()

func positionStatusTiming(openFillTimestamp int64) (string, int64, int64) {
	if openFillTimestamp <= 0 {
		return "", 0, 0
	}
	var openedAt time.Time
	if openFillTimestamp < 1e12 {
		openedAt = time.Unix(openFillTimestamp, 0)
	} else {
		openedAt = time.UnixMilli(openFillTimestamp)
	}
	openedAtText := openedAt.In(shanghaiLocation).Format("2006-01-02 15:04:05")
	if openedAt.IsZero() {
		return "", 0, 0
	}
	now := time.Now()
	durationMin := int64(0)
	if now.After(openedAt) {
		durationMin = int64(now.Sub(openedAt).Minutes())
	}
	return openedAtText, durationMin, durationMin * 60
}
