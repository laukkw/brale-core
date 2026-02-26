// 本文件主要内容：生成 System/Symbol/Strategy 配置哈希，保证顺序稳定与可审计。

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func HashSystemConfig(cfg SystemConfig) (string, error) {
	input := buildSystemHashInput(cfg)
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return sha256Hex(raw), nil
}

func HashSymbolConfig(cfg SymbolConfig) (string, error) {
	input := buildSymbolHashInput(cfg)
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return sha256Hex(raw), nil
}

func HashStrategyConfig(cfg StrategyConfig) (string, error) {
	input := buildStrategyHashInput(cfg)
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return sha256Hex(raw), nil
}

func CombineHashes(parts ...string) string {
	if len(parts) == 0 {
		return sha256Hex(nil)
	}
	joined := strings.Join(parts, "|")
	return sha256Hex([]byte(joined))
}

type systemHashInput struct {
	LogFormat               string          `json:"log_format,omitempty"`
	LogLevel                string          `json:"log_level,omitempty"`
	LogPath                 string          `json:"log_path,omitempty"`
	DBPath                  string          `json:"db_path,omitempty"`
	PersistMode             string          `json:"persist_mode,omitempty"`
	ExecutionSystem         string          `json:"execution_system,omitempty"`
	ExecEndpoint            string          `json:"exec_endpoint,omitempty"`
	ExecAPIKey              string          `json:"exec_api_key,omitempty"`
	ExecAPISecret           string          `json:"exec_api_secret,omitempty"`
	ExecAuth                string          `json:"exec_auth,omitempty"`
	LLMMinInterval          string          `json:"llm_min_interval,omitempty"`
	LLMModels               []llmModelEntry `json:"llm_models,omitempty"`
	Webhook                 webhookHash     `json:"webhook,omitempty"`
	EnableScheduledDecision bool            `json:"enable_scheduled_decision,omitempty"`
}

type llmModelEntry struct {
	Model       string `json:"model"`
	Endpoint    string `json:"endpoint,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	TimeoutSec  *int   `json:"timeout_sec,omitempty"`
	Concurrency *int   `json:"concurrency,omitempty"`
}

type webhookHash struct {
	Enabled              bool     `json:"enabled,omitempty"`
	Addr                 string   `json:"addr,omitempty"`
	Secret               string   `json:"secret,omitempty"`
	IPAllowlist          []string `json:"ip_allowlist,omitempty"`
	QueueSize            int      `json:"queue_size,omitempty"`
	WorkerCount          int      `json:"worker_count,omitempty"`
	FallbackOrderPollSec int      `json:"fallback_order_poll_sec,omitempty"`
	FallbackReconcileSec int      `json:"fallback_reconcile_sec,omitempty"`
}

type symbolHashInput struct {
	Symbol     string             `json:"symbol"`
	Intervals  []string           `json:"intervals,omitempty"`
	KlineLimit int                `json:"kline_limit,omitempty"`
	Agent      symbolAgentHash    `json:"agent,omitempty"`
	Require    SymbolRequire      `json:"require,omitempty"`
	Indicators IndicatorConfig    `json:"indicators,omitempty"`
	Consensus  ConsensusConfig    `json:"consensus,omitempty"`
	Cooldown   CooldownConfig     `json:"cooldown,omitempty"`
	LLM        symbolLLMHashInput `json:"llm,omitempty"`
}

type symbolLLMHashInput struct {
	Agent    llmRoleSetHash `json:"agent,omitempty"`
	Provider llmRoleSetHash `json:"provider,omitempty"`
}

type symbolAgentHash struct {
	Indicator bool `json:"indicator"`
	Structure bool `json:"structure"`
	Mechanics bool `json:"mechanics"`
}

type llmRoleSetHash struct {
	Indicator llmRoleHash `json:"indicator,omitempty"`
	Structure llmRoleHash `json:"structure,omitempty"`
	Mechanics llmRoleHash `json:"mechanics,omitempty"`
}

type llmRoleHash struct {
	Model       string  `json:"model,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type strategyHashInput struct {
	Symbol         string               `json:"symbol"`
	ID             string               `json:"id"`
	RuleChainPath  string               `json:"rule_chain"`
	RiskManagement RiskManagementConfig `json:"risk_management"`
}

func buildSystemHashInput(cfg SystemConfig) systemHashInput {
	return systemHashInput{
		LogFormat:               cfg.LogFormat,
		LogLevel:                cfg.LogLevel,
		LogPath:                 cfg.LogPath,
		DBPath:                  cfg.DBPath,
		PersistMode:             cfg.PersistMode,
		ExecutionSystem:         cfg.ExecutionSystem,
		ExecEndpoint:            cfg.ExecEndpoint,
		ExecAPIKey:              cfg.ExecAPIKey,
		ExecAPISecret:           cfg.ExecAPISecret,
		ExecAuth:                cfg.ExecAuth,
		LLMMinInterval:          cfg.LLMMinInterval,
		LLMModels:               sortedLLMModels(cfg.LLMModels),
		Webhook:                 buildWebhookHash(cfg.Webhook),
		EnableScheduledDecision: cfg.EnableScheduledDecision != nil && *cfg.EnableScheduledDecision,
	}
}

func buildSymbolHashInput(cfg SymbolConfig) symbolHashInput {
	return symbolHashInput{
		Symbol:     cfg.Symbol,
		Intervals:  sortedStrings(cfg.Intervals),
		KlineLimit: cfg.KlineLimit,
		Agent:      buildSymbolAgentHash(cfg.Agent),
		Require:    cfg.Require,
		Indicators: cfg.Indicators,
		Consensus:  cfg.Consensus,
		Cooldown:   cfg.Cooldown,
		LLM:        buildSymbolLLMHashInput(cfg.LLM),
	}
}

func buildSymbolLLMHashInput(cfg SymbolLLMConfig) symbolLLMHashInput {
	return symbolLLMHashInput{
		Agent:    buildRoleSetHash(cfg.Agent),
		Provider: buildRoleSetHash(cfg.Provider),
	}
}

func buildSymbolAgentHash(cfg AgentConfig) symbolAgentHash {
	return symbolAgentHash{
		Indicator: boolValue(cfg.Indicator),
		Structure: boolValue(cfg.Structure),
		Mechanics: boolValue(cfg.Mechanics),
	}
}

func buildRoleSetHash(cfg LLMRoleSet) llmRoleSetHash {
	return llmRoleSetHash{
		Indicator: buildRoleHash(cfg.Indicator),
		Structure: buildRoleHash(cfg.Structure),
		Mechanics: buildRoleHash(cfg.Mechanics),
	}
}

func buildRoleHash(cfg LLMRoleConfig) llmRoleHash {
	val := 0.0
	if cfg.Temperature != nil {
		val = *cfg.Temperature
	}
	return llmRoleHash{
		Model:       cfg.Model,
		Temperature: val,
	}
}

func boolValue(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}

func buildStrategyHashInput(cfg StrategyConfig) strategyHashInput {
	return strategyHashInput{
		Symbol:         cfg.Symbol,
		ID:             cfg.ID,
		RuleChainPath:  cfg.RuleChainPath,
		RiskManagement: cfg.RiskManagement,
	}
}

func sortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func sortedLLMModels(in map[string]LLMModelConfig) []llmModelEntry {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]llmModelEntry, 0, len(keys))
	for _, model := range keys {
		cfg := in[model]
		out = append(out, llmModelEntry{
			Model:       model,
			Endpoint:    cfg.Endpoint,
			APIKey:      cfg.APIKey,
			TimeoutSec:  cfg.TimeoutSec,
			Concurrency: cfg.Concurrency,
		})
	}
	return out
}

func buildWebhookHash(cfg WebhookConfig) webhookHash {
	return webhookHash{
		Enabled:              cfg.Enabled,
		Addr:                 cfg.Addr,
		Secret:               cfg.Secret,
		IPAllowlist:          sortedStrings(cfg.IPAllowlist),
		QueueSize:            cfg.QueueSize,
		WorkerCount:          cfg.WorkerCount,
		FallbackOrderPollSec: cfg.FallbackOrderPollSec,
		FallbackReconcileSec: cfg.FallbackReconcileSec,
	}
}

func sha256Hex(in []byte) string {
	sum := sha256.Sum256(in)
	return hex.EncodeToString(sum[:])
}
