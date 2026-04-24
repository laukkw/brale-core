package config

import (
	"strings"
	"time"
)

func ValidateSystemConfig(cfg SystemConfig) error {
	if strings.TrimSpace(cfg.Database.DSN) == "" {
		return validationErrorf("database.dsn is required")
	}
	if !IsSupportedPromptLocale(cfg.Prompt.Locale) {
		return validationErrorf("prompt.locale must be one of zh/en")
	}
	if strings.TrimSpace(cfg.ExecutionSystem) == "" {
		return validationErrorf("execution_system is required")
	}
	if cfg.ExecutionSystem == "freqtrade" {
		if strings.TrimSpace(cfg.ExecEndpoint) == "" {
			return validationErrorf("exec_endpoint is required for freqtrade")
		}
	}
	if strings.TrimSpace(cfg.LLMMinInterval) != "" {
		val, err := time.ParseDuration(strings.TrimSpace(cfg.LLMMinInterval))
		if err != nil {
			return validationErrorf("llm_min_interval must be a valid duration")
		}
		if val <= 0 {
			return validationErrorf("llm_min_interval must be > 0")
		}
	}
	if cfg.LLM.RoundRecorderTimeoutSec != nil && *cfg.LLM.RoundRecorderTimeoutSec < 0 {
		return validationErrorf("llm.round_recorder_timeout_sec must be >= 0")
	}
	if cfg.LLM.RoundRecorderRetries != nil && *cfg.LLM.RoundRecorderRetries < 0 {
		return validationErrorf("llm.round_recorder_retries must be >= 0")
	}
	if err := validateLLMModelConfigs(cfg.LLMModels); err != nil {
		return err
	}
	if err := validateWebhookConfig(cfg.Webhook); err != nil {
		return err
	}
	if err := validateNotificationConfig(cfg.Notification); err != nil {
		return err
	}
	switch cfg.Scheduler.Backend {
	case "", "river", "builtin":
	default:
		return validationErrorf("scheduler.backend must be one of river/builtin")
	}
	if strings.TrimSpace(cfg.Reconcile.CloseRecoverAfter) != "" {
		val, err := time.ParseDuration(strings.TrimSpace(cfg.Reconcile.CloseRecoverAfter))
		if err != nil {
			return validationErrorf("reconcile.close_recover_after must be a valid duration")
		}
		if val <= 0 {
			return validationErrorf("reconcile.close_recover_after must be > 0")
		}
	}
	return nil
}

func validateLLMModelConfigs(models map[string]LLMModelConfig) error {
	for model, modelCfg := range models {
		if strings.TrimSpace(model) == "" {
			return validationErrorf("llm_models contains empty model name")
		}
		if strings.TrimSpace(modelCfg.Endpoint) == "" {
			return validationErrorf("llm_models.%s.endpoint is required", model)
		}
		if strings.TrimSpace(modelCfg.APIKey) == "" {
			return validationErrorf("llm_models.%s.api_key is required", model)
		}
		if modelCfg.TimeoutSec != nil && *modelCfg.TimeoutSec < 0 {
			return validationErrorf("llm_models.%s.timeout_sec must be >=0", model)
		}
		if modelCfg.Concurrency != nil && *modelCfg.Concurrency <= 0 {
			return validationErrorf("llm_models.%s.concurrency must be > 0", model)
		}
	}
	return nil
}

func validateNotificationConfig(cfg NotificationConfig) error {
	feishuBotMode := NormalizeFeishuBotMode(cfg.Feishu.BotMode)
	if cfg.Feishu.BotEnabled {
		if feishuBotMode != "long_connection" && feishuBotMode != "callback" {
			return validationErrorf("notification.feishu.bot_mode must be one of long_connection/callback")
		}
	}
	if !cfg.Enabled {
		return nil
	}
	if !cfg.Telegram.Enabled && !cfg.Feishu.Enabled && !cfg.Email.Enabled {
		return validationErrorf("notification.enabled requires at least one outbound channel")
	}
	if cfg.Telegram.Enabled {
		if strings.TrimSpace(cfg.Telegram.Token) == "" {
			return validationErrorf("notification.telegram.token is required")
		}
	}
	if cfg.Telegram.Enabled {
		if cfg.Telegram.ChatID == 0 {
			return validationErrorf("notification.telegram.chat_id is required")
		}
	}
	if cfg.Feishu.Enabled || cfg.Feishu.BotEnabled {
		if strings.TrimSpace(cfg.Feishu.AppID) == "" {
			return validationErrorf("notification.feishu.app_id is required")
		}
		if strings.TrimSpace(cfg.Feishu.AppSecret) == "" {
			return validationErrorf("notification.feishu.app_secret is required")
		}
	}
	if cfg.Feishu.Enabled {
		typ := strings.TrimSpace(strings.ToLower(cfg.Feishu.DefaultReceiveIDType))
		if typ == "" {
			return validationErrorf("notification.feishu.default_receive_id_type is required")
		}
		switch typ {
		case "chat_id", "open_id", "user_id", "union_id", "email":
		default:
			return validationErrorf("notification.feishu.default_receive_id_type must be one of chat_id/open_id/user_id/union_id/email")
		}
		if strings.TrimSpace(cfg.Feishu.DefaultReceiveID) == "" {
			return validationErrorf("notification.feishu.default_receive_id is required")
		}
	}
	if cfg.Feishu.BotEnabled {
		if feishuBotMode == "callback" {
			if strings.TrimSpace(cfg.Feishu.VerificationToken) == "" {
				return validationErrorf("notification.feishu.verification_token is required")
			}
		}
	}
	if cfg.Email.Enabled {
		if strings.TrimSpace(cfg.Email.SMTPHost) == "" {
			return validationErrorf("notification.email.smtp_host is required")
		}
		if cfg.Email.SMTPPort <= 0 {
			return validationErrorf("notification.email.smtp_port is required")
		}
		if strings.TrimSpace(cfg.Email.SMTPUser) == "" {
			return validationErrorf("notification.email.smtp_user is required")
		}
		if strings.TrimSpace(cfg.Email.SMTPPass) == "" {
			return validationErrorf("notification.email.smtp_pass is required")
		}
		if strings.TrimSpace(cfg.Email.From) == "" {
			return validationErrorf("notification.email.from is required")
		}
		if len(cfg.Email.To) == 0 {
			return validationErrorf("notification.email.to is required")
		}
	}
	return nil
}

func validateWebhookConfig(cfg WebhookConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return validationErrorf("webhook.addr is required when webhook.enabled=true")
	}
	for _, entry := range cfg.IPAllowlist {
		if strings.TrimSpace(entry) == "" {
			return validationErrorf("webhook.ip_allowlist contains empty entry")
		}
	}
	if cfg.QueueSize < 0 {
		return validationErrorf("webhook.queue_size must be >= 0")
	}
	if cfg.WorkerCount < 0 {
		return validationErrorf("webhook.worker_count must be >= 0")
	}
	if cfg.FallbackOrderPollSec < 0 {
		return validationErrorf("webhook.fallback_order_poll_sec must be >= 0")
	}
	if cfg.FallbackReconcileSec < 0 {
		return validationErrorf("webhook.fallback_reconcile_sec must be >= 0")
	}
	return nil
}
