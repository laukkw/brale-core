package config

import (
	"strings"

	"brale-core/internal/pkg/symbol"
)

func NormalizeSystemConfig(cfg *SystemConfig) {
	if cfg == nil {
		return
	}
	cfg.PersistMode = normalizePersistMode(cfg.PersistMode)
	cfg.Notification.Feishu.BotMode = NormalizeFeishuBotMode(cfg.Notification.Feishu.BotMode)
}

func NormalizeSymbolIndexConfig(cfg *SymbolIndexConfig) {
	if cfg == nil {
		return
	}
	for i := range cfg.Symbols {
		cfg.Symbols[i].Symbol = NormalizeSymbol(cfg.Symbols[i].Symbol)
	}
}

func NormalizeSymbolConfig(cfg *SymbolConfig) {
	if cfg == nil {
		return
	}
	cfg.Symbol = NormalizeSymbol(cfg.Symbol)
	cfg.Indicators.Engine = NormalizeIndicatorEngine(cfg.Indicators.Engine)
	cfg.Indicators.ShadowEngine = NormalizeOptionalIndicatorEngine(cfg.Indicators.ShadowEngine)
}

func NormalizeStrategyConfig(cfg *StrategyConfig) {
	if cfg == nil {
		return
	}
	cfg.Symbol = NormalizeSymbol(cfg.Symbol)
}

func NormalizeSymbol(raw string) string {
	return symbol.Normalize(raw)
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

func NormalizeFeishuBotMode(value string) string {
	mode := strings.TrimSpace(strings.ToLower(value))
	if mode == "" {
		return "long_connection"
	}
	return mode
}
