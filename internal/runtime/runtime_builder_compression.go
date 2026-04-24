package runtime

import (
	"fmt"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/market"
	"brale-core/internal/market/binance"
	"brale-core/internal/snapshot"
)

func buildSnapshotFetcher(symbolCfg config.SymbolConfig, requireMechanics bool, deps SymbolRuntimeBuildDeps) *snapshot.Fetcher {
	liquidationsByWindow := deps.Liquidations
	liquidationSource := deps.LiqSource
	if !requireMechanics || !symbolCfg.Require.Liquidations {
		liquidationsByWindow = nil
		liquidationSource = nil
	}
	fetcher := binance.NewSnapshotFetcher(binance.SnapshotOptions{
		RequireOI:            requireMechanics && symbolCfg.Require.OI,
		RequireFunding:       requireMechanics && symbolCfg.Require.Funding,
		RequireLongShort:     requireMechanics && symbolCfg.Require.LongShort,
		RequireFearGreed:     requireMechanics && symbolCfg.Require.FearGreed,
		RequireLiquidations:  requireMechanics && symbolCfg.Require.Liquidations,
		LiquidationsByWindow: liquidationsByWindow,
		LiquidationSource:    liquidationSource,
	})
	fetcher.MinKlineBars = config.RequiredKlineLimit(symbolCfg)
	if requireMechanics {
		return fetcher
	}
	fetcher.OI = nil
	fetcher.Funding = nil
	fetcher.LongShort = nil
	fetcher.FearGreed = nil
	fetcher.Liquidations = nil
	fetcher.LiquidationsByWindow = nil
	fetcher.LiquidationSource = nil
	fetcher.RequireOI = false
	fetcher.RequireFunding = false
	fetcher.RequireLongShort = false
	fetcher.RequireFearGreed = false
	fetcher.RequireLiquidations = false
	return fetcher
}

func buildMetricsService(symbolCfg config.SymbolConfig, enabled config.AgentEnabled) *market.MetricsService {
	if !enabled.Mechanics || len(symbolCfg.Intervals) == 0 {
		return nil
	}
	svc, err := market.NewMetricsService(binance.NewFuturesMarket(), []string{symbolCfg.Symbol}, symbolCfg.Intervals)
	if err != nil || svc == nil {
		return nil
	}
	return svc
}

func buildCompressor(symbolCfg config.SymbolConfig, enabled config.AgentEnabled, enabledMap map[string]decision.AgentEnabled) (decision.Compressor, []RuntimeService, error) {
	primaryEngine := config.NormalizeIndicatorEngine(symbolCfg.Indicators.Engine)
	computer, err := decision.IndicatorComputerForEngine(primaryEngine)
	if err != nil {
		return nil, nil, fmt.Errorf("indicator engine: %w", err)
	}
	metricsSvc := buildMetricsService(symbolCfg, enabled)
	services := make([]RuntimeService, 0, 1)
	if metricsSvc != nil {
		services = append(services, newMetricsRuntimeService(metricsSvc, symbolCfg.Symbol))
	}
	trendPresets := config.TrendPresetForIntervals(symbolCfg.Intervals)
	trendOptions := make(map[string]decision.TrendCompressOptions, len(trendPresets))
	for iv, preset := range trendPresets {
		trendOptions[iv] = toTrendOptionsFromPreset(preset)
	}
	defaultPreset := config.DefaultTrendPreset()
	indicatorOptions := toIndicatorOptions(symbolCfg.Indicators, enabled.Indicator)
	defaultTrendOptions := toTrendOptionsFromPreset(defaultPreset)
	base := &decision.FeatureCompressor{
		Indicators: decision.DefaultIndicatorBuilder{
			Options:  indicatorOptions,
			Computer: computer,
		},
		Trends: decision.IntervalTrendBuilder{
			OptionsByInterval: trendOptions,
			DefaultOptions:    defaultTrendOptions,
			Computer:          computer,
		},
		Mechanics: decision.ConditionalMechanicsBuilder{
			Enabled: enabledMap,
			EnabledBuilder: decision.DefaultMechanicsBuilder{Options: decision.MechanicsCompressOptions{
				Metrics: metricsSvc,
			}},
		},
	}
	shadowEngine := config.NormalizeOptionalIndicatorEngine(symbolCfg.Indicators.ShadowEngine)
	if shadowEngine == "" {
		return base, services, nil
	}
	shadowComputer, err := decision.IndicatorComputerForEngine(shadowEngine)
	if err != nil {
		return nil, nil, fmt.Errorf("shadow indicator engine: %w", err)
	}
	return &shadowComparingCompressor{
		primary:            base,
		primaryName:        primaryEngine,
		shadowName:         shadowEngine,
		primaryComputer:    computer,
		shadowComputer:     shadowComputer,
		indicatorOptions:   indicatorOptions,
		trendOptionsByInt:  trendOptions,
		defaultTrendOption: defaultTrendOptions,
	}, services, nil
}

func toIndicatorOptions(cfg config.IndicatorConfig, indicatorEnabled bool) decision.IndicatorCompressOptions {
	opts := decision.IndicatorCompressOptions{
		EMAFast:        cfg.EMAFast,
		EMAMid:         cfg.EMAMid,
		EMASlow:        cfg.EMASlow,
		RSIPeriod:      cfg.RSIPeriod,
		ATRPeriod:      cfg.ATRPeriod,
		STCFast:        cfg.STCFast,
		STCSlow:        cfg.STCSlow,
		BBPeriod:       cfg.BBPeriod,
		BBMultiplier:   cfg.BBMultiplier,
		CHOPPeriod:     cfg.CHOPPeriod,
		StochRSIPeriod: cfg.StochRSIPeriod,
		AroonPeriod:    cfg.AroonPeriod,
		LastN:          cfg.LastN,
		Pretty:         cfg.Pretty,
		SkipSTC:        cfg.SkipSTC,
	}
	if !indicatorEnabled {
		opts.SkipEMA = true
		opts.SkipRSI = true
		opts.SkipSTC = true
	}
	return opts
}

func toTrendOptionsFromPreset(preset config.TrendPreset) decision.TrendCompressOptions {
	return decision.TrendCompressOptions{
		FractalSpan:          preset.FractalSpan,
		MaxStructurePoints:   preset.MaxStructurePoints,
		DedupDistanceBars:    preset.DedupDistanceBars,
		DedupATRFactor:       preset.DedupATRFactor,
		SuperTrendPeriod:     preset.SuperTrendPeriod,
		SuperTrendMultiplier: preset.SuperTrendMultiplier,
		SkipSuperTrend:       preset.SkipSuperTrend,
		RSIPeriod:            preset.RSIPeriod,
		ATRPeriod:            preset.ATRPeriod,
		RecentCandles:        preset.RecentCandles,
		VolumeMAPeriod:       preset.VolumeMAPeriod,
		EMA20Period:          preset.EMA20Period,
		EMA50Period:          preset.EMA50Period,
		EMA200Period:         preset.EMA200Period,
		PatternMinScore:      preset.PatternMinScore,
		PatternMaxDetected:   preset.PatternMaxDetected,
		Pretty:               preset.Pretty,
		IncludeCurrentRSI:    preset.IncludeCurrentRSI,
		IncludeStructureRSI:  preset.IncludeStructureRSI,
	}
}
