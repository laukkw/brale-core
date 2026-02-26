// 本文件主要内容：封装持仓与下单意图的创建与状态更新。
package position

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/errclass"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type PositionService struct {
	Store     store.Store
	Executor  execution.Executor
	Notifier  Notifier
	Cache     *PositionCache
	PlanCache *PlanCache
	RiskPlans *RiskPlanService
}

var ErrPositionActive = errors.New("position already active")
var ErrPositionNotArmed = errors.New("position not armed")

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
			if notifyErr := s.Notifier.SendError(ctx, err.Error()); notifyErr != nil {
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

func (s *PositionService) ArmClose(ctx context.Context, pos store.PositionRecord, reason string, triggerPrice float64, closeQty float64) (string, error) {
	if err := s.ensureCloseDeps(); err != nil {
		return "", err
	}
	if existingIntentID, handled, err := s.guardArmClose(ctx, pos); handled || err != nil {
		return existingIntentID, err
	}
	intentID := uuid.NewString()
	pos, intentKind, clientOrderID, err := s.prepareCloseOrder(ctx, pos, intentID, reason, triggerPrice, closeQty)
	if err != nil {
		return "", err
	}
	if err := s.submitCloseFlow(ctx, pos, intentKind, closeQty, clientOrderID); err != nil {
		return intentID, err
	}
	s.logAndNotifyClose(ctx, pos, reason, triggerPrice, closeQty)
	return intentID, nil
}

func (s *PositionService) ensureCloseDeps() error {
	if s.Store == nil || s.Executor == nil {
		return fmt.Errorf("store/executor is required")
	}
	return nil
}

func (s *PositionService) guardArmClose(ctx context.Context, pos store.PositionRecord) (string, bool, error) {
	if isCloseInFlightStatus(pos.Status) {
		logging.FromContext(ctx).Named("execution").Debug("close intent already in flight",
			zap.String("position_id", pos.PositionID),
			zap.String("symbol", pos.Symbol),
			zap.String("status", pos.Status),
			zap.String("close_intent_id", strings.TrimSpace(pos.CloseIntentID)),
		)
		return strings.TrimSpace(pos.CloseIntentID), true, nil
	}
	if pos.Status != PositionOpenActive {
		return "", false, fmt.Errorf("position not active")
	}
	return "", false, nil
}

func (s *PositionService) prepareCloseOrder(ctx context.Context, pos store.PositionRecord, intentID string, reason string, triggerPrice float64, closeQty float64) (store.PositionRecord, string, string, error) {
	pos, err := s.latchClose(ctx, pos, intentID, triggerPrice)
	if err != nil {
		return store.PositionRecord{}, "", "", err
	}
	if s.Cache != nil {
		pos = s.Cache.HydratePosition(pos)
	}
	intentKind := resolveCloseIntentKind(pos, closeQty)
	clientOrderID := "brale-" + intentID
	if s.Cache != nil {
		s.Cache.SetCloseReason(pos.ExecutorPositionID, reason)
	}
	s.logCloseIntent(ctx, pos, intentKind, reason, triggerPrice, closeQty, clientOrderID)
	return pos, intentKind, clientOrderID, nil
}

func (s *PositionService) submitCloseFlow(ctx context.Context, pos store.PositionRecord, intentKind string, closeQty float64, clientOrderID string) error {
	submittingPos, err := s.markCloseSubmitting(ctx, pos)
	if err != nil {
		return err
	}
	if _, err := s.submitCloseOrder(ctx, submittingPos, intentKind, closeQty, clientOrderID); err != nil {
		return err
	}
	return s.markClosePending(ctx, submittingPos)
}

type closeSummary struct {
	stopPrice  float64
	tpPrices   []float64
	qty        float64
	entryPrice float64
}

func (s *PositionService) buildCloseSummary(pos store.PositionRecord, triggerPrice float64, closeQty float64) closeSummary {
	stopPrice := float64(0)
	tpPrices := []float64{}
	if len(pos.RiskJSON) > 0 {
		if plan, err := DecodeRiskPlan(pos.RiskJSON); err == nil {
			stopPrice = plan.StopPrice
			for _, level := range plan.TPLevels {
				if level.Price > 0 {
					tpPrices = append(tpPrices, level.Price)
				}
			}
		}
	}
	qty := pos.Qty
	if qty <= 0 {
		qty = closeQty
	}
	if qty <= 0 && pos.InitialStake > 0 && pos.AvgEntry > 0 {
		qty = pos.InitialStake / pos.AvgEntry
	}
	entryPrice := pos.AvgEntry
	if entryPrice <= 0 && pos.LastPrice > 0 {
		entryPrice = pos.LastPrice
	}
	if entryPrice <= 0 && triggerPrice > 0 {
		entryPrice = triggerPrice
	}
	return closeSummary{
		stopPrice:  stopPrice,
		tpPrices:   tpPrices,
		qty:        qty,
		entryPrice: entryPrice,
	}
}

func (s *PositionService) logAndNotifyClose(ctx context.Context, pos store.PositionRecord, reason string, triggerPrice float64, closeQty float64) {
	summary := s.buildCloseSummary(pos, triggerPrice, closeQty)
	logging.FromContext(ctx).Named("execution").Info("position close detail",
		zap.String("symbol", pos.Symbol),
		zap.String("direction", strings.TrimSpace(pos.Side)),
		zap.Float64("qty", summary.qty),
		zap.Float64("close_qty", closeQty),
		zap.Float64("entry", summary.entryPrice),
		zap.Float64("trigger_price", triggerPrice),
		zap.Float64("stop", summary.stopPrice),
		zap.Float64s("take_profits", summary.tpPrices),
		zap.String("reason", strings.TrimSpace(reason)),
		zap.Float64("risk_pct", pos.RiskPct),
		zap.Float64("leverage", pos.Leverage),
	)
	if s.Notifier == nil {
		return
	}
	err := s.Notifier.SendPositionClose(ctx, PositionCloseNotice{
		Symbol:       pos.Symbol,
		Direction:    strings.TrimSpace(pos.Side),
		Qty:          summary.qty,
		CloseQty:     closeQty,
		EntryPrice:   summary.entryPrice,
		TriggerPrice: triggerPrice,
		StopPrice:    summary.stopPrice,
		TakeProfits:  summary.tpPrices,
		Reason:       strings.TrimSpace(reason),
		RiskPct:      pos.RiskPct,
		Leverage:     pos.Leverage,
		PositionID:   pos.PositionID,
	})
	if err != nil {
		logging.FromContext(ctx).Named("execution").Error("position close notify failed", zap.Error(err))
	}
}

func (s *PositionService) latchClose(ctx context.Context, pos store.PositionRecord, intentID string, triggerPrice float64) (store.PositionRecord, error) {
	now := time.Now().UnixMilli()
	ok, err := s.Store.UpdatePosition(ctx, pos.PositionID, pos.Version, map[string]any{
		"close_intent_id":    intentID,
		"close_submitted_at": int64(0),
		"status":             PositionCloseArmed,
		"version":            pos.Version + 1,
	})
	if err != nil {
		return store.PositionRecord{}, err
	}
	if !ok {
		return store.PositionRecord{}, fmt.Errorf("close latch denied")
	}
	pos.CloseIntentID = intentID
	pos.Status = PositionCloseArmed
	pos.Version++
	if s.Cache != nil {
		s.Cache.UpdatePrice(pos.PositionID, pos.Symbol, triggerPrice, now)
	}
	return pos, nil
}

func resolveCloseIntentKind(pos store.PositionRecord, closeQty float64) string {
	if closeQty > 0 && pos.Qty > 0 && closeQty < pos.Qty {
		return "REDUCE"
	}
	return "CLOSE"
}

func (s *PositionService) logCloseIntent(ctx context.Context, pos store.PositionRecord, intentKind string, reason string, triggerPrice float64, closeQty float64, clientOrderID string) {
	logging.FromContext(ctx).Named("execution").Info("close intent created",
		zap.String("position_id", pos.PositionID),
		zap.String("symbol", pos.Symbol),
		zap.String("side", pos.Side),
		zap.String("intent_kind", intentKind),
		zap.String("reason", reason),
		zap.Float64("trigger_price", triggerPrice),
		zap.Float64("close_qty", closeQty),
		zap.String("client_order_id", clientOrderID),
	)
}

func (s *PositionService) submitCloseOrder(ctx context.Context, pos store.PositionRecord, intentKind string, closeQty float64, clientOrderID string) (execution.PlaceOrderResp, error) {
	kind := execution.OrderClose
	if intentKind == "REDUCE" {
		kind = execution.OrderReduce
	}
	reqQty := resolveCloseQuantity(s.Executor, kind, closeQty)
	resp, err := s.Executor.PlaceOrder(ctx, execution.PlaceOrderReq{
		Kind:          kind,
		Symbol:        pos.Symbol,
		Side:          pos.Side,
		Quantity:      reqQty,
		PositionID:    pos.ExecutorPositionID,
		ClientOrderID: clientOrderID,
		OrderType:     "market",
	})
	if err == nil {
		logging.FromContext(ctx).Named("execution").Info("close order submitted",
			zap.String("position_id", pos.PositionID),
			zap.String("symbol", pos.Symbol),
			zap.String("intent_kind", intentKind),
			zap.String("client_order_id", clientOrderID),
			zap.String("external_order_id", strings.TrimSpace(resp.ExternalID)),
		)
		return resp, nil
	}
	s.logCloseSubmitError(ctx, pos, clientOrderID, intentKind, err)
	return resp, err
}

func (s *PositionService) logCloseSubmitError(ctx context.Context, pos store.PositionRecord, clientOrderID string, intentKind string, err error) {
	logger := logging.FromContext(ctx).Named("execution")
	logger.Error("order submit failed",
		zap.String("position_id", pos.PositionID),
		zap.String("symbol", pos.Symbol),
		zap.String("client_order_id", clientOrderID),
		zap.String("intent_kind", intentKind),
		zap.Error(err),
	)
	if s.Notifier != nil {
		if notifyErr := s.Notifier.SendError(ctx, err.Error()); notifyErr != nil {
			logger.Error("notify error failed", zap.Error(notifyErr))
		}
	}
}

type closeQuantityResolver interface {
	ResolveCloseQuantity(kind execution.OrderKind, requestedQty float64) float64
}

func resolveCloseQuantity(executor execution.Executor, kind execution.OrderKind, closeQty float64) float64 {
	if executor == nil {
		return closeQty
	}
	resolver, ok := executor.(closeQuantityResolver)
	if !ok {
		return closeQty
	}
	resolved := resolver.ResolveCloseQuantity(kind, closeQty)
	if resolved < 0 {
		return 0
	}
	return resolved
}

func (s *PositionService) markCloseSubmitting(ctx context.Context, pos store.PositionRecord) (store.PositionRecord, error) {
	now := time.Now().UnixMilli()
	ok, err := s.Store.UpdatePosition(ctx, pos.PositionID, pos.Version, map[string]any{
		"status":             PositionCloseSubmitting,
		"close_submitted_at": now,
		"version":            pos.Version + 1,
	})
	if err != nil {
		return store.PositionRecord{}, err
	}
	if !ok {
		return store.PositionRecord{}, fmt.Errorf("close submitting update denied")
	}
	pos.Status = PositionCloseSubmitting
	pos.CloseSubmittedAt = now
	pos.Version++
	return pos, nil
}

func (s *PositionService) markClosePending(ctx context.Context, pos store.PositionRecord) error {
	ok, err := s.Store.UpdatePosition(ctx, pos.PositionID, pos.Version, map[string]any{
		"status":  PositionClosePending,
		"version": pos.Version + 1,
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("close pending update denied")
	}
	return nil
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

func shouldResetOpenPlanOrder(err error) bool {
	class := errclass.ClassifyError(err)
	if string(class.Scope) != "execution" {
		return false
	}
	return string(class.Kind) == "validation"
}

func isCloseInFlightStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case PositionCloseArmed, PositionCloseSubmitting, PositionClosePending:
		return true
	default:
		return false
	}
}
