package config

import "testing"

func TestValidateSymbolConfig_DoesNotRequireMACDFields(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 0.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	if err := ValidateSymbolConfig(cfg); err != nil {
		t.Fatalf("ValidateSymbolConfig() error = %v", err)
	}
}

func TestValidateSymbolConfig_RejectsTemperatureAboveTwo(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 2.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() expected error")
	}
	if got := err.Error(); got != "llm.agent.indicator.temperature must be <= 2" {
		t.Fatalf("ValidateSymbolConfig() error = %q", got)
	}
}

func TestValidateSymbolConfig_RejectsUnsupportedIndicatorEngine(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 0.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			Engine:         "mystery",
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() error = nil, want unsupported engine error")
	}
	if got := err.Error(); got != "indicators.engine must be one of [ta reference talib]" {
		t.Fatalf("ValidateSymbolConfig() error = %q", got)
	}
}

func TestValidateSymbolConfig_RejectsMatchingShadowIndicatorEngine(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 0.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			Engine:         IndicatorEngineTalib,
			ShadowEngine:   "TALIB",
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() error = nil, want shadow engine mismatch error")
	}
	if got := err.Error(); got != "indicators.shadow_engine must differ from indicators.engine" {
		t.Fatalf("ValidateSymbolConfig() error = %q", got)
	}
}

func TestValidateSymbolConfig_RejectsOversizedWorkingMemory(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 0.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			Engine:         IndicatorEngineTalib,
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		Memory: MemoryConfig{
			Enabled:           true,
			WorkingMemorySize: 6,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() error = nil, want working memory size error")
	}
	if got := err.Error(); got != "memory.working_memory_size must be in [1,5]" {
		t.Fatalf("ValidateSymbolConfig() error = %q", got)
	}
}

func TestValidateSymbolConfig_RejectsNonPositiveBBMultiplier(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 0.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       20,
			BBMultiplier:   0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() expected error")
	}
	if got := err.Error(); got != "indicators.bb_multiplier must be > 0" {
		t.Fatalf("ValidateSymbolConfig() error = %q", got)
	}
}

func TestValidateSymbolConfig_RejectsBBPeriodOne(t *testing.T) {
	indicatorEnabled := true
	structureEnabled := false
	mechanicsEnabled := false
	temp := 0.1

	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 250,
		Agent: AgentConfig{
			Indicator: &indicatorEnabled,
			Structure: &structureEnabled,
			Mechanics: &mechanicsEnabled,
		},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			BBPeriod:       1,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
			SkipSTC:        true,
		},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "test-model", Temperature: &temp},
			},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() expected error")
	}
	if got := err.Error(); got != "indicators.bb_period must be > 1" {
		t.Fatalf("ValidateSymbolConfig() error = %q", got)
	}
}
