package features

import (
	"context"
	"encoding/json"
	"fmt"

	"brale-core/internal/snapshot"
)

// TrendCompressBuilder wraps BuildTrendCompressedInput to satisfy the TrendBuilder interface.
type TrendCompressBuilder struct {
	Options  TrendCompressOptions
	Computer IndicatorComputer
}

func (b TrendCompressBuilder) BuildIndicator(_ context.Context, _ snapshot.MarketSnapshot, _ string, _ string) (IndicatorJSON, error) {
	return IndicatorJSON{}, fmt.Errorf("indicator builder not supported in TrendCompressBuilder")
}

func (b TrendCompressBuilder) BuildTrend(_ context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (TrendJSON, error) {
	candles := snap.Klines[symbol][interval]
	if len(candles) == 0 {
		return TrendJSON{}, fmt.Errorf("no candles for %s %s", symbol, interval)
	}
	opts := b.Options
	if opts == (TrendCompressOptions{}) {
		opts = DefaultTrendCompressOptions()
	}
	input, err := BuildTrendCompressedInputWithComputer(symbol, interval, candles, opts, b.Computer)
	if err != nil {
		return TrendJSON{}, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return TrendJSON{}, err
	}
	return TrendJSON{Symbol: symbol, Interval: interval, RawJSON: raw}, nil
}

func (b TrendCompressBuilder) BuildMechanics(_ context.Context, _ snapshot.MarketSnapshot, _ string) (MechanicsSnapshot, error) {
	return MechanicsSnapshot{}, fmt.Errorf("mechanics builder not supported in TrendCompressBuilder")
}
