package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredKlineLimit_Minimum(t *testing.T) {
	got := requiredKlineLimit(SymbolConfig{})
	if got != 3 {
		t.Fatalf("kline limit=%d, want 3", got)
	}
}

func TestValidateWebhookFallbackDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system.toml")
	data := []byte(`db_path = ":memory:"
persist_mode = "minimal"
execution_system = "freqtrade"
exec_endpoint = "http://127.0.0.1:8080/api/v1"

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

func TestLoadSystemConfigNormalizesPersistModeAliases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "live alias maps to minimal", input: "live", want: "minimal"},
		{name: "backtest alias maps to full", input: "backtest", want: "full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "system.toml")
			data := []byte(strings.Join([]string{
				`db_path = ":memory:"`,
				`persist_mode = "` + tt.input + `"`,
				`execution_system = "freqtrade"`,
				`exec_endpoint = "http://127.0.0.1:8080/api/v1"`,
			}, "\n"))
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatalf("write system.toml: %v", err)
			}

			cfg, err := LoadSystemConfig(path)
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if cfg.PersistMode != tt.want {
				t.Fatalf("persist_mode=%q want %q", cfg.PersistMode, tt.want)
			}
		})
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
		Indicators: IndicatorConfig{EMAFast: 21, EMAMid: 50, EMASlow: 200, RSIPeriod: 14, ATRPeriod: 14, MACDFast: 12, MACDSlow: 26, MACDSignal: 9, LastN: 5},
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
