// 本文件主要内容：简单特征构建器实现。
package features

import (
	"context"
	"encoding/json"
	"sort"

	"brale-core/internal/snapshot"
)

type SimpleBuilder struct{}

type simpleIndicator struct {
	Open  float64 `json:"open"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
	Close float64 `json:"close"`
	ATR   float64 `json:"atr"`
}

type simpleTrend struct {
	Regime string `json:"regime"`
}

type simpleMechanics struct {
	OI float64 `json:"oi"`
}

func (SimpleBuilder) BuildIndicator(_ context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (IndicatorJSON, error) {
	candle := latestCandle(snap, symbol, interval)
	atr := candle.High - candle.Low
	data, _ := json.Marshal(simpleIndicator{
		Open:  candle.Open,
		High:  candle.High,
		Low:   candle.Low,
		Close: candle.Close,
		ATR:   atr,
	})
	return IndicatorJSON{Symbol: symbol, Interval: interval, RawJSON: data}, nil
}

func (SimpleBuilder) BuildTrend(_ context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (TrendJSON, error) {
	candle := latestCandle(snap, symbol, interval)
	regime := "range"
	if candle.Close > candle.Open {
		regime = "trend_up"
	} else if candle.Close < candle.Open {
		regime = "trend_down"
	}
	data, _ := json.Marshal(simpleTrend{Regime: regime})
	return TrendJSON{Symbol: symbol, Interval: interval, RawJSON: data}, nil
}

func (SimpleBuilder) BuildMechanics(_ context.Context, snap snapshot.MarketSnapshot, symbol string) (MechanicsSnapshot, error) {
	oi := snap.OI[symbol].Value
	data, _ := json.Marshal(simpleMechanics{OI: oi})
	return MechanicsSnapshot{Symbol: symbol, RawJSON: data}, nil
}

func latestCandle(snap snapshot.MarketSnapshot, symbol, interval string) snapshot.Candle {
	if byInterval, ok := snap.Klines[symbol]; ok {
		if candles, ok := byInterval[interval]; ok && len(candles) > 0 {
			return candles[len(candles)-1]
		}
		for _, iv := range sortedKeys(byInterval) {
			candles := byInterval[iv]
			if len(candles) > 0 {
				return candles[len(candles)-1]
			}
		}
	}
	return snapshot.Candle{}
}

func sortedKeys(m map[string][]snapshot.Candle) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
