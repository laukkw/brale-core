package decision

import (
	"context"
	"strings"
	"time"

	braleOtel "brale-core/internal/otel"

	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/llm/llmround"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/snapshot"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
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
	roundID, err := p.newRoundID()
	if err != nil {
		logger.Error("pipeline round init failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	ctx = llm.WithSessionRoundID(ctx, roundID)
	logger = logger.With(zap.String("round_id", roundID.String()))
	roundOutcome := "ok"
	var recorder *llmround.Recorder
	decisionCtx, err := p.resolveDecisionContexts(ctx, symbols)
	if err != nil {
		logger.Error("resolve decision context failed", zap.Error(err))
		p.notifyError(ctx, err)
		roundOutcome = "context_error"
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
		normalized := decisionutil.NormalizeSymbol(symbol)
		if normalized != "" && normalized != symbol {
			modeBySymbol[normalized] = ctxInfo.Mode
		}
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
	ctx, recorder = p.attachRoundRecorder(ctx, roundID, "decide", runnableSymbols)
	defer func() {
		if recorder == nil {
			return
		}
		if err := recorder.Finish(ctx, roundOutcome); err != nil {
			logger.Warn("save llm round failed", zap.Error(err))
		}
	}()
	runOpts := opts
	runOpts = p.enrichRunOptions(runOpts, runnableSymbols, modeBySymbol)
	results, snap, comp, err := p.Runner.RunOnceWithOptions(ctx, runnableSymbols, intervals, limit, acct, risk, runOpts)
	if err != nil {
		logger.Error("pipeline runner failed", zap.Error(err))
		p.notifyError(ctx, err)
		roundOutcome = "runner_error"
		braleOtel.PipelineErrorsTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("phase", "runner")))
		return nil, err
	}
	snapID := resolveSnapshotID(snap)
	applyRoundSummary(recorder, snapID, results)
	if failed, ok := firstFailedSymbolResult(results); ok {
		if err := p.handleSymbolError(ctx, logger.With(zap.String("symbol", failed.Symbol)), failed); err != nil {
			roundOutcome = "symbol_error"
			return nil, err
		}
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
		if state != fsm.StateInPosition {
			p.applyReportMarkPrice(ctx, res)
		}
		pr, err := p.handleSymbol(ctx, *res, snapID, snap, comp, state, posID)
		if err != nil {
			logger.Error("persist error", zap.Error(err), zap.String("symbol", res.Symbol))
			roundOutcome = "persist_error"
			braleOtel.PipelineErrorsTotal.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("phase", "persist")))
			return nil, err
		}
		outBySymbol[res.Symbol] = pr
	}
	out := make([]PersistResult, 0, len(outBySymbol))
	for _, symbol := range symbols {
		if pr, ok := outBySymbol[symbol]; ok {
			out = append(out, pr)
			continue
		}
		if pr, ok := outBySymbol[decisionutil.NormalizeSymbol(symbol)]; ok {
			out = append(out, pr)
		}
	}
	logger.Debug("pipeline run complete",
		zap.Int("results", len(out)),
		zap.Duration("latency", time.Since(start)),
	)
	applyRoundSummary(recorder, snapID, results)

	// Record pipeline metrics
	latencyMs := time.Since(start).Milliseconds()
	attrs := otelmetric.WithAttributes(attribute.String("outcome", roundOutcome))
	braleOtel.PipelineRoundsTotal.Add(ctx, 1, attrs)
	braleOtel.PipelineLatencyMs.Record(ctx, latencyMs, attrs)
	if recorder != nil {
		braleOtel.PipelineTokensTotal.Add(ctx, int64(recorder.TotalTokenIn()+recorder.TotalTokenOut()), attrs)
	}
	for _, res := range results {
		action := strings.TrimSpace(res.Gate.DecisionAction)
		if action != "" {
			braleOtel.PipelineGateDecisions.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("action", action)))
		}
	}
	return out, nil
}

func firstFailedSymbolResult(results []SymbolResult) (SymbolResult, bool) {
	for _, res := range results {
		if res.Err != nil {
			return res, true
		}
	}
	return SymbolResult{}, false
}

func (p *Pipeline) enrichRunOptions(opts RunOptions, symbols []string, modeBySymbol map[string]decisionmode.Mode) RunOptions {
	runOpts := opts
	runOpts.ModeBySymbol = modeBySymbol

	existing := runOpts.RiskStrategyModeBySymbol
	riskModeBySymbol := make(map[string]string, len(symbols))
	for symbol, mode := range existing {
		riskModeBySymbol[symbol] = mode
	}

	for _, symbol := range symbols {
		if _, ok := riskModeBySymbol[symbol]; ok {
			continue
		}
		bind, err := p.getBinding(symbol)
		if err != nil {
			continue
		}
		riskModeBySymbol[symbol] = resolveTightenPlanSource(bind)
		normalized := decisionutil.NormalizeSymbol(symbol)
		if normalized != "" && normalized != symbol {
			riskModeBySymbol[normalized] = riskModeBySymbol[symbol]
		}
	}
	runOpts.RiskStrategyModeBySymbol = riskModeBySymbol
	return runOpts
}

func (p *Pipeline) newRoundID() (llm.RoundID, error) {
	if p != nil && p.RoundIDFactory != nil {
		return p.RoundIDFactory()
	}
	return llm.NewRoundID(uuid.NewString())
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
	recordMemory := p != nil && p.WorkingMemory != nil
	defer func() {
		if recordMemory {
			p.recordWorkingMemory(ctx, res, comp)
		}
	}()
	if err := p.handleSymbolError(ctx, logger, res); err != nil {
		recordMemory = false
		out.Err = err
		return out, err
	}
	state, posID, err := p.resolveState(ctx, res.Symbol, state, posID, logger)
	if err != nil {
		recordMemory = false
		out.Err = err
		return out, err
	}
	if state == fsm.StateInPosition {
		recordMemory = false
		return p.handleInPosition(ctx, logger, out, res, snapID, snap, comp, posID)
	}
	if err := p.persistSymbolStores(ctx, snapID, snap, res, logger); err != nil {
		recordMemory = false
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
		out.Err = err
		return out, err
	}
	if fsmHit.Name != "" {
		logger.Debug("fsm rule hit", zap.String("rule", fsmHit.Name))
	}
	if !hasFSMAction(fsmActions, fsm.ActionOpen) {
		logger.Info("fsm blocked open")
		return out, nil
	}
	finalOut, err := p.handlePlan(ctx, out, res, posID, state)
	if err != nil {
		recordMemory = false
	}
	return finalOut, err
}

func (p *Pipeline) handleSymbolError(ctx context.Context, logger *zap.Logger, res SymbolResult) error {
	if res.Err == nil {
		return nil
	}
	logger.Error("symbol result error", zap.Error(res.Err))
	p.notifyError(ctx, res.Err)
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
