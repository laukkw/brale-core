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
	if err := validateLLMConfig(cfg.LLM, enabled); err != nil {
		return err
	}
	requiredLimit := requiredKlineLimit(cfg)
	if cfg.KlineLimit < requiredLimit {
		return validationErrorf("kline_limit must be >= %d", requiredLimit)
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

func requiredKlineLimit(cfg SymbolConfig) int {
	trendRequired := TrendPresetRequiredBars(cfg.Intervals)
	stcRequired := 0
	if !cfg.Indicators.SkipSTC && cfg.Indicators.STCFast > 0 && cfg.Indicators.STCSlow > 0 {
		stcRequired = STCRequiredBars(cfg.Indicators.STCFast, cfg.Indicators.STCSlow)
	}
	required := max(
		EMARequiredBars(cfg.Indicators.EMAFast),
		EMARequiredBars(cfg.Indicators.EMAMid),
		EMARequiredBars(cfg.Indicators.EMASlow),
		RSIRequiredBars(cfg.Indicators.RSIPeriod),
		ATRRequiredBars(cfg.Indicators.ATRPeriod),
		BBRequiredBars(cfg.Indicators.BBPeriod),
		CHOPRequiredBars(cfg.Indicators.CHOPPeriod),
		StochRSIRequiredBars(cfg.Indicators.RSIPeriod, cfg.Indicators.StochRSIPeriod),
		AroonRequiredBars(cfg.Indicators.AroonPeriod),
		stcRequired,
		trendRequired,
	)
	return max(1, required)
}
