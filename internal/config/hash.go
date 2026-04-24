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
	LogFormat                  string          `json:"log_format,omitempty"`
	LogLevel                   string          `json:"log_level,omitempty"`
	LogPath                    string          `json:"log_path,omitempty"`
	PromptLocale               string          `json:"prompt_locale,omitempty"`
	Database                   DatabaseConfig  `json:"database"`
	ExecutionSystem            string          `json:"execution_system,omitempty"`
	ExecEndpoint               string          `json:"exec_endpoint,omitempty"`
	ExecAPIKey                 string          `json:"exec_api_key,omitempty"`
	ExecAPISecret              string          `json:"exec_api_secret,omitempty"`
	ExecAuth                   string          `json:"exec_auth,omitempty"`
	LLMMinInterval             string          `json:"llm_min_interval,omitempty"`
	LLMModels                  []llmModelEntry `json:"llm_models,omitempty"`
	Webhook                    webhookHash     `json:"webhook,omitempty"`
	SchedulerBackend           string          `json:"scheduler_backend,omitempty"`
	ReconcileCloseRecoverAfter string          `json:"reconcile_close_recover_after,omitempty"`
	EnableScheduledDecision    bool            `json:"enable_scheduled_decision,omitempty"`
	TokenBudgetPerRound        int             `json:"token_budget_per_round,omitempty"`
	TokenBudgetWarnPct         int             `json:"token_budget_warn_pct,omitempty"`
	RiskGuardMaxDrawdownPct    float64         `json:"risk_guard_max_drawdown_pct,omitempty"`
	RoundRecorderTimeoutSec    *int            `json:"round_recorder_timeout_sec,omitempty"`
	RoundRecorderRetries       *int            `json:"round_recorder_retries,omitempty"`
}

type llmModelEntry struct {
	Model            string `json:"model"`
	Endpoint         string `json:"endpoint,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	TimeoutSec       *int   `json:"timeout_sec,omitempty"`
	Concurrency      *int   `json:"concurrency,omitempty"`
	StructuredOutput bool   `json:"structured_output,omitempty"`
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
	Symbol     string                `json:"symbol"`
	Intervals  []string              `json:"intervals,omitempty"`
	KlineLimit int                   `json:"kline_limit,omitempty"`
	Agent      symbolAgentHash       `json:"agent,omitempty"`
	Require    SymbolRequire         `json:"require,omitempty"`
	Features   symbolFeaturesHash    `json:"features,omitempty"`
	Indicators IndicatorConfig       `json:"indicators,omitempty"`
	Memory     MemoryConfig          `json:"memory,omitempty"`
	Consensus  ConsensusConfig       `json:"consensus,omitempty"`
	Cooldown   CooldownConfig        `json:"cooldown,omitempty"`
	LLM        llmRoleSetByStageHash `json:"llm,omitempty"`
}

type llmRoleSetByStageHash struct {
	Agent    llmRoleSetHash `json:"agent,omitempty"`
	Provider llmRoleSetHash `json:"provider,omitempty"`
}

type symbolAgentHash struct {
	Indicator bool `json:"indicator"`
	Structure bool `json:"structure"`
	Mechanics bool `json:"mechanics"`
}

type symbolFeaturesHash struct {
	Indicator indicatorFeaturesHash `json:"indicator,omitempty"`
	Structure structureFeaturesHash `json:"structure,omitempty"`
	Mechanics mechanicsFeaturesHash `json:"mechanics,omitempty"`
}

type indicatorFeaturesHash struct {
	EMA          bool `json:"ema,omitempty"`
	RSI          bool `json:"rsi,omitempty"`
	ATR          bool `json:"atr,omitempty"`
	OBV          bool `json:"obv,omitempty"`
	STC          bool `json:"stc,omitempty"`
	BB           bool `json:"bb,omitempty"`
	CHOP         bool `json:"chop,omitempty"`
	StochRSI     bool `json:"stoch_rsi,omitempty"`
	Aroon        bool `json:"aroon,omitempty"`
	TDSequential bool `json:"td_sequential,omitempty"`
}

type structureFeaturesHash struct {
	Supertrend bool `json:"supertrend,omitempty"`
	EMAContext bool `json:"ema_context,omitempty"`
	RSIContext bool `json:"rsi_context,omitempty"`
	Patterns   bool `json:"patterns,omitempty"`
	SMC        bool `json:"smc,omitempty"`
}

type mechanicsFeaturesHash struct {
	OI               bool `json:"oi,omitempty"`
	Funding          bool `json:"funding,omitempty"`
	LongShort        bool `json:"long_short,omitempty"`
	FearGreed        bool `json:"fear_greed,omitempty"`
	Liquidations     bool `json:"liquidations,omitempty"`
	CVD              bool `json:"cvd,omitempty"`
	Sentiment        bool `json:"sentiment,omitempty"`
	FuturesSentiment bool `json:"futures_sentiment,omitempty"`
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
		LogFormat:                  cfg.LogFormat,
		LogLevel:                   cfg.LogLevel,
		LogPath:                    cfg.LogPath,
		PromptLocale:               NormalizePromptLocale(cfg.Prompt.Locale),
		Database:                   cfg.Database,
		ExecutionSystem:            cfg.ExecutionSystem,
		ExecEndpoint:               cfg.ExecEndpoint,
		ExecAPIKey:                 cfg.ExecAPIKey,
		ExecAPISecret:              cfg.ExecAPISecret,
		ExecAuth:                   cfg.ExecAuth,
		LLMMinInterval:             cfg.LLMMinInterval,
		LLMModels:                  sortedLLMModels(cfg.LLMModels),
		Webhook:                    buildWebhookHash(cfg.Webhook),
		SchedulerBackend:           cfg.Scheduler.Backend,
		ReconcileCloseRecoverAfter: cfg.Reconcile.CloseRecoverAfter,
		EnableScheduledDecision:    cfg.EnableScheduledDecision != nil && *cfg.EnableScheduledDecision,
		TokenBudgetPerRound:        cfg.LLM.TokenBudgetPerRound,
		TokenBudgetWarnPct:         cfg.LLM.TokenBudgetWarnPct,
		RiskGuardMaxDrawdownPct:    cfg.RiskGuard.MaxDrawdownPct,
		RoundRecorderTimeoutSec:    cloneIntPtr(cfg.LLM.RoundRecorderTimeoutSec),
		RoundRecorderRetries:       cloneIntPtr(cfg.LLM.RoundRecorderRetries),
	}
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func buildSymbolHashInput(cfg SymbolConfig) symbolHashInput {
	return symbolHashInput{
		Symbol:     cfg.Symbol,
		Intervals:  sortedStrings(cfg.Intervals),
		KlineLimit: cfg.KlineLimit,
		Agent:      buildSymbolAgentHash(cfg.Agent),
		Require:    cfg.Require,
		Features:   buildSymbolFeaturesHash(cfg.Features),
		Indicators: cfg.Indicators,
		Memory:     cfg.Memory,
		Consensus:  cfg.Consensus,
		Cooldown:   cfg.Cooldown,
		LLM:        buildSymbolLLMHashInput(cfg.LLM),
	}
}

func buildSymbolLLMHashInput(cfg SymbolLLMConfig) llmRoleSetByStageHash {
	return llmRoleSetByStageHash{
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

func buildSymbolFeaturesHash(cfg SymbolFeatures) symbolFeaturesHash {
	return symbolFeaturesHash{
		Indicator: indicatorFeaturesHash{
			EMA:          boolValue(cfg.Indicator.EMA),
			RSI:          boolValue(cfg.Indicator.RSI),
			ATR:          boolValue(cfg.Indicator.ATR),
			OBV:          boolValue(cfg.Indicator.OBV),
			STC:          boolValue(cfg.Indicator.STC),
			BB:           boolValue(cfg.Indicator.BB),
			CHOP:         boolValue(cfg.Indicator.CHOP),
			StochRSI:     boolValue(cfg.Indicator.StochRSI),
			Aroon:        boolValue(cfg.Indicator.Aroon),
			TDSequential: boolValue(cfg.Indicator.TDSequential),
		},
		Structure: structureFeaturesHash{
			Supertrend: boolValue(cfg.Structure.Supertrend),
			EMAContext: boolValue(cfg.Structure.EMAContext),
			RSIContext: boolValue(cfg.Structure.RSIContext),
			Patterns:   boolValue(cfg.Structure.Patterns),
			SMC:        boolValue(cfg.Structure.SMC),
		},
		Mechanics: mechanicsFeaturesHash{
			OI:               boolValue(cfg.Mechanics.OI),
			Funding:          boolValue(cfg.Mechanics.Funding),
			LongShort:        boolValue(cfg.Mechanics.LongShort),
			FearGreed:        boolValue(cfg.Mechanics.FearGreed),
			Liquidations:     boolValue(cfg.Mechanics.Liquidations),
			CVD:              boolValue(cfg.Mechanics.CVD),
			Sentiment:        boolValue(cfg.Mechanics.Sentiment),
			FuturesSentiment: boolValue(cfg.Mechanics.FuturesSentiment),
		},
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
			Model:            model,
			Endpoint:         cfg.Endpoint,
			APIKey:           cfg.APIKey,
			TimeoutSec:       cfg.TimeoutSec,
			Concurrency:      cfg.Concurrency,
			StructuredOutput: boolValue(cfg.StructuredOutput),
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
