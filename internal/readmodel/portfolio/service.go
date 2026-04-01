package portfolio

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/position"
	dashboard "brale-core/internal/readmodel/dashboard"
	"brale-core/internal/runtime"
	"brale-core/internal/store"
)

type ExecClient interface {
	execution.BalanceReader
	execution.OpenTradesReader
	execution.TradesReader
}

type Store interface {
	store.PositionQueryStore
	store.RiskPlanQueryStore
}

type PositionStatusItem struct {
	Symbol           string
	Amount           float64
	AmountRequested  float64
	MarginAmount     float64
	EntryPrice       float64
	CurrentPrice     float64
	Side             string
	ProfitTotal      float64
	ProfitRealized   float64
	ProfitUnrealized float64
	OpenedAt         string
	DurationMin      int64
	DurationSec      int64
	TakeProfits      []float64
	StopLoss         float64
}

type PnLCard = dashboard.PnLCard
type Reconciliation = dashboard.Reconciliation
type RiskPlanTimelineItem = dashboard.RiskPlanTimelineItem

type OverviewSymbol struct {
	Symbol         string
	Position       PositionCard
	PnL            PnLCard
	Reconciliation Reconciliation
}

type PositionCard struct {
	Side             string
	Amount           float64
	Leverage         float64
	EntryPrice       float64
	CurrentPrice     float64
	TakeProfits      []float64
	StopLoss         float64
	RiskPlanTimeline []RiskPlanTimelineItem
}

type TradeHistoryItem struct {
	Symbol       string
	Side         string
	Amount       float64
	MarginAmount float64
	OpenedAt     time.Time
	ClosedAt     time.Time
	DurationSec  int64
	Profit       float64
	StopLoss     float64
	TakeProfits  []float64
	Timeline     []RiskPlanTimelineItem
}

func BuildPositionStatus(ctx context.Context, execClient ExecClient, st Store, allowSymbol func(string) bool, timelineLimit int) ([]PositionStatusItem, error) {
	trades, err := execClient.ListOpenTrades(ctx)
	if err != nil {
		return nil, err
	}
	positions := make([]PositionStatusItem, 0, len(trades))
	for _, tr := range trades {
		symbol := dashboard.NormalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if allowSymbol != nil && !allowSymbol(symbol) {
			continue
		}
		margin := float64(tr.StakeAmount)
		if margin <= 0 {
			margin = float64(tr.OpenTradeValue)
		}
		pnl, _ := dashboard.ResolvePnLFromTrade(tr)
		openedAt, durationMin, durationSec := dashboard.PositionStatusTiming(int64(tr.OpenFillTimestamp))
		riskState := lookupRiskState(ctx, st, symbol, timelineLimit)
		side := "long"
		if tr.IsShort {
			side = "short"
		}
		positions = append(positions, PositionStatusItem{
			Symbol:           symbol,
			Amount:           float64(tr.Amount),
			AmountRequested:  float64(tr.AmountRequested),
			MarginAmount:     margin,
			EntryPrice:       float64(tr.OpenRate),
			CurrentPrice:     float64(tr.CurrentRate),
			Side:             side,
			ProfitTotal:      pnl.Total,
			ProfitRealized:   pnl.Realized,
			ProfitUnrealized: pnl.Unrealized,
			OpenedAt:         openedAt,
			DurationMin:      durationMin,
			DurationSec:      durationSec,
			TakeProfits:      riskState.TakeProfits,
			StopLoss:         riskState.StopLoss,
		})
	}
	return positions, nil
}

func BuildOverview(ctx context.Context, execClient ExecClient, st Store, rawSymbol string, allowSymbol func(string) bool, timelineLimit int) (string, []OverviewSymbol, error) {
	trimmedRaw := strings.TrimSpace(rawSymbol)
	normalizedSymbol := runtime.NormalizeSymbol(trimmedRaw)
	trades, err := execClient.ListOpenTrades(ctx)
	if err != nil {
		return "", nil, err
	}
	cards := make([]OverviewSymbol, 0, len(trades))
	for _, tr := range trades {
		symbol := dashboard.NormalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if allowSymbol != nil && !allowSymbol(symbol) {
			continue
		}
		if normalizedSymbol != "" && symbol != normalizedSymbol {
			continue
		}
		cards = append(cards, mapOverviewSymbol(ctx, st, tr, timelineLimit))
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].Symbol < cards[j].Symbol })
	if len(cards) == 0 {
		return normalizedSymbol, []OverviewSymbol{}, nil
	}
	return normalizedSymbol, cards, nil
}

func BuildAccountPnL(ctx context.Context, execClient ExecClient, allowSymbol func(string) bool) (PnLCard, bool) {
	quote, err := execClient.Balance(ctx)
	if err != nil {
		return PnLCard{}, false
	}
	totalProfit, ok := dashboard.ExtractAccountTotalProfit(quote)
	if !ok {
		return PnLCard{}, false
	}
	unrealized := sumOpenUnrealizedPnL(ctx, execClient, allowSymbol)
	realized := totalProfit - unrealized
	return PnLCard{Realized: realized, Unrealized: unrealized, Total: totalProfit}, true
}

func BuildTradeHistory(ctx context.Context, execClient ExecClient, st Store, limit, offset int, symbolFilter string, allowSymbol func(string) bool, timelineLimit int) ([]TradeHistoryItem, error) {
	resp, err := execClient.ListTrades(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	if limit > 0 && offset <= 0 && resp.TotalTrades > limit {
		latestOffset := resp.TotalTrades - limit
		if latestOffset < 0 {
			latestOffset = 0
		}
		resp, err = execClient.ListTrades(ctx, limit, latestOffset)
		if err != nil {
			return nil, err
		}
	}
	normalizedFilter := runtime.NormalizeSymbol(strings.TrimSpace(symbolFilter))
	positionByExecID := map[string]store.PositionRecord{}
	if st != nil {
		positions, err := st.ListPositionsByStatus(ctx, []string{position.PositionClosed})
		if err == nil {
			for _, pos := range positions {
				execID := strings.TrimSpace(pos.ExecutorPositionID)
				if execID != "" {
					positionByExecID[execID] = pos
				}
			}
		}
	}
	items := make([]TradeHistoryItem, 0, len(resp.Trades))
	for _, tr := range resp.Trades {
		symbol := dashboard.NormalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if allowSymbol != nil && !allowSymbol(symbol) {
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
		openedAt := dashboard.ParseMillisTimestamp(int64(tr.OpenFillTimestamp))
		closedAt := dashboard.ParseMillisTimestamp(int64(tr.CloseTimestamp))
		if closedAt.IsZero() && !openedAt.IsZero() && durationSec > 0 {
			closedAt = openedAt.Add(time.Duration(durationSec) * time.Second)
		}
		side := "long"
		if tr.IsShort {
			side = "short"
		}
		riskState := dashboard.RiskState{}
		if st != nil {
			execID := strconv.Itoa(int(tr.ID))
			if pos, ok := positionByExecID[execID]; ok {
				riskState = dashboard.LoadRiskState(ctx, st, pos, timelineLimit)
			}
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
			StopLoss:     riskState.StopLoss,
			TakeProfits:  riskState.TakeProfits,
			Timeline:     riskState.Timeline,
		})
	}
	return items, nil
}

func lookupRiskState(ctx context.Context, st Store, symbol string, timelineLimit int) dashboard.RiskState {
	if st == nil {
		return dashboard.RiskState{}
	}
	pos, ok, err := st.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil || !ok {
		return dashboard.RiskState{}
	}
	return dashboard.LoadRiskState(ctx, st, pos, timelineLimit)
}

func mapOverviewSymbol(ctx context.Context, st Store, tr execution.Trade, timelineLimit int) OverviewSymbol {
	symbol := dashboard.NormalizeFreqtradePair(tr.Pair)
	riskState := lookupRiskState(ctx, st, symbol, timelineLimit)
	pnl, _ := dashboard.ResolvePnLFromTrade(tr)
	return OverviewSymbol{
		Symbol: symbol,
		Position: PositionCard{
			Side:             tradeSide(tr),
			Amount:           float64(tr.Amount),
			Leverage:         dashboard.ResolveLeverage(tr),
			EntryPrice:       float64(tr.OpenRate),
			CurrentPrice:     float64(tr.CurrentRate),
			TakeProfits:      riskState.TakeProfits,
			StopLoss:         riskState.StopLoss,
			RiskPlanTimeline: riskState.Timeline,
		},
		PnL:            pnl,
		Reconciliation: dashboard.ReconcilePnL(pnl),
	}
}

func sumOpenUnrealizedPnL(ctx context.Context, execClient ExecClient, allowSymbol func(string) bool) float64 {
	trades, err := execClient.ListOpenTrades(ctx)
	if err != nil {
		return 0
	}
	sum := 0.0
	for _, tr := range trades {
		symbol := dashboard.NormalizeFreqtradePair(tr.Pair)
		if symbol == "" {
			continue
		}
		if allowSymbol != nil && !allowSymbol(symbol) {
			continue
		}
		sum += float64(tr.ProfitAbs)
	}
	return sum
}

func tradeSide(tr execution.Trade) string {
	if tr.IsShort {
		return "short"
	}
	return "long"
}
