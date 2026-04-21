package memory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type ReconcileReflectorAdapter struct {
	TradeFinder   execution.TradeFinder
	Reflector     *Reflector
	RetryAttempts int
	RetryDelay    time.Duration
}

func (a *ReconcileReflectorAdapter) ReflectOnClose(ctx context.Context, pos store.PositionRecord) {
	if a.Reflector == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	baseCtx := context.WithoutCancel(ctx)
	logger := logging.FromContext(baseCtx).Named("reflector")
	if a.TradeFinder == nil {
		logger.Warn("position reflection skipped: trade finder unavailable", zap.String("position_id", pos.PositionID))
		return
	}

	tradeID, err := strconv.Atoi(strings.TrimSpace(pos.ExecutorPositionID))
	if err != nil || tradeID <= 0 {
		logger.Warn("position reflection skipped: invalid executor position id",
			zap.String("position_id", pos.PositionID),
			zap.String("executor_position_id", pos.ExecutorPositionID),
		)
		return
	}

	runCtx, cancel := context.WithTimeout(
		logging.WithLogger(baseCtx, logger),
		reflectionLookupTimeout(a.retryAttempts(), a.retryDelay()),
	)
	defer cancel()

	trade, ok := a.lookupClosedTrade(runCtx, tradeID, logger)
	if !ok {
		logger.Warn("position reflection skipped: verified closed trade unavailable",
			zap.String("position_id", pos.PositionID),
			zap.Int("trade_id", tradeID),
		)
		return
	}

	input, ok := reflectionInputFromTrade(pos, trade)
	if !ok {
		logger.Warn("position reflection skipped: incomplete verified trade data",
			zap.String("position_id", pos.PositionID),
			zap.Int("trade_id", tradeID),
		)
		return
	}
	input.Context = buildReflectionContext(runCtx, a.Reflector.Store, pos, trade, input)

	if err := a.Reflector.Reflect(runCtx, input); err != nil {
		logger.Error("position reflection failed",
			zap.Error(err),
			zap.String("position_id", pos.PositionID),
			zap.Int("trade_id", tradeID),
		)
	}
}

func (a *ReconcileReflectorAdapter) lookupClosedTrade(ctx context.Context, tradeID int, logger *zap.Logger) (execution.Trade, bool) {
	attempts := a.retryAttempts()
	delay := a.retryDelay()
	for attempt := 1; attempt <= attempts; attempt++ {
		trade, ok, err := a.TradeFinder.FindTradeByID(ctx, tradeID)
		switch {
		case err != nil:
			logger.Warn("verified close lookup failed",
				zap.Error(err),
				zap.Int("trade_id", tradeID),
				zap.Int("attempt", attempt),
			)
		case ok && !trade.IsOpen:
			return trade, true
		}
		if attempt == attempts {
			break
		}
		if !sleepWithContext(ctx, delay) {
			break
		}
	}
	return execution.Trade{}, false
}

func reflectionInputFromTrade(pos store.PositionRecord, trade execution.Trade) (ReflectionInput, bool) {
	entryPrice := float64(trade.OpenRate)
	if entryPrice <= 0 {
		entryPrice = pos.AvgEntry
	}
	exitPrice := resolvedTradeCloseRate(trade)
	if entryPrice <= 0 || exitPrice <= 0 {
		return ReflectionInput{}, false
	}

	direction := strings.TrimSpace(pos.Side)
	if direction == "" {
		direction = "long"
		if trade.IsShort || strings.EqualFold(strings.TrimSpace(trade.Side), "short") {
			direction = "short"
		}
	}

	context := reflectionPromptContext{
		TradeResult: reflectionTradeResult{
			Symbol:     strings.TrimSpace(pos.Symbol),
			Direction:  direction,
			EntryPrice: fmt.Sprintf("%.8f", entryPrice),
			ExitPrice:  fmt.Sprintf("%.8f", exitPrice),
			PnLPercent: fmt.Sprintf("%.2f", resolvedTradePnLPercent(trade)),
			Duration:   resolvedTradeDuration(trade),
		},
	}
	if enterTag := strings.TrimSpace(string(trade.EnterTag)); enterTag != "" {
		context.EntryContext = &reflectionEntryContext{EnterTag: enterTag}
	}
	return ReflectionInput{
		Symbol:     strings.TrimSpace(pos.Symbol),
		PositionID: pos.PositionID,
		Direction:  direction,
		EntryPrice: fmt.Sprintf("%.8f", entryPrice),
		ExitPrice:  fmt.Sprintf("%.8f", exitPrice),
		PnLPercent: fmt.Sprintf("%.2f", resolvedTradePnLPercent(trade)),
		Duration:   resolvedTradeDuration(trade),
		Context:    context,
	}, true
}

func resolvedTradeCloseRate(trade execution.Trade) float64 {
	if trade.CloseRate > 0 {
		return float64(trade.CloseRate)
	}
	order, ok := latestExitOrder(trade)
	if !ok {
		return 0
	}
	switch {
	case order.Average > 0:
		return float64(order.Average)
	case order.SafePrice > 0:
		return float64(order.SafePrice)
	case order.Price > 0:
		return float64(order.Price)
	default:
		return 0
	}
}

func resolvedTradePnLPercent(trade execution.Trade) float64 {
	if trade.CloseProfitPct != 0 {
		return float64(trade.CloseProfitPct)
	}
	return float64(trade.ProfitPct)
}

func resolvedTradeDuration(trade execution.Trade) string {
	seconds := int64(trade.TradeDurationSeconds)
	if seconds <= 0 && trade.TradeDuration > 0 {
		seconds = int64(trade.TradeDuration) * 60
	}
	if seconds <= 0 {
		return "unknown"
	}
	return (time.Duration(seconds) * time.Second).String()
}

func latestExitOrder(trade execution.Trade) (execution.TradeOrder, bool) {
	var chosen execution.TradeOrder
	var found bool
	var latest int64
	for _, order := range trade.Orders {
		if order.FTIsEntry {
			continue
		}
		filledAt := int64(order.OrderFilledAt)
		if filledAt <= 0 {
			filledAt = int64(order.OrderTimestamp)
		}
		if !found || filledAt > latest {
			chosen = order
			latest = filledAt
			found = true
		}
	}
	return chosen, found
}

func reflectionLookupTimeout(attempts int, delay time.Duration) time.Duration {
	if attempts <= 0 {
		attempts = 1
	}
	if delay < 0 {
		delay = 0
	}
	return time.Duration(attempts+1)*delay + 10*time.Second
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (a *ReconcileReflectorAdapter) retryAttempts() int {
	if a != nil && a.RetryAttempts > 0 {
		return a.RetryAttempts
	}
	return 6
}

func (a *ReconcileReflectorAdapter) retryDelay() time.Duration {
	if a != nil && a.RetryDelay > 0 {
		return a.RetryDelay
	}
	return 5 * time.Second
}
