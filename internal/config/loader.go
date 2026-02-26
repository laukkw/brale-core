// 本文件主要内容：读取 system/symbol/strategy 配置并执行校验与哈希。

package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

var envPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func LoadSystemConfig(path string) (SystemConfig, error) {
	cfg, err := loadFromFile[SystemConfig](path)
	if err != nil {
		return SystemConfig{}, err
	}
	if cfg.EnableScheduledDecision == nil {
		defaultOn := true
		cfg.EnableScheduledDecision = &defaultOn
	}
	applyNewsOverlayDefaults(&cfg.NewsOverlay)
	applyWebhookDefaults(&cfg.Webhook)
	cfg.PersistMode = normalizePersistMode(cfg.PersistMode)
	if err := ValidateSystemConfig(cfg); err != nil {
		return SystemConfig{}, err
	}
	hash, err := HashSystemConfig(cfg)
	if err != nil {
		return SystemConfig{}, err
	}
	cfg.Hash = hash
	return cfg, nil
}

func applyNewsOverlayDefaults(cfg *NewsOverlayConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.SourceMode) == "" {
		cfg.SourceMode = "doc"
	}
	if strings.TrimSpace(cfg.Interval) == "" {
		cfg.Interval = "1h"
	}
	if strings.TrimSpace(cfg.SnapshotStaleAfter) == "" {
		cfg.SnapshotStaleAfter = "4h"
	}
	if cfg.MaxRecords <= 0 {
		cfg.MaxRecords = 20
	}
	if cfg.MinItems1H <= 0 {
		cfg.MinItems1H = 3
	}
	if cfg.MinItems4H <= 0 {
		cfg.MinItems4H = 5
	}
	if cfg.MinEffectiveItems1H <= 0 {
		cfg.MinEffectiveItems1H = 2
	}
	if cfg.MinEffectiveItems4H <= 0 {
		cfg.MinEffectiveItems4H = 3
	}
	if cfg.MaxItemsPerDomain <= 0 {
		cfg.MaxItemsPerDomain = 4
	}
	if cfg.TightenThreshold1H <= 0 {
		cfg.TightenThreshold1H = 80
	}
	if cfg.TightenThreshold4H <= 0 {
		cfg.TightenThreshold4H = 75
	}
	cfg.Queries = normalizeQueryList(cfg.Queries)
	if len(cfg.Queries) == 0 && strings.TrimSpace(cfg.Query) == "" {
		cfg.Queries = []string{
			`(liquidation OR hack OR exploit OR "funding rate" OR "open interest") sourcelang:english`,
			`("spot bitcoin etf" OR "spot ethereum etf" OR SEC OR CFTC) sourcelang:english`,
			`(bitcoin OR BTC OR ethereum OR ETH OR crypto) sourcelang:english`,
			`(solana OR SOL OR XRP OR stablecoin OR "crypto exchange") sourcelang:english`,
			`("Federal Reserve" OR FOMC OR Treasury OR "digital asset") sourcelang:english`,
		}
	}
	if len(cfg.Queries) == 0 && strings.TrimSpace(cfg.Query) != "" {
		cfg.Queries = []string{strings.TrimSpace(cfg.Query)}
	}
	if strings.TrimSpace(cfg.Query) == "" && len(cfg.Queries) > 0 {
		cfg.Query = strings.Join(cfg.Queries, " || ")
	}
	if len(cfg.BlockedDomains) == 0 {
		cfg.BlockedDomains = []string{
			"themarketsdaily.com",
			"tickerreport.com",
			"dailypolitical.com",
		}
	}
	if len(cfg.BlockedTitleKeywords) == 0 {
		cfg.BlockedTitleKeywords = []string{
			"shares sold by",
			"boosts stock position in",
			"raises stake in",
			"largest position",
			"stock position lifted by",
			"shares acquired by",
			"holdings in",
			"position in",
			"purchases",
		}
	}
	cfg.BlockedDomains = normalizeStringList(cfg.BlockedDomains)
	cfg.BlockedTitleKeywords = normalizeStringList(cfg.BlockedTitleKeywords)
}

func applyWebhookDefaults(cfg *WebhookConfig) {
	if cfg == nil {
		return
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1024
	}
	if cfg.WorkerCount == 0 {
		cfg.WorkerCount = 4
	}
	if cfg.FallbackOrderPollSec == 0 {
		cfg.FallbackOrderPollSec = 180
	}
	if cfg.FallbackReconcileSec == 0 {
		cfg.FallbackReconcileSec = 300
	}
}

func LoadSymbolIndexConfig(path string) (SymbolIndexConfig, error) {
	cfg, err := loadFromFile[SymbolIndexConfig](path)
	if err != nil {
		return SymbolIndexConfig{}, err
	}
	if err := ValidateSymbolIndexConfig(cfg); err != nil {
		return SymbolIndexConfig{}, err
	}
	return cfg, nil
}

func LoadSymbolConfig(path string) (SymbolConfig, error) {
	cfg, err := loadFromFile[SymbolConfig](path)
	if err != nil {
		return SymbolConfig{}, err
	}
	if err := ValidateSymbolConfig(cfg); err != nil {
		return SymbolConfig{}, err
	}
	hash, err := HashSymbolConfig(cfg)
	if err != nil {
		return SymbolConfig{}, err
	}
	cfg.Hash = hash
	return cfg, nil
}

func LoadStrategyConfig(path string) (StrategyConfig, error) {
	cfg, err := loadFromFile[StrategyConfig](path)
	if err != nil {
		return StrategyConfig{}, err
	}
	return finalizeStrategyConfig(cfg)
}

func LoadStrategyConfigWithSymbol(path, symbol string) (StrategyConfig, error) {
	cfg, err := loadFromFile[StrategyConfig](path)
	if err != nil {
		return StrategyConfig{}, err
	}
	if strings.TrimSpace(cfg.Symbol) == "" {
		cfg.Symbol = symbol
	}
	return finalizeStrategyConfig(cfg)
}

func finalizeStrategyConfig(cfg StrategyConfig) (StrategyConfig, error) {
	if err := ValidateStrategyConfig(cfg); err != nil {
		return StrategyConfig{}, err
	}
	hash, err := HashStrategyConfig(cfg)
	if err != nil {
		return StrategyConfig{}, err
	}
	cfg.Hash = hash
	return cfg, nil
}

func loadFromFile[T any](path string) (T, error) {
	var cfg T
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	resolved, err := resolveTomlEnvPlaceholders(path, data)
	if err != nil {
		return cfg, err
	}
	normalized := deduplicateTomlTables(resolved)
	// Preserve dots in keys like model names (e.g. "openai/gpt-5.2").
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigType("toml")
	if err := v.ReadConfig(bytes.NewReader(normalized)); err != nil {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal config %s: %w", path, err)
	}
	if symCfg, ok := any(&cfg).(*SymbolConfig); ok {
		defaults, err := DefaultSymbolConfig(SystemConfig{}, symCfg.Symbol)
		if err != nil {
			return cfg, err
		}
		ApplyDecisionDefaults(symCfg, defaults)
	}
	if stratCfg, ok := any(&cfg).(*StrategyConfig); ok {
		defaults := DefaultStrategyConfig(stratCfg.Symbol)
		ApplyStrategyDefaults(stratCfg, defaults)
	}
	return cfg, nil
}

func resolveTomlEnvPlaceholders(configPath string, data []byte) ([]byte, error) {
	dotEnvVars, err := loadNearestDotEnv(configPath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	missing := map[string]struct{}{}
	expanded := envPlaceholderPattern.ReplaceAllStringFunc(content, func(token string) string {
		matches := envPlaceholderPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			return token
		}
		key := matches[1]
		if val, ok := lookupEnvValue(key, dotEnvVars); ok {
			return val
		}
		missing[key] = struct{}{}
		return token
	})
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for key := range missing {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("resolve config %s: missing env vars: %s", configPath, strings.Join(keys, ", "))
	}
	return []byte(expanded), nil
}

func lookupEnvValue(key string, dotEnvVars map[string]string) (string, bool) {
	if val, ok := dotEnvVars[key]; ok {
		return val, true
	}
	return os.LookupEnv(key)
}

func loadNearestDotEnv(configPath string) (map[string]string, error) {
	dotEnvPath, ok, err := findNearestDotEnvPath(configPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	vars, err := parseDotEnvFile(dotEnvPath)
	if err != nil {
		return nil, fmt.Errorf("read .env %s: %w", dotEnvPath, err)
	}
	return vars, nil
}

func findNearestDotEnvPath(configPath string) (string, bool, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", false, fmt.Errorf("resolve config path %s: %w", configPath, err)
	}
	dir := filepath.Dir(absPath)
	for {
		candidate := filepath.Join(dir, ".env")
		info, statErr := os.Stat(candidate)
		switch {
		case statErr == nil && !info.IsDir():
			return candidate, true, nil
		case statErr != nil && !os.IsNotExist(statErr):
			return "", false, statErr
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false, nil
}

func parseDotEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	vars := make(map[string]string)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid line %d", lineNo)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("empty key on line %d", lineNo)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		vars[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return vars, nil
}

type tomlSection struct {
	name    string
	lines   []string
	isArray bool
}

func deduplicateTomlTables(data []byte) []byte {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	current := tomlSection{name: ""}
	var sections []tomlSection
	for scanner.Scan() {
		line := scanner.Text()
		name, isArray, ok := parseTomlHeader(line)
		if ok {
			if current.name != "" || len(current.lines) > 0 {
				sections = append(sections, current)
			}
			current = tomlSection{name: name, isArray: isArray, lines: []string{line}}
			continue
		}
		current.lines = append(current.lines, line)
	}
	if current.name != "" || len(current.lines) > 0 {
		sections = append(sections, current)
	}

	lastIndex := map[string]int{}
	for i, section := range sections {
		if section.isArray {
			continue
		}
		lastIndex[section.name] = i
	}

	var merged []string
	for i, section := range sections {
		if !section.isArray {
			if idx, ok := lastIndex[section.name]; ok && idx != i {
				continue
			}
		}
		merged = append(merged, section.lines...)
	}
	return []byte(strings.Join(merged, "\n"))
}

func parseTomlHeader(line string) (string, bool, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "[") {
		return "", false, false
	}
	header := strings.TrimSpace(strings.SplitN(trimmed, "#", 2)[0])
	if !strings.HasPrefix(header, "[") || !strings.HasSuffix(header, "]") {
		return "", false, false
	}
	isArray := strings.HasPrefix(header, "[[")
	name := strings.Trim(header, "[]")
	if name == "" {
		return "", false, false
	}
	return name, isArray, true
}

func normalizeStringList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		val := strings.ToLower(strings.TrimSpace(item))
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	return out
}

func normalizeQueryList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		val := strings.TrimSpace(item)
		if val == "" {
			continue
		}
		key := strings.ToLower(val)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, val)
	}
	return out
}
