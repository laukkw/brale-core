package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultStrategyConfigUsesLLMRiskMode(t *testing.T) {
	defaults := DefaultStrategyConfig("BTCUSDT")
	if defaults.RiskManagement.RiskStrategy.Mode != "llm" {
		t.Fatalf("risk strategy mode=%q, want llm", defaults.RiskManagement.RiskStrategy.Mode)
	}
}

func TestApplyStrategyDefaultsFillsMissingRiskMode(t *testing.T) {
	defaults := DefaultStrategyConfig("BTCUSDT")
	cfg := &StrategyConfig{}

	ApplyStrategyDefaults(cfg, defaults)

	if cfg.RiskManagement.RiskStrategy.Mode != "llm" {
		t.Fatalf("risk strategy mode=%q, want llm", cfg.RiskManagement.RiskStrategy.Mode)
	}
	if cfg.RiskManagement.EntryMode != defaults.RiskManagement.EntryMode {
		t.Fatalf("entry mode=%q, want %q", cfg.RiskManagement.EntryMode, defaults.RiskManagement.EntryMode)
	}
	if cfg.RiskManagement.Gate.QualityThreshold != defaults.RiskManagement.Gate.QualityThreshold {
		t.Fatalf("quality threshold=%v, want %v", cfg.RiskManagement.Gate.QualityThreshold, defaults.RiskManagement.Gate.QualityThreshold)
	}
	if cfg.RiskManagement.Gate.EdgeThreshold != defaults.RiskManagement.Gate.EdgeThreshold {
		t.Fatalf("edge threshold=%v, want %v", cfg.RiskManagement.Gate.EdgeThreshold, defaults.RiskManagement.Gate.EdgeThreshold)
	}
}

func TestLoadStrategyConfigDefaultsRiskModeToLLMWhenOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strategy.toml")
	data := []byte(strings.Join([]string{
		`symbol = "BTCUSDT"`,
		`id = "btc-default"`,
		`rule_chain = "configs/rules/default.json"`,
		``,
		`[risk_management]`,
		`risk_per_trade_pct = 0.01`,
		`max_invest_pct = 1.0`,
		`max_leverage = 3.0`,
		`grade_1_factor = 0.3`,
		`grade_2_factor = 0.6`,
		`grade_3_factor = 1.0`,
		`entry_mode = "orderbook"`,
		`orderbook_depth = 5`,
		`entry_offset_atr = 0.1`,
		`breakeven_fee_pct = 0.0`,
		`slippage_buffer_pct = 0.0005`,
		``,
		`[risk_management.initial_exit]`,
		`policy = "atr_structure_v1"`,
		`structure_interval = "auto"`,
		``,
		`[risk_management.initial_exit.params]`,
		`stop_atr_multiplier = 2.0`,
		`stop_min_distance_pct = 0.005`,
		`take_profit_rr = [1.5, 3.0]`,
		``,
		`[risk_management.tighten_atr]`,
		`structure_threatened = 0.5`,
		`tp1_atr = 0.0`,
		`tp2_atr = 0.0`,
		`min_tp_distance_pct = 0.0`,
		`min_tp_gap_pct = 0.0`,
		`min_update_interval_sec = 300`,
		``,
		`[risk_management.sieve]`,
		`min_size_factor = 0.1`,
		`default_gate_action = "ALLOW"`,
		`default_size_factor = 1.0`,
	}, "\n"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write strategy.toml: %v", err)
	}

	cfg, err := LoadStrategyConfig(path)
	if err != nil {
		t.Fatalf("load strategy config: %v", err)
	}
	if cfg.RiskManagement.RiskStrategy.Mode != "llm" {
		t.Fatalf("loaded risk strategy mode=%q, want llm", cfg.RiskManagement.RiskStrategy.Mode)
	}
	if cfg.RiskManagement.Gate.QualityThreshold != 0.35 {
		t.Fatalf("loaded quality threshold=%v, want 0.35", cfg.RiskManagement.Gate.QualityThreshold)
	}
	if cfg.RiskManagement.Gate.EdgeThreshold != 0.10 {
		t.Fatalf("loaded edge threshold=%v, want 0.10", cfg.RiskManagement.Gate.EdgeThreshold)
	}
}

func TestValidateStrategyConfigRejectsUnknownRiskMode(t *testing.T) {
	cfg := DefaultStrategyConfig("BTCUSDT")
	cfg.RiskManagement.RiskStrategy.Mode = "auto"

	err := ValidateStrategyConfig(cfg)
	if err == nil {
		t.Fatalf("expected validation error for unknown risk mode")
	}
	if !strings.Contains(err.Error(), "risk_management.risk_strategy.mode") {
		t.Fatalf("error=%v, want risk strategy field", err)
	}
}

func TestValidateStrategyConfigRejectsGateThresholdOutsideRange(t *testing.T) {
	cfg := DefaultStrategyConfig("BTCUSDT")
	cfg.RiskManagement.Gate.QualityThreshold = 1.1

	err := ValidateStrategyConfig(cfg)
	if err == nil {
		t.Fatalf("expected validation error for invalid gate quality threshold")
	}
	if !strings.Contains(err.Error(), "risk_management.gate.quality_threshold") {
		t.Fatalf("error=%v, want gate quality threshold field", err)
	}
}

func TestDefaultStrategyConfigSetsGateThresholdDefaults(t *testing.T) {
	defaults := DefaultStrategyConfig("BTCUSDT")
	if defaults.RiskManagement.Gate.QualityThreshold != 0.35 {
		t.Fatalf("quality threshold=%v, want 0.35", defaults.RiskManagement.Gate.QualityThreshold)
	}
	if defaults.RiskManagement.Gate.EdgeThreshold != 0.10 {
		t.Fatalf("edge threshold=%v, want 0.10", defaults.RiskManagement.Gate.EdgeThreshold)
	}
}

func TestDefaultConfigsNormalizeInputSymbol(t *testing.T) {
	symbolCfg, err := DefaultSymbolConfig(SystemConfig{}, " btc ")
	if err != nil {
		t.Fatalf("default symbol config: %v", err)
	}
	if symbolCfg.Symbol != "BTCUSDT" {
		t.Fatalf("default symbol config symbol=%q, want BTCUSDT", symbolCfg.Symbol)
	}

	strategyCfg := DefaultStrategyConfig("eth/usdt:usdt")
	if strategyCfg.Symbol != "ETHUSDT" {
		t.Fatalf("default strategy config symbol=%q, want ETHUSDT", strategyCfg.Symbol)
	}
}

func TestApplyDecisionDefaultsFillsNewIndicatorConfigFields(t *testing.T) {
	defaults, err := DefaultSymbolConfig(SystemConfig{}, "BTCUSDT")
	if err != nil {
		t.Fatalf("DefaultSymbolConfig() error = %v", err)
	}

	cfg := &SymbolConfig{
		Symbol: "BTCUSDT",
		Indicators: IndicatorConfig{
			EMAFast:   21,
			EMAMid:    50,
			EMASlow:   200,
			RSIPeriod: 14,
			ATRPeriod: 14,
			STCFast:   23,
			STCSlow:   50,
			LastN:     5,
		},
	}

	ApplyDecisionDefaults(cfg, defaults)

	if cfg.Indicators.BBPeriod != defaults.Indicators.BBPeriod {
		t.Fatalf("BBPeriod=%d want %d", cfg.Indicators.BBPeriod, defaults.Indicators.BBPeriod)
	}
	if cfg.Indicators.BBMultiplier != defaults.Indicators.BBMultiplier {
		t.Fatalf("BBMultiplier=%v want %v", cfg.Indicators.BBMultiplier, defaults.Indicators.BBMultiplier)
	}
	if cfg.Indicators.CHOPPeriod != defaults.Indicators.CHOPPeriod {
		t.Fatalf("CHOPPeriod=%d want %d", cfg.Indicators.CHOPPeriod, defaults.Indicators.CHOPPeriod)
	}
	if cfg.Indicators.StochRSIPeriod != defaults.Indicators.StochRSIPeriod {
		t.Fatalf("StochRSIPeriod=%d want %d", cfg.Indicators.StochRSIPeriod, defaults.Indicators.StochRSIPeriod)
	}
	if cfg.Indicators.AroonPeriod != defaults.Indicators.AroonPeriod {
		t.Fatalf("AroonPeriod=%d want %d", cfg.Indicators.AroonPeriod, defaults.Indicators.AroonPeriod)
	}
	if cfg.Indicators.Engine != defaults.Indicators.Engine {
		t.Fatalf("Engine=%q want %q", cfg.Indicators.Engine, defaults.Indicators.Engine)
	}
	if cfg.Indicators.ShadowEngine != "" {
		t.Fatalf("ShadowEngine=%q want empty", cfg.Indicators.ShadowEngine)
	}
	if cfg.Memory.Enabled {
		t.Fatalf("Memory.Enabled=%v want false", cfg.Memory.Enabled)
	}
	if cfg.Memory.WorkingMemorySize != defaults.Memory.WorkingMemorySize {
		t.Fatalf("WorkingMemorySize=%d want %d", cfg.Memory.WorkingMemorySize, defaults.Memory.WorkingMemorySize)
	}
}

func TestLoadSymbolConfigBackfillsNewIndicatorFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbol.toml")
	data := []byte(strings.Join([]string{
		`symbol = "BTCUSDT"`,
		`intervals = ["1h"]`,
		``,
		`[agent]`,
		`indicator = true`,
		`structure = false`,
		`mechanics = false`,
		``,
		`[indicators]`,
		`ema_fast = 21`,
		`ema_mid = 50`,
		`ema_slow = 200`,
		`rsi_period = 14`,
		`atr_period = 14`,
		`stc_fast = 23`,
		`stc_slow = 50`,
		`last_n = 5`,
		``,
		`[llm.agent.indicator]`,
		`model = "test-model"`,
		`temperature = 0.1`,
		``,
		`[llm.provider.indicator]`,
		`model = "test-model"`,
		`temperature = 0.1`,
	}, "\n"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write symbol.toml: %v", err)
	}

	cfg, err := LoadSymbolConfig(path)
	if err != nil {
		t.Fatalf("LoadSymbolConfig() error = %v", err)
	}
	if cfg.Indicators.BBPeriod != 20 {
		t.Fatalf("BBPeriod=%d want 20", cfg.Indicators.BBPeriod)
	}
	if cfg.Indicators.BBMultiplier != 2.0 {
		t.Fatalf("BBMultiplier=%v want 2.0", cfg.Indicators.BBMultiplier)
	}
	if cfg.Indicators.CHOPPeriod != 14 {
		t.Fatalf("CHOPPeriod=%d want 14", cfg.Indicators.CHOPPeriod)
	}
	if cfg.Indicators.StochRSIPeriod != 14 {
		t.Fatalf("StochRSIPeriod=%d want 14", cfg.Indicators.StochRSIPeriod)
	}
	if cfg.Indicators.AroonPeriod != 25 {
		t.Fatalf("AroonPeriod=%d want 25", cfg.Indicators.AroonPeriod)
	}
	if cfg.Indicators.Engine != IndicatorEngineTA {
		t.Fatalf("Engine=%q want %q", cfg.Indicators.Engine, IndicatorEngineTA)
	}
}

func TestDefaultAgentStructurePromptExplainsIdxOrdering(t *testing.T) {
	if !strings.Contains(defaultAgentStructurePrompt, "idx 越小越早，idx 越大越晚") {
		t.Fatalf("defaultAgentStructurePrompt should explain idx ordering")
	}
	if !strings.Contains(defaultAgentStructurePrompt, "level_idx 表示被突破的关键位") {
		t.Fatalf("defaultAgentStructurePrompt should explain level_idx")
	}
}
