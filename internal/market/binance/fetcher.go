package binance

import (
	"brale-core/internal/market"
	"brale-core/internal/snapshot"
)

type SnapshotOptions struct {
	RequireOI            bool
	RequireFunding       bool
	RequireLongShort     bool
	RequireFearGreed     bool
	RequireLiquidations  bool
	LiquidationsByWindow snapshot.LiquidationWindowProvider
	LiquidationSource    snapshot.LiquidationSourceProvider
}

func NewSnapshotFetcher(opts SnapshotOptions) *snapshot.Fetcher {
	futuresMarket := NewFuturesMarket()
	fetcher := &snapshot.Fetcher{
		Klines:               futuresMarket,
		OI:                   futuresMarket,
		Funding:              futuresMarket,
		LongShort:            futuresMarket,
		FearGreed:            nil,
		Liquidations:         nil,
		LiquidationsByWindow: opts.LiquidationsByWindow,
		LiquidationSource:    opts.LiquidationSource,
		RequireOI:            opts.RequireOI,
		RequireFunding:       opts.RequireFunding,
		RequireLongShort:     opts.RequireLongShort,
		RequireFearGreed:     opts.RequireFearGreed,
		RequireLiquidations:  opts.RequireLiquidations,
	}
	if opts.RequireFearGreed {
		fetcher.FearGreed = market.NewFearGreedService()
	}
	return fetcher
}
