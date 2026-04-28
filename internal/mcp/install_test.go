package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/creachadair/tomledit"
)

func TestInstallMergesMCPConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	auditPath := filepath.Join(dir, "audit.jsonl")
	commandPath := filepath.Join(dir, "bralectl")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{"existing":{"command":"existing"}}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origEnsure := ensureHTTPAvailableFunc
	t.Cleanup(func() {
		ensureHTTPAvailableFunc = origEnsure
	})
	ensureCalled := false
	ensureHTTPAvailableFunc = func(prepared preparedInstall) error {
		ensureCalled = true
		return nil
	}

	result, err := Install(InstallOptions{
		Target:     "opencode",
		Name:       "brale-core",
		Command:    commandPath,
		ConfigPath: configPath,
		Endpoint:   "http://127.0.0.1:9991",
		SystemPath: systemPath,
		IndexPath:  indexPath,
		AuditPath:  auditPath,
		Mode:       "stdio",
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if ensureCalled {
		t.Fatal("ensureHTTPAvailable should not run in stdio mode")
	}
	if result.ConfigPath != configPath {
		t.Fatalf("ConfigPath=%s want %s", result.ConfigPath, configPath)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	servers := doc["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Fatalf("existing server missing: %v", servers)
	}
	brale := servers["brale-core"].(map[string]any)
	if brale["command"] != commandPath {
		t.Fatalf("command=%v want %s", brale["command"], commandPath)
	}
	args := toStringSlice(t, brale["args"])
	for _, want := range []string{
		"--endpoint",
		"http://127.0.0.1:9991",
		"mcp",
		"serve",
		"--mode",
		"stdio",
		"--system",
		systemPath,
		"--index",
		indexPath,
		"--audit-log",
		auditPath,
	} {
		if !containsString(args, want) {
			t.Fatalf("args=%v missing %q", args, want)
		}
	}
}

func TestInstallDefaultsToHTTPForClaudeCodeAndRemovesLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repoRoot := filepath.Join(home, "repo")
	configsDir := filepath.Join(repoRoot, "configs")
	legacyDir := filepath.Join(home, ".config", "claude")
	legacyPath := filepath.Join(legacyDir, "mcp_settings.json")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"mcpServers":{"legacy":{"command":"old"}}}`), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatalf("mkdir configs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "docker-compose.yml"), []byte("services:\n"), 0o644); err != nil {
		t.Fatalf("write docker-compose: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module brale-core\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	systemPath := filepath.Join(configsDir, "system.toml")
	indexPath := filepath.Join(configsDir, "symbols-index.toml")
	auditPath := filepath.Join(home, "audit.jsonl")
	commandPath := filepath.Join(home, "bralectl")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	origEnsure := ensureHTTPAvailableFunc
	t.Cleanup(func() {
		ensureHTTPAvailableFunc = origEnsure
	})
	var ensured preparedInstall
	ensureHTTPAvailableFunc = func(prepared preparedInstall) error {
		ensured = prepared
		return nil
	}

	result, err := Install(InstallOptions{
		Target:     "claude-code",
		Name:       "brale-core",
		Command:    commandPath,
		Endpoint:   "http://127.0.0.1:9991",
		SystemPath: systemPath,
		IndexPath:  indexPath,
		AuditPath:  auditPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	wantConfigPath := filepath.Join(home, ".claude.json")
	if result.ConfigPath != wantConfigPath {
		t.Fatalf("ConfigPath=%s want %s", result.ConfigPath, wantConfigPath)
	}
	if ensured.mode != "http" {
		t.Fatalf("ensure mode=%q want http", ensured.mode)
	}
	if ensured.httpURL != "http://127.0.0.1:8765/mcp" {
		t.Fatalf("ensure httpURL=%q want %q", ensured.httpURL, "http://127.0.0.1:8765/mcp")
	}

	raw, err := os.ReadFile(wantConfigPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	servers := doc["mcpServers"].(map[string]any)
	brale := servers["brale-core"].(map[string]any)
	if brale["type"] != "streamable-http" {
		t.Fatalf("type=%v want streamable-http", brale["type"])
	}
	if brale["url"] != "http://127.0.0.1:8765/mcp" {
		t.Fatalf("url=%v want %s", brale["url"], "http://127.0.0.1:8765/mcp")
	}
	if _, ok := brale["command"]; ok {
		t.Fatalf("unexpected command for http config: %v", brale)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: err=%v", err)
	}
}

func TestDockerComposeMCPServiceGetsRuntimeConfigEnv(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	block := dockerComposeServiceBlock(t, string(data), "mcp")
	for _, want := range []string{
		"env_file:",
		"- path: .env",
		"- path: ${STACK_PROXY_ENV_FILE:-./data/freqtrade/proxy.env}",
		"environment:",
		"TZ: Asia/Shanghai",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("mcp service block missing %q:\n%s", want, block)
		}
	}
	const wantDSN = `DATABASE_DSN: "postgres://${POSTGRES_USER:-brale}:${POSTGRES_PASSWORD:-brale}@timescaledb:5432/${POSTGRES_DB:-brale}?sslmode=disable"`
	if !strings.Contains(block, wantDSN) {
		t.Fatalf("mcp service block missing DATABASE_DSN override:\n%s", block)
	}
}

func dockerComposeServiceBlock(t *testing.T, compose string, service string) string {
	t.Helper()
	lines := strings.Split(compose, "\n")
	start := -1
	for i, line := range lines {
		if line == "  "+service+":" {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("service %q not found", service)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func TestInstallSkipsLocalHTTPAutoStartWhenRepoRootCannotBeResolved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(home)

	origEnsure := ensureHTTPAvailableFunc
	t.Cleanup(func() {
		ensureHTTPAvailableFunc = origEnsure
	})
	ensureCalled := false
	ensureHTTPAvailableFunc = func(prepared preparedInstall) error {
		ensureCalled = true
		return nil
	}

	result, err := Install(InstallOptions{
		Target:   "claude-code",
		Name:     "brale-core",
		Endpoint: "http://127.0.0.1:9991",
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if ensureCalled {
		t.Fatal("ensureHTTPAvailable should not run when repo root cannot be resolved confidently")
	}
	if result.ConfigPath != filepath.Join(home, ".claude.json") {
		t.Fatalf("ConfigPath=%s", result.ConfigPath)
	}
}

func TestInstallWritesClaudeCodeConfigToClaudeJSONAndRemovesLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyDir := filepath.Join(home, ".config", "claude")
	legacyPath := filepath.Join(legacyDir, "mcp_settings.json")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"mcpServers":{"legacy":{"command":"old"}}}`), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	systemPath := filepath.Join(home, "system.toml")
	indexPath := filepath.Join(home, "symbols-index.toml")
	auditPath := filepath.Join(home, "audit.jsonl")
	commandPath := filepath.Join(home, "bralectl")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	origEnsure := ensureHTTPAvailableFunc
	t.Cleanup(func() {
		ensureHTTPAvailableFunc = origEnsure
	})
	ensureHTTPAvailableFunc = func(prepared preparedInstall) error {
		t.Fatal("ensureHTTPAvailable should not run in stdio mode")
		return nil
	}

	result, err := Install(InstallOptions{
		Target:     "claude-code",
		Name:       "brale-core",
		Command:    commandPath,
		Endpoint:   "http://127.0.0.1:9991",
		SystemPath: systemPath,
		IndexPath:  indexPath,
		AuditPath:  auditPath,
		Mode:       "stdio",
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	wantConfigPath := filepath.Join(home, ".claude.json")
	if result.ConfigPath != wantConfigPath {
		t.Fatalf("ConfigPath=%s want %s", result.ConfigPath, wantConfigPath)
	}

	raw, err := os.ReadFile(wantConfigPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	servers := doc["mcpServers"].(map[string]any)
	brale := servers["brale-core"].(map[string]any)
	if brale["command"] != commandPath {
		t.Fatalf("command=%v want %s", brale["command"], commandPath)
	}
	if brale["type"] != "stdio" {
		t.Fatalf("type=%v want stdio", brale["type"])
	}
	if brale["command"] != commandPath {
		t.Fatalf("command=%v want %s", brale["command"], commandPath)
	}
	args := toStringSlice(t, brale["args"])
	for _, want := range []string{
		"--endpoint",
		"http://127.0.0.1:9991",
		"mcp",
		"serve",
		"--mode",
		"stdio",
		"--system",
		systemPath,
		"--index",
		indexPath,
		"--audit-log",
		auditPath,
	} {
		if !containsString(args, want) {
			t.Fatalf("args=%v missing %q", args, want)
		}
	}
	if env, ok := brale["env"].(map[string]any); !ok || len(env) != 0 {
		t.Fatalf("env=%v want empty object", brale["env"])
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy config still exists: err=%v", err)
	}
}

func TestInstallRejectsMissingCommand(t *testing.T) {
	dir := t.TempDir()
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	_, err := Install(InstallOptions{
		Command:    filepath.Join(dir, "missing-bralectl"),
		ConfigPath: filepath.Join(dir, "mcp.json"),
		SystemPath: systemPath,
		IndexPath:  indexPath,
		Mode:       "stdio",
	})
	if err == nil || !strings.Contains(err.Error(), "command path") {
		t.Fatalf("err=%v", err)
	}
}

func TestInstallRejectsDirectoryForSystemPath(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "symbols-index.toml")
	commandPath := filepath.Join(dir, "bralectl")
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}
	_, err := Install(InstallOptions{
		Command:    commandPath,
		ConfigPath: filepath.Join(dir, "mcp.json"),
		SystemPath: dir,
		IndexPath:  indexPath,
		Mode:       "stdio",
	})
	if err == nil || !strings.Contains(err.Error(), "file path") {
		t.Fatalf("err=%v", err)
	}
}

func TestDefaultInstallConfigPathSupportsAdditionalTargets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	}

	tests := []struct {
		target string
		want   string
	}{
		{
			target: "claude-code",
			want:   filepath.Join(home, ".claude.json"),
		},
		{
			target: "opencode",
			want:   filepath.Join(home, ".config", "opencode", "config.json"),
		},
		{
			target: "codex",
			want:   filepath.Join(home, ".codex", "config.toml"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got, err := defaultInstallConfigPath(tt.target)
			if err != nil {
				t.Fatalf("defaultInstallConfigPath(%q) error = %v", tt.target, err)
			}
			if got != tt.want {
				t.Fatalf("defaultInstallConfigPath(%q) = %q want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestInstallWritesCodexConfigTOML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	auditPath := filepath.Join(dir, "audit.jsonl")
	commandPath := filepath.Join(dir, "bralectl")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.4\"\n\n[mcp_servers.old]\ncommand = \"old\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origEnsure := ensureHTTPAvailableFunc
	t.Cleanup(func() {
		ensureHTTPAvailableFunc = origEnsure
	})
	ensureHTTPAvailableFunc = func(prepared preparedInstall) error {
		t.Fatal("ensureHTTPAvailable should not run for remote HTTP installs")
		return nil
	}

	result, err := Install(InstallOptions{
		Target:     "codex",
		Name:       "brale-core",
		ConfigPath: configPath,
		Endpoint:   "https://remote.example.com:9991",
		SystemPath: systemPath,
		IndexPath:  indexPath,
		AuditPath:  auditPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.ConfigPath != configPath {
		t.Fatalf("ConfigPath=%s want %s", result.ConfigPath, configPath)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	doc, err := tomledit.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := doc.First("model"); got == nil || got.KeyValue == nil || !strings.Contains(got.KeyValue.String(), "\"gpt-5.4\"") {
		t.Fatalf("top-level model setting missing:\n%s", raw)
	}
	section := doc.First("mcp_servers", "brale-core")
	if section == nil || !section.IsSection() {
		t.Fatalf("brale-core section missing:\n%s", raw)
	}
	url := doc.First("mcp_servers", "brale-core", "url")
	if url == nil || url.KeyValue == nil || !strings.Contains(url.KeyValue.String(), "https://remote.example.com:8765/mcp") {
		t.Fatalf("url mapping missing or invalid:\n%s", raw)
	}
	if command := doc.First("mcp_servers", "brale-core", "command"); command != nil {
		t.Fatalf("unexpected command mapping in HTTP config:\n%s", raw)
	}
	if args := doc.First("mcp_servers", "brale-core", "args"); args != nil {
		t.Fatalf("unexpected args mapping in HTTP config:\n%s", raw)
	}
}

func TestInstallWritesCodexStdioConfigTOMLWhenRequested(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	auditPath := filepath.Join(dir, "audit.jsonl")
	commandPath := filepath.Join(dir, "bralectl")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.4\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	result, err := Install(InstallOptions{
		Target:     "codex",
		Name:       "brale-core",
		Command:    commandPath,
		ConfigPath: configPath,
		Endpoint:   "http://127.0.0.1:9991",
		SystemPath: systemPath,
		IndexPath:  indexPath,
		AuditPath:  auditPath,
		Mode:       "stdio",
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.ConfigPath != configPath {
		t.Fatalf("ConfigPath=%s want %s", result.ConfigPath, configPath)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	doc, err := tomledit.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	command := doc.First("mcp_servers", "brale-core", "command")
	if command == nil || command.KeyValue == nil || !strings.Contains(command.KeyValue.String(), commandPath) {
		t.Fatalf("command mapping missing or invalid:\n%s", raw)
	}
	args := doc.First("mcp_servers", "brale-core", "args")
	if args == nil || args.KeyValue == nil {
		t.Fatalf("args mapping missing:\n%s", raw)
	}
	for _, want := range []string{
		"--endpoint",
		"http://127.0.0.1:9991",
		"mcp",
		"serve",
		"--mode",
		"stdio",
		"--system",
		systemPath,
		"--index",
		indexPath,
		"--audit-log",
		auditPath,
	} {
		if !strings.Contains(args.KeyValue.String(), want) {
			t.Fatalf("args=%s missing %q", args.KeyValue.String(), want)
		}
	}
}

func TestDefaultInstallConfigPathRejectsCustomWithoutExplicitPath(t *testing.T) {
	_, err := defaultInstallConfigPath("custom")
	if err == nil || !strings.Contains(err.Error(), "requires --config") {
		t.Fatalf("err=%v", err)
	}
}

func TestInstallRejectsUnsupportedTargetEvenWithExplicitConfigPath(t *testing.T) {
	dir := t.TempDir()
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	commandPath := filepath.Join(dir, "bralectl")
	configPath := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}
	_, err := Install(InstallOptions{
		Target:     "codxe",
		ConfigPath: configPath,
		Command:    commandPath,
		SystemPath: systemPath,
		IndexPath:  indexPath,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported install target") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidateInstallOptionsAllowsCodexHTTP(t *testing.T) {
	dir := t.TempDir()
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	err := ValidateInstallOptions(InstallOptions{
		Target:     "codex",
		Mode:       "http",
		ConfigPath: filepath.Join(dir, "config.toml"),
		SystemPath: systemPath,
		IndexPath:  indexPath,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestInstallSkipsLocalDockerEnsureForRemoteHTTP(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	if err := os.WriteFile(systemPath, []byte("[database]\ndsn = \"postgres://brale:brale@localhost:5432/brale?sslmode=disable\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	origEnsure := ensureHTTPAvailableFunc
	t.Cleanup(func() {
		ensureHTTPAvailableFunc = origEnsure
	})
	ensureHTTPAvailableFunc = func(prepared preparedInstall) error {
		t.Fatal("ensureHTTPAvailable should not run for remote HTTP installs")
		return nil
	}

	result, err := Install(InstallOptions{
		Target:     "claude-code",
		ConfigPath: configPath,
		Endpoint:   "https://remote.example.com:9991",
		SystemPath: systemPath,
		IndexPath:  indexPath,
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if result.ConfigPath != configPath {
		t.Fatalf("ConfigPath=%s want %s", result.ConfigPath, configPath)
	}
}

func toStringSlice(t *testing.T, v any) []string {
	t.Helper()
	raw := v.([]any)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(string))
	}
	return out
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
