// 本文件主要内容：压缩快照为指标/结构/力学 JSON。
package features

import (
	"context"
	"fmt"

	"brale-core/internal/snapshot"
)

type IndicatorBuilder interface {
	BuildIndicator(ctx context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (IndicatorJSON, error)
}

type TrendBuilder interface {
	BuildTrend(ctx context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (TrendJSON, error)
}

type MechanicsBuilder interface {
	BuildMechanics(ctx context.Context, snap snapshot.MarketSnapshot, symbol string) (MechanicsSnapshot, error)
}

type Compressor struct {
	Indicators IndicatorBuilder
	Trends     TrendBuilder
	Mechanics  MechanicsBuilder
}

func NewDefaultCompressor() *Compressor {
	return &Compressor{
		Indicators: DefaultIndicatorBuilder{},
		Trends:     DefaultTrendBuilder{},
		Mechanics:  DefaultMechanicsBuilder{},
	}
}

func (c *Compressor) Compress(ctx context.Context, snap snapshot.MarketSnapshot) (CompressionResult, []FeatureError, error) {
	if c == nil || c.Indicators == nil || c.Trends == nil || c.Mechanics == nil {
		return CompressionResult{}, nil, fmt.Errorf("feature builders not configured")
	}
	if len(snap.Klines) == 0 {
		return CompressionResult{}, nil, fmt.Errorf("snapshot missing klines data")
	}
	out := CompressionResult{
		Indicators: map[string]map[string]IndicatorJSON{},
		Trends:     map[string]map[string]TrendJSON{},
		Mechanics:  map[string]MechanicsSnapshot{},
	}
	var errs []FeatureError
	for sym, byInterval := range snap.Klines {
		if err := c.buildBySymbol(ctx, snap, sym, byInterval, &out, &errs); err != nil {
			return CompressionResult{}, nil, err
		}
	}
	return out, errs, nil
}

func (c *Compressor) buildBySymbol(ctx context.Context, snap snapshot.MarketSnapshot, sym string, byInterval map[string][]snapshot.Candle, out *CompressionResult, errs *[]FeatureError) error {
	if out.Indicators[sym] == nil {
		out.Indicators[sym] = map[string]IndicatorJSON{}
	}
	if out.Trends[sym] == nil {
		out.Trends[sym] = map[string]TrendJSON{}
	}
	for iv := range byInterval {
		c.buildIndicator(ctx, snap, sym, iv, out, errs)
		c.buildTrend(ctx, snap, sym, iv, out, errs)
	}
	c.buildMechanics(ctx, snap, sym, out, errs)
	return nil
}

func (c *Compressor) buildIndicator(ctx context.Context, snap snapshot.MarketSnapshot, sym, iv string, out *CompressionResult, errs *[]FeatureError) {
	ind, err := c.Indicators.BuildIndicator(ctx, snap, sym, iv)
	if err != nil {
		*errs = append(*errs, FeatureError{Symbol: sym, Stage: "indicator", Err: err})
		return
	}
	out.Indicators[sym][iv] = ind
}

func (c *Compressor) buildTrend(ctx context.Context, snap snapshot.MarketSnapshot, sym, iv string, out *CompressionResult, errs *[]FeatureError) {
	tr, err := c.Trends.BuildTrend(ctx, snap, sym, iv)
	if err != nil {
		*errs = append(*errs, FeatureError{Symbol: sym, Stage: "trend", Err: err})
		return
	}
	out.Trends[sym][iv] = tr
}

func (c *Compressor) buildMechanics(ctx context.Context, snap snapshot.MarketSnapshot, sym string, out *CompressionResult, errs *[]FeatureError) {
	mech, err := c.Mechanics.BuildMechanics(ctx, snap, sym)
	if err != nil {
		*errs = append(*errs, FeatureError{Symbol: sym, Stage: "mechanics", Err: err})
		return
	}
	out.Mechanics[sym] = mech
}
