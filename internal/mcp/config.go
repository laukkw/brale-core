package mcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	runtimecfg "brale-core/internal/runtime"
)

type LocalConfigSource struct {
	SystemPath string
	IndexPath  string
}

type IndicatorSpec struct {
	KlineLimit int
	Engine     string
	Options    features.IndicatorCompressOptions
}

type ConfigView struct {
	SystemPath string                    `json:"system_path"`
	IndexPath  string                    `json:"index_path"`
	System     sanitizedSystemConfig     `json:"system"`
	Index      []config.SymbolIndexEntry `json:"index"`
	Symbol     *config.SymbolConfig      `json:"symbol,omitempty"`
	Strategy   *config.StrategyConfig    `json:"strategy,omitempty"`
}

type sanitizedSystemConfig struct {
	LogFormat               string                       `json:"log_format,omitempty"`
	LogLevel                string                       `json:"log_level,omitempty"`
	LogPath                 string                       `json:"log_path,omitempty"`
	PromptLocale            string                       `json:"prompt_locale,omitempty"`
	Database                config.DatabaseConfig        `json:"database"`
	ExecutionSystem         string                       `json:"execution_system,omitempty"`
	ExecEndpoint            string                       `json:"exec_endpoint,omitempty"`
	ExecAuth                string                       `json:"exec_auth,omitempty"`
	HasExecAPIKey           bool                         `json:"has_exec_api_key"`
	HasExecAPISecret        bool                         `json:"has_exec_api_secret"`
	LLMModels               map[string]sanitizedLLMModel `json:"llm_models,omitempty"`
	Webhook                 sanitizedWebhookConfig       `json:"webhook"`
	Notification            sanitizedNotificationConfig  `json:"notification"`
	EnableScheduledDecision *bool                        `json:"enable_scheduled_decision,omitempty"`
}

type sanitizedLLMModel struct {
	Endpoint         string `json:"endpoint,omitempty"`
	TimeoutSec       *int   `json:"timeout_sec,omitempty"`
	Concurrency      *int   `json:"concurrency,omitempty"`
	StructuredOutput *bool  `json:"structured_output,omitempty"`
	HasAPIKey        bool   `json:"has_api_key"`
}

type sanitizedWebhookConfig struct {
	Enabled              bool     `json:"enabled"`
	Addr                 string   `json:"addr,omitempty"`
	IPAllowlist          []string `json:"ip_allowlist,omitempty"`
	QueueSize            int      `json:"queue_size,omitempty"`
	WorkerCount          int      `json:"worker_count,omitempty"`
	FallbackOrderPollSec int      `json:"fallback_order_poll_sec,omitempty"`
	FallbackReconcileSec int      `json:"fallback_reconcile_sec,omitempty"`
	HasSecret            bool     `json:"has_secret"`
}

type sanitizedNotificationConfig struct {
	Enabled              bool                    `json:"enabled"`
	StartupNotifyEnabled bool                    `json:"startup_notify_enabled"`
	Telegram             sanitizedTelegramConfig `json:"telegram"`
	Feishu               sanitizedFeishuConfig   `json:"feishu"`
	Email                sanitizedEmailConfig    `json:"email"`
}

type sanitizedTelegramConfig struct {
	Enabled  bool  `json:"enabled"`
	ChatID   int64 `json:"chat_id,omitempty"`
	HasToken bool  `json:"has_token"`
}

type sanitizedFeishuConfig struct {
	Enabled              bool   `json:"enabled"`
	AppID                string `json:"app_id,omitempty"`
	BotEnabled           bool   `json:"bot_enabled"`
	BotMode              string `json:"bot_mode,omitempty"`
	DefaultReceiveIDType string `json:"default_receive_id_type,omitempty"`
	DefaultReceiveID     string `json:"default_receive_id,omitempty"`
	HasAppSecret         bool   `json:"has_app_secret"`
	HasVerificationToken bool   `json:"has_verification_token"`
	HasEncryptKey        bool   `json:"has_encrypt_key"`
}

type sanitizedEmailConfig struct {
	Enabled     bool     `json:"enabled"`
	SMTPHost    string   `json:"smtp_host,omitempty"`
	SMTPPort    int      `json:"smtp_port,omitempty"`
	SMTPUser    string   `json:"smtp_user,omitempty"`
	From        string   `json:"from,omitempty"`
	To          []string `json:"to,omitempty"`
	HasSMTPPass bool     `json:"has_smtp_pass"`
}

func NewLocalConfigSource(systemPath, indexPath string) *LocalConfigSource {
	return &LocalConfigSource{
		SystemPath: filepath.Clean(systemPath),
		IndexPath:  filepath.Clean(indexPath),
	}
}

func (s *LocalConfigSource) LoadConfigView(symbol string) (ConfigView, error) {
	if s == nil {
		return ConfigView{}, fmt.Errorf("config source is required")
	}
	sys, err := config.LoadSystemConfig(s.SystemPath)
	if err != nil {
		return ConfigView{}, fmt.Errorf("load system config: %w", err)
	}
	indexCfg, err := config.LoadSymbolIndexConfig(s.IndexPath)
	if err != nil {
		return ConfigView{}, fmt.Errorf("load symbol index config: %w", err)
	}
	view := ConfigView{
		SystemPath: s.SystemPath,
		IndexPath:  s.IndexPath,
		System:     sanitizeSystemConfig(sys),
		Index:      indexCfg.Symbols,
	}
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return view, nil
	}
	for _, entry := range indexCfg.Symbols {
		if decisionutil.NormalizeSymbol(entry.Symbol) != symbol {
			continue
		}
		symbolCfg, strategyCfg, _, err := runtimecfg.LoadSymbolConfigs(sys, s.IndexPath, entry)
		if err != nil {
			return ConfigView{}, fmt.Errorf("load symbol configs: %w", err)
		}
		view.Symbol = &symbolCfg
		view.Strategy = &strategyCfg
		return view, nil
	}
	return ConfigView{}, fmt.Errorf("symbol %s not found in %s", symbol, s.IndexPath)
}

func (s *LocalConfigSource) LoadIndicatorSpec(symbol string) (IndicatorSpec, error) {
	view, err := s.LoadConfigView(symbol)
	if err != nil {
		return IndicatorSpec{}, err
	}
	if view.Symbol == nil {
		return IndicatorSpec{}, fmt.Errorf("symbol %s config is required", decisionutil.NormalizeSymbol(symbol))
	}
	enabled, err := config.ResolveAgentEnabled(view.Symbol.Agent)
	if err != nil {
		return IndicatorSpec{}, fmt.Errorf("resolve agent enabled: %w", err)
	}
	plan := config.ResolveFeaturePlan(*view.Symbol)
	opts := features.IndicatorCompressOptions{
		EMAFast:        view.Symbol.Indicators.EMAFast,
		EMAMid:         view.Symbol.Indicators.EMAMid,
		EMASlow:        view.Symbol.Indicators.EMASlow,
		RSIPeriod:      view.Symbol.Indicators.RSIPeriod,
		ATRPeriod:      view.Symbol.Indicators.ATRPeriod,
		STCFast:        view.Symbol.Indicators.STCFast,
		STCSlow:        view.Symbol.Indicators.STCSlow,
		BBPeriod:       view.Symbol.Indicators.BBPeriod,
		BBMultiplier:   view.Symbol.Indicators.BBMultiplier,
		CHOPPeriod:     view.Symbol.Indicators.CHOPPeriod,
		StochRSIPeriod: view.Symbol.Indicators.StochRSIPeriod,
		AroonPeriod:    view.Symbol.Indicators.AroonPeriod,
		LastN:          view.Symbol.Indicators.LastN,
		Pretty:         view.Symbol.Indicators.Pretty,
	}
	if !enabled.Indicator {
		opts.SkipEMA = true
		opts.SkipRSI = true
		opts.SkipATR = true
		opts.SkipOBV = true
		opts.SkipSTC = true
		opts.SkipBB = true
		opts.SkipCHOP = true
		opts.SkipStochRSI = true
		opts.SkipAroon = true
		opts.SkipTDSequential = true
	} else {
		opts.SkipEMA = !plan.Indicator.EMA
		opts.SkipRSI = !plan.Indicator.RSI
		opts.SkipATR = !plan.Indicator.ATR
		opts.SkipOBV = !plan.Indicator.OBV
		opts.SkipSTC = view.Symbol.Indicators.SkipSTC || !plan.Indicator.STC
		opts.SkipBB = !plan.Indicator.BB
		opts.SkipCHOP = !plan.Indicator.CHOP
		opts.SkipStochRSI = !plan.Indicator.StochRSI
		opts.SkipAroon = !plan.Indicator.Aroon
		opts.SkipTDSequential = !plan.Indicator.TDSequential
	}
	return IndicatorSpec{
		KlineLimit: view.Symbol.KlineLimit,
		Engine:     view.Symbol.Indicators.Engine,
		Options:    opts,
	}, nil
}

func sanitizeSystemConfig(cfg config.SystemConfig) sanitizedSystemConfig {
	out := sanitizedSystemConfig{
		LogFormat:               cfg.LogFormat,
		LogLevel:                cfg.LogLevel,
		LogPath:                 cfg.LogPath,
		PromptLocale:            config.NormalizePromptLocale(cfg.Prompt.Locale),
		Database:                cfg.Database,
		ExecutionSystem:         cfg.ExecutionSystem,
		ExecEndpoint:            cfg.ExecEndpoint,
		ExecAuth:                cfg.ExecAuth,
		HasExecAPIKey:           strings.TrimSpace(cfg.ExecAPIKey) != "",
		HasExecAPISecret:        strings.TrimSpace(cfg.ExecAPISecret) != "",
		EnableScheduledDecision: cfg.EnableScheduledDecision,
		Webhook: sanitizedWebhookConfig{
			Enabled:              cfg.Webhook.Enabled,
			Addr:                 cfg.Webhook.Addr,
			IPAllowlist:          cfg.Webhook.IPAllowlist,
			QueueSize:            cfg.Webhook.QueueSize,
			WorkerCount:          cfg.Webhook.WorkerCount,
			FallbackOrderPollSec: cfg.Webhook.FallbackOrderPollSec,
			FallbackReconcileSec: cfg.Webhook.FallbackReconcileSec,
			HasSecret:            strings.TrimSpace(cfg.Webhook.Secret) != "",
		},
		Notification: sanitizedNotificationConfig{
			Enabled:              cfg.Notification.Enabled,
			StartupNotifyEnabled: cfg.Notification.StartupNotifyEnabled,
			Telegram: sanitizedTelegramConfig{
				Enabled:  cfg.Notification.Telegram.Enabled,
				ChatID:   cfg.Notification.Telegram.ChatID,
				HasToken: strings.TrimSpace(cfg.Notification.Telegram.Token) != "",
			},
			Feishu: sanitizedFeishuConfig{
				Enabled:              cfg.Notification.Feishu.Enabled,
				AppID:                cfg.Notification.Feishu.AppID,
				BotEnabled:           cfg.Notification.Feishu.BotEnabled,
				BotMode:              cfg.Notification.Feishu.BotMode,
				DefaultReceiveIDType: cfg.Notification.Feishu.DefaultReceiveIDType,
				DefaultReceiveID:     cfg.Notification.Feishu.DefaultReceiveID,
				HasAppSecret:         strings.TrimSpace(cfg.Notification.Feishu.AppSecret) != "",
				HasVerificationToken: strings.TrimSpace(cfg.Notification.Feishu.VerificationToken) != "",
				HasEncryptKey:        strings.TrimSpace(cfg.Notification.Feishu.EncryptKey) != "",
			},
			Email: sanitizedEmailConfig{
				Enabled:     cfg.Notification.Email.Enabled,
				SMTPHost:    cfg.Notification.Email.SMTPHost,
				SMTPPort:    cfg.Notification.Email.SMTPPort,
				SMTPUser:    cfg.Notification.Email.SMTPUser,
				From:        cfg.Notification.Email.From,
				To:          cfg.Notification.Email.To,
				HasSMTPPass: strings.TrimSpace(cfg.Notification.Email.SMTPPass) != "",
			},
		},
	}
	if len(cfg.LLMModels) > 0 {
		out.LLMModels = make(map[string]sanitizedLLMModel, len(cfg.LLMModels))
		for name, model := range cfg.LLMModels {
			out.LLMModels[name] = sanitizedLLMModel{
				Endpoint:         model.Endpoint,
				TimeoutSec:       model.TimeoutSec,
				Concurrency:      model.Concurrency,
				StructuredOutput: model.StructuredOutput,
				HasAPIKey:        strings.TrimSpace(model.APIKey) != "",
			}
		}
	}
	return out
}
