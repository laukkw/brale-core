package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBacktestRulesCommandJSONOutput(t *testing.T) {
	t.Skip("requires PostgreSQL")
}

func TestBacktestRulesCommandTextOutput(t *testing.T) {
	t.Skip("requires PostgreSQL")
}

func TestBacktestRulesCommandHTMLRequiresOutput(t *testing.T) {
	t.Skip("requires PostgreSQL")
}

func TestBacktestRulesCommandWritesHTMLFile(t *testing.T) {
	t.Skip("requires PostgreSQL")
}

func writeBacktestConfigTree(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	symbolDir := filepath.Join(dir, "symbols")
	strategyDir := filepath.Join(dir, "strategies")
	if err := os.MkdirAll(symbolDir, 0o755); err != nil {
		t.Fatalf("mkdir symbols: %v", err)
	}
	if err := os.MkdirAll(strategyDir, 0o755); err != nil {
		t.Fatalf("mkdir strategies: %v", err)
	}
	writeTestFile(t, systemPath, `
execution_system = "freqtrade"
exec_endpoint = "http://localhost:8080"

[database]
dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"

[llm_models.mock]
endpoint = "http://localhost:11434/v1"
api_key = "dummy"
`)
	writeTestFile(t, indexPath, `
[[symbols]]
symbol = "BTCUSDT"
config = "symbols/BTCUSDT.toml"
strategy = "strategies/BTCUSDT.toml"
`)
	writeTestFile(t, filepath.Join(symbolDir, "BTCUSDT.toml"), `
symbol = "BTCUSDT"
intervals = ["15m", "1h", "4h"]
kline_limit = 300

[agent]
indicator = true
structure = true
mechanics = true

[indicators]
ema_fast = 21
ema_mid = 50
ema_slow = 200
rsi_period = 14
atr_period = 14
stc_fast = 23
stc_slow = 50
bb_period = 20
bb_multiplier = 2.0
chop_period = 14
stoch_rsi_period = 14
aroon_period = 25
last_n = 5

[consensus]
score_threshold = 0.35
confidence_threshold = 0.52

[cooldown]
enabled = false

[llm.agent.indicator]
model = "mock"
temperature = 0.2
[llm.agent.structure]
model = "mock"
temperature = 0.1
[llm.agent.mechanics]
model = "mock"
temperature = 0.2
[llm.provider.indicator]
model = "mock"
temperature = 0.2
[llm.provider.structure]
model = "mock"
temperature = 0.1
[llm.provider.mechanics]
model = "mock"
temperature = 0.2
`)
	writeTestFile(t, filepath.Join(strategyDir, "BTCUSDT.toml"), `
symbol = "BTCUSDT"
id = "default-BTCUSDT"
rule_chain = "configs/rules/default.json"

[risk_management]
risk_per_trade_pct = 0.01
max_invest_pct = 1.0
max_leverage = 3.0
grade_1_factor = 0.3
grade_2_factor = 0.6
grade_3_factor = 1.0
entry_offset_atr = 0.1
entry_mode = "atr_offset"
orderbook_depth = 5
breakeven_fee_pct = 0.0
slippage_buffer_pct = 0.0005

[risk_management.risk_strategy]
mode = "native"

[risk_management.initial_exit]
policy = "atr_structure_v1"
structure_interval = "auto"

[risk_management.initial_exit.params]
stop_atr_multiplier = 2.0
stop_min_distance_pct = 0.005
take_profit_rr = [1.5, 3.0]

[risk_management.tighten_atr]
structure_threatened = 0.5
tp1_atr = 0.0
tp2_atr = 0.0
min_tp_distance_pct = 0.0
min_tp_gap_pct = 0.0
min_update_interval_sec = 300

[risk_management.gate]
quality_threshold = 0.35
edge_threshold = 0.1

[risk_management.sieve]
min_size_factor = 0.1
default_gate_action = "ALLOW"
default_size_factor = 1.0
`)
	return systemPath, indexPath
}
