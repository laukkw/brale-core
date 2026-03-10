package execution

import "context"

type BalanceReader interface {
	Balance(ctx context.Context) (map[string]any, error)
}

type OpenTradesReader interface {
	ListOpenTrades(ctx context.Context) ([]Trade, error)
}

type TradesReader interface {
	ListTrades(ctx context.Context, limit, offset int) (TradeResponse, error)
}

type TradeFinder interface {
	FindTradeByID(ctx context.Context, tradeID int) (Trade, bool, error)
}

type ProfitAllReader interface {
	ProfitAll(ctx context.Context) (map[string]any, error)
}
