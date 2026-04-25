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
	plan := config.ResolveFeaturePlan(symbolCfg)
	fetcher := binance.NewSnapshotFetcher(binance.SnapshotOptions{
		RequireOI:            requireMechanics && plan.Mechanics.OI && symbolCfg.Require.OI,
		RequireFunding:       requireMechanics && plan.Mechanics.Funding && symbolCfg.Require.Funding,
		RequireLongShort:     requireMechanics && plan.Mechanics.LongShort && symbolCfg.Require.LongShort,
		RequireFearGreed:     requireMechanics && plan.Mechanics.FearGreed && symbolCfg.Require.FearGreed,
		RequireLiquidations:  requireMechanics && plan.Mechanics.Liquidations && symbolCfg.Require.Liquidations,
		LiquidationsByWindow: deps.Liquidations,
		LiquidationSource:    deps.LiqSource,
	})
	fetcher.MinKlineBars = config.RequiredKlineLimit(symbolCfg)
	if !requireMechanics || !plan.Mechanics.OI {
		fetcher.OI = nil
		fetcher.RequireOI = false
	}
	if !requireMechanics || !plan.Mechanics.Funding {
		fetcher.Funding = nil
		fetcher.RequireFunding = false
	}
	if !requireMechanics || !plan.Mechanics.LongShort {
		fetcher.LongShort = nil
		fetcher.RequireLongShort = false
	}
	if !requireMechanics || !plan.Mechanics.FearGreed {
		fetcher.FearGreed = nil
		fetcher.RequireFearGreed = false
	}
	if !requireMechanics || !plan.Mechanics.Liquidations {
		fetcher.Liquidations = nil
		fetcher.LiquidationsByWindow = nil
		fetcher.LiquidationSource = nil
		fetcher.RequireLiquidations = false
	}
	return fetcher
}

func buildMetricsService(symbolCfg config.SymbolConfig, enabled config.AgentEnabled, plan config.ResolvedFeaturePlan) *market.MetricsService {
	if !enabled.Mechanics || len(symbolCfg.Intervals) == 0 {
		return nil
	}
	if !plan.Mechanics.OI && !plan.Mechanics.Sentiment {
		return nil
	}
	svc, err := market.NewMetricsService(binance.NewFuturesMarket(), []string{symbolCfg.Symbol}, symbolCfg.Intervals)
	if err != nil || svc == nil {
		return nil
	}
	return svc
}

func buildSentimentService(enabled config.AgentEnabled, plan config.ResolvedFeaturePlan, metrics *market.MetricsService) *market.SentimentService {
	if !enabled.Mechanics || !plan.Mechanics.Sentiment || metrics == nil {
		return nil
	}
	return market.NewSentimentService(nil, binance.NewFuturesMarket(), metrics)
}

func buildCompressor(symbolCfg config.SymbolConfig, enabled config.AgentEnabled, enabledMap map[string]decision.AgentEnabled) (decision.Compressor, []RuntimeService, error) {
	primaryEngine := config.NormalizeIndicatorEngine(symbolCfg.Indicators.Engine)
	computer, err := decision.IndicatorComputerForEngine(primaryEngine)
	if err != nil {
		return nil, nil, fmt.Errorf("indicator engine: %w", err)
	}
	plan := config.ResolveFeaturePlan(symbolCfg)
	metricsSvc := buildMetricsService(symbolCfg, enabled, plan)
	sentimentSvc := buildSentimentService(enabled, plan, metricsSvc)
	services := make([]RuntimeService, 0, 1)
	if metricsSvc != nil {
		services = append(services, newMetricsRuntimeService(metricsSvc, symbolCfg.Symbol))
	}
	trendPresets := config.TrendPresetForIntervals(symbolCfg.Intervals)
	trendOptions := make(map[string]decision.TrendCompressOptions, len(trendPresets))
	for iv, preset := range trendPresets {
		trendOptions[iv] = toTrendOptionsFromPreset(preset, plan, enabled.Structure)
	}
	defaultPreset := config.DefaultTrendPreset()
	indicatorOptions := toIndicatorOptions(symbolCfg.Indicators, plan, enabled.Indicator)
	defaultTrendOptions := toTrendOptionsFromPreset(defaultPreset, plan, enabled.Structure)
	base := &decision.FeatureCompressor{
		Indicators: decision.ConditionalIndicatorBuilder{
			Enabled: enabledMap,
			EnabledBuilder: decision.DefaultIndicatorBuilder{
				Options:  indicatorOptions,
				Computer: computer,
			},
		},
		Trends: decision.ConditionalTrendBuilder{
			Enabled: enabledMap,
			EnabledBuilder: decision.IntervalTrendBuilder{
				OptionsByInterval: trendOptions,
				DefaultOptions:    defaultTrendOptions,
				Computer:          computer,
			},
		},
		Mechanics: decision.ConditionalMechanicsBuilder{
			Enabled: enabledMap,
			EnabledBuilder: decision.DefaultMechanicsBuilder{Options: decision.MechanicsCompressOptions{
				Metrics:              metricsSvc,
				Sentiment:            sentimentSvc,
				FeatureFlagsExplicit: true,
				EnableOI:             plan.Mechanics.OI,
				EnableFunding:        plan.Mechanics.Funding,
				EnableLongShort:      plan.Mechanics.LongShort,
				EnableFearGreed:      plan.Mechanics.FearGreed,
				EnableLiquidations:   plan.Mechanics.Liquidations,
				EnableCVD:            plan.Mechanics.CVD,
				EnableSentiment:      plan.Mechanics.Sentiment,
				EnableFutSentiment:   plan.Mechanics.FuturesSentiment,
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

func toIndicatorOptions(cfg config.IndicatorConfig, plan config.ResolvedFeaturePlan, indicatorEnabled bool) decision.IndicatorCompressOptions {
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
	}
	if !indicatorEnabled {
		opts.SkipEMA = true
		opts.SkipRSI = true
		opts.SkipATR = true
		opts.SkipOBV = true
		opts.SkipSTC = true
		opts.SkipBB = true
		opts.SkipCHOP = true
		opts.SkipStochRSI = true
		opts.SkipAroon = true
		opts.SkipTDSequential = true
		return opts
	}
	opts.SkipEMA = !plan.Indicator.EMA
	opts.SkipRSI = !plan.Indicator.RSI
	opts.SkipATR = !plan.Indicator.ATR
	opts.SkipOBV = !plan.Indicator.OBV
	opts.SkipSTC = cfg.SkipSTC || !plan.Indicator.STC
	opts.SkipBB = !plan.Indicator.BB
	opts.SkipCHOP = !plan.Indicator.CHOP
	opts.SkipStochRSI = !plan.Indicator.StochRSI
	opts.SkipAroon = !plan.Indicator.Aroon
	opts.SkipTDSequential = !plan.Indicator.TDSequential
	return opts
}

func toTrendOptionsFromPreset(preset config.TrendPreset, plan config.ResolvedFeaturePlan, structureEnabled bool) decision.TrendCompressOptions {
	return decision.TrendCompressOptions{
		FractalSpan:          preset.FractalSpan,
		MaxStructurePoints:   preset.MaxStructurePoints,
		DedupDistanceBars:    preset.DedupDistanceBars,
		DedupATRFactor:       preset.DedupATRFactor,
		SuperTrendPeriod:     preset.SuperTrendPeriod,
		SuperTrendMultiplier: preset.SuperTrendMultiplier,
		SkipSuperTrend:       preset.SkipSuperTrend || !structureEnabled || !plan.Structure.Supertrend,
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
		IncludeCurrentRSI:    structureEnabled && plan.Structure.RSIContext,
		IncludeStructureRSI:  structureEnabled && plan.Structure.RSIContext,
		EmitEMAContext:       structureEnabled && plan.Structure.EMAContext,
		EmitPatterns:         structureEnabled && plan.Structure.Patterns,
		EmitSMC:              structureEnabled && plan.Structure.SMC,
	}
}
