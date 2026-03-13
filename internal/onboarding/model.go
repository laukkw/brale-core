package onboarding

type Request struct {
	DryRun                     bool                    `json:"dry_run"`
	MaxOpenTrades              int                     `json:"max_open_trades"`
	Symbols                    []string                `json:"symbols"`
	SymbolDetail               map[string]SymbolDetail `json:"symbol_detail"`
	ExecUsername               string                  `json:"exec_username"`
	ExecSecret                 string                  `json:"exec_secret"`
	ProxyEnabled               bool                    `json:"proxy_enabled"`
	ProxyHost                  string                  `json:"proxy_host"`
	ProxyPort                  int                     `json:"proxy_port"`
	ProxyScheme                string                  `json:"proxy_scheme"`
	ProxyNoProxy               string                  `json:"proxy_no_proxy"`
	LLMModelIndicator          string                  `json:"llm_model_indicator"`
	LLMIndicatorEndpoint       string                  `json:"llm_indicator_endpoint"`
	LLMIndicatorKey            string                  `json:"llm_indicator_key"`
	LLMModelStructure          string                  `json:"llm_model_structure"`
	LLMStructureEndpoint       string                  `json:"llm_structure_endpoint"`
	LLMStructureKey            string                  `json:"llm_structure_key"`
	LLMModelMechanics          string                  `json:"llm_model_mechanics"`
	LLMMechanicsEndpoint       string                  `json:"llm_mechanics_endpoint"`
	LLMMechanicsKey            string                  `json:"llm_mechanics_key"`
	TelegramEnabled            bool                    `json:"telegram_enabled"`
	TelegramToken              string                  `json:"telegram_token"`
	TelegramChatID             string                  `json:"telegram_chat_id"`
	FeishuEnabled              bool                    `json:"feishu_enabled"`
	FeishuAppID                string                  `json:"feishu_app_id"`
	FeishuAppSecret            string                  `json:"feishu_app_secret"`
	FeishuBotEnabled           bool                    `json:"feishu_bot_enabled"`
	FeishuBotMode              string                  `json:"feishu_bot_mode"`
	FeishuVerificationToken    string                  `json:"feishu_verification_token"`
	FeishuEncryptKey           string                  `json:"feishu_encrypt_key"`
	FeishuDefaultReceiveIDType string                  `json:"feishu_default_receive_id_type"`
	FeishuDefaultReceiveID     string                  `json:"feishu_default_receive_id"`
}

type SymbolDetail struct {
	RiskPerTradePct             float64  `json:"risk_per_trade_pct"`
	MaxInvestPct                float64  `json:"max_invest_pct"`
	MaxLeverage                 int      `json:"max_leverage"`
	Intervals                   []string `json:"intervals"`
	EntryMode                   string   `json:"entry_mode"`
	ExitPolicy                  string   `json:"exit_policy"`
	TightenMinUpdateIntervalSec int      `json:"tighten_min_update_interval_sec"`
	EMAFast                     int      `json:"ema_fast"`
	EMAMid                      int      `json:"ema_mid"`
	EMASlow                     int      `json:"ema_slow"`
	RSIPeriod                   int      `json:"rsi_period"`
	LastN                       int      `json:"last_n"`
	MACDFast                    int      `json:"macd_fast"`
	MACDSlow                    int      `json:"macd_slow"`
	MACDSignal                  int      `json:"macd_signal"`
}
