package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupCommandInitializesEnvAndInstallsMCP(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, filepath.Join(repo, ".env.example"), "EXEC_USERNAME=\nEXEC_SECRET=\n")
	systemPath := filepath.Join(repo, "configs", "system.toml")
	indexPath := filepath.Join(repo, "configs", "symbols-index.toml")
	commandPath := filepath.Join(repo, "data", "brale", "bin", "bralectl")
	configPath := filepath.Join(repo, "opencode.json")
	auditPath := filepath.Join(repo, "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o755); err != nil {
		t.Fatalf("mkdir command dir: %v", err)
	}
	writeSetupFile(t, systemPath, `[database]
dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"`)
	writeSetupFile(t, indexPath, `
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
		"--endpoint", "http://127.0.0.1:9991",
		"setup",
		"--repo", repo,
		"--lang", "en",
		"--install-mcp",
		"--mcp-target", "opencode",
		"--mcp-mode", "stdio",
		"--config", configPath,
		"--command", commandPath,
		"--system", systemPath,
		"--index", indexPath,
		"--audit-log", auditPath,
	)
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	if !strings.Contains(out, "created .env from .env.example") {
		t.Fatalf("stdout=%s", out)
	}
	if !strings.Contains(out, configPath) {
		t.Fatalf("stdout=%s", out)
	}
	envRaw, readErr := os.ReadFile(filepath.Join(repo, ".env"))
	if readErr != nil {
		t.Fatalf("read .env: %v", readErr)
	}
	if string(envRaw) != "EXEC_USERNAME=\nEXEC_SECRET=\n" {
		t.Fatalf(".env=%q", string(envRaw))
	}
	configRaw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read mcp config: %v", readErr)
	}
	if !strings.Contains(string(configRaw), `"mcpServers"`) || !strings.Contains(string(configRaw), `"brale-core"`) {
		t.Fatalf("config=%s", string(configRaw))
	}
}

func TestSetupCommandRejectsCustomTargetWithoutConfigPath(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, filepath.Join(repo, ".env.example"), "EXEC_USERNAME=\nEXEC_SECRET=\n")
	systemPath := filepath.Join(repo, "configs", "system.toml")
	indexPath := filepath.Join(repo, "configs", "symbols-index.toml")
	commandPath := filepath.Join(repo, "data", "brale", "bin", "bralectl")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o755); err != nil {
		t.Fatalf("mkdir command dir: %v", err)
	}
	writeSetupFile(t, systemPath, `[database]
dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"`)
	writeSetupFile(t, indexPath, `
[[symbols]]
symbol = "BTCUSDT"
config = "symbols/BTCUSDT.toml"
strategy = "strategies/BTCUSDT.toml"
`)
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	_, errOut, err := executeRootCommand(
		t,
		"setup",
		"--repo", repo,
		"--lang", "en",
		"--install-mcp",
		"--mcp-target", "custom",
		"--command", commandPath,
		"--system", systemPath,
		"--index", indexPath,
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(errOut, "requires --config") && !strings.Contains(err.Error(), "requires --config") {
		t.Fatalf("stderr=%s err=%v", errOut, err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".env")); !os.IsNotExist(statErr) {
		t.Fatalf(".env should not be created on invalid setup, got err=%v", statErr)
	}
}

func TestSetupCommandRejectsSkipMCPWithMCPMode(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, filepath.Join(repo, ".env.example"), "EXEC_USERNAME=\nEXEC_SECRET=\n")

	_, errOut, err := executeRootCommand(
		t,
		"setup",
		"--repo", repo,
		"--lang", "en",
		"--skip-mcp",
		"--mcp-mode", "stdio",
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(errOut, "--skip-mcp cannot be combined") && !strings.Contains(err.Error(), "--skip-mcp cannot be combined") {
		t.Fatalf("stderr=%s err=%v", errOut, err)
	}
}

func TestSetupCommandDefaultsCodexToHTTP(t *testing.T) {
	repo := t.TempDir()
	writeSetupFile(t, filepath.Join(repo, ".env.example"), "EXEC_USERNAME=\nEXEC_SECRET=\n")
	systemPath := filepath.Join(repo, "configs", "system.toml")
	indexPath := filepath.Join(repo, "configs", "symbols-index.toml")
	commandPath := filepath.Join(repo, "data", "brale", "bin", "bralectl")
	configPath := filepath.Join(repo, "config.toml")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o755); err != nil {
		t.Fatalf("mkdir command dir: %v", err)
	}
	writeSetupFile(t, systemPath, `[database]
dsn = "postgres://brale:brale@localhost:5432/brale?sslmode=disable"`)
	writeSetupFile(t, indexPath, `
[[symbols]]
symbol = "BTCUSDT"
config = "symbols/BTCUSDT.toml"
strategy = "strategies/BTCUSDT.toml"
`)
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command: %v", err)
	}

	_, errOut, err := executeRootCommand(
		t,
		"--endpoint", "https://remote.example.com:9991",
		"setup",
		"--repo", repo,
		"--lang", "en",
		"--install-mcp",
		"--mcp-target", "codex",
		"--config", configPath,
		"--command", commandPath,
		"--system", systemPath,
		"--index", indexPath,
	)
	if err != nil {
		t.Fatalf("execute command: %v\nstderr=%s", err, errOut)
	}
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read config: %v", readErr)
	}
	if !strings.Contains(string(raw), `[mcp_servers.brale-core]`) || !strings.Contains(string(raw), `url = "https://remote.example.com:8765/mcp"`) {
		t.Fatalf("config=%s", string(raw))
	}
}

func writeSetupFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
