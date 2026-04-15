package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredKlineLimit_Minimum(t *testing.T) {
	got := requiredKlineLimit(SymbolConfig{})
	if got != 1 {
		t.Fatalf("kline limit=%d, want 1", got)
	}
}

func TestValidateSymbolConfigAcceptsSTCWithoutMACD(t *testing.T) {
	temp := 0.1
	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 300,
		Agent:      AgentConfig{Indicator: boolPtr(true), Structure: boolPtr(true), Mechanics: boolPtr(true)},
		Indicators: IndicatorConfig{
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
		Consensus: ConsensusConfig{ScoreThreshold: 0.3, ConfidenceThreshold: 0.6},
		LLM: SymbolLLMConfig{
			Agent:    LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
			Provider: LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
		},
	}

	if err := ValidateSymbolConfig(cfg); err != nil {
		t.Fatalf("ValidateSymbolConfig() error = %v", err)
	}
}

func TestRequiredKlineLimitIncludesSTCWarmup(t *testing.T) {
	cfg := SymbolConfig{
		Intervals: []string{"1h"},
		Indicators: IndicatorConfig{
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
	}

	got := requiredKlineLimit(cfg)
	want := STCRequiredBars(23, 50)
	if trend := TrendPresetRequiredBars(cfg.Intervals); trend > want {
		want = trend
	}
	if got != want {
		t.Fatalf("requiredKlineLimit()=%d want %d", got, want)
	}
}

func TestRequiredKlineLimitSkipsSTCWarmupWhenDisabled(t *testing.T) {
	cfg := SymbolConfig{
		Intervals: []string{"1h"},
		Indicators: IndicatorConfig{
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
			SkipSTC:        true,
			LastN:          5,
		},
	}

	got := requiredKlineLimit(cfg)
	want := TrendPresetRequiredBars(cfg.Intervals)
	if cfg.Indicators.EMAFast > want {
		want = cfg.Indicators.EMAFast
	}
	if cfg.Indicators.EMAMid > want {
		want = cfg.Indicators.EMAMid
	}
	if cfg.Indicators.EMASlow > want {
		want = cfg.Indicators.EMASlow
	}
	if cfg.Indicators.RSIPeriod > want {
		want = cfg.Indicators.RSIPeriod
	}
	if cfg.Indicators.ATRPeriod > want {
		want = cfg.Indicators.ATRPeriod
	}
	if got != want {
		t.Fatalf("requiredKlineLimit()=%d want %d", got, want)
	}
}

func TestTrendPresetRequiredBarsIncludesSuperTrendWarmup(t *testing.T) {
	intervals := []string{"1h"}

	got := TrendPresetRequiredBars(intervals)
	preset := TrendPresetForIntervals(intervals)["1h"]
	want := SuperTrendRequiredBars(preset.SuperTrendPeriod, preset.SuperTrendMultiplier)
	if preset.EMA200Period > want {
		want = preset.EMA200Period
	}
	if preset.VolumeMAPeriod > want {
		want = preset.VolumeMAPeriod
	}
	if preset.RecentCandles > want {
		want = preset.RecentCandles
	}
	if v := preset.FractalSpan*2 + 1; v > want {
		want = v
	}
	if got != want {
		t.Fatalf("TrendPresetRequiredBars()=%d want %d", got, want)
	}
}

func TestTalibRequiredBarsHelpers(t *testing.T) {
	if got := EMARequiredBars(21); got != 21 {
		t.Fatalf("EMARequiredBars(21)=%d want 21", got)
	}
	if got := RSIRequiredBars(14); got != 15 {
		t.Fatalf("RSIRequiredBars(14)=%d want 15", got)
	}
	if got := ATRRequiredBars(14); got != 15 {
		t.Fatalf("ATRRequiredBars(14)=%d want 15", got)
	}
	if got := BBRequiredBars(20); got != 20 {
		t.Fatalf("BBRequiredBars(20)=%d want 20", got)
	}
	if got := CHOPRequiredBars(14); got != 15 {
		t.Fatalf("CHOPRequiredBars(14)=%d want 15", got)
	}
	if got := CHOPRequiredBars(1); got != 2 {
		t.Fatalf("CHOPRequiredBars(1)=%d want 2", got)
	}
	if got := StochRSIRequiredBars(14, 14); got != 28 {
		t.Fatalf("StochRSIRequiredBars(14, 14)=%d want 28", got)
	}
	if got := AroonRequiredBars(25); got != 26 {
		t.Fatalf("AroonRequiredBars(25)=%d want 26", got)
	}
}

func TestRequiredKlineLimitIncludesNewIndicatorWarmups(t *testing.T) {
	cfg := SymbolConfig{
		Intervals: []string{"1h"},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			STCFast:        23,
			STCSlow:        50,
			SkipSTC:        true,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 40,
			AroonPeriod:    25,
			LastN:          5,
		},
	}

	got := requiredKlineLimit(cfg)
	want := StochRSIRequiredBars(cfg.Indicators.RSIPeriod, cfg.Indicators.StochRSIPeriod)
	if trend := TrendPresetRequiredBars(cfg.Intervals); trend > want {
		want = trend
	}
	if got != want {
		t.Fatalf("requiredKlineLimit()=%d want %d", got, want)
	}
}

func TestValidateWebhookFallbackDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system.toml")
	data := []byte(`execution_system = "freqtrade"
exec_endpoint = "http://127.0.0.1:8080/api/v1"

[database]
dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"

[webhook]
enabled = true
addr = ":9991"
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write system.toml: %v", err)
	}

	cfg, err := LoadSystemConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Webhook.QueueSize != 1024 {
		t.Fatalf("queue_size=%d, want 1024", cfg.Webhook.QueueSize)
	}
	if cfg.Webhook.WorkerCount != 4 {
		t.Fatalf("worker_count=%d, want 4", cfg.Webhook.WorkerCount)
	}
	if cfg.Webhook.FallbackOrderPollSec != 180 {
		t.Fatalf("fallback_order_poll_sec=%d, want 180", cfg.Webhook.FallbackOrderPollSec)
	}
	if cfg.Webhook.FallbackReconcileSec != 300 {
		t.Fatalf("fallback_reconcile_sec=%d, want 300", cfg.Webhook.FallbackReconcileSec)
	}
}

func TestValidateSymbolIndexConfigRejectsCanonicalDuplicate(t *testing.T) {
	err := ValidateSymbolIndexConfig(SymbolIndexConfig{Symbols: []SymbolIndexEntry{
		{Symbol: "BTCUSDT", Config: "symbols/btc.toml", Strategy: "strategies/btc.toml"},
		{Symbol: "BTCUSDT", Config: "symbols/btc2.toml", Strategy: "strategies/btc2.toml"},
	}})
	if err == nil {
		t.Fatalf("expected duplicate symbol validation error")
	}
	if !strings.Contains(err.Error(), "duplicate symbol=BTCUSDT") {
		t.Fatalf("error=%v, want canonical duplicate message", err)
	}
}

func TestValidateSymbolIndexConfigRejectsNonCanonicalSymbol(t *testing.T) {
	err := ValidateSymbolIndexConfig(SymbolIndexConfig{Symbols: []SymbolIndexEntry{
		{Symbol: "btc", Config: "symbols/btc.toml", Strategy: "strategies/btc.toml"},
	}})
	if err == nil {
		t.Fatalf("expected non-canonical symbol validation error")
	}
	if !strings.Contains(err.Error(), "must be canonical symbol") {
		t.Fatalf("error=%v, want canonical symbol validation message", err)
	}
}

func TestLoadSymbolIndexConfigNormalizesSymbolKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols-index.toml")
	data := []byte(strings.Join([]string{
		"[[symbols]]",
		"symbol = \"eth/usdt:usdt\"",
		"config = \"symbols/eth.toml\"",
		"strategy = \"strategies/eth.toml\"",
		"",
		"[[symbols]]",
		"symbol = \"btc\"",
		"config = \"symbols/btc.toml\"",
		"strategy = \"strategies/btc.toml\"",
	}, "\n"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write symbols-index.toml: %v", err)
	}

	cfg, err := LoadSymbolIndexConfig(path)
	if err != nil {
		t.Fatalf("load symbol index: %v", err)
	}
	if len(cfg.Symbols) != 2 {
		t.Fatalf("symbols len=%d, want 2", len(cfg.Symbols))
	}
	if cfg.Symbols[0].Symbol != "ETHUSDT" {
		t.Fatalf("first symbol=%q, want ETHUSDT", cfg.Symbols[0].Symbol)
	}
	if cfg.Symbols[1].Symbol != "BTCUSDT" {
		t.Fatalf("second symbol=%q, want BTCUSDT", cfg.Symbols[1].Symbol)
	}
}

func TestValidateSymbolConfigRejectsNonCanonicalSymbol(t *testing.T) {
	temp := 0.1
	cfg := SymbolConfig{
		Symbol:     "btc",
		Intervals:  []string{"1h"},
		KlineLimit: 300,
		Agent:      AgentConfig{Indicator: boolPtr(true), Structure: boolPtr(true), Mechanics: boolPtr(true)},
		Indicators: IndicatorConfig{EMAFast: 21, EMAMid: 50, EMASlow: 200, RSIPeriod: 14, ATRPeriod: 14, STCFast: 23, STCSlow: 50, BBPeriod: 20, BBMultiplier: 2.0, CHOPPeriod: 14, StochRSIPeriod: 14, AroonPeriod: 25, LastN: 5},
		Consensus:  ConsensusConfig{ScoreThreshold: 0.3, ConfidenceThreshold: 0.6},
		LLM: SymbolLLMConfig{
			Agent:    LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
			Provider: LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
		},
	}
	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatalf("expected non-canonical symbol validation error")
	}
	if !strings.Contains(err.Error(), "must be canonical symbol") {
		t.Fatalf("error=%v, want canonical symbol validation message", err)
	}
}

func TestValidateSymbolConfigRejectsUnorderedEMAPeriods(t *testing.T) {
	temp := 0.1
	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 300,
		Agent:      AgentConfig{Indicator: boolPtr(true), Structure: boolPtr(true), Mechanics: boolPtr(true)},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         21,
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
		Consensus: ConsensusConfig{ScoreThreshold: 0.3, ConfidenceThreshold: 0.6},
		LLM: SymbolLLMConfig{
			Agent:    LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
			Provider: LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() error = nil, want EMA ordering error")
	}
	if !strings.Contains(err.Error(), "ema_fast < ema_mid < ema_slow") {
		t.Fatalf("error=%q should mention EMA ordering", err.Error())
	}
}

func TestValidateSymbolConfigRejectsUnorderedSTCPeriods(t *testing.T) {
	temp := 0.1
	cfg := SymbolConfig{
		Symbol:     "BTCUSDT",
		Intervals:  []string{"1h"},
		KlineLimit: 300,
		Agent:      AgentConfig{Indicator: boolPtr(true), Structure: boolPtr(true), Mechanics: boolPtr(true)},
		Indicators: IndicatorConfig{
			EMAFast:        21,
			EMAMid:         50,
			EMASlow:        200,
			RSIPeriod:      14,
			ATRPeriod:      14,
			STCFast:        50,
			STCSlow:        50,
			BBPeriod:       20,
			BBMultiplier:   2.0,
			CHOPPeriod:     14,
			StochRSIPeriod: 14,
			AroonPeriod:    25,
			LastN:          5,
		},
		Consensus: ConsensusConfig{ScoreThreshold: 0.3, ConfidenceThreshold: 0.6},
		LLM: SymbolLLMConfig{
			Agent:    LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
			Provider: LLMRoleSet{Indicator: LLMRoleConfig{Model: "a", Temperature: &temp}, Structure: LLMRoleConfig{Model: "b", Temperature: &temp}, Mechanics: LLMRoleConfig{Model: "c", Temperature: &temp}},
		},
	}

	err := ValidateSymbolConfig(cfg)
	if err == nil {
		t.Fatal("ValidateSymbolConfig() error = nil, want STC ordering error")
	}
	if !strings.Contains(err.Error(), "stc_fast must be < stc_slow") {
		t.Fatalf("error=%q should mention STC ordering", err.Error())
	}
}

func TestValidateSymbolLLMModelsMatchesCaseInsensitiveSystemKeys(t *testing.T) {
	temp := 0.1
	sys := SystemConfig{
		LLMModels: map[string]LLMModelConfig{
			"minimax-m2.5": {Endpoint: "https://example.com", APIKey: "secret"},
		},
	}
	cfg := SymbolConfig{
		Agent: AgentConfig{Indicator: boolPtr(true), Structure: boolPtr(false), Mechanics: boolPtr(false)},
		LLM: SymbolLLMConfig{
			Agent: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "MiniMax-M2.5", Temperature: &temp},
			},
			Provider: LLMRoleSet{
				Indicator: LLMRoleConfig{Model: "MiniMax-M2.5", Temperature: &temp},
			},
		},
	}

	if err := ValidateSymbolLLMModels(sys, cfg); err != nil {
		t.Fatalf("ValidateSymbolLLMModels() error = %v", err)
	}
}

func TestLoadSystemConfigParsesStructuredOutputFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system.toml")
	data := []byte(strings.Join([]string{
		`execution_system = "freqtrade"`,
		`exec_endpoint = "http://127.0.0.1:8080/api/v1"`,
		``,
		`[database]`,
		`dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"`,
		``,
		`[llm_models."gpt-4o"]`,
		`endpoint = "https://api.openai.com/v1"`,
		`api_key = "secret"`,
		`structured_output = true`,
	}, "\n"))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write system.toml: %v", err)
	}

	cfg, err := LoadSystemConfig(path)
	if err != nil {
		t.Fatalf("LoadSystemConfig() error = %v", err)
	}
	model := cfg.LLMModels["gpt-4o"]
	if model.StructuredOutput == nil || !*model.StructuredOutput {
		t.Fatalf("structured_output=%v want true", model.StructuredOutput)
	}
}
