package runtime

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (s *RuntimeScheduler) startLoops(ctx context.Context) {
	s.startBarLoops(ctx)
	s.startPeriodicLoops(ctx)
}

func (s *RuntimeScheduler) startBarLoops(ctx context.Context) {
	for symbol, rt := range s.Symbols {
		if rt.BarInterval <= 0 {
			continue
		}
		go s.barLoop(ctx, symbol, rt.BarInterval)
	}
}

func (s *RuntimeScheduler) startPeriodicLoops(ctx context.Context) {
	if s.PriceTickInterval > 0 {
		go s.priceTickLoop(ctx)
	}
	if s.ReconcileInterval > 0 {
		go s.reconcileLoop(ctx)
	}
}

func (s *RuntimeScheduler) barLoop(ctx context.Context, symbol string, interval time.Duration) {
	policy := s.policy()
	for {
		next := nextBarClose(time.Now(), interval)
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		scheduled := s.GetScheduledDecision()
		mode := s.getSymbolMode(symbol)
		if !policy.ShouldEnqueueBar(scheduled, mode) {
			continue
		}
		s.enqueueTaskWithWarning(RuntimeTask{Type: TaskBarDecide, Symbol: symbol, EnqueuedAt: time.Now()}, "enqueue bar task failed")
	}
}

func (s *RuntimeScheduler) priceTickLoop(ctx context.Context) {
	if s.RiskMonitor == nil {
		return
	}
	s.runPeriodicTaskLoop(ctx, s.PriceTickInterval, TaskPriceTick, "enqueue price tick failed")
}

func (s *RuntimeScheduler) reconcileLoop(ctx context.Context) {
	if s.Reconciler == nil {
		return
	}
	s.runPeriodicTaskLoop(ctx, s.ReconcileInterval, TaskReconcile, "enqueue reconcile task failed")
}

func (s *RuntimeScheduler) runPeriodicTaskLoop(ctx context.Context, interval time.Duration, taskType TaskType, warnMessage string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	policy := s.policy()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scheduled := s.GetScheduledDecision()
			for symbol := range s.Symbols {
				mode := s.getSymbolMode(symbol)
				monitored := s.isSymbolMonitored(symbol)
				if !policy.ShouldEnqueuePeriodic(scheduled, mode, monitored) {
					continue
				}
				s.enqueueTaskWithWarning(RuntimeTask{Type: taskType, Symbol: symbol, EnqueuedAt: time.Now()}, warnMessage)
			}
		}
	}
}

func (s *RuntimeScheduler) enqueueTaskWithWarning(task RuntimeTask, warnMessage string) {
	if err := s.Enqueue(task); err != nil && s.Logger != nil {
		s.Logger.Warn(warnMessage, zap.String("symbol", task.Symbol), zap.Error(err))
	}
}

func nextBarClose(now time.Time, interval time.Duration) time.Time {
	next := now.Truncate(interval).Add(interval)
	return next.Add(10 * time.Second)
}
