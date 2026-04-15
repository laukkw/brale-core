package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObserveReportCommandJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/observe/report" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","status":"ok","summary":"observe ok","report_markdown":"# BTC report"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "--json", "observe", "report", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, `"symbol": "BTCUSDT"`) {
		t.Fatalf("stdout=%s", out)
	}
}

func TestObserveReportCommandSummaryFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/observe/report" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","status":"ok","summary":"observe summary"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "observe", "report", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "observe summary") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestPositionListCommandTextOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/position/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","summary":"1 open position","positions":[{"symbol":"ETHUSDT","side":"short","entry_price":3000,"current_price":2950,"profit_total":12.5}]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "position", "list")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "ETHUSDT") || !strings.Contains(out, "short") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestPositionListCommandEmptyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/position/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","summary":"no positions","positions":[]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "position", "list")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "No open positions.") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestDecisionLatestCommandTextOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/decision/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","symbol":"BTCUSDT","summary":"gate allow","report_markdown":"## Decision\nALLOW"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "decision", "latest", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "ALLOW") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestDecisionLatestCommandSummaryFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/decision/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","symbol":"BTCUSDT","summary":"gate wait"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "decision", "latest", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "gate wait") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestPositionHistoryCommandTextOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/position/history" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","summary":"history","trades":[{"symbol":"BTCUSDT","side":"long","opened_at":"2026-04-13T00:00:00Z","duration_sec":3600,"profit":25.5}]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "position", "history")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "BTCUSDT") || !strings.Contains(out, "25.5000") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestPositionHistoryCommandEmptyOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/position/history" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","summary":"history","trades":[]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "position", "history")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "No trade history.") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestDecisionHistoryCommandTextOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/dashboard/decision_history" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","symbol":"BTCUSDT","limit":20,"items":[{"snapshot_id":11,"action":"ALLOW","reason":"consensus","at":"2026-04-13T00:00:00Z"}],"detail":{"snapshot_id":11,"action":"ALLOW","reason":"consensus","tradeable":true,"providers":[],"agents":[],"report_markdown":"### Detail\nGate allow","decision_view_url":"/decision-view/?symbol=BTCUSDT&snapshot_id=11"},"summary":"history"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "decision", "history", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "SNAPSHOT") || !strings.Contains(out, "Gate allow") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestDecisionHistoryCommandMessageFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/dashboard/decision_history" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","symbol":"BTCUSDT","limit":20,"items":[],"message":"no decisions yet","summary":"history"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "decision", "history", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "no decisions yet") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestConfigShowCommandJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/monitor/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","summary":"monitor ready","symbols":[{"symbol":"SOLUSDT","kline_interval":"1h","risk_pct":0.01,"max_leverage":3}]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "--json", "config", "show")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, `"SOLUSDT"`) {
		t.Fatalf("stdout=%s", out)
	}
}

func TestConfigShowCommandTextOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/monitor/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","summary":"monitor ready","symbols":[{"symbol":"SOLUSDT","next_run":"2026-04-13T00:00:00Z","kline_interval":"1h","risk_pct":0.01,"max_leverage":3}]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "config", "show")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "SOLUSDT") || !strings.Contains(out, "1h") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestConfigValidateCommand(t *testing.T) {
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
db_path = ":memory:"
execution_system = "freqtrade"
exec_endpoint = "http://localhost:8080"

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
entry_mode = "orderbook"
orderbook_depth = 5
breakeven_fee_pct = 0.0
slippage_buffer_pct = 0.0005

[risk_management.risk_strategy]
mode = "llm"

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

	out, errOut, err := executeRootCommand(t, "config", "validate", "--system", systemPath, "--index", indexPath)
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "validated 1 symbol") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestScheduleEnableCommandWithYes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/schedule/enable" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		_, _ = w.Write([]byte(`{"status":"ok","llm_scheduled":true,"mode":"scheduled","summary":"enabled"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "schedule", "enable", "--yes")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "enabled") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestScheduleDisableCommandInteractiveConfirm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/schedule/disable" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","llm_scheduled":false,"mode":"manual","summary":"disabled"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommandWithInput(t, strings.NewReader("y\n"), "--endpoint", srv.URL, "schedule", "disable")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "disabled") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestScheduleStatusCommandJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/schedule/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","llm_scheduled":true,"mode":"scheduled","summary":"status ok"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "--json", "schedule", "status")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, `"mode": "scheduled"`) {
		t.Fatalf("stdout=%s", out)
	}
}

func TestScheduleStatusCommandTextOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/runtime/schedule/status" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","llm_scheduled":true,"mode":"scheduled","summary":"status ok","next_runs":[{"symbol":"BTCUSDT","bar_interval":"1h","next_execution":"2026-04-13T00:00:00Z"}]}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "schedule", "status")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "BTCUSDT") || !strings.Contains(out, "scheduled=true") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestObserveRunCommandWithYes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/observe/run" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		_, _ = w.Write([]byte(`{"symbol":"BTCUSDT","status":"ok","summary":"observe executed","report_markdown":"# Run report"}`))
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommand(t, "--endpoint", srv.URL, "observe", "run", "--symbol", "BTCUSDT", "--yes")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "Run report") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestObserveRunCommandCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request")
		return
	}))
	defer srv.Close()

	out, errOut, err := executeRootCommandWithInput(t, strings.NewReader("n\n"), "--endpoint", srv.URL, "observe", "run", "--symbol", "BTCUSDT")
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "Canceled.") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestResolveConfigPathFallsBackToIndexRelative(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "configs", "symbols-index.toml")
	targetPath := filepath.Join(dir, "configs", "symbols", "BTCUSDT.toml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	writeTestFile(t, targetPath, `symbol = "BTCUSDT"`)

	got, err := resolveConfigPath(indexPath, "symbols/BTCUSDT.toml")
	if err != nil {
		t.Fatalf("resolveConfigPath: %v", err)
	}
	if got != targetPath {
		t.Fatalf("got=%s want=%s", got, targetPath)
	}
}

func TestResolveConfigPathAbsolute(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "BTCUSDT.toml")
	writeTestFile(t, targetPath, `symbol = "BTCUSDT"`)

	got, err := resolveConfigPath(filepath.Join(dir, "symbols-index.toml"), targetPath)
	if err != nil {
		t.Fatalf("resolveConfigPath: %v", err)
	}
	if got != targetPath {
		t.Fatalf("got=%s want=%s", got, targetPath)
	}
}

func TestResolveConfigPathEmpty(t *testing.T) {
	_, err := resolveConfigPath("configs/symbols-index.toml", "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateConfigTreeMissingModelConfig(t *testing.T) {
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
db_path = ":memory:"
execution_system = "freqtrade"
exec_endpoint = "http://localhost:8080"

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
structure = false
mechanics = false
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
model = "missing"
temperature = 0.2
[llm.agent.structure]
model = ""
temperature = 0.1
[llm.agent.mechanics]
model = ""
temperature = 0.2
[llm.provider.indicator]
model = "missing"
temperature = 0.2
[llm.provider.structure]
model = ""
temperature = 0.1
[llm.provider.mechanics]
model = ""
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
entry_mode = "orderbook"
orderbook_depth = 5
breakeven_fee_pct = 0.0
slippage_buffer_pct = 0.0005
[risk_management.risk_strategy]
mode = "llm"
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

	_, err := validateConfigTree(systemPath, indexPath)
	if err == nil || !strings.Contains(err.Error(), "not found in system llm_models") {
		t.Fatalf("err=%v", err)
	}
}

func TestLLMProbeCommandReturnsLoadError(t *testing.T) {
	dir := t.TempDir()
	_, errOut, err := executeRootCommand(t, "llm", "probe", "--repo", dir, "--stage", "indicator")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(errOut, "Error:") && !strings.Contains(err.Error(), "config") {
		t.Fatalf("stderr=%s err=%v", errOut, err)
	}
}

func executeRootCommand(t *testing.T, args ...string) (string, string, error) {
	return executeRootCommandWithInput(t, strings.NewReader(""), args...)
}

func executeRootCommandWithInput(t *testing.T, input *strings.Reader, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(input)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
