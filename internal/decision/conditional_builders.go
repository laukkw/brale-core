// 本文件主要内容：条件化的特征构建器封装。
package decision

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/decision/features"
	"brale-core/internal/snapshot"
)

type ConditionalMechanicsBuilder struct {
	Enabled        map[string]AgentEnabled
	EnabledBuilder features.MechanicsBuilder
}

type ConditionalIndicatorBuilder struct {
	Enabled        map[string]AgentEnabled
	EnabledBuilder features.IndicatorBuilder
}

func (b ConditionalIndicatorBuilder) BuildIndicator(ctx context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (features.IndicatorJSON, error) {
	if enabled, ok := lookupEnabled(b.Enabled, symbol); ok && !enabled.Indicator {
		return features.IndicatorJSON{Symbol: symbol, Interval: interval}, nil
	}
	if b.EnabledBuilder == nil {
		return features.IndicatorJSON{}, fmt.Errorf("indicator builder is required")
	}
	return b.EnabledBuilder.BuildIndicator(ctx, snap, symbol, interval)
}

type ConditionalTrendBuilder struct {
	Enabled        map[string]AgentEnabled
	EnabledBuilder features.TrendBuilder
}

func (b ConditionalTrendBuilder) BuildTrend(ctx context.Context, snap snapshot.MarketSnapshot, symbol, interval string) (features.TrendJSON, error) {
	if enabled, ok := lookupEnabled(b.Enabled, symbol); ok && !enabled.Structure {
		return features.TrendJSON{Symbol: symbol, Interval: interval}, nil
	}
	if b.EnabledBuilder == nil {
		return features.TrendJSON{}, fmt.Errorf("trend builder is required")
	}
	return b.EnabledBuilder.BuildTrend(ctx, snap, symbol, interval)
}

func (b ConditionalMechanicsBuilder) BuildMechanics(ctx context.Context, snap snapshot.MarketSnapshot, symbol string) (features.MechanicsSnapshot, error) {
	if enabled, ok := lookupEnabled(b.Enabled, symbol); ok && !enabled.Mechanics {
		return features.MechanicsSnapshot{Symbol: symbol}, nil
	}
	if b.EnabledBuilder == nil {
		return features.MechanicsSnapshot{}, fmt.Errorf("mechanics builder is required")
	}
	return b.EnabledBuilder.BuildMechanics(ctx, snap, symbol)
}

func lookupEnabled(enabled map[string]AgentEnabled, symbol string) (AgentEnabled, bool) {
	if enabled == nil {
		return AgentEnabled{}, false
	}
	if v, ok := enabled[symbol]; ok {
		return v, true
	}
	key := strings.ToUpper(strings.TrimSpace(symbol))
	if v, ok := enabled[key]; ok {
		return v, true
	}
	for k, v := range enabled {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return AgentEnabled{}, false
}
