package position

import (
	"context"
	"fmt"
	"strings"
	"time"

	braleOtel "brale-core/internal/otel"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/store"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

func (s *PositionService) ArmClose(ctx context.Context, pos store.PositionRecord, reason string, triggerPrice float64, closeQty float64, positionQty float64) (string, error) {
	if err := s.ensureCloseDeps(); err != nil {
		return "", err
	}
	if existingIntentID, handled, err := s.guardArmClose(ctx, pos); handled || err != nil {
		return existingIntentID, err
	}
	intentID := uuid.NewString()
	pos, intentKind, clientOrderID, err := s.prepareCloseOrder(ctx, pos, intentID, reason, triggerPrice, closeQty, positionQty)
	if err != nil {
		return "", err
	}
	if err := s.submitCloseFlow(ctx, pos, intentKind, closeQty, clientOrderID); err != nil {
		return intentID, err
	}
	s.logAndNotifyClose(ctx, pos, reason, triggerPrice, closeQty)
	braleOtel.PositionCloseTotal.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("symbol", pos.Symbol),
		attribute.String("side", pos.Side),
	))
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

func (s *PositionService) prepareCloseOrder(ctx context.Context, pos store.PositionRecord, intentID string, reason string, triggerPrice float64, closeQty float64, positionQty float64) (store.PositionRecord, string, string, error) {
	pos, err := s.latchClose(ctx, pos, intentID, triggerPrice)
	if err != nil {
		return store.PositionRecord{}, "", "", err
	}
	if s.Cache != nil {
		pos = s.Cache.HydratePosition(pos)
	}
	effectiveQty := resolveClosePositionQty(pos, positionQty)
	if effectiveQty > 0 {
		pos.Qty = effectiveQty
	}
	intentKind := resolveCloseIntentKind(effectiveQty, closeQty)
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

func (s *PositionService) latchClose(ctx context.Context, pos store.PositionRecord, intentID string, triggerPrice float64) (store.PositionRecord, error) {
	now := time.Now().UnixMilli()
	ok, err := s.Store.UpdatePositionPatch(ctx, store.PositionPatch{
		PositionID:       pos.PositionID,
		ExpectedVersion:  pos.Version,
		NextVersion:      pos.Version + 1,
		CloseIntentID:    store.PtrString(intentID),
		CloseSubmittedAt: store.PtrInt64(0),
		Status:           store.PtrString(PositionCloseArmed),
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
	ctx, span := braleOtel.Tracer("brale-core/execution").Start(ctx, "brale.execute.place_order")
	span.SetAttributes(
		attribute.String("execution.order_kind", intentKind),
		attribute.String("execution.symbol", pos.Symbol),
		attribute.String("execution.side", pos.Side),
		attribute.String("execution.position_id", pos.PositionID),
	)
	defer span.End()

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
		span.SetAttributes(attribute.String("execution.external_order_id", strings.TrimSpace(resp.ExternalID)))
		logging.FromContext(ctx).Named("execution").Info("close order submitted",
			zap.String("position_id", pos.PositionID),
			zap.String("symbol", pos.Symbol),
			zap.String("intent_kind", intentKind),
			zap.String("client_order_id", clientOrderID),
			zap.String("external_order_id", strings.TrimSpace(resp.ExternalID)),
		)
		return resp, nil
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	s.logCloseSubmitError(ctx, pos, clientOrderID, intentKind, err)
	return resp, err
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
	ok, err := s.Store.UpdatePositionPatch(ctx, store.PositionPatch{
		PositionID:       pos.PositionID,
		ExpectedVersion:  pos.Version,
		NextVersion:      pos.Version + 1,
		Status:           store.PtrString(PositionCloseSubmitting),
		CloseSubmittedAt: store.PtrInt64(now),
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
	ok, err := s.Store.UpdatePositionPatch(ctx, store.PositionPatch{
		PositionID:      pos.PositionID,
		ExpectedVersion: pos.Version,
		NextVersion:     pos.Version + 1,
		Status:          store.PtrString(PositionClosePending),
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("close pending update denied")
	}
	return nil
}
