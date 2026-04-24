package config

type ResolvedFeaturePlan struct {
	Indicator ResolvedIndicatorFeatures
	Structure ResolvedStructureFeatures
	Mechanics ResolvedMechanicsFeatures
}

type ResolvedIndicatorFeatures struct {
	EMA          bool
	RSI          bool
	ATR          bool
	OBV          bool
	STC          bool
	BB           bool
	CHOP         bool
	StochRSI     bool
	Aroon        bool
	TDSequential bool
}

type ResolvedStructureFeatures struct {
	Supertrend bool
	EMAContext bool
	RSIContext bool
	Patterns   bool
	SMC        bool
}

type ResolvedMechanicsFeatures struct {
	OI               bool
	Funding          bool
	LongShort        bool
	FearGreed        bool
	Liquidations     bool
	CVD              bool
	Sentiment        bool
	FuturesSentiment bool
}

func ResolveFeaturePlan(cfg SymbolConfig) ResolvedFeaturePlan {
	stcDefault := !cfg.Indicators.SkipSTC
	return ResolvedFeaturePlan{
		Indicator: ResolvedIndicatorFeatures{
			EMA:          featureBool(cfg.Features.Indicator.EMA, true),
			RSI:          featureBool(cfg.Features.Indicator.RSI, true),
			ATR:          featureBool(cfg.Features.Indicator.ATR, true),
			OBV:          featureBool(cfg.Features.Indicator.OBV, true),
			STC:          featureBool(cfg.Features.Indicator.STC, stcDefault),
			BB:           featureBool(cfg.Features.Indicator.BB, true),
			CHOP:         featureBool(cfg.Features.Indicator.CHOP, true),
			StochRSI:     featureBool(cfg.Features.Indicator.StochRSI, true),
			Aroon:        featureBool(cfg.Features.Indicator.Aroon, true),
			TDSequential: featureBool(cfg.Features.Indicator.TDSequential, true),
		},
		Structure: ResolvedStructureFeatures{
			Supertrend: featureBool(cfg.Features.Structure.Supertrend, true),
			EMAContext: featureBool(cfg.Features.Structure.EMAContext, true),
			RSIContext: featureBool(cfg.Features.Structure.RSIContext, true),
			Patterns:   featureBool(cfg.Features.Structure.Patterns, true),
			SMC:        featureBool(cfg.Features.Structure.SMC, true),
		},
		Mechanics: ResolvedMechanicsFeatures{
			OI:               featureBool(cfg.Features.Mechanics.OI, cfg.Require.OI),
			Funding:          featureBool(cfg.Features.Mechanics.Funding, cfg.Require.Funding),
			LongShort:        featureBool(cfg.Features.Mechanics.LongShort, cfg.Require.LongShort),
			FearGreed:        featureBool(cfg.Features.Mechanics.FearGreed, cfg.Require.FearGreed),
			Liquidations:     featureBool(cfg.Features.Mechanics.Liquidations, cfg.Require.Liquidations),
			CVD:              featureBool(cfg.Features.Mechanics.CVD, true),
			Sentiment:        featureBool(cfg.Features.Mechanics.Sentiment, true),
			FuturesSentiment: featureBool(cfg.Features.Mechanics.FuturesSentiment, true),
		},
	}
}

func RequiredKlineLimitForFeatures(cfg SymbolConfig, enabled AgentEnabled, plan ResolvedFeaturePlan) int {
	required := 1
	if enabled.Indicator {
		required = max(required, indicatorRequiredBarsForFeatures(cfg, plan.Indicator))
	}
	if enabled.Structure {
		required = max(required, trendRequiredBarsForFeatures(cfg, plan.Structure))
	}
	if enabled.Mechanics {
		required = max(required, mechanicsRequiredBarsForFeatures(plan.Mechanics))
	}
	return max(1, required)
}

func indicatorRequiredBarsForFeatures(cfg SymbolConfig, plan ResolvedIndicatorFeatures) int {
	required := 1
	if plan.EMA {
		required = max(required,
			EMARequiredBars(cfg.Indicators.EMAFast),
			EMARequiredBars(cfg.Indicators.EMAMid),
			EMARequiredBars(cfg.Indicators.EMASlow),
		)
	}
	if plan.RSI {
		required = max(required, RSIRequiredBars(cfg.Indicators.RSIPeriod))
	}
	if plan.ATR {
		required = max(required, ATRRequiredBars(cfg.Indicators.ATRPeriod))
	}
	if plan.OBV {
		required = max(required, 2)
	}
	if plan.STC {
		required = max(required, STCRequiredBars(cfg.Indicators.STCFast, cfg.Indicators.STCSlow))
	}
	if plan.BB {
		required = max(required, BBRequiredBars(cfg.Indicators.BBPeriod))
	}
	if plan.CHOP {
		required = max(required, CHOPRequiredBars(cfg.Indicators.CHOPPeriod))
	}
	if plan.StochRSI {
		required = max(required, StochRSIRequiredBars(cfg.Indicators.RSIPeriod, cfg.Indicators.StochRSIPeriod))
	}
	if plan.Aroon {
		required = max(required, AroonRequiredBars(cfg.Indicators.AroonPeriod))
	}
	if plan.TDSequential {
		required = max(required, 5)
	}
	return max(1, required)
}

func trendRequiredBarsForFeatures(cfg SymbolConfig, plan ResolvedStructureFeatures) int {
	maxRequired := 1
	presets := TrendPresetForIntervals(cfg.Intervals)
	for _, preset := range presets {
		required := max(
			preset.RecentCandles,
			preset.FractalSpan*2+1,
			preset.VolumeMAPeriod,
		)
		if plan.RSIContext {
			required = max(required, RSIRequiredBars(preset.RSIPeriod))
		}
		if plan.Supertrend || plan.Patterns || plan.SMC {
			required = max(required, ATRRequiredBars(preset.ATRPeriod))
		}
		if plan.Supertrend && !preset.SkipSuperTrend && preset.SuperTrendPeriod > 0 {
			required = max(required, SuperTrendRequiredBars(preset.SuperTrendPeriod, preset.SuperTrendMultiplier))
		}
		if plan.EMAContext {
			required = max(required,
				EMARequiredBars(preset.EMA20Period),
				EMARequiredBars(preset.EMA50Period),
				EMARequiredBars(preset.EMA200Period),
			)
		}
		if required > maxRequired {
			maxRequired = required
		}
	}
	return max(1, maxRequired)
}

func mechanicsRequiredBarsForFeatures(plan ResolvedMechanicsFeatures) int {
	required := 1
	if plan.CVD || plan.FuturesSentiment {
		required = max(required, 2)
	}
	if plan.Sentiment {
		required = max(required, 20)
	}
	return max(1, required)
}

func featureBool(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}
