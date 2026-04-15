package position

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (s *PositionService) OpenFromPlan(ctx context.Context, plan execution.ExecutionPlan) (store.PositionRecord, error) {
	if s.Store == nil || s.Executor == nil {
		return store.PositionRecord{}, fmt.Errorf("store/executor is required")
	}
	if strings.TrimSpace(plan.PositionID) == "" {
		return store.PositionRecord{}, fmt.Errorf("position_id is required")
	}
	_, exists, err := s.Store.FindPositionBySymbol(ctx, plan.Symbol, OpenPositionStatuses)
	if err != nil {
		return store.PositionRecord{}, err
	}
	if exists {
		return store.PositionRecord{}, ErrPositionActive
	}
	if s.PlanCache != nil {
		if entry, ok := s.PlanCache.GetEntry(plan.Symbol); ok && entry != nil {
			if entry.ExternalID != "" || entry.ClientOrderID != "" {
				return store.PositionRecord{}, ErrPositionActive
			}
		}
	} else {
		return store.PositionRecord{}, fmt.Errorf("plan_cache is required")
	}
	rec := store.PositionRecord{
		PositionID:   plan.PositionID,
		Symbol:       plan.Symbol,
		Side:         plan.Direction,
		Status:       PositionOpenActive,
		ExecutorName: s.Executor.Name(),
		Version:      1,
		Source:       "entry_fill",
		StopReason:   strings.TrimSpace(plan.RiskAnnotations.StopReason),
		Qty:          plan.PositionSize,
		AvgEntry:     plan.Entry,
		RiskPct:      plan.RiskPct,
		Leverage:     plan.Leverage,
	}
	riskPlan := risk.BuildRiskPlan(risk.RiskPlanInput{
		Entry:            plan.Entry,
		StopLoss:         plan.StopLoss,
		PositionSize:     plan.PositionSize,
		TakeProfits:      plan.TakeProfits,
		TakeProfitRatios: plan.TakeProfitRatios,
	})
	raw, err := json.Marshal(risk.CompactRiskPlan(riskPlan))
	if err != nil {
		return store.PositionRecord{}, err
	}
	rec.RiskJSON = raw
	if err := s.Store.SavePosition(ctx, &rec); err != nil {
		if isUniqueConstraint(err) {
			return store.PositionRecord{}, ErrPositionActive
		}
		return store.PositionRecord{}, err
	}
	if s.RiskPlans != nil {
		if _, err := s.RiskPlans.SaveHistory(ctx, rec.PositionID, riskPlan, "entry_fill"); err != nil {
			return store.PositionRecord{}, err
		}
	}
	logging.FromContext(ctx).Named("execution").Info("open filled",
		zap.String("position_id", rec.PositionID),
		zap.String("symbol", plan.Symbol),
		zap.String("side", plan.Direction),
		zap.Float64("entry", plan.Entry),
		zap.Float64("risk_pct", plan.RiskPct),
	)
	return rec, nil
}

func (s *PositionService) SubmitOpenFromPlan(ctx context.Context, plan execution.ExecutionPlan, triggerPrice float64) (execution.PlaceOrderResp, error) {
	if s.Store == nil || s.Executor == nil {
		return execution.PlaceOrderResp{}, fmt.Errorf("store/executor is required")
	}
	if plan.StopLoss <= 0 {
		return execution.PlaceOrderResp{}, fmt.Errorf("stop_loss is required")
	}
	if s.PlanCache != nil {
		if entry, ok := s.PlanCache.GetEntry(plan.Symbol); ok && entry != nil {
			if entry.ExternalID != "" || entry.ClientOrderID != "" {
				return execution.PlaceOrderResp{}, ErrPositionActive
			}
		}
	} else {
		return execution.PlaceOrderResp{}, fmt.Errorf("plan_cache is required")
	}
	if s.Cache != nil {
		s.Cache.UpdatePrice(plan.PositionID, plan.Symbol, triggerPrice, time.Now().UnixMilli())
	}
	clientOrderID := uuid.NewString()
	if s.PlanCache != nil {
		s.PlanCache.UpdateOrder(plan.Symbol, "", clientOrderID, time.Now().UnixMilli())
	}
	orderType := "limit"
	price := plan.Entry
	if strings.TrimSpace(plan.Template) == "debug_inject" {
		orderType = "market"
		if price <= 0 {
			price = triggerPrice
		}
	}
	logging.FromContext(ctx).Named("execution").Info("open intent created",
		zap.String("position_id", plan.PositionID),
		zap.String("symbol", plan.Symbol),
		zap.String("side", plan.Direction),
		zap.Float64("qty", plan.PositionSize),
		zap.String("order_type", orderType),
		zap.Float64("price", price),
		zap.String("client_order_id", clientOrderID),
	)
	resp, err := s.Executor.PlaceOrder(ctx, execution.PlaceOrderReq{
		Kind:          execution.OrderOpen,
		Symbol:        plan.Symbol,
		Side:          plan.Direction,
		Quantity:      plan.PositionSize,
		Price:         price,
		PositionID:    plan.PositionID,
		ClientOrderID: clientOrderID,
		Tag:           clientOrderID,
		OrderType:     orderType,
		Leverage:      plan.Leverage,
	})
	if err != nil {
		logger := logging.FromContext(ctx).Named("execution")
		logger.Error("order submit failed",
			zap.String("position_id", plan.PositionID),
			zap.String("symbol", plan.Symbol),
			zap.String("client_order_id", clientOrderID),
			zap.Error(err),
		)
		if s.PlanCache != nil && shouldResetOpenPlanOrder(err) {
			if _, resetOK := s.PlanCache.ResetOrder(plan.Symbol); !resetOK {
				logger.Warn("reset open plan cache failed",
					zap.String("symbol", plan.Symbol),
					zap.String("reason", "entry_not_found"),
				)
			}
		}
		if s.Notifier != nil {
			if notifyErr := s.Notifier.SendError(ctx, ErrorNotice{Component: "execution", Symbol: strings.TrimSpace(plan.Symbol), Message: err.Error()}); notifyErr != nil {
				logger.Error("notify error failed", zap.Error(notifyErr))
			}
		}
		return execution.PlaceOrderResp{}, err
	}
	logging.FromContext(ctx).Named("execution").Info("open order submitted",
		zap.String("position_id", plan.PositionID),
		zap.String("symbol", plan.Symbol),
		zap.String("client_order_id", clientOrderID),
		zap.String("external_order_id", strings.TrimSpace(resp.ExternalID)),
	)
	if s.PlanCache != nil {
		s.PlanCache.UpdateOrder(plan.Symbol, strings.TrimSpace(resp.ExternalID), clientOrderID, time.Now().UnixMilli())
	}
	return resp, nil
}

func (s *PositionService) CancelOpenBySymbol(ctx context.Context, symbol string, reason string) (bool, error) {
	if s.Store == nil || s.Executor == nil {
		return false, fmt.Errorf("store/executor is required")
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return false, fmt.Errorf("symbol is required")
	}
	if s.PlanCache == nil {
		return false, nil
	}
	entry, ok := s.PlanCache.GetEntry(symbol)
	if !ok || entry == nil {
		return false, nil
	}
	return s.CancelOpenByEntry(ctx, *entry, reason)
}

func (s *PositionService) CancelOpenByEntry(ctx context.Context, entry PlanEntry, reason string) (bool, error) {
	if s.Store == nil || s.Executor == nil {
		return false, fmt.Errorf("store/executor is required")
	}
	symbol := strings.TrimSpace(entry.Plan.Symbol)
	if symbol == "" {
		return false, fmt.Errorf("symbol is required")
	}
	if _, exists, err := s.Store.FindPositionBySymbol(ctx, symbol, OpenPositionStatuses); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	if strings.TrimSpace(entry.ExternalID) == "" {
		logging.FromContext(ctx).Named("execution").Info("open cancel skipped",
			zap.String("symbol", symbol),
			zap.String("reason", reason),
			zap.String("status", "plan_not_submitted"),
		)
		return false, nil
	}
	pos := store.PositionRecord{
		PositionID:         entry.Plan.PositionID,
		Symbol:             entry.Plan.Symbol,
		Side:               entry.Plan.Direction,
		ExecutorPositionID: entry.ExternalID,
	}
	if filled, filledQty, err := s.hasPartialEntryFill(ctx, pos); err != nil {
		return false, err
	} else if filled {
		logging.FromContext(ctx).Named("execution").Info("open cancel skipped",
			zap.String("position_id", entry.Plan.PositionID),
			zap.String("symbol", entry.Plan.Symbol),
			zap.String("reason", "partial_fill"),
			zap.Float64("filled_qty", filledQty),
		)
		return false, nil
	}
	_, err := s.Executor.CancelOrder(ctx, execution.CancelOrderReq{ExternalID: entry.ExternalID})
	if err != nil {
		return false, err
	}
	if s.PlanCache != nil {
		s.PlanCache.ClearIfMatch(symbol, entry.ExternalID, entry.ClientOrderID)
	}
	logging.FromContext(ctx).Named("execution").Info("open cancel submitted",
		zap.String("position_id", entry.Plan.PositionID),
		zap.String("symbol", entry.Plan.Symbol),
		zap.String("reason", reason),
		zap.String("external_order_id", entry.ExternalID),
	)
	return true, nil
}

func (s *PositionService) hasPartialEntryFill(ctx context.Context, pos store.PositionRecord) (bool, float64, error) {
	if s.Executor == nil {
		return false, 0, fmt.Errorf("executor is required")
	}
	if strings.TrimSpace(pos.ExecutorPositionID) == "" {
		return false, 0, nil
	}
	ord, err := s.Executor.GetOrder(ctx, pos.ExecutorPositionID)
	if err != nil {
		return false, 0, fmt.Errorf("query order %s failed: %w", pos.ExecutorPositionID, err)
	}
	if ord.FilledQty > 0 {
		return true, ord.FilledQty, nil
	}
	return false, 0, nil
}
