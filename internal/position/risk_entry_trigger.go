package position

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/risk"

	"go.uber.org/zap"
)

func (m *RiskMonitor) handlePlanEntry(ctx context.Context, symbol string) error {
	entry, planSymbol, logger, ok := m.loadPlanEntry(ctx, symbol)
	if !ok {
		return nil
	}
	expired, err := m.expirePlanEntry(ctx, planSymbol, logger)
	if err != nil {
		return &riskMonitorOpError{Op: "expire plan entry", Symbol: planSymbol, Err: err}
	}
	if expired {
		return nil
	}
	if m.entryAlreadySubmitted(entry, logger) {
		return nil
	}
	quote, triggered, err := m.fetchTriggeredEntryQuote(ctx, entry.Plan, planSymbol, logger)
	if err != nil {
		return &riskMonitorOpError{Op: "fetch triggered entry quote", Symbol: planSymbol, Err: err}
	}
	if !triggered {
		return nil
	}
	acct, err := m.fetchAccountState(ctx, planSymbol, logger)
	if err != nil {
		return &riskMonitorOpError{Op: "fetch account state", Symbol: planSymbol, Err: err}
	}
	plan, err := m.refreshPlanSizing(entry.Plan, acct, logger)
	if err != nil {
		return &riskMonitorOpError{Op: "refresh plan sizing", Symbol: planSymbol, Err: err}
	}
	if updated, ok := m.PlanCache.UpdatePlan(planSymbol, plan); ok {
		plan = updated.Plan
	}
	logger.Info("entry size refreshed",
		zap.Float64("available", acct.Available),
		zap.String("currency", strings.TrimSpace(acct.Currency)),
		zap.Float64("risk_pct", plan.RiskPct),
		zap.Float64("risk_distance", plan.RiskAnnotations.RiskDistance),
		zap.Float64("position_size", plan.PositionSize),
	)
	resp, err := m.Positions.SubmitOpenFromPlan(ctx, plan, quote.Price)
	if err != nil {
		return &riskMonitorOpError{Op: "submit open from plan", Symbol: planSymbol, Err: err}
	}
	logger.Info("entry trigger hit",
		zap.Float64("mark_price", quote.Price),
		zap.Float64("entry_price", plan.Entry),
		zap.String("external_order_id", strings.TrimSpace(resp.ExternalID)),
	)
	return nil
}

func (m *RiskMonitor) loadPlanEntry(ctx context.Context, symbol string) (*PlanEntry, string, *zap.Logger, bool) {
	if strings.TrimSpace(symbol) == "" {
		return nil, "", nil, false
	}
	entry, ok := m.PlanCache.GetEntry(symbol)
	if !ok || entry == nil {
		return nil, "", nil, false
	}
	planSymbol := entry.Plan.Symbol
	if strings.TrimSpace(planSymbol) == "" {
		planSymbol = symbol
	}
	logger := logging.FromContext(ctx).Named("risk").With(
		zap.String("symbol", planSymbol),
		zap.String("position_id", entry.Plan.PositionID),
	)
	return entry, planSymbol, logger, true
}

func (m *RiskMonitor) expirePlanEntry(ctx context.Context, planSymbol string, logger *zap.Logger) (bool, error) {
	expired, expiredEntry := m.PlanCache.ExpireIfNeeded(planSymbol, time.Now().UTC())
	if !expired {
		return false, nil
	}
	if expiredEntry == nil {
		return true, nil
	}
	logger.Info("plan expired",
		zap.String("position_id", expiredEntry.Plan.PositionID),
		zap.String("symbol", planSymbol),
		zap.Float64("entry", expiredEntry.Plan.Entry),
		zap.Time("expires_at", expiredEntry.Plan.ExpiresAt),
	)
	reason := "plan_expired"
	if _, err := m.Positions.CancelOpenByEntry(ctx, *expiredEntry, reason); err != nil {
		logger.Error("open cancel failed", zap.Error(err), zap.String("reason", reason))
	} else {
		logger.Info("open cancel submitted", zap.String("reason", reason))
	}
	return true, nil
}

func (m *RiskMonitor) entryAlreadySubmitted(entry *PlanEntry, logger *zap.Logger) bool {
	if strings.TrimSpace(entry.ExternalID) == "" && strings.TrimSpace(entry.ClientOrderID) == "" {
		return false
	}
	logger.Debug("open already submitted",
		zap.String("client_order_id", strings.TrimSpace(entry.ClientOrderID)),
		zap.String("external_order_id", strings.TrimSpace(entry.ExternalID)),
		zap.Int64("submitted_at", entry.SubmittedAt),
	)
	return true
}

func (m *RiskMonitor) fetchTriggeredEntryQuote(ctx context.Context, plan execution.ExecutionPlan, planSymbol string, logger *zap.Logger) (market.PriceQuote, bool, error) {
	priceCtx, cancel := riskMonitorChildTimeout(ctx, riskMonitorPriceFetchTimeout)
	quote, err := m.PriceSource.MarkPrice(priceCtx, planSymbol)
	cancel()
	if err != nil {
		if errors.Is(err, market.ErrPriceUnavailable) {
			return market.PriceQuote{}, false, nil
		}
		return market.PriceQuote{}, false, fmt.Errorf("mark price for entry trigger %s: %w", planSymbol, err)
	}
	if quote.Price == 0 {
		return market.PriceQuote{}, false, nil
	}
	if plan.Entry <= 0 {
		return market.PriceQuote{}, false, riskValidationErrorf("entry is required")
	}
	triggered := isEntryTriggered(plan.Direction, quote.Price, plan.Entry)
	if triggered {
		logger.Info("entry trigger check",
			zap.Float64("mark_price", quote.Price),
			zap.Float64("entry_price", plan.Entry),
			zap.String("side", plan.Direction),
		)
		return quote, true, nil
	}
	logger.Debug("entry trigger check",
		zap.Float64("mark_price", quote.Price),
		zap.Float64("entry_price", plan.Entry),
		zap.String("side", plan.Direction),
	)
	return market.PriceQuote{}, false, nil
}

func (m *RiskMonitor) refreshPlanSizing(plan execution.ExecutionPlan, acct execution.AccountState, logger *zap.Logger) (execution.ExecutionPlan, error) {
	riskDist := plan.RiskAnnotations.RiskDistance
	if riskDist <= 0 {
		return plan, riskValidationErrorf("risk_distance is required")
	}
	if plan.RiskPct <= 0 {
		return plan, riskValidationErrorf("risk_pct is required")
	}
	positionSize := (acct.Available * plan.RiskPct) / riskDist
	if positionSize <= 0 {
		return plan, riskValidationErrorf("position_size invalid")
	}
	maxInvestPct := plan.RiskAnnotations.MaxInvestPct
	if maxInvestPct <= 0 {
		maxInvestPct = 1
	}
	maxInvestAmt := acct.Available * maxInvestPct
	if maxInvestAmt <= 0 {
		maxInvestAmt = acct.Available
	}
	maxLeverage := plan.RiskAnnotations.MaxLeverage
	if maxLeverage <= 0 {
		maxLeverage = plan.Leverage
	}
	leverageResult := risk.ResolveLeverageAndLiquidation(plan.Entry, positionSize, maxInvestAmt, maxLeverage, plan.Direction)
	if risk.IsStopBeyondLiquidation(plan.Direction, plan.StopLoss, leverageResult.LiquidationPrice) {
		logger.Warn("stop loss beyond liquidation",
			zap.Float64("stop_loss", plan.StopLoss),
			zap.Float64("liquidation_price", leverageResult.LiquidationPrice),
			zap.Float64("leverage", leverageResult.Leverage),
		)
		return plan, riskValidationErrorf("stop loss beyond liquidation")
	}
	plan.PositionSize = leverageResult.PositionSize
	plan.Leverage = leverageResult.Leverage
	return plan, nil
}

func isEntryTriggered(side string, mark float64, entry float64) bool {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "short":
		return mark >= entry
	default:
		return mark <= entry
	}
}
