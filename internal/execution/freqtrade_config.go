package execution

import (
	"context"
	"strconv"

	"go.uber.org/zap"
)

func (c *FreqtradeClient) ShowConfig(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := c.doGet(ctx, "/show_config", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func CheckFreqtradeStoplossSafetyNet(ctx context.Context, logger *zap.Logger, executor *FreqtradeAdapter) {
	if logger == nil || executor == nil || executor.Client == nil {
		return
	}
	cfg, err := executor.Client.ShowConfig(ctx)
	if err != nil {
		logger.Warn("freqtrade stoploss safety-net check failed", zap.Error(err))
		return
	}
	orderTypes, _ := cfg["order_types"].(map[string]any)
	if !boolValue(orderTypes["stoploss_on_exchange"]) {
		logger.Warn("freqtrade stoploss_on_exchange disabled")
		return
	}
	logger.Info("freqtrade stoploss_on_exchange enabled")
}

func boolValue(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(v)
		return err == nil && parsed
	default:
		return false
	}
}
