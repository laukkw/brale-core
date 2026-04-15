package runtime

import (
	"context"
	"strings"

	"brale-core/internal/execution"

	"go.uber.org/zap"
)

type RuntimeTaskExecutor interface {
	Execute(ctx context.Context, scheduler *RuntimeScheduler, task RuntimeTask)
}

type defaultRuntimeTaskExecutor struct{}

func (defaultRuntimeTaskExecutor) Execute(ctx context.Context, scheduler *RuntimeScheduler, task RuntimeTask) {
	if scheduler == nil {
		return
	}
	fields := []zap.Field{
		zap.String("symbol", task.Symbol),
		zap.String("task", string(task.Type)),
	}
	if task.Type == TaskWebhookEvent {
		if strings.TrimSpace(task.WebhookEventType) != "" {
			fields = append(fields, zap.String("webhook_event_type", strings.TrimSpace(task.WebhookEventType)))
		}
		if task.WebhookTradeID > 0 {
			fields = append(fields, zap.Int("webhook_trade_id", task.WebhookTradeID))
		}
		if task.WebhookTimestamp > 0 {
			fields = append(fields, zap.Int64("webhook_timestamp", task.WebhookTimestamp))
		}
		if strings.TrimSpace(task.WebhookExitReason) != "" {
			fields = append(fields, zap.String("webhook_exit_reason", strings.TrimSpace(task.WebhookExitReason)))
		}
	}
	logger := scheduler.Logger.With(fields...)
	rt, ok := scheduler.Symbols[task.Symbol]
	if !ok {
		logger.Error("symbol runtime missing")
		return
	}
	switch task.Type {
	case TaskBarDecide:
		executeBarDecide(ctx, scheduler, task.Symbol, rt)
	case TaskPriceTick:
		executeRiskMonitor(ctx, scheduler, task.Symbol, logger, true)
	case TaskReconcile:
		executeReconcile(ctx, scheduler, task.Symbol, logger, true)
	case TaskWebhookEvent:
		executeReconcile(ctx, scheduler, task.Symbol, logger, false)
		executeRiskMonitor(ctx, scheduler, task.Symbol, logger, false)
	}
}

func executeBarDecide(ctx context.Context, scheduler *RuntimeScheduler, symbol string, rt SymbolRuntime) {
	if scheduler == nil {
		return
	}
	if !scheduler.GetScheduledDecision() {
		return
	}
	mode := scheduler.getSymbolMode(symbol)
	if mode == SymbolModeOff || mode == SymbolModeObserve {
		return
	}
	logger := scheduler.Logger.With(zap.String("symbol", symbol), zap.String("task", string(TaskBarDecide)), zap.Strings("intervals", rt.Intervals))
	if rt.Pipeline == nil {
		logger.Error("pipeline missing")
		return
	}
	acct := execution.AccountState{}
	if scheduler.AccountFetcher != nil {
		var err error
		acct, err = scheduler.AccountFetcher(ctx, symbol)
		if err != nil {
			logger.Error("account state error", zap.Error(err))
			return
		}
		logger.Info("account state refreshed",
			zap.Float64("available", acct.Available),
			zap.String("currency", strings.TrimSpace(acct.Currency)),
		)
	}
	risk := execution.RiskParams{RiskPerTradePct: rt.RiskPerTradePct}
	if risk.RiskPerTradePct <= 0 {
		logger.Error("risk_per_trade_pct invalid")
		return
	}
	if _, err := rt.Pipeline.RunOnce(ctx, []string{symbol}, rt.Intervals, rt.KlineLimit, acct, risk); err != nil {
		logger.Error("bar decide failed", zap.Error(err))
	}
}

func executeReconcile(ctx context.Context, scheduler *RuntimeScheduler, symbol string, logger *zap.Logger, strict bool) {
	if scheduler == nil || scheduler.Reconciler == nil {
		return
	}
	if err := scheduler.Reconciler.RunOnce(ctx, symbol); err != nil && strict {
		logger.Error("reconcile failed", zap.Error(err))
	}
}

func executeRiskMonitor(ctx context.Context, scheduler *RuntimeScheduler, symbol string, logger *zap.Logger, strict bool) {
	if scheduler == nil || scheduler.RiskMonitor == nil {
		return
	}
	if err := scheduler.RiskMonitor.RunOnce(ctx, symbol); err != nil && strict {
		logger.Error("price tick failed", zap.Error(err))
	}
}

func (s *RuntimeScheduler) taskExecutor() RuntimeTaskExecutor {
	if s == nil || s.TaskExecutor == nil {
		return defaultRuntimeTaskExecutor{}
	}
	return s.TaskExecutor
}
