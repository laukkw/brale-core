package runtime

import (
	"context"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/market"
	"brale-core/internal/market/binance"
	"brale-core/internal/snapshot"
)

func buildSnapshotFetcher(symbolCfg config.SymbolConfig, requireMechanics bool) *snapshot.Fetcher {
	fetcher := binance.NewSnapshotFetcher(binance.SnapshotOptions{
		RequireOI:           requireMechanics && symbolCfg.Require.OI,
		RequireFunding:      requireMechanics && symbolCfg.Require.Funding,
		RequireLongShort:    requireMechanics && symbolCfg.Require.LongShort,
		RequireFearGreed:    requireMechanics && symbolCfg.Require.FearGreed,
		RequireLiquidations: requireMechanics && symbolCfg.Require.Liquidations,
	})
	if requireMechanics {
		return fetcher
	}
	fetcher.OI = nil
	fetcher.Funding = nil
	fetcher.LongShort = nil
	fetcher.FearGreed = nil
	fetcher.Liquidations = nil
	fetcher.RequireOI = false
	fetcher.RequireFunding = false
	fetcher.RequireLongShort = false
	fetcher.RequireFearGreed = false
	fetcher.RequireLiquidations = false
	return fetcher
}

func buildMetricsService(metricsCtx context.Context, symbolCfg config.SymbolConfig, enabled config.AgentEnabled) *market.MetricsService {
	if !enabled.Mechanics || len(symbolCfg.Intervals) == 0 {
		return nil
	}
	svc, err := market.NewMetricsService(binance.NewFuturesMarket(), []string{symbolCfg.Symbol}, symbolCfg.Intervals)
	if err != nil || svc == nil {
		return nil
	}
	if metricsCtx == nil {
		metricsCtx = context.Background()
	}
	go svc.Start(metricsCtx)
	svc.RefreshSymbol(metricsCtx, symbolCfg.Symbol)
	return svc
}

func buildCompressor(metricsCtx context.Context, symbolCfg config.SymbolConfig, enabled config.AgentEnabled, enabledMap map[string]decision.AgentEnabled) *decision.FeatureCompressor {
	metricsSvc := buildMetricsService(metricsCtx, symbolCfg, enabled)
	trendPresets := config.TrendPresetForIntervals(symbolCfg.Intervals)
	trendOptions := make(map[string]decision.TrendCompressOptions, len(trendPresets))
	for iv, preset := range trendPresets {
		trendOptions[iv] = toTrendOptionsFromPreset(preset)
	}
	defaultPreset := config.DefaultTrendPreset()
	return &decision.FeatureCompressor{
		Indicators: decision.DefaultIndicatorBuilder{Options: toIndicatorOptions(symbolCfg.Indicators, enabled.Indicator)},
		Trends: decision.IntervalTrendBuilder{
			OptionsByInterval: trendOptions,
			DefaultOptions:    toTrendOptionsFromPreset(defaultPreset),
		},
		Mechanics: decision.ConditionalMechanicsBuilder{
			Enabled: enabledMap,
			EnabledBuilder: decision.DefaultMechanicsBuilder{Options: decision.MechanicsCompressOptions{
				Metrics: metricsSvc,
			}},
		},
	}
}

func toIndicatorOptions(cfg config.IndicatorConfig, indicatorEnabled bool) decision.IndicatorCompressOptions {
	opts := decision.IndicatorCompressOptions{
		EMAFast:    cfg.EMAFast,
		EMAMid:     cfg.EMAMid,
		EMASlow:    cfg.EMASlow,
		RSIPeriod:  cfg.RSIPeriod,
		ATRPeriod:  cfg.ATRPeriod,
		MACDFast:   cfg.MACDFast,
		MACDSlow:   cfg.MACDSlow,
		MACDSignal: cfg.MACDSignal,
		LastN:      cfg.LastN,
		Pretty:     cfg.Pretty,
	}
	if !indicatorEnabled {
		opts.SkipEMA = true
		opts.SkipRSI = true
		opts.SkipMACD = true
	}
	return opts
}

func toTrendOptionsFromPreset(preset config.TrendPreset) decision.TrendCompressOptions {
	return decision.TrendCompressOptions{
		FractalSpan:         preset.FractalSpan,
		MaxStructurePoints:  preset.MaxStructurePoints,
		DedupDistanceBars:   preset.DedupDistanceBars,
		DedupATRFactor:      preset.DedupATRFactor,
		RSIPeriod:           preset.RSIPeriod,
		ATRPeriod:           preset.ATRPeriod,
		RecentCandles:       preset.RecentCandles,
		VolumeMAPeriod:      preset.VolumeMAPeriod,
		EMA20Period:         preset.EMA20Period,
		EMA50Period:         preset.EMA50Period,
		EMA200Period:        preset.EMA200Period,
		PatternMinScore:     preset.PatternMinScore,
		PatternMaxDetected:  preset.PatternMaxDetected,
		Pretty:              preset.Pretty,
		IncludeCurrentRSI:   preset.IncludeCurrentRSI,
		IncludeStructureRSI: preset.IncludeStructureRSI,
	}
}
