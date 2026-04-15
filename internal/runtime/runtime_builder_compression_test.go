package runtime

import (
	"testing"

	"brale-core/internal/config"
	"brale-core/internal/decision"
)

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
