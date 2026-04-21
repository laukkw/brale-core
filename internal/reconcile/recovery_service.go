// 本文件主要内容：启动时恢复持仓与未完成意图。
package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	symbolpkg "brale-core/internal/pkg/symbol"
	"brale-core/internal/position"
	"brale-core/internal/risk"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

type RecoveryService struct {
	Store       store.Store
	Executor    execution.Executor
	Notifier    Notifier
	Cache       *position.PositionCache
	PlanCache   *position.PlanCache
	RiskPlans   *position.RiskPlanService
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
			keepInCache, err := s.shouldCacheRecoveredExternalPosition(ctx, positions, extPos)
			if err != nil {
				return s.reportError(ctx, logger, "decide recovery cache refresh failed", err)
			}
			if !keepInCache {
				s.Cache.DeleteBySymbol(extPos.Symbol)
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
	if !ok || !isMeaningfulCloseFlowExternalQty(pos, extPos) {
		if s.Cache != nil {
			s.Cache.DeleteBySymbol(pos.Symbol)
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
	var recoveredRisk recoveredRiskPlan
	if pos.InitialStake == 0 && extPos.InitialStake > 0 {
		patch.InitialStake = store.PtrFloat64(extPos.InitialStake)
		updates["initial_stake"] = extPos.InitialStake
	}
	if strings.TrimSpace(pos.Source) == "" {
		patch.Source = store.PtrString("freqtrade")
		updates["source"] = "freqtrade"
	}
	if len(pos.RiskJSON) == 0 {
		nextRisk, err := s.resolveRecoveredRiskPlan(ctx, pos, extPos)
		if err != nil {
			return s.reportPositionError(ctx, logger, "recover active position risk plan failed", err, pos)
		}
		if nextRisk.OK {
			recoveredRisk = nextRisk
			patch.RiskJSON = store.PtrBytes(recoveredRisk.Raw)
			updates["risk_json"] = recoveredRisk.Raw
		} else {
			logger.Warn("recovery active position has no local risk plan",
				zap.String("position_id", pos.PositionID),
				zap.String("symbol", pos.Symbol),
				zap.String("executor_position_id", extPos.PositionID),
			)
		}
	}
	_, err := s.Store.UpdatePositionPatch(ctx, patch)
	if err != nil {
		return s.reportPositionError(ctx, logger, "update active position failed", err, pos)
	}
	if patch.RiskJSON != nil {
		if err := s.saveRecoveredRiskPlanHistory(ctx, pos.PositionID, recoveredRisk, "recovery_active"); err != nil {
			return s.reportPositionError(ctx, logger, "save recovered active risk plan failed", err, pos)
		}
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
		skipRestore, err := s.shouldSkipResidualRestore(ctx, extPos)
		if err != nil {
			return s.reportError(ctx, logger, "residual restore lookup failed", err)
		}
		if skipRestore {
			logger.Info("skip residual close-flow restore",
				zap.String("symbol", extPos.Symbol),
				zap.String("executor_position_id", extPos.PositionID),
				zap.Float64("qty", extPos.Quantity),
			)
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
		recoveredRisk, err := s.resolveRecoveredRiskPlan(ctx, store.PositionRecord{}, extPos)
		if err != nil {
			return s.reportError(ctx, logger, "recover restored position risk plan failed", err)
		}
		if recoveredRisk.OK {
			pos.RiskJSON = recoveredRisk.Raw
			pos.RiskPct = recoveredRisk.RiskPct
			pos.Leverage = recoveredRisk.Leverage
			pos.StopReason = recoveredRisk.StopReason
		} else {
			logger.Warn("restored external position has no local risk plan",
				zap.String("symbol", extPos.Symbol),
				zap.String("executor_position_id", extPos.PositionID),
			)
		}
		if err := s.Store.SavePosition(ctx, &pos); err != nil {
			return s.reportError(ctx, logger, "restore position failed", err)
		}
		if recoveredRisk.OK {
			if err := s.saveRecoveredRiskPlanHistory(ctx, pos.PositionID, recoveredRisk, "external_restore"); err != nil {
				return s.reportError(ctx, logger, "save restored position risk plan failed", err)
			}
		}
		if s.Cache != nil {
			s.Cache.UpdateFromExternal(extPos)
		}
		s.logRecoveryPosition(logger, pos, extPos, map[string]any{"source": pos.Source}, "external_restore")
	}
	return nil
}

func (s *RecoveryService) shouldCacheRecoveredExternalPosition(ctx context.Context, existing []store.PositionRecord, extPos execution.ExternalPosition) (bool, error) {
	if extPos.Quantity <= 0 {
		return false, nil
	}
	for _, pos := range existing {
		if !matchesExternalPosition(pos, extPos) {
			continue
		}
		if isCloseFlowStatus(pos.Status) && position.IsResidualCloseFlowQty(pos, extPos.Quantity) {
			return false, nil
		}
	}
	skipRestore, err := s.shouldSkipResidualRestore(ctx, extPos)
	if err != nil {
		return false, err
	}
	return !skipRestore, nil
}

func (s *RecoveryService) shouldSkipResidualRestore(ctx context.Context, extPos execution.ExternalPosition) (bool, error) {
	if s == nil || s.Store == nil || extPos.Quantity <= 0 {
		return false, nil
	}
	latestPos, ok, err := s.Store.FindPositionBySymbol(ctx, extPos.Symbol, nil)
	if err != nil {
		return false, fmt.Errorf("find latest position for residual restore %s: %w", strings.TrimSpace(extPos.Symbol), err)
	}
	if !ok {
		return false, nil
	}
	return isClosedResidualExternalPosition(latestPos, extPos), nil
}

type recoveredRiskPlan struct {
	Raw        []byte
	Plan       risk.RiskPlan
	RiskPct    float64
	Leverage   float64
	StopReason string
	Source     string
	OK         bool
}

var errRecoveredRiskPlanNoControls = errors.New("risk plan has no stop or take-profit levels")

func (s *RecoveryService) resolveRecoveredRiskPlan(ctx context.Context, pos store.PositionRecord, extPos execution.ExternalPosition) (recoveredRiskPlan, error) {
	if plan, ok, err := s.recoveredRiskFromPosition(ctx, pos, extPos); err != nil || ok {
		return plan, err
	}
	if plan, ok, err := s.recoveredRiskFromPlanCache(extPos); err != nil || ok {
		return plan, err
	}
	latestPos, ok, err := s.findLatestMatchingRiskPosition(ctx, extPos)
	if err != nil || !ok {
		return recoveredRiskPlan{}, err
	}
	plan, _, err := s.recoveredRiskFromPosition(ctx, latestPos, extPos)
	return plan, err
}

func (s *RecoveryService) recoveredRiskFromPosition(ctx context.Context, pos store.PositionRecord, extPos execution.ExternalPosition) (recoveredRiskPlan, bool, error) {
	if strings.TrimSpace(pos.PositionID) == "" {
		return recoveredRiskPlan{}, false, nil
	}
	if strings.TrimSpace(extPos.PositionID) != "" && !matchesExternalPosition(pos, extPos) {
		return recoveredRiskPlan{}, false, nil
	}
	if len(pos.RiskJSON) > 0 {
		recovered, err := recoveredRiskFromRaw(pos.RiskJSON)
		if err != nil {
			if errors.Is(err, errRecoveredRiskPlanNoControls) {
				return recoveredRiskPlan{}, false, nil
			}
			return recoveredRiskPlan{}, false, fmt.Errorf("decode local risk plan %s: %w", pos.PositionID, err)
		}
		recovered.RiskPct = pos.RiskPct
		recovered.Leverage = pos.Leverage
		recovered.StopReason = strings.TrimSpace(pos.StopReason)
		recovered.Source = "position"
		return recovered, true, nil
	}
	if s == nil || s.Store == nil {
		return recoveredRiskPlan{}, false, nil
	}
	history, ok, err := s.Store.FindLatestRiskPlanHistory(ctx, pos.PositionID)
	if err != nil {
		return recoveredRiskPlan{}, false, fmt.Errorf("find latest risk plan history %s: %w", pos.PositionID, err)
	}
	if !ok || len(history.PayloadJSON) == 0 {
		return recoveredRiskPlan{}, false, nil
	}
	recovered, err := recoveredRiskFromRaw(history.PayloadJSON)
	if err != nil {
		if errors.Is(err, errRecoveredRiskPlanNoControls) {
			return recoveredRiskPlan{}, false, nil
		}
		return recoveredRiskPlan{}, false, fmt.Errorf("decode risk plan history %s: %w", pos.PositionID, err)
	}
	recovered.RiskPct = pos.RiskPct
	recovered.Leverage = pos.Leverage
	recovered.StopReason = strings.TrimSpace(pos.StopReason)
	recovered.Source = "risk_plan_history"
	return recovered, true, nil
}

func (s *RecoveryService) recoveredRiskFromPlanCache(extPos execution.ExternalPosition) (recoveredRiskPlan, bool, error) {
	if s == nil || s.PlanCache == nil {
		return recoveredRiskPlan{}, false, nil
	}
	entry, ok := s.PlanCache.GetEntry(extPos.Symbol)
	if !ok || entry == nil {
		return recoveredRiskPlan{}, false, nil
	}
	if !planEntryMatchesExternal(*entry, extPos) {
		return recoveredRiskPlan{}, false, nil
	}
	plan := entry.Plan
	riskPlan := risk.BuildRiskPlan(risk.RiskPlanInput{
		Entry:            plan.Entry,
		StopLoss:         plan.StopLoss,
		PositionSize:     plan.PositionSize,
		TakeProfits:      plan.TakeProfits,
		TakeProfitRatios: plan.TakeProfitRatios,
	})
	recovered, err := recoveredRiskFromPlan(riskPlan)
	if err != nil {
		if errors.Is(err, errRecoveredRiskPlanNoControls) {
			return recoveredRiskPlan{}, false, nil
		}
		return recoveredRiskPlan{}, false, err
	}
	recovered.RiskPct = plan.RiskPct
	recovered.Leverage = plan.Leverage
	recovered.StopReason = strings.TrimSpace(plan.RiskAnnotations.StopReason)
	recovered.Source = "plan_cache"
	return recovered, true, nil
}

func (s *RecoveryService) findLatestMatchingRiskPosition(ctx context.Context, extPos execution.ExternalPosition) (store.PositionRecord, bool, error) {
	if s == nil || s.Store == nil {
		return store.PositionRecord{}, false, nil
	}
	latestPos, ok, err := s.Store.FindPositionBySymbol(ctx, extPos.Symbol, nil)
	if err != nil {
		return store.PositionRecord{}, false, fmt.Errorf("find latest risk position %s: %w", strings.TrimSpace(extPos.Symbol), err)
	}
	if !ok || !matchesExactExternalPositionID(latestPos, extPos) {
		return store.PositionRecord{}, false, nil
	}
	return latestPos, true, nil
}

func matchesExactExternalPositionID(pos store.PositionRecord, extPos execution.ExternalPosition) bool {
	localID := strings.TrimSpace(pos.ExecutorPositionID)
	externalID := strings.TrimSpace(extPos.PositionID)
	return localID != "" && externalID != "" && localID == externalID
}

func planEntryMatchesExternal(entry position.PlanEntry, extPos execution.ExternalPosition) bool {
	if !strings.EqualFold(strings.TrimSpace(entry.Plan.Symbol), strings.TrimSpace(extPos.Symbol)) {
		return false
	}
	externalID := strings.TrimSpace(entry.ExternalID)
	if externalID == "" {
		return false
	}
	return externalID == strings.TrimSpace(extPos.PositionID)
}

func recoveredRiskFromRaw(raw []byte) (recoveredRiskPlan, error) {
	plan, err := position.DecodeRiskPlan(raw)
	if err != nil {
		return recoveredRiskPlan{}, err
	}
	if !riskPlanHasControls(plan) {
		return recoveredRiskPlan{}, errRecoveredRiskPlanNoControls
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return recoveredRiskPlan{Raw: out, Plan: plan, OK: true}, nil
}

func recoveredRiskFromPlan(plan risk.RiskPlan) (recoveredRiskPlan, error) {
	if !riskPlanHasControls(plan) {
		return recoveredRiskPlan{}, errRecoveredRiskPlanNoControls
	}
	raw, err := json.Marshal(risk.CompactRiskPlan(plan))
	if err != nil {
		return recoveredRiskPlan{}, err
	}
	return recoveredRiskPlan{Raw: raw, Plan: plan, OK: true}, nil
}

func riskPlanHasControls(plan risk.RiskPlan) bool {
	if plan.StopPrice > 0 {
		return true
	}
	for _, level := range plan.TPLevels {
		if level.Price > 0 && level.QtyPct > 0 {
			return true
		}
	}
	return false
}

func (s *RecoveryService) saveRecoveredRiskPlanHistory(ctx context.Context, positionID string, recovered recoveredRiskPlan, source string) error {
	if s == nil || s.RiskPlans == nil || !recovered.OK {
		return nil
	}
	if _, err := s.RiskPlans.SaveHistory(ctx, positionID, recovered.Plan, source); err != nil {
		return err
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
