package decision

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
	"brale-core/internal/pkg/errclass"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"

	"go.uber.org/zap"
)

func (p *Pipeline) handlePlan(ctx context.Context, out PersistResult, res SymbolResult, posID string, state fsm.PositionState) (PersistResult, error) {
	planCtx, planLogger, err := p.preparePlanContext(ctx, res.Plan, posID, state)
	if err != nil {
		out.Err = err
		return out, err
	}
	valid := res.Plan != nil && res.Plan.Valid
	result := p.planCache().UpsertIfAllow(res.Symbol, *res.Plan, res.Gate.DecisionAction, valid)
	if !result.Replaced {
		planLogger.Info("plan not stored",
			zap.String("reason", result.Reason),
		)
		return out, nil
	}
	planLogger.Info("plan cached",
		zap.String("position_id", res.Plan.PositionID),
		zap.Time("expires_at", res.Plan.ExpiresAt),
	)
	if result.PreviousEntry != nil {
		reason := "plan_replaced"
		if _, err := p.positioner().CancelOpenByEntry(planCtx, *result.PreviousEntry, reason); err != nil {
			planLogger.Error("open cancel failed", zap.Error(err), zap.String("reason", reason))
		} else {
			planLogger.Info("open cancel submitted", zap.String("reason", reason))
		}
	}
	return out, nil
}

func (p *Pipeline) persistSymbolStores(ctx context.Context, snapID uint, snap snapshot.MarketSnapshot, res SymbolResult, logger *zap.Logger) error {
	return p.persistStores(ctx, snapID, snap, res, logger, func() error {
		if p.ProviderStore == nil {
			return nil
		}
		return p.ProviderStore(ctx, snap, snapID, res.Symbol, res.Providers, BuildProviderDataContext(res.AgentInputs), res.ProviderPrompts)
	}, "provider store failed")
}

func (p *Pipeline) persistInPositionStores(ctx context.Context, snapID uint, snap snapshot.MarketSnapshot, res SymbolResult, ind provider.InPositionIndicatorOut, st provider.InPositionStructureOut, mech provider.InPositionMechanicsOut, prompts ProviderPromptSet, logger *zap.Logger) error {
	return p.persistStores(ctx, snapID, snap, res, logger, func() error {
		if p.ProviderInPositionStore == nil {
			return nil
		}
		return p.ProviderInPositionStore(ctx, snap, snapID, res.Symbol, ind, st, mech, prompts, res.EnabledAgents)
	}, "provider in position store failed")
}

func (p *Pipeline) persistStores(ctx context.Context, snapID uint, snap snapshot.MarketSnapshot, res SymbolResult, logger *zap.Logger, providerStore func() error, providerErrMsg string) error {
	if err := p.runStore(ctx, logger, "agent store failed", func() error {
		if p.AgentStore == nil {
			return nil
		}
		return p.AgentStore(ctx, snap, snapID, res.Symbol, res.AgentIndicator, res.AgentStructure, res.AgentMechanics, res.AgentInputs, res.EnabledAgents, res.AgentPrompts)
	}); err != nil {
		return err
	}
	if err := p.runStore(ctx, logger, providerErrMsg, providerStore); err != nil {
		return err
	}
	if err := p.runStore(ctx, logger, "gate store failed", func() error {
		if p.GateStore == nil {
			return nil
		}
		return p.GateStore(ctx, snap, snapID, res.Symbol, res.Gate, res.Providers)
	}); err != nil {
		return err
	}
	return nil
}

func (p *Pipeline) runStore(ctx context.Context, logger *zap.Logger, msg string, fn func() error) error {
	if fn == nil {
		return nil
	}
	if err := fn(); err != nil {
		logger.Error(msg, zap.Error(err))
		p.notifyError(ctx, err)
		return err
	}
	return nil
}

func resolveSnapshotID(snap snapshot.MarketSnapshot) uint {
	snapID := uint(snap.Timestamp.Unix())
	if snapID == 0 {
		snapID = uint(time.Now().Unix())
	}
	return snapID
}

func (p *Pipeline) preparePlanContext(ctx context.Context, plan *execution.ExecutionPlan, posID string, state fsm.PositionState) (context.Context, *zap.Logger, error) {
	if plan == nil {
		err := fmt.Errorf("plan is required")
		p.notifyError(ctx, err)
		return ctx, logging.FromContext(ctx).Named("pipeline"), err
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now().UTC()
	}
	if plan.ExpiresAt.IsZero() {
		plan.ExpiresAt = plan.CreatedAt.Add(p.BarInterval)
	}
	if posID != "" && state != fsm.StateFlat {
		plan.PositionID = posID
	}
	if plan.PositionID == "" {
		plan.PositionID = posID
	}
	if plan.PositionID == "" {
		plan.PositionID = fmt.Sprintf("%s-%d", plan.Symbol, time.Now().UnixNano())
	}
	ctx = logging.With(ctx, zap.String("position_id", plan.PositionID))
	logger := logging.FromContext(ctx).Named("pipeline")
	if err := validatePlan(*plan); err != nil {
		logger.Error("plan validation failed", zap.Error(err))
		p.notifyError(ctx, err)
		return ctx, logger, err
	}
	logger.Info("plan ready",
		zap.String("symbol", plan.Symbol),
		zap.Bool("valid", plan.Valid),
		zap.String("invalid_reason", plan.InvalidReason),
		zap.String("direction", plan.Direction),
		zap.Float64("entry", plan.Entry),
		zap.Float64("stop_loss", plan.StopLoss),
		zap.Float64s("take_profits", plan.TakeProfits),
		zap.Float64s("take_profit_ratios", plan.TakeProfitRatios),
		zap.Float64("risk_pct", plan.RiskPct),
		zap.Float64("risk_distance", plan.RiskAnnotations.RiskDistance),
		zap.Float64("position_size", plan.PositionSize),
		zap.Float64("position_value", plan.PositionSize*plan.Entry),
		zap.Float64("leverage", plan.Leverage),
		zap.Float64("r_multiple", plan.RMultiple),
		zap.String("template", plan.Template),
		zap.String("strategy_id", plan.StrategyID),
		zap.String("system_config_hash", plan.SystemConfigHash),
		zap.String("strategy_config_hash", plan.StrategyConfigHash),
		zap.String("stop_source", plan.RiskAnnotations.StopSource),
		zap.String("stop_reason", plan.RiskAnnotations.StopReason),
		zap.Float64("atr", plan.RiskAnnotations.ATR),
		zap.Float64("buffer_atr", plan.RiskAnnotations.BufferATR),
		zap.Time("created_at", plan.CreatedAt),
		zap.Time("expires_at", plan.ExpiresAt),
	)
	return ctx, logger, nil
}

const planValidationScope errclass.Scope = "decision"
const planValidationReason = "invalid_plan"

func validatePlan(plan execution.ExecutionPlan) error {
	if plan.PositionID == "" {
		return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan position_id is required")
	}
	if plan.CreatedAt.IsZero() {
		return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan created_at is required")
	}
	if plan.Valid {
		if plan.Entry <= 0 {
			return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan entry must be > 0")
		}
		if plan.StopLoss <= 0 {
			return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan stop_loss must be > 0")
		}
		if len(plan.TakeProfits) == 0 {
			return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan take_profits is required")
		}
		if len(plan.TakeProfitRatios) == 0 {
			return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan take_profit_ratios is required")
		}
		if len(plan.TakeProfits) != len(plan.TakeProfitRatios) {
			return errclass.ValidationErrorf(planValidationScope, planValidationReason, "plan take_profits and take_profit_ratios length mismatch")
		}
	}
	return nil
}
