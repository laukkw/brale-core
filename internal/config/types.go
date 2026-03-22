// 本文件主要内容：定义系统与策略配置结构。
package config

type SystemConfig struct {
	Hash                    string                    `mapstructure:"-"`
	LogFormat               string                    `mapstructure:"log_format"`
	LogLevel                string                    `mapstructure:"log_level"`
	LogPath                 string                    `mapstructure:"log_path"`
	DBPath                  string                    `mapstructure:"db_path"`
	PersistMode             string                    `mapstructure:"persist_mode"`
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
	EnableScheduledDecision *bool                     `mapstructure:"enable_scheduled_decision"`
}

type SystemLLMConfig struct {
	SessionMode string `mapstructure:"session_mode"`
}

type LLMModelConfig struct {
	Endpoint    string `mapstructure:"endpoint"`
	APIKey      string `mapstructure:"api_key"`
	TimeoutSec  *int   `mapstructure:"timeout_sec"`
	Concurrency *int   `mapstructure:"concurrency"`
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
	Indicators IndicatorConfig `mapstructure:"indicators"`
	Consensus  ConsensusConfig `mapstructure:"consensus"`
	Cooldown   CooldownConfig  `mapstructure:"cooldown"`
	LLM        SymbolLLMConfig `mapstructure:"llm"`
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
	EMAFast    int  `mapstructure:"ema_fast"`
	EMAMid     int  `mapstructure:"ema_mid"`
	EMASlow    int  `mapstructure:"ema_slow"`
	RSIPeriod  int  `mapstructure:"rsi_period"`
	ATRPeriod  int  `mapstructure:"atr_period"`
	MACDFast   int  `mapstructure:"macd_fast"`
	MACDSlow   int  `mapstructure:"macd_slow"`
	MACDSignal int  `mapstructure:"macd_signal"`
	LastN      int  `mapstructure:"last_n"`
	Pretty     bool `mapstructure:"pretty"`
}

type CooldownConfig struct {
	Enabled          bool  `mapstructure:"enabled"`
	EntryCooldownSec int64 `mapstructure:"entry_cooldown_sec"`
}

type SymbolLLMConfig struct {
	SessionMode string     `mapstructure:"session_mode"`
	Agent       LLMRoleSet `mapstructure:"agent"`
	Provider    LLMRoleSet `mapstructure:"provider"`
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
	Sieve             RiskManagementSieveConfig `mapstructure:"sieve"`
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
