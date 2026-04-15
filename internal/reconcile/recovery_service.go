// 本文件主要内容：启动时恢复持仓与未完成意图。
package reconcile

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type RecoveryService struct {
	Store       store.Store
	Executor    execution.Executor
	Notifier    Notifier
	Cache       *position.PositionCache
	AllowSymbol func(symbol string) bool
}

func (s *RecoveryService) RunOnce(ctx context.Context, symbol string) error {
	logger := logging.FromContext(ctx).Named("recovery")
	if err := s.ensureDependencies(ctx, logger); err != nil {
		return err
	}
	positions, err := s.Store.ListPositionsByStatus(ctx, position.OpenPositionStatuses)
	if err != nil {
		return s.reportError(ctx, logger, "list positions failed", err)
	}
	ext, err := s.Executor.GetOpenPositions(ctx, symbol)
	if err != nil {
		return s.reportError(ctx, logger, "fetch external positions failed", err)
	}
	byID := mapExternalPositions(ext)
	bySymbol := mapExternalPositionsBySymbol(ext)
	for _, pos := range positions {
		if !matchSymbol(symbol, pos.Symbol) {
			continue
		}
		if s.AllowSymbol != nil && !s.AllowSymbol(pos.Symbol) {
			continue
		}
		if err := s.reconcilePosition(ctx, logger, pos, byID, bySymbol); err != nil {
			return err
		}
	}
	if err := s.restoreMissingPositions(ctx, logger, positions, ext, bySymbol); err != nil {
		return err
	}
	if s.Cache != nil {
		for _, extPos := range ext {
			if s.AllowSymbol != nil && !s.AllowSymbol(extPos.Symbol) {
				continue
			}
			s.Cache.UpdateFromExternal(extPos)
		}
	}
	logger.Info("recovery completed", zap.String("symbol", symbol))
	return nil
}

func (s *RecoveryService) ensureDependencies(ctx context.Context, logger *zap.Logger) error {
	if s.Store != nil && s.Executor != nil {
		return nil
	}
	err := reconcileValidationErrorf("store/executor is required")
	return s.reportError(ctx, logger, "recovery dependencies missing", err)
}

func mapExternalPositions(ext []execution.ExternalPosition) map[string]execution.ExternalPosition {
	byID := make(map[string]execution.ExternalPosition, len(ext))
	for _, p := range ext {
		byID[p.PositionID] = p
	}
	return byID
}

func mapExternalPositionsBySymbol(ext []execution.ExternalPosition) map[string][]execution.ExternalPosition {
	bySymbol := make(map[string][]execution.ExternalPosition)
	for _, p := range ext {
		key := symbolpkg.Normalize(p.Symbol)
		if key == "" {
			continue
		}
		bySymbol[key] = append(bySymbol[key], p)
	}
	return bySymbol
}

func matchSymbol(filter string, symbol string) bool {
	return filter == "" || strings.EqualFold(symbol, filter)
}

func (s *RecoveryService) reconcilePosition(ctx context.Context, logger *zap.Logger, pos store.PositionRecord, byID map[string]execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) error {
	extPos, ok := resolveRecoveryPosition(pos, byID, bySymbol)
	if !ok || extPos.Quantity <= 0 {
		if s.Cache != nil {
			s.Cache.DeleteByID(pos.PositionID)
		}
		return s.handleMissingExternal(ctx, logger, pos)
	}
	return s.updateActivePosition(ctx, logger, pos, extPos)
}

func (s *RecoveryService) handleMissingExternal(ctx context.Context, logger *zap.Logger, pos store.PositionRecord) error {
	if pos.Status == position.PositionOpenAborting || pos.Status == position.PositionOpenAborted {
		return nil
	}
	_, err := s.Store.UpdatePositionPatch(ctx, store.PositionPatch{
		PositionID:       pos.PositionID,
		ExpectedVersion:  pos.Version,
		NextVersion:      pos.Version + 1,
		Status:           store.PtrString(position.PositionClosed),
		CloseIntentID:    store.PtrString(""),
		CloseSubmittedAt: store.PtrInt64(0),
	})
	if err != nil {
		return s.reportPositionError(ctx, logger, "update closed position failed", err, pos)
	}
	logger.Info("recovery position closed",
		zap.String("position_id", pos.PositionID),
		zap.String("symbol", pos.Symbol),
		zap.String("status", pos.Status),
		zap.String("source", pos.Source),
	)
	return nil
}

func (s *RecoveryService) updateActivePosition(ctx context.Context, logger *zap.Logger, pos store.PositionRecord, extPos execution.ExternalPosition) error {
	if s.Cache != nil {
		s.Cache.UpdateFromExternal(extPos)
	}
	patch := store.PositionPatch{
		PositionID:         pos.PositionID,
		ExpectedVersion:    pos.Version,
		NextVersion:        pos.Version + 1,
		Status:             store.PtrString(position.PositionOpenActive),
		ExecutorPositionID: store.PtrString(extPos.PositionID),
		Qty:                store.PtrFloat64(extPos.Quantity),
		AvgEntry:           store.PtrFloat64(extPos.AvgEntry),
	}
	updates := patch.Updates()
	if pos.InitialStake == 0 && extPos.InitialStake > 0 {
		patch.InitialStake = store.PtrFloat64(extPos.InitialStake)
		updates["initial_stake"] = extPos.InitialStake
	}
	if strings.TrimSpace(pos.Source) == "" {
		patch.Source = store.PtrString("freqtrade")
		updates["source"] = "freqtrade"
	}
	_, err := s.Store.UpdatePositionPatch(ctx, patch)
	if err != nil {
		return s.reportPositionError(ctx, logger, "update active position failed", err, pos)
	}
	s.logRecoveryPosition(logger, pos, extPos, updates, "external_present")
	return nil
}

func resolveRecoveryPosition(pos store.PositionRecord, byID map[string]execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) (execution.ExternalPosition, bool) {
	if strings.TrimSpace(pos.ExecutorPositionID) != "" {
		if ext, ok := byID[pos.ExecutorPositionID]; ok {
			return ext, true
		}
	}
	candidates := bySymbol[symbolpkg.Normalize(pos.Symbol)]
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return execution.ExternalPosition{}, false
}

func (s *RecoveryService) restoreMissingPositions(ctx context.Context, logger *zap.Logger, existing []store.PositionRecord, ext []execution.ExternalPosition, bySymbol map[string][]execution.ExternalPosition) error {
	existingTradeIDs := make(map[string]struct{}, len(existing))
	existingSymbols := make(map[string]struct{})
	for _, pos := range existing {
		if strings.TrimSpace(pos.ExecutorPositionID) != "" {
			existingTradeIDs[pos.ExecutorPositionID] = struct{}{}
		}
		sym := symbolpkg.Normalize(pos.Symbol)
		if sym != "" {
			existingSymbols[sym] = struct{}{}
		}
	}
	for _, extPos := range ext {
		if s.AllowSymbol != nil && !s.AllowSymbol(extPos.Symbol) {
			continue
		}
		if strings.TrimSpace(extPos.PositionID) == "" {
			continue
		}
		if _, ok := existingTradeIDs[extPos.PositionID]; ok {
			continue
		}
		symKey := symbolpkg.Normalize(extPos.Symbol)
		if symKey == "" {
			continue
		}
		candidates := bySymbol[symKey]
		if len(candidates) != 1 {
			continue
		}
		if _, ok := existingSymbols[symKey]; ok {
			continue
		}
		pos := store.PositionRecord{
			PositionID:         fmt.Sprintf("restore-%s", extPos.PositionID),
			Symbol:             extPos.Symbol,
			Side:               extPos.Side,
			Status:             position.PositionOpenActive,
			ExecutorName:       s.Executor.Name(),
			ExecutorPositionID: extPos.PositionID,
			Qty:                extPos.Quantity,
			AvgEntry:           extPos.AvgEntry,
			InitialStake:       extPos.InitialStake,
			Source:             "freqtrade-restore",
			Version:            1,
		}
		if err := s.Store.SavePosition(ctx, &pos); err != nil {
			return s.reportError(ctx, logger, "restore position failed", err)
		}
		if s.Cache != nil {

			s.Cache.UpdateFromExternal(extPos)
		}
		s.logRecoveryPosition(logger, pos, extPos, map[string]any{"source": pos.Source}, "external_restore")
	}
	return nil
}

func (s *RecoveryService) logRecoveryPosition(logger *zap.Logger, pos store.PositionRecord, extPos execution.ExternalPosition, updates map[string]any, reason string) {
	if logger == nil {
		return
	}
	source := strings.TrimSpace(pos.Source)
	if source == "" {
		if raw, ok := updates["source"]; ok {
			source = strings.TrimSpace(fmt.Sprintf("%v", raw))
		}
	}
	logger.Info("recovery position sync",
		zap.String("reason", reason),
		zap.String("position_id", pos.PositionID),
		zap.String("symbol", pos.Symbol),
		zap.String("side", pos.Side),
		zap.String("status", pos.Status),
		zap.String("source", source),
		zap.String("executor_position_id", extPos.PositionID),
		zap.String("executor_name", pos.ExecutorName),
		zap.Float64("qty", extPos.Quantity),
		zap.Float64("avg_entry", extPos.AvgEntry),
		zap.Float64("initial_stake", extPos.InitialStake),
		zap.String("open_order_id", extPos.OpenOrderID),
		zap.String("close_order_id", extPos.CloseOrderID),
		zap.String("entry_tag", extPos.EntryTag),
		zap.Float64("current_price", extPos.CurrentPrice),
		zap.Int64("updated_at", extPos.UpdatedAt),
	)
}

func (s *RecoveryService) reportError(ctx context.Context, logger *zap.Logger, msg string, err error) error {
	logger.Error(msg, zap.Error(err))
	s.notifyError(ctx, logger, err)
	return err
}

func (s *RecoveryService) reportPositionError(ctx context.Context, logger *zap.Logger, msg string, err error, pos store.PositionRecord) error {
	logger.Error(msg, zap.Error(err), zap.String("position_id", pos.PositionID), zap.String("symbol", pos.Symbol))
	s.notifyError(ctx, logger, err)
	return err
}

func (s *RecoveryService) notifyError(ctx context.Context, logger *zap.Logger, err error) {
	if s.Notifier == nil || err == nil {
		return
	}
	if notifyErr := s.Notifier.SendError(ctx, ErrorNotice{Severity: "error", Component: "reconcile", Message: err.Error()}); notifyErr != nil {
		logger.Error("notify error failed", zap.Error(notifyErr))
	}
}
