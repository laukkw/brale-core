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
	if err := os.WriteFile(systemPath, []byte("db_path = \"db.sqlite\"\n"), 0o644); err != nil {
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

	result, err := Install(InstallOptions{
		Name:       "brale-core",
		Command:    commandPath,
		ConfigPath: configPath,
		Endpoint:   "http://127.0.0.1:9991",
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
	if err := os.WriteFile(systemPath, []byte("db_path = \"db.sqlite\"\n"), 0o644); err != nil {
		t.Fatalf("write system: %v", err)
	}
	if err := os.WriteFile(indexPath, []byte("[[symbols]]\nsymbol = \"BTCUSDT\"\nconfig = \"symbols/BTCUSDT.toml\"\nstrategy = \"strategies/BTCUSDT.toml\"\n"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
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
	if err := os.WriteFile(systemPath, []byte("db_path = \"db.sqlite\"\n"), 0o644); err != nil {
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
	if err := os.WriteFile(systemPath, []byte("db_path = \"db.sqlite\"\n"), 0o644); err != nil {
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

	result, err := Install(InstallOptions{
		Target:     "codex",
		Name:       "brale-core",
		Command:    commandPath,
		ConfigPath: configPath,
		Endpoint:   "http://127.0.0.1:9991",
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
	if err := os.WriteFile(systemPath, []byte("db_path = \"db.sqlite\"\n"), 0o644); err != nil {
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
