package decision

import (
	"context"
	"strings"
	"time"

	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"

	"go.uber.org/zap"
)

const (
	defaultEntryCooldownRoundsAfterExit = 2
	gateReasonEntryCooldownActive       = "ENTRY_COOLDOWN_ACTIVE"
)

func (p *Pipeline) runOnceWithOptions(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams, opts RunOptions) ([]PersistResult, error) {
	logger := logging.FromContext(ctx).Named("pipeline")
	start := time.Now()
	logger.Debug("pipeline run start",
		zap.Int("symbols", len(symbols)),
		zap.Int("intervals", len(intervals)),
		zap.Int("limit", limit),
	)
	if err := p.validate(); err != nil {
		logger.Error("pipeline validate failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	decisionCtx, err := p.resolveDecisionContexts(ctx, symbols)
	if err != nil {
		logger.Error("resolve decision context failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	modeBySymbol := make(map[string]decisionmode.Mode, len(decisionCtx))
	runnableSymbols := make([]string, 0, len(symbols))
	cooldownSkipped := make(map[string]PersistResult)
	for _, symbol := range symbols {
		ctxInfo, ok := decisionCtx[symbol]
		if !ok {
			continue
		}
		modeBySymbol[symbol] = ctxInfo.Mode
		if p.shouldSkipSymbolByEntryCooldown(symbol, ctxInfo.Mode, logger) {
			cooldownSkipped[symbol] = PersistResult{Symbol: symbol, Gate: gateReasonEntryCooldownActive}
			continue
		}
		runnableSymbols = append(runnableSymbols, symbol)
	}
	if len(runnableSymbols) == 0 {
		out := make([]PersistResult, 0, len(symbols))
		for _, symbol := range symbols {
			if skipped, ok := cooldownSkipped[symbol]; ok {
				out = append(out, skipped)
			}
		}
		logger.Debug("pipeline run complete",
			zap.Int("results", len(out)),
			zap.Duration("latency", time.Since(start)),
		)
		return out, nil
	}
	runOpts := opts
	runOpts.ModeBySymbol = modeBySymbol
	results, snap, comp, err := p.Runner.RunOnceWithOptions(ctx, runnableSymbols, intervals, limit, acct, risk, runOpts)
	if err != nil {
		logger.Error("pipeline runner failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	outBySymbol := make(map[string]PersistResult, len(results)+len(cooldownSkipped))
	for symbol, skipped := range cooldownSkipped {
		outBySymbol[symbol] = skipped
	}
	for i := range results {
		res := &results[i]
		ctxInfo, ok := decisionCtx[res.Symbol]
		state := fsm.PositionState("")
		posID := ""
		if ok {
			posID = ctxInfo.PositionID
			if ctxInfo.Mode == decisionmode.ModeInPosition {
				state = fsm.StateInPosition
			} else {
				state = fsm.StateFlat
			}
		}
		snapID := resolveSnapshotID(snap)
		if state != fsm.StateInPosition {
			p.applyReportMarkPrice(ctx, res)
		}
		pr, err := p.handleSymbol(ctx, *res, snapID, snap, comp, state, posID)
		if err != nil {
			logger.Error("persist error", zap.Error(err), zap.String("symbol", res.Symbol))
			return nil, err
		}
		outBySymbol[res.Symbol] = pr
	}
	out := make([]PersistResult, 0, len(outBySymbol))
	for _, symbol := range symbols {
		if pr, ok := outBySymbol[symbol]; ok {
			out = append(out, pr)
		}
	}
	logger.Debug("pipeline run complete",
		zap.Int("results", len(out)),
		zap.Duration("latency", time.Since(start)),
	)
	return out, nil
}

func (p *Pipeline) shouldSkipSymbolByEntryCooldown(symbol string, mode decisionmode.Mode, logger *zap.Logger) bool {
	if p == nil || p.EntryCooldownCache == nil {
		return false
	}
	if mode == decisionmode.ModeInPosition {
		return false
	}
	remaining, active := p.EntryCooldownCache.Consume(symbol)
	if !active {
		return false
	}
	if logger != nil {
		logger.Info("entry cooldown active, skip symbol decision",
			zap.String("symbol", symbol),
			zap.Int("remaining_rounds", remaining),
		)
	}
	return true
}

func (p *Pipeline) armEntryCooldownOnExitSignal(res SymbolResult, logger *zap.Logger) {
	if p == nil || p.EntryCooldownCache == nil {
		return
	}
	if !res.InPositionEvaluated {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(res.Gate.DecisionAction), "EXIT") {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(res.Gate.GateReason), "REVERSAL_CONFIRMED") {
		return
	}
	symbol := strings.TrimSpace(res.Symbol)
	if symbol == "" {
		return
	}
	rounds := p.EntryCooldownRounds
	if rounds <= 0 {
		rounds = defaultEntryCooldownRoundsAfterExit
	}
	p.EntryCooldownCache.Arm(symbol, rounds)
	if logger != nil {
		logger.Info("entry cooldown armed after in-position exit",
			zap.String("symbol", symbol),
			zap.Int("rounds", rounds),
		)
	}
}

func (p *Pipeline) handleSymbol(ctx context.Context, res SymbolResult, snapID uint, snap snapshot.MarketSnapshot, comp features.CompressionResult, state fsm.PositionState, posID string) (PersistResult, error) {
	logger := logging.FromContext(ctx).Named("pipeline").With(zap.String("symbol", res.Symbol))
	out := PersistResult{Symbol: res.Symbol, Gate: res.Gate.GateReason}
	if err := p.handleSymbolError(ctx, logger, res, snapID, snap); err != nil {
		out.Err = err
		return out, err
	}
	state, posID, err := p.resolveState(ctx, res.Symbol, state, posID, logger)
	if err != nil {
		out.Err = err
		return out, err
	}
	if state == fsm.StateInPosition {
		return p.handleInPosition(ctx, logger, out, res, snapID, snap, comp, posID)
	}
	if err := p.persistSymbolStores(ctx, snapID, snap, res, logger); err != nil {
		return out, err
	}
	if !res.Gate.GlobalTradeable || res.Plan == nil {
		logger.Info("gate blocked trade", zap.String("gate_reason", res.Gate.GateReason))
		return out, nil
	}
	if !res.Plan.Valid {
		logger.Info("plan invalid", zap.String("reason", res.Plan.InvalidReason))
		return out, nil
	}
	logger.Info("gate allowed trade",
		zap.String("symbol", res.Symbol),
		zap.String("gate_reason", res.Gate.GateReason),
		zap.String("decision_action", res.Gate.DecisionAction),
	)
	fsmNext, fsmActions, fsmHit, err := p.evaluateFSM(ctx, res, res.Gate, res.Plan, state, posID, logger)
	out.NextState = fsmNext
	if err != nil {
		logger.Error("fsm eval failed", zap.Error(err))
		p.notifyError(ctx, err)
		return out, nil
	}
	if fsmHit.Name != "" {
		logger.Debug("fsm rule hit", zap.String("rule", fsmHit.Name))
	}
	if !hasFSMAction(fsmActions, fsm.ActionOpen) {
		logger.Info("fsm blocked open")
		return out, nil
	}
	return p.handlePlan(ctx, out, res, posID, state)
}

func (p *Pipeline) handleSymbolError(ctx context.Context, logger *zap.Logger, res SymbolResult, snapID uint, snap snapshot.MarketSnapshot) error {
	if res.Err == nil {
		return nil
	}
	logger.Error("symbol result error", zap.Error(res.Err))
	p.notifyError(ctx, res.Err)
	if p.GateStore == nil {
		return res.Err
	}
	gate := res.Gate
	if gate.GateReason == "" {
		gate.GateReason = "GATE_ERROR"
	}
	if gate.DecisionAction == "" {
		gate.DecisionAction = "VETO"
	}
	if err := p.GateStore(ctx, snap, snapID, res.Symbol, gate, res.Providers); err != nil {
		logger.Error("gate store failed on error", zap.Error(err))
	}
	return res.Err
}

func (p *Pipeline) resolveState(ctx context.Context, symbol string, state fsm.PositionState, posID string, logger *zap.Logger) (fsm.PositionState, string, error) {
	if state != "" {
		return state, posID, nil
	}
	resolved, resolvedPos, err := p.loadState(ctx, symbol)
	if err != nil {
		logger.Error("load state failed", zap.Error(err))
		p.notifyError(ctx, err)
		return "", "", err
	}
	return resolved, resolvedPos, nil
}
