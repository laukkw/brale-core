package config

import "strings"

func ValidateSymbolIndexConfig(cfg SymbolIndexConfig) error {
	if len(cfg.Symbols) == 0 {
		return validationErrorf("symbols is required")
	}
	seen := make(map[string]struct{}, len(cfg.Symbols))
	for i, item := range cfg.Symbols {
		sym, err := validateCanonicalSymbol("symbols.symbol", item.Symbol)
		if err != nil {
			return validationErrorf("symbols[%d]: %v", i, err)
		}
		if _, ok := seen[sym]; ok {
			return validationErrorf("symbols contains duplicate symbol=%s", sym)
		}
		seen[sym] = struct{}{}
		if strings.TrimSpace(item.Config) == "" {
			return validationErrorf("symbols.%s config path is required", sym)
		}
		if strings.TrimSpace(item.Strategy) == "" {
			return validationErrorf("symbols.%s strategy path is required", sym)
		}
	}
	return nil
}

func ValidateSymbolConfig(cfg SymbolConfig) error {
	if _, err := validateCanonicalSymbol("symbol", cfg.Symbol); err != nil {
		return err
	}
	if len(cfg.Intervals) == 0 {
		return validationErrorf("intervals is required")
	}
	if cfg.KlineLimit <= 0 {
		return validationErrorf("kline_limit must be > 0")
	}
	enabled, err := ResolveAgentEnabled(cfg.Agent)
	if err != nil {
		return err
	}
	if err := validateIndicatorConfig(cfg.Indicators); err != nil {
		return err
	}
	if err := validateMemoryConfig(cfg.Memory); err != nil {
		return err
	}
	if err := validateConsensusConfig(cfg.Consensus); err != nil {
		return err
	}
	if err := validateCooldownConfig(cfg.Cooldown); err != nil {
		return err
	}
	if err := validateFeatureConfig(cfg); err != nil {
		return err
	}
	if err := validateLLMConfig(cfg.LLM, enabled); err != nil {
		return err
	}
	requiredLimit := requiredKlineLimit(cfg)
	if cfg.KlineLimit < requiredLimit {
		return validationErrorf("kline_limit must be >= %d", requiredLimit)
	}
	return nil
}

func validateFeatureConfig(cfg SymbolConfig) error {
	plan := ResolveFeaturePlan(cfg)
	if cfg.Indicators.SkipSTC && plan.Indicator.STC {
		return validationErrorf("features.indicator.stc cannot be true when indicators.skip_stc=true")
	}
	if cfg.Require.OI && !plan.Mechanics.OI {
		return validationErrorf("require.oi=true requires features.mechanics.oi=true")
	}
	if cfg.Require.Funding && !plan.Mechanics.Funding {
		return validationErrorf("require.funding=true requires features.mechanics.funding=true")
	}
	if cfg.Require.LongShort && !plan.Mechanics.LongShort {
		return validationErrorf("require.long_short=true requires features.mechanics.long_short=true")
	}
	if cfg.Require.FearGreed && !plan.Mechanics.FearGreed {
		return validationErrorf("require.fear_greed=true requires features.mechanics.fear_greed=true")
	}
	if cfg.Require.Liquidations && !plan.Mechanics.Liquidations {
		return validationErrorf("require.liquidations=true requires features.mechanics.liquidations=true")
	}
	if plan.Indicator.StochRSI && !plan.Indicator.RSI {
		return validationErrorf("features.indicator.stoch_rsi=true requires features.indicator.rsi=true")
	}
	return nil
}

func validateIndicatorConfig(cfg IndicatorConfig) error {
	if err := ValidateIndicatorEngine(cfg.Engine); err != nil {
		return validationErrorf("indicators.engine %s", err.Error())
	}
	if shadow := NormalizeOptionalIndicatorEngine(cfg.ShadowEngine); shadow != "" {
		if err := ValidateIndicatorEngine(shadow); err != nil {
			return validationErrorf("indicators.shadow_engine %s", err.Error())
		}
		if shadow == NormalizeIndicatorEngine(cfg.Engine) {
			return validationErrorf("indicators.shadow_engine must differ from indicators.engine")
		}
	}
	if cfg.EMAFast <= 0 || cfg.EMAMid <= 0 || cfg.EMASlow <= 0 {
		return validationErrorf("indicators.ema_fast/ema_mid/ema_slow must be > 0")
	}
	if !(cfg.EMAFast < cfg.EMAMid && cfg.EMAMid < cfg.EMASlow) {
		return validationErrorf("indicators must satisfy ema_fast < ema_mid < ema_slow")
	}
	if cfg.RSIPeriod <= 0 {
		return validationErrorf("indicators.rsi_period must be > 0")
	}
	if cfg.ATRPeriod <= 0 {
		return validationErrorf("indicators.atr_period must be > 0")
	}
	if !cfg.SkipSTC && (cfg.STCFast <= 0 || cfg.STCSlow <= 0) {
		return validationErrorf("indicators.stc_fast/stc_slow must be > 0 when STC is enabled")
	}
	if !cfg.SkipSTC && cfg.STCFast >= cfg.STCSlow {
		return validationErrorf("indicators.stc_fast must be < stc_slow")
	}
	if cfg.BBPeriod <= 1 {
		return validationErrorf("indicators.bb_period must be > 1")
	}
	if cfg.BBMultiplier <= 0 {
		return validationErrorf("indicators.bb_multiplier must be > 0")
	}
	if cfg.CHOPPeriod <= 1 {
		return validationErrorf("indicators.chop_period must be > 1")
	}
	if cfg.StochRSIPeriod <= 0 {
		return validationErrorf("indicators.stoch_rsi_period must be > 0")
	}
	if cfg.AroonPeriod <= 0 {
		return validationErrorf("indicators.aroon_period must be > 0")
	}
	if cfg.LastN <= 0 {
		return validationErrorf("indicators.last_n must be > 0")
	}
	return nil
}

func validateCooldownConfig(cfg CooldownConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.EntryCooldownSec <= 0 {
		return validationErrorf("cooldown.entry_cooldown_sec must be > 0")
	}
	return nil
}

func validateMemoryConfig(cfg MemoryConfig) error {
	if cfg.WorkingMemorySize < 0 || cfg.WorkingMemorySize > MaxWorkingMemorySize {
		return validationErrorf("memory.working_memory_size must be in [1,5]")
	}
	return nil
}

func validateConsensusConfig(cfg ConsensusConfig) error {
	if cfg.ScoreThreshold < 0 || cfg.ScoreThreshold > 1 {
		return validationErrorf("consensus.score_threshold must be in [0,1]")
	}
	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return validationErrorf("consensus.confidence_threshold must be in [0,1]")
	}
	return nil
}

func validateLLMConfig(cfg SymbolLLMConfig, enabled AgentEnabled) error {
	if err := validateLLMRoleEnabled("llm.agent.indicator", cfg.Agent.Indicator, enabled.Indicator); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.agent.structure", cfg.Agent.Structure, enabled.Structure); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.agent.mechanics", cfg.Agent.Mechanics, enabled.Mechanics); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.provider.indicator", cfg.Provider.Indicator, enabled.Indicator); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.provider.structure", cfg.Provider.Structure, enabled.Structure); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.provider.mechanics", cfg.Provider.Mechanics, enabled.Mechanics); err != nil {
		return err
	}
	return nil
}

func validateLLMRoleEnabled(prefix string, cfg LLMRoleConfig, enabled bool) error {
	if !enabled {
		return nil
	}
	return validateLLMRole(prefix, cfg)
}

func validateLLMRole(prefix string, cfg LLMRoleConfig) error {
	if strings.TrimSpace(cfg.Model) == "" {
		return validationErrorf("%s.model is required", prefix)
	}
	if cfg.Temperature == nil {
		return validationErrorf("%s.temperature is required", prefix)
	}
	if *cfg.Temperature < 0 {
		return validationErrorf("%s.temperature must be >= 0", prefix)
	}
	if *cfg.Temperature > 2 {
		return validationErrorf("%s.temperature must be <= 2", prefix)
	}
	return nil
}

func ValidateSymbolLLMModels(sys SystemConfig, cfg SymbolConfig) error {
	enabled, err := ResolveAgentEnabled(cfg.Agent)
	if err != nil {
		return err
	}
	roles := []struct {
		path  string
		model string
		need  bool
	}{
		{"llm.agent.indicator", cfg.LLM.Agent.Indicator.Model, enabled.Indicator},
		{"llm.agent.structure", cfg.LLM.Agent.Structure.Model, enabled.Structure},
		{"llm.agent.mechanics", cfg.LLM.Agent.Mechanics.Model, enabled.Mechanics},
		{"llm.provider.indicator", cfg.LLM.Provider.Indicator.Model, enabled.Indicator},
		{"llm.provider.structure", cfg.LLM.Provider.Structure.Model, enabled.Structure},
		{"llm.provider.mechanics", cfg.LLM.Provider.Mechanics.Model, enabled.Mechanics},
	}
	for _, role := range roles {
		if !role.need {
			continue
		}
		model := strings.TrimSpace(role.model)
		if model == "" {
			continue
		}
		if _, ok := LookupLLMModelConfig(sys, model); !ok {
			return validationErrorf("%s.model=%s not found in system llm_models", role.path, model)
		}
	}
	return nil
}

// RequiredKlineLimit returns the minimum closed candles required by the
// configured indicator and trend warmup windows.
func RequiredKlineLimit(cfg SymbolConfig) int {
	return requiredKlineLimit(cfg)
}

func requiredKlineLimit(cfg SymbolConfig) int {
	enabled, err := ResolveAgentEnabled(cfg.Agent)
	if err != nil {
		enabled = resolveAgentEnabledForKlineLimit(cfg)
	}
	return RequiredKlineLimitForFeatures(cfg, enabled, ResolveFeaturePlan(cfg))
}

func resolveAgentEnabledForKlineLimit(cfg SymbolConfig) AgentEnabled {
	return AgentEnabled{
		Indicator: defaultAgentEnabled(cfg.Agent.Indicator, hasIndicatorWarmupConfig(cfg)),
		Structure: defaultAgentEnabled(cfg.Agent.Structure, hasStructureWarmupConfig(cfg)),
		Mechanics: defaultAgentEnabled(cfg.Agent.Mechanics, hasMechanicsWarmupConfig(cfg)),
	}
}

func defaultAgentEnabled(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func hasIndicatorWarmupConfig(cfg SymbolConfig) bool {
	if cfg.Indicators.EMAFast > 0 || cfg.Indicators.EMAMid > 0 || cfg.Indicators.EMASlow > 0 {
		return true
	}
	if cfg.Indicators.RSIPeriod > 0 || cfg.Indicators.ATRPeriod > 0 {
		return true
	}
	if cfg.Indicators.STCFast > 0 || cfg.Indicators.STCSlow > 0 {
		return true
	}
	if cfg.Indicators.BBPeriod > 0 || cfg.Indicators.BBMultiplier > 0 {
		return true
	}
	if cfg.Indicators.CHOPPeriod > 0 || cfg.Indicators.StochRSIPeriod > 0 || cfg.Indicators.AroonPeriod > 0 {
		return true
	}
	if cfg.Indicators.LastN > 0 {
		return true
	}
	return hasAnyFeaturePointer(
		cfg.Features.Indicator.EMA,
		cfg.Features.Indicator.RSI,
		cfg.Features.Indicator.ATR,
		cfg.Features.Indicator.OBV,
		cfg.Features.Indicator.STC,
		cfg.Features.Indicator.BB,
		cfg.Features.Indicator.CHOP,
		cfg.Features.Indicator.StochRSI,
		cfg.Features.Indicator.Aroon,
		cfg.Features.Indicator.TDSequential,
	)
}

func hasStructureWarmupConfig(cfg SymbolConfig) bool {
	if len(cfg.Intervals) > 0 {
		return true
	}
	return hasAnyFeaturePointer(
		cfg.Features.Structure.Supertrend,
		cfg.Features.Structure.EMAContext,
		cfg.Features.Structure.RSIContext,
		cfg.Features.Structure.Patterns,
		cfg.Features.Structure.SMC,
	)
}

func hasMechanicsWarmupConfig(cfg SymbolConfig) bool {
	if cfg.Require.OI || cfg.Require.Funding || cfg.Require.LongShort || cfg.Require.FearGreed || cfg.Require.Liquidations {
		return true
	}
	return hasAnyTruePointer(
		cfg.Features.Mechanics.OI,
		cfg.Features.Mechanics.Funding,
		cfg.Features.Mechanics.LongShort,
		cfg.Features.Mechanics.FearGreed,
		cfg.Features.Mechanics.Liquidations,
		cfg.Features.Mechanics.CVD,
		cfg.Features.Mechanics.Sentiment,
		cfg.Features.Mechanics.FuturesSentiment,
	)
}

func hasAnyFeaturePointer(values ...*bool) bool {
	for _, value := range values {
		if value != nil {
			return true
		}
	}
	return false
}

func hasAnyTruePointer(values ...*bool) bool {
	for _, value := range values {
		if value != nil && *value {
			return true
		}
	}
	return false
}
