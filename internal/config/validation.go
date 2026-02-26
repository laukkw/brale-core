// 本文件主要内容：校验 system/symbol/strategy 配置的必填项与范围。

package config

import (
	"strings"
	"time"

	"brale-core/internal/interval"
	"brale-core/internal/pkg/errclass"
)

const validationScope errclass.Scope = "config"
const validationReason = "invalid_config"

func validationErrorf(format string, args ...any) error {
	return errclass.ValidationErrorf(validationScope, validationReason, format, args...)
}

func ValidateSystemConfig(cfg SystemConfig) error {
	if err := validatePersistMode(cfg.PersistMode); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return validationErrorf("db_path is required")
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
	for model, modelCfg := range cfg.LLMModels {
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
	if err := validateNewsOverlayConfig(cfg.NewsOverlay, cfg.LLMModels); err != nil {
		return err
	}
	if err := validateWebhookConfig(cfg.Webhook); err != nil {
		return err
	}
	if err := validateNotificationConfig(cfg.Notification); err != nil {
		return err
	}
	return nil
}

func validateNewsOverlayConfig(cfg NewsOverlayConfig, llmModels map[string]LLMModelConfig) error {
	if !cfg.Enabled {
		return nil
	}
	intervalVal, err := parseRequiredPositiveDuration(
		cfg.Interval,
		"news_overlay.interval is required when enabled=true",
		"news_overlay.interval must be a valid duration > 0",
	)
	if err != nil {
		return err
	}
	staleVal, err := parseRequiredPositiveDuration(
		cfg.SnapshotStaleAfter,
		"news_overlay.snapshot_stale_after is required when enabled=true",
		"news_overlay.snapshot_stale_after must be a valid duration > 0",
	)
	if err != nil {
		return err
	}
	if staleVal < intervalVal {
		return validationErrorf("news_overlay.snapshot_stale_after must be >= news_overlay.interval")
	}
	if err := validateNewsOverlayLimits(cfg); err != nil {
		return err
	}
	if err := validateNewsOverlaySourceMode(cfg.SourceMode); err != nil {
		return err
	}
	if err := validateNoEmptyItems(cfg.BlockedDomains, "news_overlay.blocked_domains contains empty value"); err != nil {
		return err
	}
	if err := validateNoEmptyItems(cfg.BlockedTitleKeywords, "news_overlay.blocked_title_keywords contains empty value"); err != nil {
		return err
	}
	if err := validateNewsOverlayTightenThresholds(cfg); err != nil {
		return err
	}
	if err := validateNewsOverlayQueries(cfg); err != nil {
		return err
	}
	return validateNewsOverlayModel(cfg.Model, llmModels)
}

func parseRequiredPositiveDuration(raw, requiredErr, invalidErr string) (time.Duration, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, validationErrorf("%s", requiredErr)
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, validationErrorf("%s", invalidErr)
	}
	return duration, nil
}

func validateNewsOverlayLimits(cfg NewsOverlayConfig) error {
	if cfg.MaxRecords <= 0 || cfg.MaxRecords > 250 {
		return validationErrorf("news_overlay.maxrecords must be in [1,250]")
	}
	if cfg.MinItems1H <= 0 {
		return validationErrorf("news_overlay.min_items_1h must be > 0")
	}
	if cfg.MinItems4H <= 0 {
		return validationErrorf("news_overlay.min_items_4h must be > 0")
	}
	if cfg.MinEffectiveItems1H <= 0 {
		return validationErrorf("news_overlay.min_effective_items_1h must be > 0")
	}
	if cfg.MinEffectiveItems4H <= 0 {
		return validationErrorf("news_overlay.min_effective_items_4h must be > 0")
	}
	if cfg.MinEffectiveItems1H > cfg.MaxRecords {
		return validationErrorf("news_overlay.min_effective_items_1h must be <= news_overlay.maxrecords")
	}
	if cfg.MinEffectiveItems4H > cfg.MaxRecords {
		return validationErrorf("news_overlay.min_effective_items_4h must be <= news_overlay.maxrecords")
	}
	if cfg.MaxItemsPerDomain <= 0 {
		return validationErrorf("news_overlay.max_items_per_domain must be > 0")
	}
	if cfg.MaxItemsPerDomain > cfg.MaxRecords {
		return validationErrorf("news_overlay.max_items_per_domain must be <= news_overlay.maxrecords")
	}
	return nil
}

func validateNewsOverlaySourceMode(raw string) error {
	sourceMode := strings.ToLower(strings.TrimSpace(raw))
	if sourceMode == "" {
		sourceMode = "doc"
	}
	if sourceMode != "doc" {
		return validationErrorf("news_overlay.source_mode must be doc")
	}
	return nil
}

func validateNoEmptyItems(items []string, errMsg string) error {
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			return validationErrorf("%s", errMsg)
		}
	}
	return nil
}

func validateNewsOverlayTightenThresholds(cfg NewsOverlayConfig) error {
	if cfg.TightenThreshold1H < 0 || cfg.TightenThreshold1H > 100 {
		return validationErrorf("news_overlay.tighten_threshold_1h must be in [0,100]")
	}
	if cfg.TightenThreshold4H < 0 || cfg.TightenThreshold4H > 100 {
		return validationErrorf("news_overlay.tighten_threshold_4h must be in [0,100]")
	}
	return nil
}

func validateNewsOverlayQueries(cfg NewsOverlayConfig) error {
	queries := cfg.Queries
	if len(queries) == 0 && strings.TrimSpace(cfg.Query) != "" {
		queries = []string{cfg.Query}
	}
	if len(queries) == 0 {
		return validationErrorf("news_overlay.query or news_overlay.queries is required when enabled=true")
	}
	if len(queries) > 8 {
		return validationErrorf("news_overlay.queries must contain at most 8 items")
	}
	for i, query := range queries {
		if strings.TrimSpace(query) == "" {
			return validationErrorf("news_overlay.queries[%d] is empty", i)
		}
	}
	return nil
}

func validateNewsOverlayModel(rawModel string, llmModels map[string]LLMModelConfig) error {
	model := strings.TrimSpace(rawModel)
	if model != "" {
		if len(llmModels) == 0 {
			return validationErrorf("news_overlay.model is set but llm_models is empty")
		}
		if _, ok := llmModels[model]; !ok {
			return validationErrorf("news_overlay.model=%s not found in llm_models", model)
		}
		return nil
	}
	if len(llmModels) == 0 {
		return validationErrorf("news_overlay.enabled=true requires llm_models to be configured")
	}
	return nil
}

func normalizePersistMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	switch mode {
	case "", "minimal", "live":
		return "minimal"
	case "full", "backtest":
		return "full"
	default:
		return mode
	}
}

func validatePersistMode(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "minimal", "full", "live", "backtest":
		return nil
	default:
		return validationErrorf("persist_mode must be minimal or full")
	}
}

func validateNotificationConfig(cfg NotificationConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if !cfg.Telegram.Enabled && !cfg.Email.Enabled {
		return validationErrorf("notification enabled but no channel enabled")
	}
	if cfg.Telegram.Enabled {
		if strings.TrimSpace(cfg.Telegram.Token) == "" {
			return validationErrorf("notification.telegram.token is required")
		}
		if cfg.Telegram.ChatID == 0 {
			return validationErrorf("notification.telegram.chat_id is required")
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

func ValidateSymbolIndexConfig(cfg SymbolIndexConfig) error {
	if len(cfg.Symbols) == 0 {
		return validationErrorf("symbols is required")
	}
	seen := make(map[string]struct{}, len(cfg.Symbols))
	for _, item := range cfg.Symbols {
		sym := strings.TrimSpace(item.Symbol)
		if sym == "" {
			return validationErrorf("symbols.symbol is required")
		}
		if _, ok := seen[sym]; ok {
			return validationErrorf("symbols contains duplicate symbol=%s", sym)
		}
		seen[sym] = struct{}{}
		if strings.TrimSpace(item.Config) == "" {
			return validationErrorf("symbols.%s config path is required", sym)
		}
		if strings.TrimSpace(item.Strategy) == "" {
			return validationErrorf("symbols.%s strategy path is required", sym)
		}
	}
	return nil
}

func ValidateSymbolConfig(cfg SymbolConfig) error {
	if strings.TrimSpace(cfg.Symbol) == "" {
		return validationErrorf("symbol is required")
	}
	if len(cfg.Intervals) == 0 {
		return validationErrorf("intervals is required")
	}
	if cfg.KlineLimit <= 0 {
		return validationErrorf("kline_limit must be > 0")
	}
	enabled, err := ResolveAgentEnabled(cfg.Agent)
	if err != nil {
		return err
	}
	if err := validateIndicatorConfig(cfg.Indicators); err != nil {
		return err
	}
	if err := validateConsensusConfig(cfg.Consensus); err != nil {
		return err
	}
	if err := validateCooldownConfig(cfg.Cooldown); err != nil {
		return err
	}
	if err := validateLLMConfig(cfg.LLM, enabled); err != nil {
		return err
	}
	requiredLimit := requiredKlineLimit(cfg)
	if cfg.KlineLimit < requiredLimit {
		return validationErrorf("kline_limit must be >= %d", requiredLimit)
	}
	return nil
}

func ValidateStrategyConfig(cfg StrategyConfig) error {
	if strings.TrimSpace(cfg.Symbol) == "" {
		return validationErrorf("symbol is required")
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return validationErrorf("id is required")
	}
	if strings.TrimSpace(cfg.RuleChainPath) == "" {
		return validationErrorf("rule_chain is required")
	}
	if err := validateRiskManagement(cfg.RiskManagement); err != nil {
		return err
	}
	return nil
}

func validateRiskManagement(cfg RiskManagementConfig) error {
	if err := validateRiskManagementValues(cfg); err != nil {
		return err
	}
	if err := validateRiskManagementEntry(cfg); err != nil {
		return err
	}
	if err := validateRiskManagementInitialExit(cfg); err != nil {
		return err
	}
	if err := validateRiskManagementTighten(cfg); err != nil {
		return err
	}
	return validateSieveConfig(cfg.Sieve)
}

func validateRiskManagementValues(cfg RiskManagementConfig) error {
	if cfg.RiskPerTradePct <= 0 {
		return validationErrorf("risk_management.risk_per_trade_pct must be > 0")
	}
	if cfg.MaxInvestPct <= 0 || cfg.MaxInvestPct > 1 {
		return validationErrorf("risk_management.max_invest_pct must be in (0,1]")
	}
	if cfg.MaxLeverage <= 0 {
		return validationErrorf("risk_management.max_leverage must be > 0")
	}
	if cfg.Grade1Factor <= 0 || cfg.Grade1Factor > 1 {
		return validationErrorf("risk_management.grade_1_factor must be in (0,1]")
	}
	if cfg.Grade2Factor <= 0 || cfg.Grade2Factor > 1 {
		return validationErrorf("risk_management.grade_2_factor must be in (0,1]")
	}
	if cfg.Grade3Factor <= 0 || cfg.Grade3Factor > 1 {
		return validationErrorf("risk_management.grade_3_factor must be in (0,1]")
	}
	if cfg.EntryOffsetATR < 0 {
		return validationErrorf("risk_management.entry_offset_atr must be >= 0")
	}
	if cfg.BreakevenFeePct < 0 {
		return validationErrorf("risk_management.breakeven_fee_pct must be >= 0")
	}
	if cfg.SlippageBufferPct < 0 {
		return validationErrorf("risk_management.slippage_buffer_pct must be >= 0")
	}
	return nil
}

func validateRiskManagementEntry(cfg RiskManagementConfig) error {
	entryMode := strings.ToLower(strings.TrimSpace(cfg.EntryMode))
	if entryMode != "" {
		switch entryMode {
		case "orderbook", "atr_offset", "market":
			// ok
		default:
			return validationErrorf("risk_management.entry_mode must be orderbook/atr_offset/market")
		}
	}
	if cfg.OrderbookDepth != 0 {
		allowed := map[int]struct{}{5: {}, 10: {}, 20: {}, 50: {}, 100: {}, 500: {}, 1000: {}}
		if cfg.OrderbookDepth <= 0 {
			return validationErrorf("risk_management.orderbook_depth must be > 0")
		}
		if _, ok := allowed[cfg.OrderbookDepth]; !ok {
			return validationErrorf("risk_management.orderbook_depth must be one of 5/10/20/50/100/500/1000")
		}
	}
	return nil
}

func validateRiskManagementTighten(cfg RiskManagementConfig) error {
	if cfg.TightenATR.StructureThreatened <= 0 {
		return validationErrorf("risk_management.tighten_atr.structure_threatened must be > 0")
	}
	if cfg.TightenATR.MinUpdateIntervalSec < 0 {
		return validationErrorf("risk_management.tighten_atr.min_update_interval_sec must be >= 0")
	}
	return nil
}

func validateRiskManagementInitialExit(cfg RiskManagementConfig) error {
	policy := strings.TrimSpace(cfg.InitialExit.Policy)
	if policy == "" {
		return validationErrorf("risk_management.initial_exit.policy is required")
	}
	structureInterval := strings.ToLower(strings.TrimSpace(cfg.InitialExit.StructureInterval))
	if structureInterval != "" && structureInterval != "auto" {
		if _, err := interval.ParseInterval(structureInterval); err != nil {
			return validationErrorf("risk_management.initial_exit.structure_interval must be auto or a valid interval")
		}
	}
	if err := initialExitPolicyValidator(policy, cfg.InitialExit.Params); err != nil {
		return validationErrorf("risk_management.initial_exit invalid: %v", err)
	}
	return nil
}

func validateSieveConfig(cfg RiskManagementSieveConfig) error {
	if cfg.MinSizeFactor < 0 || cfg.MinSizeFactor > 1 {
		return validationErrorf("risk_management.sieve.min_size_factor must be in [0,1]")
	}
	if cfg.DefaultSizeFactor < 0 || cfg.DefaultSizeFactor > 1 {
		return validationErrorf("risk_management.sieve.default_size_factor must be in [0,1]")
	}
	defaultAction := strings.ToUpper(strings.TrimSpace(cfg.DefaultGateAction))
	if defaultAction != "" && defaultAction != "ALLOW" && defaultAction != "WAIT" && defaultAction != "VETO" {
		return validationErrorf("risk_management.sieve.default_gate_action must be ALLOW/WAIT/VETO")
	}
	allowedMechanics := map[string]struct{}{
		"fuel_ready":          {},
		"neutral":             {},
		"crowded_long":        {},
		"crowded_short":       {},
		"liquidation_cascade": {},
	}
	allowedConf := map[string]struct{}{
		"high": {},
		"low":  {},
	}
	for idx, row := range cfg.Rows {
		mech := strings.ToLower(strings.TrimSpace(row.MechanicsTag))
		if mech == "" {
			return validationErrorf("risk_management.sieve.rows[%d].mechanics_tag is required", idx)
		}
		if _, ok := allowedMechanics[mech]; !ok {
			return validationErrorf("risk_management.sieve.rows[%d].mechanics_tag must be one of fuel_ready/neutral/crowded_long/crowded_short/liquidation_cascade", idx)
		}
		conf := strings.ToLower(strings.TrimSpace(row.LiqConfidence))
		if conf == "" {
			return validationErrorf("risk_management.sieve.rows[%d].liq_confidence is required", idx)
		}
		if _, ok := allowedConf[conf]; !ok {
			return validationErrorf("risk_management.sieve.rows[%d].liq_confidence must be high/low", idx)
		}
		action := strings.ToUpper(strings.TrimSpace(row.GateAction))
		if action == "" {
			return validationErrorf("risk_management.sieve.rows[%d].gate_action is required", idx)
		}
		if action != "ALLOW" && action != "WAIT" && action != "VETO" {
			return validationErrorf("risk_management.sieve.rows[%d].gate_action must be ALLOW/WAIT/VETO", idx)
		}
		if row.SizeFactor < 0 || row.SizeFactor > 1 {
			return validationErrorf("risk_management.sieve.rows[%d].size_factor must be in [0,1]", idx)
		}
	}
	return nil
}

func validateIndicatorConfig(cfg IndicatorConfig) error {
	if cfg.EMAFast <= 0 || cfg.EMAMid <= 0 || cfg.EMASlow <= 0 {
		return validationErrorf("indicators.ema_fast/ema_mid/ema_slow must be > 0")
	}
	if cfg.RSIPeriod <= 0 {
		return validationErrorf("indicators.rsi_period must be > 0")
	}
	if cfg.ATRPeriod <= 0 {
		return validationErrorf("indicators.atr_period must be > 0")
	}
	if cfg.MACDFast <= 0 || cfg.MACDSlow <= 0 || cfg.MACDSignal <= 0 {
		return validationErrorf("indicators.macd_fast/macd_slow/macd_signal must be > 0")
	}
	if cfg.LastN <= 0 {
		return validationErrorf("indicators.last_n must be > 0")
	}
	return nil
}

func validateCooldownConfig(cfg CooldownConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.EntryCooldownSec <= 0 {
		return validationErrorf("cooldown.entry_cooldown_sec must be > 0")
	}
	return nil
}

func validateConsensusConfig(cfg ConsensusConfig) error {
	if cfg.ScoreThreshold < 0 || cfg.ScoreThreshold > 1 {
		return validationErrorf("consensus.score_threshold must be in [0,1]")
	}
	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return validationErrorf("consensus.confidence_threshold must be in [0,1]")
	}
	return nil
}

func validateLLMConfig(cfg SymbolLLMConfig, enabled AgentEnabled) error {
	if err := validateLLMRoleEnabled("llm.agent.indicator", cfg.Agent.Indicator, enabled.Indicator); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.agent.structure", cfg.Agent.Structure, enabled.Structure); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.agent.mechanics", cfg.Agent.Mechanics, enabled.Mechanics); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.provider.indicator", cfg.Provider.Indicator, enabled.Indicator); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.provider.structure", cfg.Provider.Structure, enabled.Structure); err != nil {
		return err
	}
	if err := validateLLMRoleEnabled("llm.provider.mechanics", cfg.Provider.Mechanics, enabled.Mechanics); err != nil {
		return err
	}
	return nil
}

func validateLLMRoleEnabled(prefix string, cfg LLMRoleConfig, enabled bool) error {
	if !enabled {
		return nil
	}
	return validateLLMRole(prefix, cfg)
}

func validateLLMRole(prefix string, cfg LLMRoleConfig) error {
	if strings.TrimSpace(cfg.Model) == "" {
		return validationErrorf("%s.model is required", prefix)
	}
	if cfg.Temperature == nil {
		return validationErrorf("%s.temperature is required", prefix)
	}
	if *cfg.Temperature < 0 {
		return validationErrorf("%s.temperature must be >= 0", prefix)
	}
	return nil
}

func ValidateSymbolLLMModels(sys SystemConfig, cfg SymbolConfig) error {
	enabled, err := ResolveAgentEnabled(cfg.Agent)
	if err != nil {
		return err
	}
	roles := []struct {
		path  string
		model string
		need  bool
	}{
		{"llm.agent.indicator", cfg.LLM.Agent.Indicator.Model, enabled.Indicator},
		{"llm.agent.structure", cfg.LLM.Agent.Structure.Model, enabled.Structure},
		{"llm.agent.mechanics", cfg.LLM.Agent.Mechanics.Model, enabled.Mechanics},
		{"llm.provider.indicator", cfg.LLM.Provider.Indicator.Model, enabled.Indicator},
		{"llm.provider.structure", cfg.LLM.Provider.Structure.Model, enabled.Structure},
		{"llm.provider.mechanics", cfg.LLM.Provider.Mechanics.Model, enabled.Mechanics},
	}
	for _, role := range roles {
		if !role.need {
			continue
		}
		model := strings.TrimSpace(role.model)
		if model == "" {
			continue
		}
		if _, ok := sys.LLMModels[model]; !ok {
			return validationErrorf("%s.model=%s not found in system llm_models", role.path, model)
		}
	}
	return nil
}

func requiredKlineLimit(cfg SymbolConfig) int {
	trendRequired := TrendPresetRequiredBars(cfg.Intervals)
	required := maxInt(
		cfg.Indicators.EMAFast,
		cfg.Indicators.EMAMid,
		cfg.Indicators.EMASlow,
		cfg.Indicators.RSIPeriod,
		cfg.Indicators.ATRPeriod,
		cfg.Indicators.MACDFast,
		cfg.Indicators.MACDSlow,
		cfg.Indicators.MACDSignal,
		trendRequired,
	)
	required = max(1, required)
	return required + 1
}

func maxInt(values ...int) int {
	maxVal := 0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
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
