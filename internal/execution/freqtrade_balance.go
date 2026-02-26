package execution

import (
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"
)

func CheckFreqtradeBalance(ctx context.Context, logger *zap.Logger, executor *FreqtradeAdapter) {
	quote, err := executor.Client.Balance(ctx)
	if err != nil {
		logger.Warn("freqtrade balance check failed", zap.Error(err))
		return
	}
	stake := ResolveStakeCurrency(quote)
	equity, ok := ExtractUSDTBalance(quote)
	available, availOK := ExtractUSDTAvailable(quote)
	used, usedOK := ExtractUSDTUsed(quote)
	if !ok || equity <= 0 {
		logger.Warn("freqtrade balance parsed empty",
			zap.String("stake_currency", strings.TrimSpace(stake)),
		)
		return
	}
	if !availOK || available <= 0 {
		logger.Warn("freqtrade available balance parsed empty",
			zap.String("stake_currency", strings.TrimSpace(stake)),
		)
	}
	if !usedOK {
		logger.Warn("freqtrade used balance parsed empty",
			zap.String("stake_currency", strings.TrimSpace(stake)),
		)
	}
	logger.Info("freqtrade 连接正常",
		zap.Float64("available", available),
		zap.Float64("used", used),
	)
}

func ExtractUSDTBalance(data map[string]any) (float64, bool) {
	entry, _, ok := FindStakeCurrencyEntry(data)
	if !ok {
		return 0, false
	}
	return AsFloat(entry["balance"])
}

func ExtractUSDTAvailable(data map[string]any) (float64, bool) {
	entry, _, ok := FindStakeCurrencyEntry(data)
	if !ok {
		return 0, false
	}
	return AsFloat(entry["free"])
}

func ExtractUSDTUsed(data map[string]any) (float64, bool) {
	entry, _, ok := FindStakeCurrencyEntry(data)
	if !ok {
		return 0, false
	}
	return AsFloat(entry["used"])
}

func ResolveStakeCurrency(data map[string]any) string {
	if raw, ok := data["stake_currency"].(string); ok {
		return strings.ToUpper(strings.TrimSpace(raw))
	}
	if raw, ok := data["stake"].(string); ok {
		return strings.ToUpper(strings.TrimSpace(raw))
	}
	return ""
}

func FindStakeCurrencyEntry(data map[string]any) (map[string]any, string, bool) {
	if data == nil {
		return nil, "", false
	}
	stakeCurrency := ResolveStakeCurrency(data)
	if stakeCurrency == "" {
		return nil, "", false
	}
	raw, ok := data["currencies"].([]any)
	if !ok {
		return nil, stakeCurrency, false
	}
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		curRaw, _ := entry["currency"].(string)
		currency := strings.ToUpper(strings.TrimSpace(curRaw))
		if currency == stakeCurrency {
			return entry, stakeCurrency, true
		}
	}
	return nil, stakeCurrency, false
}

func AsFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}
