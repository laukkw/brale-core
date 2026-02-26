package decision

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/prompt/positionprompt"

	"go.uber.org/zap"
)

func (p *Pipeline) RunOnceObserveWithResults(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams) ([]SymbolResult, error) {
	logger := logging.FromContext(ctx).Named("pipeline")
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
	for symbol, ctxInfo := range decisionCtx {
		modeBySymbol[symbol] = ctxInfo.Mode
	}
	return p.runObserveWithDecisionCtx(ctx, symbols, intervals, limit, acct, risk, decisionCtx, modeBySymbol)
}

func (p *Pipeline) RunOnceObserveAsFlat(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams) ([]SymbolResult, error) {
	logger := logging.FromContext(ctx).Named("pipeline")
	if err := p.validate(); err != nil {
		logger.Error("pipeline validate failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	decisionCtx := make(map[string]symbolDecisionContext, len(symbols))
	modeBySymbol := make(map[string]decisionmode.Mode, len(symbols))
	for _, symbol := range symbols {
		decisionCtx[symbol] = symbolDecisionContext{Mode: decisionmode.ModeFlat}
		modeBySymbol[symbol] = decisionmode.ModeFlat
	}
	return p.runObserveWithDecisionCtx(ctx, symbols, intervals, limit, acct, risk, decisionCtx, modeBySymbol)
}

func (p *Pipeline) runObserveWithDecisionCtx(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams, decisionCtx map[string]symbolDecisionContext, modeBySymbol map[string]decisionmode.Mode) ([]SymbolResult, error) {
	logger := logging.FromContext(ctx).Named("pipeline")
	start := time.Now()
	logger.Debug("pipeline observe start",
		zap.Int("symbols", len(symbols)),
		zap.Int("intervals", len(intervals)),
		zap.Int("limit", limit),
	)
	if err := p.validate(); err != nil {
		logger.Error("pipeline validate failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	if decisionCtx == nil {
		decisionCtx = make(map[string]symbolDecisionContext)
	}
	results, snap, comp, err := p.Runner.RunOnceWithOptions(ctx, symbols, intervals, limit, acct, risk, RunOptions{BuildPlan: true, ModeBySymbol: modeBySymbol})
	if err != nil {
		logger.Error("pipeline runner failed", zap.Error(err))
		p.notifyError(ctx, err)
		return nil, err
	}
	for i := range results {
		ctxInfo, ok := decisionCtx[results[i].Symbol]
		state := fsm.PositionState("")
		posID := ""
		if ok {
			posID = ctxInfo.PositionID
			if ctxInfo.Mode == decisionmode.ModeInPosition {
				state = fsm.StateInPosition
			} else {
				state = fsm.StateFlat
			}
		} else {
			var err error
			state, posID, err = p.loadState(ctx, results[i].Symbol)
			if err != nil {
				logger.Error("load state failed", zap.Error(err), zap.String("symbol", results[i].Symbol))
				p.notifyError(ctx, err)
				return nil, err
			}
			ctxInfo = symbolDecisionContext{
				Mode:       decisionmode.Resolve(state == fsm.StateInPosition),
				PositionID: posID,
			}
			decisionCtx[results[i].Symbol] = ctxInfo
		}
		if state != fsm.StateInPosition {
			p.applyReportMarkPrice(ctx, &results[i])
			continue
		}
		holdGate, indHold, stHold, mechHold, prompts, evaluated, err := p.buildHoldGate(ctx, results[i].Symbol, results[i], comp, posID)
		if err != nil {
			logger.Error("hold gate build failed", zap.Error(err), zap.String("symbol", results[i].Symbol))
			p.notifyError(ctx, err)
			return nil, err
		}
		results[i].Gate = holdGate.Gate
		results[i].RuleflowResult = &holdGate
		results[i].FSMNext = holdGate.FSMNext
		results[i].FSMActions = holdGate.FSMActions
		results[i].FSMRuleHit = holdGate.FSMRuleHit
		results[i].InPositionIndicator = indHold
		results[i].InPositionStructure = stHold
		results[i].InPositionMechanics = mechHold
		results[i].InPositionPrompts = prompts
		results[i].InPositionEvaluated = evaluated
		p.applyReportMarkPrice(ctx, &results[i])
	}
	_ = snap
	_ = comp
	logger.Debug("pipeline observe complete",
		zap.Int("results", len(results)),
		zap.Duration("latency", time.Since(start)),
	)
	return results, nil
}

func (p *Pipeline) RunOnceObserveWithInjectedPosition(ctx context.Context, symbol string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams, pos positionprompt.Summary) (SymbolResult, error) {
	logger := logging.FromContext(ctx).Named("pipeline").With(zap.String("symbol", symbol))
	if symbol == "" {
		err := fmt.Errorf("symbol is required")
		logger.Error("inject observe failed", zap.Error(err))
		p.notifyError(ctx, err)
		return SymbolResult{}, err
	}
	if len(intervals) == 0 {
		err := fmt.Errorf("intervals is required")
		logger.Error("inject observe failed", zap.Error(err))
		p.notifyError(ctx, err)
		return SymbolResult{}, err
	}

	if p.Runner == nil || p.Runner.Provider == nil {
		err := fmt.Errorf("provider is required")
		logger.Error("inject observe failed", zap.Error(err))
		p.notifyError(ctx, err)
		return SymbolResult{}, err
	}
	opts := RunOptions{BuildPlan: true, ModeBySymbol: map[string]decisionmode.Mode{symbol: decisionmode.ModeInPosition}}
	results, _, comp, err := p.Runner.RunOnceWithOptions(ctx, []string{symbol}, intervals, limit, acct, risk, opts)
	if err != nil {
		logger.Error("pipeline runner failed", zap.Error(err))
		p.notifyError(ctx, err)
		return SymbolResult{}, err
	}
	if len(results) == 0 {
		err := fmt.Errorf("symbol result is empty")
		logger.Error("inject observe failed", zap.Error(err))
		p.notifyError(ctx, err)
		return SymbolResult{}, err
	}
	res := results[0]
	if res.Err != nil {
		logger.Error("symbol result error", zap.Error(res.Err))
		p.notifyError(ctx, res.Err)
		return res, res.Err
	}
	indOut, stOut, mechOut, prompts, evaluated, err := p.judgeInPositionWithFallback(ctx, symbol, res, pos)
	if err != nil {
		logger.Error("provider judge failed", zap.Error(err))
		p.notifyError(ctx, err)
		return res, err
	}
	gateDecision, err := p.evaluateRuleflowHoldGate(ctx, res.Symbol, res, indOut, stOut, mechOut, comp, "", evaluated, ruleflow.HardGuardPosition{})
	if err != nil {
		return res, err
	}
	res.Gate = gateDecision.Gate
	res.RuleflowResult = &gateDecision
	res.InPositionIndicator = indOut
	res.InPositionStructure = stOut
	res.InPositionMechanics = mechOut
	res.InPositionPrompts = prompts
	res.InPositionEvaluated = evaluated
	p.applyReportMarkPrice(ctx, &res)
	return res, nil
}

func (p *Pipeline) resolveDecisionContexts(ctx context.Context, symbols []string) (map[string]symbolDecisionContext, error) {
	contexts := make(map[string]symbolDecisionContext, len(symbols))
	for _, symbol := range symbols {
		state, posID, err := p.loadState(ctx, symbol)
		if err != nil {
			return nil, err
		}
		contexts[symbol] = symbolDecisionContext{
			Mode:       decisionmode.Resolve(state == fsm.StateInPosition),
			PositionID: posID,
		}
	}
	return contexts, nil
}
