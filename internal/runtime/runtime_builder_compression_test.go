package runtime

import (
	"context"
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/snapshot"
)

type testRuntimeLiqProvider struct{}

func (testRuntimeLiqProvider) LiquidationsByWindow(context.Context, string) (map[string]snapshot.LiqWindow, error) {
	return map[string]snapshot.LiqWindow{snapshot.LiqWindow1h: {Status: "ok"}}, nil
}

func (testRuntimeLiqProvider) LiquidationSource(context.Context, string) (snapshot.LiqSource, error) {
	return snapshot.LiqSource{Source: "binance_force_order_snapshot_ws", Status: "ok"}, nil
}

func TestBuildCompressorUsesConfiguredIndicatorEngine(t *testing.T) {
	compressor, services, err := buildCompressor(config.SymbolConfig{
		Symbol:    "BTCUSDT",
		Intervals: []string{"1h"},
		Indicators: config.IndicatorConfig{
			Engine:         config.IndicatorEngineReference,
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			STCFast:        23,
			STCSlow:        50,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
		},
	}, config.AgentEnabled{Indicator: true, Structure: true, Mechanics: false}, map[string]decision.AgentEnabled{
		"BTCUSDT": {Indicator: true, Structure: true, Mechanics: false},
	})
	if err != nil {
		t.Fatalf("buildCompressor() error = %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("services len=%d want 0", len(services))
	}
	base, ok := compressor.(*decision.FeatureCompressor)
	if !ok {
		t.Fatalf("compressor=%T want *decision.FeatureCompressor", compressor)
	}
	indicatorBuilder, ok := base.Indicators.(decision.DefaultIndicatorBuilder)
	if !ok {
		t.Fatalf("indicator builder=%T", base.Indicators)
	}
	if _, ok := indicatorBuilder.Computer.(decision.ReferenceComputer); !ok {
		t.Fatalf("indicator computer=%T", indicatorBuilder.Computer)
	}
	trendBuilder, ok := base.Trends.(decision.IntervalTrendBuilder)
	if !ok {
		t.Fatalf("trend builder=%T", base.Trends)
	}
	if _, ok := trendBuilder.Computer.(decision.ReferenceComputer); !ok {
		t.Fatalf("trend computer=%T", trendBuilder.Computer)
	}
}

func TestBuildCompressorWrapsShadowEngineComparison(t *testing.T) {
	compressor, _, err := buildCompressor(config.SymbolConfig{
		Symbol:    "BTCUSDT",
		Intervals: []string{"1h"},
		Indicators: config.IndicatorConfig{
			Engine:         config.IndicatorEngineTalib,
			ShadowEngine:   config.IndicatorEngineReference,
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			STCFast:        23,
			STCSlow:        50,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
		},
	}, config.AgentEnabled{Indicator: true, Structure: true, Mechanics: false}, map[string]decision.AgentEnabled{
		"BTCUSDT": {Indicator: true, Structure: true, Mechanics: false},
	})
	if err != nil {
		t.Fatalf("buildCompressor() error = %v", err)
	}
	shadow, ok := compressor.(*shadowComparingCompressor)
	if !ok {
		t.Fatalf("compressor=%T want *shadowComparingCompressor", compressor)
	}
	if _, ok := shadow.primaryComputer.(decision.TalibComputer); !ok {
		t.Fatalf("primary computer=%T", shadow.primaryComputer)
	}
	if _, ok := shadow.shadowComputer.(decision.ReferenceComputer); !ok {
		t.Fatalf("shadow computer=%T", shadow.shadowComputer)
	}
}

func TestBuildSnapshotFetcherDisablesLiquidationsWhenConfigDisabled(t *testing.T) {
	t.Parallel()

	provider := testRuntimeLiqProvider{}
	fetcher := buildSnapshotFetcher(config.SymbolConfig{
		Symbol: "BTCUSDT",
		Require: config.SymbolRequire{
			OI:           true,
			Funding:      true,
			LongShort:    true,
			FearGreed:    true,
			Liquidations: false,
		},
	}, true, SymbolRuntimeBuildDeps{
		Liquidations: provider,
		LiqSource:    provider,
	})

	if fetcher.RequireLiquidations {
		t.Fatal("RequireLiquidations=true want false when config disables liquidations")
	}
	if fetcher.LiquidationsByWindow != nil {
		t.Fatalf("LiquidationsByWindow=%T want nil when config disables liquidations", fetcher.LiquidationsByWindow)
	}
	if fetcher.LiquidationSource != nil {
		t.Fatalf("LiquidationSource=%T want nil when config disables liquidations", fetcher.LiquidationSource)
	}
}

func TestBuildSnapshotFetcherInjectsLiquidationsWhenConfigEnabled(t *testing.T) {
	t.Parallel()

	provider := testRuntimeLiqProvider{}
	fetcher := buildSnapshotFetcher(config.SymbolConfig{
		Symbol: "BTCUSDT",
		Require: config.SymbolRequire{
			OI:           true,
			Funding:      true,
			LongShort:    true,
			FearGreed:    true,
			Liquidations: true,
		},
	}, true, SymbolRuntimeBuildDeps{
		Liquidations: provider,
		LiqSource:    provider,
	})

	if !fetcher.RequireLiquidations {
		t.Fatal("RequireLiquidations=false want true when config enables liquidations")
	}
	if fetcher.LiquidationsByWindow == nil {
		t.Fatal("expected LiquidationsByWindow provider when config enables liquidations")
	}
	if fetcher.LiquidationSource == nil {
		t.Fatal("expected LiquidationSource provider when config enables liquidations")
	}
}
