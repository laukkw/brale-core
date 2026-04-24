package snapshot

import "context"

type KlineProvider interface {
	Klines(ctx context.Context, symbol, interval string, limit int) ([]Candle, error)
}

type OIProvider interface {
	OpenInterest(ctx context.Context, symbol string) (OIBlock, error)
}

type FundingProvider interface {
	Funding(ctx context.Context, symbol string) (FundingBlock, error)
}

type LongShortProvider interface {
	LongShortRatio(ctx context.Context, symbol, interval string) (LSRBlock, error)
}

type FearGreedProvider interface {
	FearGreed(ctx context.Context) (FearGreedPoint, error)
}

type LiquidationProvider interface {
	Liquidations(ctx context.Context, symbol string) (LiqBlock, error)
}

type LiquidationWindowProvider interface {
	LiquidationsByWindow(ctx context.Context, symbol string) (map[string]LiqWindow, error)
}

type LiquidationSourceProvider interface {
	LiquidationSource(ctx context.Context, symbol string) (LiqSource, error)
}
