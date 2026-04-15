package features

import (
	"context"

	"brale-core/internal/snapshot"
)

type IntervalTrendBuilder struct {
	OptionsByInterval map[string]TrendCompressOptions
	DefaultOptions    TrendCompressOptions
	Computer          IndicatorComputer
}

func (b IntervalTrendBuilder) BuildTrend(_ context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (TrendJSON, error) {
	candles, err := candlesFor(snap, symbol, interval)
	if err != nil {
		return TrendJSON{}, err
	}
	opts := b.DefaultOptions
	if selected, ok := b.OptionsByInterval[interval]; ok {
		opts = selected
	}
	if opts == (TrendCompressOptions{}) {
		opts = DefaultTrendCompressOptions()
	}
	raw, err := BuildTrendCompressedJSONWithComputer(symbol, interval, candles, opts, b.Computer)
	if err != nil {
		return TrendJSON{}, err
	}
	return TrendJSON{Symbol: symbol, Interval: interval, RawJSON: []byte(raw)}, nil
}
