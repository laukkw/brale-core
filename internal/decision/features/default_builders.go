// 本文件主要内容：默认特征构建器实现。
package features

import (
	"context"
	"fmt"

	"brale-core/internal/snapshot"
)

type DefaultIndicatorBuilder struct {
	Options IndicatorCompressOptions
}

func (b DefaultIndicatorBuilder) BuildIndicator(_ context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (IndicatorJSON, error) {
	candles, err := candlesFor(snap, symbol, interval)
	if err != nil {
		return IndicatorJSON{}, err
	}
	raw, err := BuildIndicatorCompressedJSON(symbol, interval, candles, b.Options)
	if err != nil {
		return IndicatorJSON{}, err
	}
	return IndicatorJSON{Symbol: symbol, Interval: interval, RawJSON: []byte(raw)}, nil
}

type DefaultTrendBuilder struct {
	Options TrendCompressOptions
}

func (b DefaultTrendBuilder) BuildTrend(_ context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (TrendJSON, error) {
	candles, err := candlesFor(snap, symbol, interval)
	if err != nil {
		return TrendJSON{}, err
	}
	raw, err := BuildTrendCompressedJSON(symbol, interval, candles, b.Options)
	if err != nil {
		return TrendJSON{}, err
	}
	return TrendJSON{Symbol: symbol, Interval: interval, RawJSON: []byte(raw)}, nil
}

type DefaultMechanicsBuilder struct {
	Options MechanicsCompressOptions
}

func (b DefaultMechanicsBuilder) BuildMechanics(ctx context.Context, snap snapshot.MarketSnapshot, symbol string) (MechanicsSnapshot, error) {
	return BuildMechanicsSnapshot(ctx, symbol, snap, b.Options)
}

func candlesFor(snap snapshot.MarketSnapshot, symbol, interval string) ([]snapshot.Candle, error) {
	if len(snap.Klines) == 0 {
		return nil, fmt.Errorf("no klines available")
	}
	byInterval, ok := snap.Klines[symbol]
	if !ok {
		return nil, fmt.Errorf("no klines for %s", symbol)
	}
	candles, ok := byInterval[interval]
	if !ok || len(candles) == 0 {
		return nil, fmt.Errorf("no candles for %s %s", symbol, interval)
	}
	return candles, nil
}
