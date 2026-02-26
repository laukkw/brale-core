package runtimeapi

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/execution"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
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
		profitUnrealized := float64(tr.ProfitAbs)
		profitRealized := float64(tr.RealizedProfit)
		profitTotal := float64(tr.TotalProfitAbs)
		if profitTotal == 0 && (profitUnrealized != 0 || profitRealized != 0) {
			profitTotal = profitUnrealized + profitRealized
		}
		openedAt, durationMin, durationSec := positionStatusTiming(int64(tr.OpenFillTimestamp))
		stopLoss := 0.0
		takeProfits := []float64{}
		if u.store != nil {
			pos, ok, storeErr := u.store.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
			if storeErr == nil && ok && len(pos.RiskJSON) > 0 {
				plan, decodeErr := position.DecodeRiskPlan(pos.RiskJSON)
				if decodeErr == nil {
					stopLoss = plan.StopPrice
					if len(plan.TPLevels) > 0 {
						takeProfits = make([]float64, 0, len(plan.TPLevels))
						for _, level := range plan.TPLevels {
							takeProfits = append(takeProfits, level.Price)
						}
					}
				}
			}
		}
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
			ProfitTotal:      profitTotal,
			ProfitRealized:   profitRealized,
			ProfitUnrealized: profitUnrealized,
			OpenedAt:         openedAt,
			DurationMin:      durationMin,
			DurationSec:      durationSec,
			TakeProfits:      takeProfits,
			StopLoss:         stopLoss,
		})
	}
	return positions, nil
}

func (u portfolioUsecase) buildTradeHistory(ctx context.Context, limit, offset int) ([]TradeHistoryItem, error) {
	if u.execClient == nil {
		return nil, fmt.Errorf("exec client missing")
	}
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
		profit := float64(tr.CloseProfitAbs)
		if profit == 0 {
			profit = float64(tr.RealizedProfit)
		}
		durationSec := int64(tr.TradeDuration)
		if tr.TradeDurationSeconds > 0 {
			durationSec = int64(tr.TradeDurationSeconds)
		}
		openedAt := parseMillisTimestamp(int64(tr.OpenFillTimestamp))
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
