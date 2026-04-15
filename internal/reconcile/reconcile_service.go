// 本文件主要内容：对账本地持仓与执行器持仓并收敛状态。
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/execution"
	"brale-core/internal/market"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/position"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type PositionReflector interface {
	ReflectOnClose(ctx context.Context, pos store.PositionRecord, exitPrice float64)
}

type ReconcileService struct {
	Store              store.Store
	Executor           execution.Executor
	OrderStatusFetcher OrderStatusFetcher
	PriceSource        market.PriceSource
	Now                func() int64
	CloseRecoverAfter  time.Duration
	Notifier           Notifier
	Cache              *position.PositionCache
	PlanCache          *position.PlanCache
	RiskPlans          *position.RiskPlanService
	AllowSymbol        func(symbol string) bool
	Reflector          PositionReflector
}

func (s *ReconcileService) RunOnce(ctx context.Context, symbol string) error {
	logger := logging.FromContext(ctx).Named("reconcile")
	if s.Store == nil || s.Executor == nil {
		err := reconcileValidationErrorf("store/executor is required")
		logger.Error("reconcile dependencies missing", zap.Error(err))
		s.notifyError(ctx, logger, err)
		return err
	}
	positions, err := s.Store.ListPositionsByStatus(ctx, position.ReconcilePositionStatuses)
	if err != nil {
		logger.Error("list positions failed", zap.Error(err))
		s.notifyError(ctx, logger, err)
		return err
	}
	ext, err := s.Executor.GetOpenPositions(ctx, symbol)
	if err != nil {
		logger.Error("fetch external positions failed", zap.Error(err))
		s.notifyError(ctx, logger, err)
		return err
	}
	byID, bySymbol := indexExternalPositions(ext)
	if s.PlanCache != nil {
		if err := s.reconcilePlanEntries(ctx, symbol, byID, bySymbol); err != nil {
			logger.Error("reconcile plan entries failed", zap.Error(err))
			s.notifyError(ctx, logger, err)
			return err
		}
		if err := s.reconcilePlanOrders(ctx, symbol); err != nil {
			logger.Error("reconcile plan orders failed", zap.Error(err))
			s.notifyError(ctx, logger, err)
			return err
		}
	}
	reconcileErrs := make([]error, 0)
	for _, pos := range positions {
		if symbol != "" && !strings.EqualFold(pos.Symbol, symbol) {
			continue
		}
		if s.AllowSymbol != nil && !s.AllowSymbol(pos.Symbol) {
			continue
		}
		if err := s.reconcilePosition(ctx, pos, byID, bySymbol); err != nil {
			logger.Error("reconcile position failed", zap.Error(err), zap.String("position_id", pos.PositionID), zap.String("symbol", pos.Symbol))
			reconcileErrs = append(reconcileErrs, fmt.Errorf("reconcile position %s (%s): %w", pos.PositionID, pos.Symbol, err))
		}
	}
	if s.Cache != nil {
		for _, extPos := range ext {
			if s.AllowSymbol != nil && !s.AllowSymbol(extPos.Symbol) {
				continue
			}
			s.Cache.UpdateFromExternal(extPos)
		}
	}
	if len(reconcileErrs) > 0 {
		err := errors.Join(reconcileErrs...)
		s.notifyError(ctx, logger, err)
		return err
	}
	logger.Debug("reconcile completed", zap.String("symbol", symbol))
	return nil
}

func (s *ReconcileService) notifyError(ctx context.Context, logger *zap.Logger, err error) {
	if s.Notifier == nil {
		return
	}
	if notifyErr := s.Notifier.SendError(ctx, ErrorNotice{Severity: "error", Component: "reconcile", Message: err.Error()}); notifyErr != nil {
		logger.Error("notify error failed", zap.Error(notifyErr))
	}
}

func indexExternalPositions(ext []execution.ExternalPosition) (map[string]execution.ExternalPosition, map[string][]execution.ExternalPosition) {
	byID := make(map[string]execution.ExternalPosition, len(ext))
	bySymbol := make(map[string][]execution.ExternalPosition)
	for _, p := range ext {
		byID[p.PositionID] = p
		key := strings.ToUpper(strings.TrimSpace(p.Symbol))
		bySymbol[key] = append(bySymbol[key], p)
	}
	return byID, bySymbol
}

func (s *ReconcileService) nowTime() time.Time {
	if s != nil && s.Now != nil {
		if ts := s.Now(); ts > 0 {
			return time.UnixMilli(ts).UTC()
		}
	}
	return time.Now().UTC()
}

func (s *ReconcileService) closeRecoverAfter() time.Duration {
	if s != nil && s.CloseRecoverAfter > 0 {
		return s.CloseRecoverAfter
	}
	return 10 * time.Minute
}
