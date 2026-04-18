package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	defaultInstallTarget = "claude-code"
	defaultInstallMode   = "http"
	defaultServerName    = "brale-core"
	defaultEndpoint      = "http://127.0.0.1:9991"
	defaultHTTPPort      = "8765"
	defaultHTTPPath      = "/mcp"
)

var supportedInstallTargets = []string{
	"claude-code",
	"claude-desktop",
	"opencode",
	"codex",
	"custom",
}

type InstallOptions struct {
	Name       string
	Command    string
	ConfigPath string
	Target     string
	Mode       string
	Endpoint   string
	SystemPath string
	IndexPath  string
	AuditPath  string
}

type InstallResult struct {
	ConfigPath string
	ServerName string
	Command    string
	Args       []string
}

type preparedInstall struct {
	target           string
	mode             string
	name             string
	command          string
	configPath       string
	args             []string
	endpoint         string
	httpURL          string
	repoRoot         string
	removeLegacyPath string
}

var ensureHTTPAvailableFunc = ensureHTTPAvailable

func Install(opts InstallOptions) (InstallResult, error) {
	prepared, err := prepareInstall(opts)
	if err != nil {
		return InstallResult{}, err
	}
	if prepared.mode == "http" && prepared.repoRoot != "" && shouldEnsureLocalHTTP(prepared.httpURL) {
		if err := ensureHTTPAvailableFunc(prepared); err != nil {
			return InstallResult{}, fmt.Errorf("ensure MCP HTTP endpoint %s: %w\nhint: use --mode stdio if you want a local spawned MCP process instead", prepared.httpURL, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(prepared.configPath), 0o755); err != nil {
		return InstallResult{}, fmt.Errorf("create install dir: %w", err)
	}
	switch prepared.target {
	case "codex":
		if err := installCodexConfig(prepared); err != nil {
			return InstallResult{}, err
		}
	case "claude-code":
		if err := installClaudeCodeConfig(prepared); err != nil {
			return InstallResult{}, err
		}
		if prepared.removeLegacyPath != "" {
			if err := os.Remove(prepared.removeLegacyPath); err != nil && !os.IsNotExist(err) {
				return InstallResult{}, fmt.Errorf("remove legacy claude-code config: %w", err)
			}
		}
	default:
		if err := installJSONConfig(prepared); err != nil {
			return InstallResult{}, err
		}
	}
	return InstallResult{
		ConfigPath: prepared.configPath,
		ServerName: prepared.name,
		Command:    prepared.command,
		Args:       prepared.args,
	}, nil
}

func DefaultAuditLogPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "brale-core", "mcp-audit.jsonl"), nil
}

func SupportedInstallTargets() []string {
	return slices.Clone(supportedInstallTargets)
}

func DefaultInstallConfigPath(target string) (string, error) {
	return defaultInstallConfigPath(target)
}

func ValidateInstallOptions(opts InstallOptions) error {
	_, err := prepareInstall(opts)
	return err
}

func defaultInstallConfigPath(target string) (string, error) {
	var err error
	target, err = normalizeInstallTarget(target)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch target {
	case "claude-code":
		return filepath.Join(home, ".claude.json"), nil
	case "claude-desktop":
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
		case "windows":
			appData := strings.TrimSpace(os.Getenv("APPDATA"))
			if appData != "" {
				return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
			}
			return filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json"), nil
		default:
			return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
		}
	case "opencode":
		return filepath.Join(home, ".config", "opencode", "config.json"), nil
	case "codex":
		return filepath.Join(home, ".codex", "config.toml"), nil
	case "custom":
		return "", fmt.Errorf("install target %q requires --config", target)
	default:
		return "", fmt.Errorf("unsupported install target %q", target)
	}
}

func prepareInstall(opts InstallOptions) (preparedInstall, error) {
	target, err := normalizeInstallTarget(opts.Target)
	if err != nil {
		return preparedInstall{}, err
	}
	mode, err := normalizeInstallMode(opts.Mode)
	if err != nil {
		return preparedInstall{}, err
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = defaultServerName
	}
	command := ""
	configPath := strings.TrimSpace(opts.ConfigPath)
	configPathIsDefault := configPath == ""
	if configPathIsDefault {
		configPath, err = defaultInstallConfigPath(target)
		if err != nil {
			return preparedInstall{}, err
		}
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return preparedInstall{}, fmt.Errorf("resolve config path: %w", err)
	}
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	httpURL, err := buildHTTPURL(endpoint)
	if err != nil {
		return preparedInstall{}, fmt.Errorf("resolve HTTP URL from endpoint %q: %w", endpoint, err)
	}
	repoRoot := resolveInstallRepoRoot(opts.SystemPath, opts.IndexPath)
	var args []string
	if mode == "stdio" {
		systemPath, err := absoluteExistingFile(defaultIfEmpty(opts.SystemPath, "configs/system.toml"))
		if err != nil {
			return preparedInstall{}, fmt.Errorf("resolve system path: %w", err)
		}
		indexPath, err := absoluteExistingFile(defaultIfEmpty(opts.IndexPath, "configs/symbols-index.toml"))
		if err != nil {
			return preparedInstall{}, fmt.Errorf("resolve index path: %w", err)
		}
		auditPath := strings.TrimSpace(opts.AuditPath)
		if auditPath == "" {
			auditPath, err = DefaultAuditLogPath()
			if err != nil {
				return preparedInstall{}, fmt.Errorf("resolve audit log path: %w", err)
			}
		}
		auditPath, err = filepath.Abs(auditPath)
		if err != nil {
			return preparedInstall{}, fmt.Errorf("resolve audit log path: %w", err)
		}
		command = strings.TrimSpace(opts.Command)
		if command == "" {
			exe, err := os.Executable()
			if err != nil {
				return preparedInstall{}, fmt.Errorf("resolve executable: %w", err)
			}
			command = exe
		}
		command, err = absoluteExecutablePath(command)
		if err != nil {
			return preparedInstall{}, fmt.Errorf("resolve command path: %w", err)
		}
		args = []string{
			"--endpoint", endpoint,
			"mcp", "serve",
			"--mode", "stdio",
			"--system", systemPath,
			"--index", indexPath,
			"--audit-log", auditPath,
		}
		repoRoot = resolveInstallRepoRoot(systemPath, indexPath)
	}
	removeLegacyPath := ""
	if target == "claude-code" && configPathIsDefault {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return preparedInstall{}, fmt.Errorf("resolve user home: %w", err)
		}
		removeLegacyPath = filepath.Join(homeDir, ".config", "claude", "mcp_settings.json")
	}
	return preparedInstall{
		target:           target,
		mode:             mode,
		name:             name,
		command:          command,
		configPath:       configPath,
		args:             args,
		endpoint:         endpoint,
		httpURL:          httpURL,
		repoRoot:         repoRoot,
		removeLegacyPath: removeLegacyPath,
	}, nil
}

func installClaudeCodeConfig(prepared preparedInstall) error {
	doc, err := loadInstallDocument(prepared.configPath)
	if err != nil {
		return err
	}
	servers, err := ensureMap(doc, "mcpServers")
	if err != nil {
		return err
	}
	entry, err := buildInstallEntry(prepared)
	if err != nil {
		return err
	}
	servers[prepared.name] = entry
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude-code config: %w", err)
	}
	if err := writeAtomic(prepared.configPath, append(raw, '\n')); err != nil {
		return fmt.Errorf("write claude-code config: %w", err)
	}
	return nil
}

func installJSONConfig(prepared preparedInstall) error {
	doc, err := loadInstallDocument(prepared.configPath)
	if err != nil {
		return err
	}
	servers, err := ensureMap(doc, "mcpServers")
	if err != nil {
		return err
	}
	entry, err := buildInstallEntry(prepared)
	if err != nil {
		return err
	}
	servers[prepared.name] = entry
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal install config: %w", err)
	}
	if err := writeAtomic(prepared.configPath, append(raw, '\n')); err != nil {
		return fmt.Errorf("write install config: %w", err)
	}
	return nil
}

func normalizeInstallTarget(raw string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(raw))
	if target == "" {
		return defaultInstallTarget, nil
	}
	if slices.Contains(supportedInstallTargets, target) {
		return target, nil
	}
	return "", fmt.Errorf("unsupported install target %q", raw)
}

func normalizeInstallMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return defaultInstallMode, nil
	}
	switch mode {
	case "http", "stdio":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported install mode %q", raw)
	}
}

func loadInstallDocument(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read install config: %w", err)
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse install config: %w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func ensureMap(doc map[string]any, key string) (map[string]any, error) {
	if existing, ok := doc[key]; ok {
		if typed, ok := existing.(map[string]any); ok {
			return typed, nil
		}
		return nil, fmt.Errorf("%s must be a JSON object", key)
	}
	typed := map[string]any{}
	doc[key] = typed
	return typed, nil
}

func buildInstallEntry(prepared preparedInstall) (map[string]any, error) {
	switch prepared.mode {
	case "stdio":
		return buildStdioEntry(prepared), nil
	case "http":
		return buildHTTPEntry(prepared)
	default:
		return nil, fmt.Errorf("unsupported install mode %q", prepared.mode)
	}
}

func buildStdioEntry(prepared preparedInstall) map[string]any {
	entry := map[string]any{
		"command": prepared.command,
		"args":    prepared.args,
	}
	if prepared.target == "claude-code" {
		entry["type"] = "stdio"
		entry["env"] = map[string]any{}
	}
	return entry
}

func buildHTTPEntry(prepared preparedInstall) (map[string]any, error) {
	return map[string]any{
		"type": "streamable-http",
		"url":  prepared.httpURL,
	}, nil
}

func buildHTTPURL(endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	host := parsed.Hostname()
	if host == "" {
		host = strings.TrimSpace(parsed.Host)
	}
	if host == "" {
		return "", fmt.Errorf("endpoint host is empty")
	}
	return (&url.URL{
		Scheme: parsed.Scheme,
		Host:   net.JoinHostPort(host, defaultHTTPPort),
		Path:   defaultHTTPPath,
	}).String(), nil
}

func shouldEnsureLocalHTTP(httpURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(httpURL))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func resolveInstallRepoRoot(paths ...string) string {
	for _, rawPath := range paths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		dir := filepath.Dir(path)
		if filepath.Base(dir) == "configs" {
			repoRoot := filepath.Dir(dir)
			if looksLikeInstallRepoRoot(repoRoot) {
				return repoRoot
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if looksLikeInstallRepoRoot(cwd) {
			return cwd
		}
	}
	return ""
}

func looksLikeInstallRepoRoot(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	required := []string{
		"docker-compose.yml",
		filepath.Join("configs", "system.toml"),
		filepath.Join("configs", "symbols-index.toml"),
		"go.mod",
	}
	for _, rel := range required {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

func runCommand(ctx context.Context, dir string, name string, args ...string) error {
	return runCommandFunc(ctx, dir, name, args...)
}

func absoluteExecutablePath(path string) (string, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("command path must point to a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("command path must be executable")
	}
	return filepath.Abs(path)
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func absoluteExistingFile(path string) (string, error) {
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("must be a file path")
	}
	return filepath.Abs(path)
}

func writeAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(content); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}
