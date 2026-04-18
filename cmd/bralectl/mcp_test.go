package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPInstallCommandWritesConfig(t *testing.T) {
	dir := t.TempDir()
	systemPath := filepath.Join(dir, "system.toml")
	indexPath := filepath.Join(dir, "symbols-index.toml")
	configPath := filepath.Join(dir, "mcp.json")
	auditPath := filepath.Join(dir, "audit.jsonl")
	commandPath := filepath.Join(dir, "bralectl")
	writeTestFile(t, systemPath, `[database]
dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"`)
	writeTestFile(t, indexPath, `
[[symbols]]
symbol = "BTCUSDT"
config = "symbols/BTCUSDT.toml"
strategy = "strategies/BTCUSDT.toml"
`)
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	out, errOut, err := executeRootCommand(
		t,
		"mcp", "install",
		"--mode", "stdio",
		"--config", configPath,
		"--command", commandPath,
		"--system", systemPath,
		"--index", indexPath,
		"--audit-log", auditPath,
	)
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, configPath) {
		t.Fatalf("stdout=%s", out)
	}
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config: %v", readErr)
	}
	if !strings.Contains(string(raw), `"brale-core"`) || !strings.Contains(string(raw), `"mcpServers"`) {
		t.Fatalf("config=%s", string(raw))
	}
}

func TestMCPInstallCommandDefaultsModeToHTTP(t *testing.T) {
	cmd := mcpInstallCmd()
	flag := cmd.Flags().Lookup("mode")
	if flag == nil {
		t.Fatal("mode flag is missing")
	}
	if got := flag.DefValue; got != "http" {
		t.Fatalf("default mode=%q want http", got)
	}
}

func TestMCPInstallCommandDefaultsCodexToHTTP(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	_, errOut, err := executeRootCommand(
		t,
		"--endpoint", "https://remote.example.com:9991",
		"mcp", "install",
		"--target", "codex",
		"--config", configPath,
	)
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config: %v", readErr)
	}
	if !strings.Contains(string(raw), `[mcp_servers.brale-core]`) {
		t.Fatalf("config=%s", string(raw))
	}
	if !strings.Contains(string(raw), `url = "https://remote.example.com:8765/mcp"`) {
		t.Fatalf("config=%s", string(raw))
	}
}

func TestNormalizeMCPServeModeAllowsSupportedModes(t *testing.T) {
	cases := map[string]string{
		"":      "stdio",
		"stdio": "stdio",
		"STDIO": "stdio",
		"http":  "http",
		"HTTP":  "http",
	}
	for input, want := range cases {
		got, err := normalizeMCPServeMode(input)
		if err != nil {
			t.Fatalf("normalizeMCPServeMode(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeMCPServeMode(%q) = %q want %q", input, got, want)
		}
	}
}

func TestNormalizeMCPServeModeRejectsUnsupportedMode(t *testing.T) {
	_, err := normalizeMCPServeMode("grpc")
	if err == nil || !strings.Contains(err.Error(), "unsupported MCP mode") {
		t.Fatalf("err=%v", err)
	}
}
