package runtime

import (
	"context"
	"fmt"

	"brale-core/internal/execution"
)

func RunObserveOnce(ctx context.Context, scheduler *RuntimeScheduler, symbol string) error {
	rt, acct, risk, mode, err := prepareScheduledPipelineRun(ctx, scheduler, symbol)
	if err != nil {
		return err
	}
	if !scheduler.GetScheduledDecision() || mode != SymbolModeObserve {
		return nil
	}
	_, err = rt.Pipeline.RunOnceObserveAsFlat(ctx, []string{symbol}, rt.Intervals, rt.KlineLimit, acct, risk)
	return err
}

func RunDecideOnce(ctx context.Context, scheduler *RuntimeScheduler, symbol string) error {
	rt, acct, risk, mode, err := prepareScheduledPipelineRun(ctx, scheduler, symbol)
	if err != nil {
		return err
	}
	if !scheduler.GetScheduledDecision() || mode != SymbolModeTrade {
		return nil
	}
	_, err = rt.Pipeline.RunOnce(ctx, []string{symbol}, rt.Intervals, rt.KlineLimit, acct, risk)
	return err
}

func RunReconcileOnce(ctx context.Context, scheduler *RuntimeScheduler, symbol string) error {
	if scheduler == nil || scheduler.Reconciler == nil {
		return nil
	}
	mode := scheduler.getSymbolMode(symbol)
	monitored := scheduler.isSymbolMonitored(symbol)
	if !scheduler.policy().ShouldEnqueuePeriodic(scheduler.GetScheduledDecision(), mode, monitored) {
		return nil
	}
	return scheduler.Reconciler.RunOnce(ctx, symbol)
}

func RunRiskMonitorOnce(ctx context.Context, scheduler *RuntimeScheduler, symbol string) error {
	if scheduler == nil || scheduler.RiskMonitor == nil {
		return nil
	}
	mode := scheduler.getSymbolMode(symbol)
	monitored := scheduler.isSymbolMonitored(symbol)
	if !scheduler.policy().ShouldEnqueuePeriodic(scheduler.GetScheduledDecision(), mode, monitored) {
		return nil
	}
	return scheduler.RiskMonitor.RunOnce(ctx, symbol)
}

func prepareScheduledPipelineRun(ctx context.Context, scheduler *RuntimeScheduler, symbol string) (SymbolRuntime, execution.AccountState, execution.RiskParams, SymbolMode, error) {
	if scheduler == nil {
		return SymbolRuntime{}, execution.AccountState{}, execution.RiskParams{}, "", fmt.Errorf("scheduler is required")
	}
	rt, ok := scheduler.Symbols[symbol]
	if !ok {
		return SymbolRuntime{}, execution.AccountState{}, execution.RiskParams{}, "", fmt.Errorf("symbol runtime missing: %s", symbol)
	}
	if rt.Pipeline == nil {
		return SymbolRuntime{}, execution.AccountState{}, execution.RiskParams{}, "", fmt.Errorf("pipeline missing for symbol: %s", symbol)
	}
	risk := execution.RiskParams{RiskPerTradePct: rt.RiskPerTradePct}
	if risk.RiskPerTradePct <= 0 {
		return SymbolRuntime{}, execution.AccountState{}, execution.RiskParams{}, "", fmt.Errorf("risk_per_trade_pct invalid for symbol: %s", symbol)
	}
	acct := execution.AccountState{}
	if scheduler.AccountFetcher != nil {
		var err error
		acct, err = scheduler.AccountFetcher(ctx, symbol)
		if err != nil {
			return SymbolRuntime{}, execution.AccountState{}, execution.RiskParams{}, "", err
		}
	}
	return rt, acct, risk, scheduler.getSymbolMode(symbol), nil
}
