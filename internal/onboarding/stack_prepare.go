package onboarding

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultNoProxy = "localhost,127.0.0.1,brale,freqtrade"
)

var trueValues = map[string]struct{}{
	"1":    {},
	"true": {},
	"yes":  {},
	"on":   {},
}

type stackPrepareOptions struct {
	envFile      string
	configIn     string
	configOut    string
	proxyEnvOut  string
	systemIn     string
	systemOut    string
	execEndpoint string
	checkOnly    bool
}

func RunPrepareStack(args []string, repoRoot string, out io.Writer) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is empty")
	}
	repoAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	if out == nil {
		out = io.Discard
	}

	opts, err := parseStackPrepareFlags(args)
	if err != nil {
		return err
	}

	envFile, err := resolvePathUnderRoot(repoAbs, opts.envFile)
	if err != nil {
		return fmt.Errorf("resolve env file: %w", err)
	}
	configIn, err := resolvePathUnderRoot(repoAbs, opts.configIn)
	if err != nil {
		return fmt.Errorf("resolve config input: %w", err)
	}
	configOut, err := resolvePathUnderRoot(repoAbs, opts.configOut)
	if err != nil {
		return fmt.Errorf("resolve config output: %w", err)
	}
	proxyEnvOut, err := resolvePathUnderRoot(repoAbs, opts.proxyEnvOut)
	if err != nil {
		return fmt.Errorf("resolve proxy env output: %w", err)
	}
	systemIn, err := resolvePathUnderRoot(repoAbs, opts.systemIn)
	if err != nil {
		return fmt.Errorf("resolve system input: %w", err)
	}
	var systemOut string
	if strings.TrimSpace(opts.systemOut) != "" {
		systemOut, err = resolvePathUnderRoot(repoAbs, opts.systemOut)
		if err != nil {
			return fmt.Errorf("resolve system output: %w", err)
		}
	}

	env, err := ParseEnvFile(envFile)
	if err != nil {
		return err
	}
	username, err := requireNonEmptyEnv(env, "EXEC_USERNAME")
	if err != nil {
		return err
	}
	password, err := requireNonEmptyEnv(env, "EXEC_SECRET")
	if err != nil {
		return err
	}
	proxyEnabled, proxyURL, noProxy, err := parseProxyFromEnv(env)
	if err != nil {
		return err
	}

	configRaw, err := os.ReadFile(configIn)
	if err != nil {
		return fmt.Errorf("freqtrade base config not found: %s", opts.configIn)
	}
	systemRaw, err := os.ReadFile(systemIn)
	if err != nil {
		return fmt.Errorf("system config not found: %s", opts.systemIn)
	}

	if opts.checkOnly {
		state := "disabled"
		if proxyEnabled {
			state = "enabled"
		}
		fmt.Fprintf(out, "[OK] precheck passed: proxy=%s\n", state)
		return nil
	}

	mutatedConfig, err := mutateFreqtradeConfig(configRaw, username, password, proxyEnabled, proxyURL)
	if err != nil {
		return err
	}
	proxyEnvContent := renderProxyEnvForPrepare(proxyEnabled, proxyURL, noProxy)
	var renderedSystem string
	if systemOut != "" {
		renderedSystem, err = renderSystemConfigForPrepare(systemRaw, opts.execEndpoint)
		if err != nil {
			return err
		}
	}

	if err := writeAtomic(repoAbs, configOut, mutatedConfig); err != nil {
		return fmt.Errorf("write config output: %w", err)
	}
	if err := writeAtomic(repoAbs, proxyEnvOut, proxyEnvContent); err != nil {
		return fmt.Errorf("write proxy env output: %w", err)
	}
	if systemOut != "" {
		if err := writeAtomic(repoAbs, systemOut, renderedSystem); err != nil {
			return fmt.Errorf("write system output: %w", err)
		}
	}

	state := "disabled"
	if proxyEnabled {
		state = "enabled"
	}
	fmt.Fprintf(out, "[OK] generated %s (proxy=%s)\n", opts.configOut, state)
	fmt.Fprintf(out, "[OK] generated %s\n", opts.proxyEnvOut)
	if systemOut != "" {
		fmt.Fprintf(out, "[OK] generated %s\n", opts.systemOut)
	}
	return nil
}

func parseStackPrepareFlags(args []string) (stackPrepareOptions, error) {
	opts := stackPrepareOptions{}
	fs := flag.NewFlagSet("prepare-stack", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.envFile, "env-file", ".env", "environment file")
	fs.StringVar(&opts.configIn, "config-in", "configs/freqtrade/config.base.json", "freqtrade base config")
	fs.StringVar(&opts.configOut, "config-out", "data/freqtrade/user_data/config.json", "freqtrade runtime config")
	fs.StringVar(&opts.proxyEnvOut, "proxy-env-out", "data/freqtrade/proxy.env", "stack proxy env output")
	fs.StringVar(&opts.systemIn, "system-in", "configs/system.toml", "system config input")
	fs.StringVar(&opts.systemOut, "system-out", "", "optional system config output")
	fs.StringVar(&opts.execEndpoint, "exec-endpoint", "http://freqtrade:8080/api/v1", "execution endpoint in output system config")
	fs.BoolVar(&opts.checkOnly, "check-only", false, "validate config only")
	if err := fs.Parse(args); err != nil {
		return stackPrepareOptions{}, err
	}
	return opts, nil
}

func resolvePathUnderRoot(root string, path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}
	if !filepath.IsAbs(trimmed) {
		trimmed = filepath.Join(root, trimmed)
	}
	clean := filepath.Clean(trimmed)
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository root: %s", path)
	}
	return clean, nil
}

// ParseEnvFile reads a .env file and returns a map of key-value pairs.
// It supports comments, export prefixes, and quoted values.
func ParseEnvFile(path string) (map[string]string, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("env file not found: %s", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}
		idx := strings.Index(trimmed, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:idx])
		value := strings.TrimSpace(trimmed[idx+1:])
		if key == "" {
			continue
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	return values, nil
}

func requireNonEmptyEnv(env map[string]string, key string) (string, error) {
	value := strings.TrimSpace(env[key])
	if value == "" {
		return "", fmt.Errorf("missing required env key: %s", key)
	}
	return value, nil
}

func parseProxyFromEnv(env map[string]string) (bool, string, string, error) {
	enabled := parseBool(env["PROXY_ENABLED"])
	noProxy := strings.TrimSpace(env["PROXY_NO_PROXY"])
	if noProxy == "" {
		noProxy = defaultNoProxy
	}
	if !enabled {
		return false, "", noProxy, nil
	}
	rawPort := strings.TrimSpace(env["PROXY_PORT"])
	if rawPort == "" {
		return false, "", "", errors.New("PROXY_ENABLED=true but PROXY_PORT is empty")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return false, "", "", fmt.Errorf("PROXY_PORT is not an integer: %s", rawPort)
	}
	if port < 1 || port > 65535 {
		return false, "", "", fmt.Errorf("PROXY_PORT out of range [1,65535]: %d", port)
	}
	host := strings.TrimSpace(env["PROXY_HOST"])
	if host == "" {
		host = "host.docker.internal"
	}
	scheme := strings.ToLower(strings.TrimSpace(env["PROXY_SCHEME"]))
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return false, "", "", fmt.Errorf("PROXY_SCHEME unsupported: %s", scheme)
	}
	return true, fmt.Sprintf("%s://%s:%d", scheme, host, port), noProxy, nil
}

func parseBool(raw string) bool {
	_, ok := trueValues[strings.ToLower(strings.TrimSpace(raw))]
	return ok
}

func mutateFreqtradeConfig(baseRaw []byte, username string, password string, proxyEnabled bool, proxyURL string) (string, error) {
	var base map[string]any
	if err := json.Unmarshal(baseRaw, &base); err != nil {
		return "", err
	}

	apiServer := ensureMap(base, "api_server")
	apiServer["username"] = username
	apiServer["password"] = password

	webhook := ensureMap(base, "webhook")
	webhook["url"] = "http://brale:9991/api/live/freqtrade/webhook"

	exchange := ensureMap(base, "exchange")
	ccxtCfg := ensureMap(exchange, "ccxt_config")
	ccxtAsync := ensureMap(exchange, "ccxt_async_config")
	if proxyEnabled {
		ccxtCfg["proxies"] = map[string]any{"http": proxyURL, "https": proxyURL}
		ccxtAsync["aiohttp_proxy"] = proxyURL
	} else {
		delete(ccxtCfg, "proxies")
		delete(ccxtAsync, "aiohttp_proxy")
	}

	out, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

func renderProxyEnvForPrepare(proxyEnabled bool, proxyURL string, noProxy string) string {
	lines := []string{"# generated by onboarding prepare-stack"}
	if proxyEnabled {
		lines = append(lines,
			"HTTP_PROXY="+proxyURL,
			"HTTPS_PROXY="+proxyURL,
			"http_proxy="+proxyURL,
			"https_proxy="+proxyURL,
		)
	}
	lines = append(lines,
		"NO_PROXY="+noProxy,
		"no_proxy="+noProxy,
	)
	return strings.Join(lines, "\n") + "\n"
}

func renderSystemConfigForPrepare(input []byte, execEndpoint string) (string, error) {
	lines := strings.Split(string(input), "\n")
	replaced := false
	for idx, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if replaced || !strings.HasPrefix(trimmed, "exec_endpoint") || !strings.Contains(trimmed, "=") {
			continue
		}
		indentLen := len(line) - len(trimmed)
		indent := line[:indentLen]
		lines[idx] = fmt.Sprintf("%sexec_endpoint = \"%s\" # generated for docker compose", indent, execEndpoint)
		replaced = true
	}
	if !replaced {
		return "", errors.New("exec_endpoint key not found in system config")
	}
	return strings.Join(lines, "\n") + "\n", nil
}
