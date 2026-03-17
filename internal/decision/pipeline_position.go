package decision

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/decision/features"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/decision/ruleflow"
	"brale-core/internal/execution"
	"brale-core/internal/position"
	"brale-core/internal/prompt/positionprompt"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"

	"go.uber.org/zap"
)

func (p *Pipeline) handleInPosition(ctx context.Context, logger *zap.Logger, out PersistResult, res SymbolResult, snapID uint, snap snapshot.MarketSnapshot, comp features.CompressionResult, posID string) (PersistResult, error) {
	rfResult, indHold, stHold, mechHold, prompts, evaluated, err := p.resolveHoldGate(ctx, res, comp, posID, logger)
	if err != nil {
		out.Err = err
		return out, err
	}
	res.Gate = rfResult.Gate
	res.RuleflowResult = &rfResult
	res.FSMNext = rfResult.FSMNext
	res.FSMActions = rfResult.FSMActions
	res.FSMRuleHit = rfResult.FSMRuleHit
	res.InPositionIndicator = indHold
	res.InPositionStructure = stHold
	res.InPositionMechanics = mechHold
	res.InPositionPrompts = prompts
	res.InPositionEvaluated = evaluated
	p.applyReportMarkPrice(ctx, &res)
	execResult, err := p.applyRiskPlanUpdate(ctx, res, comp, posID)
	if err != nil {
		logger.Error("risk plan update failed", zap.Error(err))
		p.notifyError(ctx, err)
	}
	applyTightenExecutionDerived(&res, execResult)
	p.applyExitConfirmFromTighten(&res, posID, execResult, logger)
	out.Gate = res.Gate.GateReason
	if err := p.persistInPositionStores(ctx, snapID, snap, res, indHold, stHold, mechHold, prompts, logger); err != nil {
		return out, err
	}
	fsmNext, fsmActions, fsmHit, err := p.evaluateFSM(ctx, res, res.Gate, nil, fsm.StateInPosition, posID, logger)
	out.NextState = fsmNext
	if err != nil {
		logger.Error("fsm eval failed", zap.Error(err))
		p.notifyError(ctx, err)
		return out, nil
	}
	if fsmHit.Name != "" {
		logger.Debug("fsm rule hit", zap.String("rule", fsmHit.Name))
	}
	if hasFSMAction(fsmActions, fsm.ActionClose) {
		p.armEntryCooldownOnExitSignal(res, logger)
		return out, nil
	}
	if hasFSMAction(fsmActions, fsm.ActionReduce) {
		return out, nil
	}
	logger.Info("fsm hold")
	return out, nil
}

func (p *Pipeline) applyExitConfirmFromTighten(res *SymbolResult, posID string, exec tightenExecution, logger *zap.Logger) {
	if p == nil || res == nil || !exec.ExitConfirmHit {
		return
	}
	count := 0
	if p.ExitConfirmCache != nil {
		count = p.ExitConfirmCache.Get(res.Symbol, fsm.StateInPosition, posID)
	}
	count++
	if p.ExitConfirmCache != nil {
		p.ExitConfirmCache.Set(res.Symbol, fsm.StateInPosition, posID, count)
	}
	if res.Gate.Derived == nil {
		res.Gate.Derived = map[string]any{}
	}
	res.Gate.Derived["tighten_exit_confirm_requested"] = true
	res.Gate.Derived["tighten_exit_confirm_count"] = count
	res.Gate.GlobalTradeable = false
	res.FSMNext = fsm.StateInPosition
	if count < 2 {
		res.Gate.DecisionAction = "TIGHTEN"
		res.Gate.GateReason = "EXIT_CONFIRM_PENDING"
		res.Gate.RuleHit = &fund.GateRuleHit{
			Name:     "EXIT_CONFIRM_PENDING",
			Priority: 2,
			Action:   "TIGHTEN",
			Reason:   "EXIT_CONFIRM_PENDING",
			Default:  false,
		}
		res.FSMActions = []fsm.Action{{Type: fsm.ActionReduce, Reason: "GATE_TIGHTEN"}}
		res.FSMRuleHit = fsm.RuleHit{
			Name:     string(fsm.ActionReduce),
			Priority: 1,
			Action:   string(fsm.ActionReduce),
			Reason:   "GATE_TIGHTEN",
			Next:     string(fsm.StateInPosition),
			Default:  false,
		}
	} else {
		res.Gate.DecisionAction = "EXIT"
		res.Gate.GateReason = "REVERSAL_CONFIRMED"
		res.Gate.RuleHit = &fund.GateRuleHit{
			Name:     "REVERSAL_CONFIRMED",
			Priority: 5,
			Action:   "EXIT",
			Reason:   "REVERSAL_CONFIRMED",
			Default:  false,
		}
		res.FSMActions = []fsm.Action{{Type: fsm.ActionClose, Reason: "GATE_EXIT"}}
		res.FSMRuleHit = fsm.RuleHit{
			Name:     string(fsm.ActionClose),
			Priority: 1,
			Action:   string(fsm.ActionClose),
			Reason:   "GATE_EXIT",
			Next:     string(fsm.StateInPosition),
			Default:  false,
		}
	}
	if res.RuleflowResult != nil {
		res.RuleflowResult.Gate = res.Gate
		res.RuleflowResult.FSMNext = res.FSMNext
		res.RuleflowResult.FSMActions = res.FSMActions
		res.RuleflowResult.FSMRuleHit = res.FSMRuleHit
		res.RuleflowResult.ExitConfirmCount = count
	}
	if logger != nil {
		logger.Info("tighten invalid -> exit confirm updated",
			zap.Int("exit_confirm_count", count),
			zap.String("gate_reason", res.Gate.GateReason),
			zap.String("decision_action", res.Gate.DecisionAction),
		)
	}
}

func (p *Pipeline) resolveHoldGate(ctx context.Context, res SymbolResult, comp features.CompressionResult, posID string, logger *zap.Logger) (ruleflow.Result, provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, bool, error) {
	indHold := res.InPositionIndicator
	stHold := res.InPositionStructure
	mechHold := res.InPositionMechanics
	prompts := res.InPositionPrompts
	if res.InPositionEvaluated && res.RuleflowResult != nil {
		return *res.RuleflowResult, indHold, stHold, mechHold, prompts, true, nil
	}
	rfResult, indOut, stOut, mechOut, promptSet, evaluated, err := p.buildHoldGate(ctx, res.Symbol, res, comp, posID)
	if err != nil {
		logger.Error("hold gate build failed", zap.Error(err))
		p.notifyError(ctx, err)
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, err
	}
	return rfResult, indOut, stOut, mechOut, promptSet, evaluated, nil
}

func (p *Pipeline) buildHoldGate(ctx context.Context, symbol string, res SymbolResult, comp features.CompressionResult, posID string) (ruleflow.Result, provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, bool, error) {
	if p == nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, fmt.Errorf("pipeline is required")
	}
	if p.PriceSource == nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, fmt.Errorf("price_source is required")
	}
	if p.BarInterval <= 0 {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, fmt.Errorf("bar_interval is required")
	}
	if p.Runner == nil || p.Runner.Provider == nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, fmt.Errorf("provider is required")
	}
	pos, err := p.loadPositionRecord(ctx, symbol, posID)
	if err != nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, err
	}
	if p.Positioner != nil && p.Positioner.Cache != nil {
		pos = p.Positioner.Cache.HydratePosition(pos)
	}
	promptSummary, err := p.buildInPositionPromptSummary(ctx, pos)
	if err != nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, err
	}
	indOut, stOut, mechOut, prompts, evaluated, err := p.judgeInPositionWithFallback(ctx, symbol, res, promptSummary)
	if err != nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, err
	}
	hardGuardInput := p.buildHardGuardPosition(ctx, symbol, pos)
	rfResult, err := p.evaluateRuleflowHoldGate(ctx, res.Symbol, res, indOut, stOut, mechOut, comp, posID, evaluated, hardGuardInput)
	if err != nil {
		return ruleflow.Result{}, provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, err
	}
	return rfResult, indOut, stOut, mechOut, prompts, evaluated, nil
}

func (p *Pipeline) evaluateRuleflowHoldGate(ctx context.Context, symbol string, res SymbolResult, ind provider.InPositionIndicatorOut, st provider.InPositionStructureOut, mech provider.InPositionMechanicsOut, comp features.CompressionResult, posID string, evaluated bool, hardGuardInput ruleflow.HardGuardPosition) (ruleflow.Result, error) {
	bind, err := p.getBinding(symbol)
	if err != nil {
		return ruleflow.Result{}, err
	}
	rfEngine := p.Runner.ensureRuleflowEngine()
	inPos := ruleflow.InPositionOutputs{Indicator: ind, Structure: st, Mechanics: mech, Ready: evaluated}
	exitConfirmCount := 0
	if p.ExitConfirmCache != nil {
		exitConfirmCount = p.ExitConfirmCache.Get(symbol, fsm.StateInPosition, posID)
	}
	rfInput := buildRuleflowInput(
		symbol,
		res,
		bind,
		fsm.StateInPosition,
		posID,
		exitConfirmCount,
		false,
		comp,
		execution.AccountState{},
		execution.RiskParams{RiskPerTradePct: bind.RiskManagement.RiskPerTradePct},
		inPos,
		hardGuardInput,
	)
	rfResult, err := rfEngine.Evaluate(ctx, bind.RuleChainPath, rfInput)
	if err != nil {
		return ruleflow.Result{}, err
	}
	if p.ExitConfirmCache != nil {
		p.ExitConfirmCache.Set(symbol, fsm.StateInPosition, posID, rfResult.ExitConfirmCount)
	}
	return rfResult, nil
}

func (p *Pipeline) buildHardGuardPosition(ctx context.Context, symbol string, pos store.PositionRecord) ruleflow.HardGuardPosition {
	out := ruleflow.HardGuardPosition{
		Side: strings.ToLower(strings.TrimSpace(pos.Side)),
	}
	if len(pos.RiskJSON) > 0 {
		if plan, decErr := position.DecodeRiskPlan(pos.RiskJSON); decErr == nil && plan.StopPrice > 0 {
			out.StopLoss = plan.StopPrice
			out.StopLossOK = true
		}
	}
	if p.PriceSource != nil {
		if quote, quoteErr := p.PriceSource.MarkPrice(ctx, symbol); quoteErr == nil && quote.Price > 0 {
			out.MarkPrice = quote.Price
			out.MarkPriceOK = true
		}
	}
	return out
}

func (p *Pipeline) judgeInPositionWithFallback(ctx context.Context, symbol string, res SymbolResult, summary positionprompt.Summary) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, ProviderPromptSet, bool, error) {
	if p == nil || p.Runner == nil || p.Runner.Provider == nil {
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, fmt.Errorf("provider is required")
	}
	providerEnabled := res.EnabledAgents
	if res.AgentPrompts.Indicator.Error != "" {
		providerEnabled.Indicator = false
	}
	if res.AgentPrompts.Structure.Error != "" {
		providerEnabled.Structure = false
	}
	if res.AgentPrompts.Mechanics.Error != "" {
		providerEnabled.Mechanics = false
	}
	indOut, stOut, mechOut, prompts, err := p.Runner.Provider.JudgeInPosition(ctx, symbol, res.AgentIndicator, res.AgentStructure, res.AgentMechanics, summary, providerEnabled)
	if err != nil {
		if provider.IsDecodeError(err) {
			return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, nil
		}
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, ProviderPromptSet{}, false, err
	}
	return indOut, stOut, mechOut, prompts, true, nil
}

func (p *Pipeline) buildInPositionPromptSummary(ctx context.Context, pos store.PositionRecord) (positionprompt.Summary, error) {
	var leverage *float64
	if pos.Leverage > 0 {
		lev := pos.Leverage
		leverage = &lev
	}
	builder := positionprompt.NewBuilder()
	base, err := builder.Build(pos.Symbol, pos.AvgEntry, pos.Qty, leverage)
	if err != nil {
		return positionprompt.Summary{}, err
	}
	if len(pos.RiskJSON) == 0 || p == nil || p.PriceSource == nil || p.BarInterval <= 0 {
		return base, nil
	}
	plan, err := position.DecodeRiskPlan(pos.RiskJSON)
	if err != nil {
		return base, nil
	}
	quote, err := p.PriceSource.MarkPrice(ctx, pos.Symbol)
	if err != nil || quote.Price <= 0 {
		return base, nil
	}
	plain, err := position.BuildPositionSummary(pos, plan, quote.Price, p.BarInterval)
	if err != nil {
		return base, nil
	}
	riskSummary, err := position.BuildPositionRiskSummary(plain)
	if err != nil {
		return base, nil
	}
	return builder.BuildWithRisk(pos.Symbol, pos.AvgEntry, pos.Qty, leverage, &riskSummary)
}

func (p *Pipeline) loadPositionRecord(ctx context.Context, symbol, posID string) (store.PositionRecord, error) {
	if p.Store == nil {
		return store.PositionRecord{}, fmt.Errorf("store is required")
	}
	if strings.TrimSpace(posID) != "" {
		pos, ok, err := p.Store.FindPositionByID(ctx, posID)
		if err != nil {
			return store.PositionRecord{}, err
		}
		if ok {
			return pos, nil
		}
	}
	pos, ok, err := p.Store.FindPositionBySymbol(ctx, symbol, position.OpenPositionStatuses)
	if err != nil {
		return store.PositionRecord{}, err
	}
	if !ok {
		return store.PositionRecord{}, fmt.Errorf("position not found")
	}
	return pos, nil
}
