// 本文件主要内容：定义系统与策略配置结构。
package config

type SystemConfig struct {
	Hash                    string                    `mapstructure:"-"`
	LogFormat               string                    `mapstructure:"log_format"`
	LogLevel                string                    `mapstructure:"log_level"`
	LogPath                 string                    `mapstructure:"log_path"`
	Database                DatabaseConfig            `mapstructure:"database"`
	ExecutionSystem         string                    `mapstructure:"execution_system"`
	ExecEndpoint            string                    `mapstructure:"exec_endpoint"`
	ExecAPIKey              string                    `mapstructure:"exec_api_key"`
	ExecAPISecret           string                    `mapstructure:"exec_api_secret"`
	ExecAuth                string                    `mapstructure:"exec_auth"`
	LLM                     SystemLLMConfig           `mapstructure:"llm"`
	LLMMinInterval          string                    `mapstructure:"llm_min_interval"`
	LLMModels               map[string]LLMModelConfig `mapstructure:"llm_models"`
	Webhook                 WebhookConfig             `mapstructure:"webhook"`
	Notification            NotificationConfig        `mapstructure:"notification"`
	Telemetry               TelemetryConfig           `mapstructure:"telemetry"`
	Scheduler               SchedulerConfig           `mapstructure:"scheduler"`
	Reconcile               ReconcileConfig           `mapstructure:"reconcile"`
	RiskGuard               RiskGuardConfig           `mapstructure:"risk_guard"`
	Prompt                  PromptConfig              `mapstructure:"prompt"`
	EnableScheduledDecision *bool                     `mapstructure:"enable_scheduled_decision"`
}

type PromptConfig struct {
	Locale string `mapstructure:"locale"`
}

type DatabaseConfig struct {
	DSN          string `mapstructure:"dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

type TelemetryConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ExporterType string `mapstructure:"exporter"`
	Endpoint     string `mapstructure:"endpoint"`
	ServiceName  string `mapstructure:"service_name"`
}

type SchedulerConfig struct {
	Backend string `mapstructure:"backend"`
}

type ReconcileConfig struct {
	CloseRecoverAfter string `mapstructure:"close_recover_after"`
}

type SystemLLMConfig struct {
	TokenBudgetPerRound     int  `mapstructure:"token_budget_per_round"`
	TokenBudgetWarnPct      int  `mapstructure:"token_budget_warn_pct"`
	RoundRecorderTimeoutSec *int `mapstructure:"round_recorder_timeout_sec"`
	RoundRecorderRetries    *int `mapstructure:"round_recorder_retries"`
}

type RiskGuardConfig struct {
	MaxDrawdownPct float64 `mapstructure:"max_drawdown_pct"`
}

type LLMModelConfig struct {
	Endpoint         string `mapstructure:"endpoint"`
	APIKey           string `mapstructure:"api_key"`
	TimeoutSec       *int   `mapstructure:"timeout_sec"`
	Concurrency      *int   `mapstructure:"concurrency"`
	StructuredOutput *bool  `mapstructure:"structured_output"`
}

type WebhookConfig struct {
	Enabled              bool     `mapstructure:"enabled"`
	Addr                 string   `mapstructure:"addr"`
	Secret               string   `mapstructure:"secret"`
	IPAllowlist          []string `mapstructure:"ip_allowlist"`
	QueueSize            int      `mapstructure:"queue_size"`
	WorkerCount          int      `mapstructure:"worker_count"`
	FallbackOrderPollSec int      `mapstructure:"fallback_order_poll_sec"`
	FallbackReconcileSec int      `mapstructure:"fallback_reconcile_sec"`
}

type NotificationConfig struct {
	Enabled              bool           `mapstructure:"enabled"`
	StartupNotifyEnabled bool           `mapstructure:"startup_notify_enabled"`
	Telegram             TelegramConfig `mapstructure:"telegram"`
	Feishu               FeishuConfig   `mapstructure:"feishu"`
	Email                EmailConfig    `mapstructure:"email"`
}

type TelegramConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Token   string `mapstructure:"token"`
	ChatID  int64  `mapstructure:"chat_id"`
}

type FeishuConfig struct {
	Enabled              bool   `mapstructure:"enabled"`
	AppID                string `mapstructure:"app_id"`
	AppSecret            string `mapstructure:"app_secret"`
	BotEnabled           bool   `mapstructure:"bot_enabled"`
	BotMode              string `mapstructure:"bot_mode"`
	VerificationToken    string `mapstructure:"verification_token"`
	EncryptKey           string `mapstructure:"encrypt_key"`
	DefaultReceiveIDType string `mapstructure:"default_receive_id_type"`
	DefaultReceiveID     string `mapstructure:"default_receive_id"`
}

type EmailConfig struct {
	Enabled  bool     `mapstructure:"enabled"`
	SMTPHost string   `mapstructure:"smtp_host"`
	SMTPPort int      `mapstructure:"smtp_port"`
	SMTPUser string   `mapstructure:"smtp_user"`
	SMTPPass string   `mapstructure:"smtp_pass"`
	From     string   `mapstructure:"from"`
	To       []string `mapstructure:"to"`
}

type SymbolIndexConfig struct {
	Symbols []SymbolIndexEntry `mapstructure:"symbols"`
}

type SymbolIndexEntry struct {
	Symbol   string `mapstructure:"symbol"`
	Config   string `mapstructure:"config"`
	Strategy string `mapstructure:"strategy"`
}

type SymbolConfig struct {
	Hash       string          `mapstructure:"-"`
	Symbol     string          `mapstructure:"symbol"`
	Intervals  []string        `mapstructure:"intervals"`
	KlineLimit int             `mapstructure:"kline_limit"`
	Agent      AgentConfig     `mapstructure:"agent"`
	Require    SymbolRequire   `mapstructure:"require"`
	Features   SymbolFeatures  `mapstructure:"features"`
	Indicators IndicatorConfig `mapstructure:"indicators"`
	Memory     MemoryConfig    `mapstructure:"memory"`
	Consensus  ConsensusConfig `mapstructure:"consensus"`
	Cooldown   CooldownConfig  `mapstructure:"cooldown"`
	LLM        SymbolLLMConfig `mapstructure:"llm"`
}

type SymbolFeatures struct {
	Indicator IndicatorFeatureConfig `mapstructure:"indicator"`
	Structure StructureFeatureConfig `mapstructure:"structure"`
	Mechanics MechanicsFeatureConfig `mapstructure:"mechanics"`
}

type IndicatorFeatureConfig struct {
	EMA          *bool `mapstructure:"ema"`
	RSI          *bool `mapstructure:"rsi"`
	ATR          *bool `mapstructure:"atr"`
	OBV          *bool `mapstructure:"obv"`
	STC          *bool `mapstructure:"stc"`
	BB           *bool `mapstructure:"bb"`
	CHOP         *bool `mapstructure:"chop"`
	StochRSI     *bool `mapstructure:"stoch_rsi"`
	Aroon        *bool `mapstructure:"aroon"`
	TDSequential *bool `mapstructure:"td_sequential"`
}

type StructureFeatureConfig struct {
	Supertrend *bool `mapstructure:"supertrend"`
	EMAContext *bool `mapstructure:"ema_context"`
	RSIContext *bool `mapstructure:"rsi_context"`
	Patterns   *bool `mapstructure:"patterns"`
	SMC        *bool `mapstructure:"smc"`
}

type MechanicsFeatureConfig struct {
	OI               *bool `mapstructure:"oi"`
	Funding          *bool `mapstructure:"funding"`
	LongShort        *bool `mapstructure:"long_short"`
	FearGreed        *bool `mapstructure:"fear_greed"`
	Liquidations     *bool `mapstructure:"liquidations"`
	CVD              *bool `mapstructure:"cvd"`
	Sentiment        *bool `mapstructure:"sentiment"`
	FuturesSentiment *bool `mapstructure:"futures_sentiment"`
}

type ConsensusConfig struct {
	ScoreThreshold      float64 `mapstructure:"score_threshold"`
	ConfidenceThreshold float64 `mapstructure:"confidence_threshold"`
}

type AgentConfig struct {
	Indicator *bool `mapstructure:"indicator"`
	Structure *bool `mapstructure:"structure"`
	Mechanics *bool `mapstructure:"mechanics"`
}

type SymbolRequire struct {
	OI           bool `mapstructure:"oi"`
	Funding      bool `mapstructure:"funding"`
	LongShort    bool `mapstructure:"long_short"`
	FearGreed    bool `mapstructure:"fear_greed"`
	Liquidations bool `mapstructure:"liquidations"`
}

type IndicatorConfig struct {
	Engine         string  `mapstructure:"engine"`
	ShadowEngine   string  `mapstructure:"shadow_engine"`
	EMAFast        int     `mapstructure:"ema_fast"`
	EMAMid         int     `mapstructure:"ema_mid"`
	EMASlow        int     `mapstructure:"ema_slow"`
	RSIPeriod      int     `mapstructure:"rsi_period"`
	ATRPeriod      int     `mapstructure:"atr_period"`
	STCFast        int     `mapstructure:"stc_fast"`
	STCSlow        int     `mapstructure:"stc_slow"`
	BBPeriod       int     `mapstructure:"bb_period"`
	BBMultiplier   float64 `mapstructure:"bb_multiplier"`
	CHOPPeriod     int     `mapstructure:"chop_period"`
	StochRSIPeriod int     `mapstructure:"stoch_rsi_period"`
	AroonPeriod    int     `mapstructure:"aroon_period"`
	SkipSTC        bool    `mapstructure:"skip_stc"`
	LastN          int     `mapstructure:"last_n"`
	Pretty         bool    `mapstructure:"pretty"`
}

type MemoryConfig struct {
	Enabled              bool `mapstructure:"enabled"`
	WorkingMemorySize    int  `mapstructure:"working_memory_size"`
	EpisodicEnabled      bool `mapstructure:"episodic_enabled"`
	EpisodicTTLDays      int  `mapstructure:"episodic_ttl_days"`
	EpisodicMaxPerSymbol int  `mapstructure:"episodic_max_per_symbol"`
	SemanticEnabled      bool `mapstructure:"semantic_enabled"`
	SemanticMaxRules     int  `mapstructure:"semantic_max_rules"`
}

type CooldownConfig struct {
	Enabled          bool  `mapstructure:"enabled"`
	EntryCooldownSec int64 `mapstructure:"entry_cooldown_sec"`
}

type SymbolLLMConfig struct {
	Agent    LLMRoleSet `mapstructure:"agent"`
	Provider LLMRoleSet `mapstructure:"provider"`
}

type LLMRoleSet struct {
	Indicator LLMRoleConfig `mapstructure:"indicator"`
	Structure LLMRoleConfig `mapstructure:"structure"`
	Mechanics LLMRoleConfig `mapstructure:"mechanics"`
}

type LLMRoleConfig struct {
	Model       string   `mapstructure:"model"`
	Temperature *float64 `mapstructure:"temperature"`
}

type StrategyConfig struct {
	Hash           string               `mapstructure:"-"`
	Symbol         string               `mapstructure:"symbol"`
	ID             string               `mapstructure:"id"`
	RuleChainPath  string               `mapstructure:"rule_chain"`
	RiskManagement RiskManagementConfig `mapstructure:"risk_management"`
}

type RiskManagementConfig struct {
	RiskPerTradePct   float64                   `mapstructure:"risk_per_trade_pct"`
	MaxInvestPct      float64                   `mapstructure:"max_invest_pct"`
	MaxLeverage       float64                   `mapstructure:"max_leverage"`
	Grade1Factor      float64                   `mapstructure:"grade_1_factor"`
	Grade2Factor      float64                   `mapstructure:"grade_2_factor"`
	Grade3Factor      float64                   `mapstructure:"grade_3_factor"`
	EntryOffsetATR    float64                   `mapstructure:"entry_offset_atr"`
	EntryMode         string                    `mapstructure:"entry_mode"`
	OrderbookDepth    int                       `mapstructure:"orderbook_depth"`
	BreakevenFeePct   float64                   `mapstructure:"breakeven_fee_pct"`
	SlippageBufferPct float64                   `mapstructure:"slippage_buffer_pct"`
	RiskStrategy      RiskStrategyConfig        `mapstructure:"risk_strategy"`
	InitialExit       InitialExitConfig         `mapstructure:"initial_exit"`
	TightenATR        TightenATRConfig          `mapstructure:"tighten_atr"`
	Gate              GateConfig                `mapstructure:"gate"`
	HardGuard         HardGuardToggleConfig     `mapstructure:"hard_guard"`
	Sieve             RiskManagementSieveConfig `mapstructure:"sieve"`
}

type GateConfig struct {
	QualityThreshold float64            `mapstructure:"quality_threshold"`
	EdgeThreshold    float64            `mapstructure:"edge_threshold"`
	HardStop         GateHardStopConfig `mapstructure:"hard_stop"`
}

type GateHardStopConfig struct {
	StructureInvalidation *bool `mapstructure:"structure_invalidation"`
	LiquidationCascade    *bool `mapstructure:"liquidation_cascade"`
}

func (cfg GateHardStopConfig) StructureInvalidationEnabled() bool {
	return boolOrDefault(cfg.StructureInvalidation, true)
}

func (cfg GateHardStopConfig) LiquidationCascadeEnabled() bool {
	return boolOrDefault(cfg.LiquidationCascade, true)
}

type HardGuardToggleConfig struct {
	Enabled        *bool `mapstructure:"enabled"`
	StopLoss       *bool `mapstructure:"stop_loss"`
	RSIExtreme     *bool `mapstructure:"rsi_extreme"`
	CircuitBreaker *bool `mapstructure:"circuit_breaker"`
}

func (cfg HardGuardToggleConfig) HardGuardEnabled() bool {
	return boolOrDefault(cfg.Enabled, true)
}

func (cfg HardGuardToggleConfig) StopLossEnabled() bool {
	return boolOrDefault(cfg.StopLoss, true)
}

func (cfg HardGuardToggleConfig) RSIExtremeEnabled() bool {
	return boolOrDefault(cfg.RSIExtreme, true)
}

func (cfg HardGuardToggleConfig) CircuitBreakerEnabled() bool {
	return boolOrDefault(cfg.CircuitBreaker, true)
}

func boolOrDefault(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

type RiskStrategyConfig struct {
	Mode string `mapstructure:"mode"`
}

type InitialExitConfig struct {
	Policy            string         `mapstructure:"policy"`
	StructureInterval string         `mapstructure:"structure_interval"`
	Params            map[string]any `mapstructure:"params"`
}

type TightenATRConfig struct {
	StructureThreatened  float64 `mapstructure:"structure_threatened"`
	TP1ATR               float64 `mapstructure:"tp1_atr"`
	TP2ATR               float64 `mapstructure:"tp2_atr"`
	MinTPDistancePct     float64 `mapstructure:"min_tp_distance_pct"`
	MinTPGapPct          float64 `mapstructure:"min_tp_gap_pct"`
	MinUpdateIntervalSec int64   `mapstructure:"min_update_interval_sec"`
}

type RiskManagementSieveConfig struct {
	MinSizeFactor     float64                  `mapstructure:"min_size_factor"`
	DefaultGateAction string                   `mapstructure:"default_gate_action"`
	DefaultSizeFactor float64                  `mapstructure:"default_size_factor"`
	Rows              []RiskManagementSieveRow `mapstructure:"rows"`
}

type RiskManagementSieveRow struct {
	MechanicsTag  string  `mapstructure:"mechanics_tag"`
	LiqConfidence string  `mapstructure:"liq_confidence"`
	CrowdingAlign *bool   `mapstructure:"crowding_align"`
	GateAction    string  `mapstructure:"gate_action"`
	SizeFactor    float64 `mapstructure:"size_factor"`
	ReasonCode    string  `mapstructure:"reason_code"`
}
