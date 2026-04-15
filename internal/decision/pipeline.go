// 本文件主要内容：编排单轮链路并持久化计划、订单与状态。
// 本文件主要内容：实现决策流水线的执行与持久化流程。

package decision

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/decisionmode"
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/fsm"
	"brale-core/internal/decision/fund"
	"brale-core/internal/decision/provider"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/market"
	"brale-core/internal/memory"

	"brale-core/internal/pkg/logging"
	"brale-core/internal/position"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"
	"brale-core/internal/strategy"

	"go.uber.org/zap"
)

type Pipeline struct {
	Runner                  *Runner
	Core                    PipelineCoreDeps
	BarInterval             time.Duration
	ExecutionSystem         string
	Bindings                map[string]strategy.StrategyBinding
	ExitConfirmCache        *ExitConfirmCache
	EntryCooldownCache      *EntryCooldownCache
	EntryCooldownRounds     int
	AgentStore              func(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, inputs AgentInputSet, enabled AgentEnabled, prompts AgentPromptSet) error
	ProviderStore           func(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, providers fund.ProviderBundle, dataCtx ProviderDataContext, prompts ProviderPromptSet) error
	ProviderInPositionStore func(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, ind provider.InPositionIndicatorOut, st provider.InPositionStructureOut, mech provider.InPositionMechanicsOut, prompts ProviderPromptSet, enabled AgentEnabled) error
	GateStore               func(ctx context.Context, snap snapshot.MarketSnapshot, snapID uint, sym string, gate fund.GateDecision, providers fund.ProviderBundle) error
	Notifier                Notifier
	RoundIDFactory          func() (llm.RoundID, error)
	TightenRiskLLM          TightenRiskUpdateLLM
	WorkingMemory           memory.Store
	EpisodicMemory          memory.EpisodicStore
	SemanticMemory          memory.SemanticStore
}

type PipelineCoreDeps struct {
	Store       store.Store
	Positioner  *position.PositionService
	RiskPlans   *position.RiskPlanService
	PriceSource market.PriceSource
	States      StateProvider
	PlanCache   *position.PlanCache
}

type PersistResult struct {
	Symbol          string
	Gate            string
	ExternalOrderID string
	NextState       fsm.PositionState
	Err             error
}

type symbolDecisionContext struct {
	Mode       decisionmode.Mode
	PositionID string
}

func (p *Pipeline) RunOnce(ctx context.Context, symbols []string, intervals []string, limit int, acct execution.AccountState, risk execution.RiskParams) ([]PersistResult, error) {
	return p.runOnceWithOptions(ctx, symbols, intervals, limit, acct, risk, RunOptions{BuildPlan: true})
}

func (p *Pipeline) notifyError(ctx context.Context, err error) {
	if err == nil || p.Notifier == nil {
		return
	}
	notice := ErrorNotice{
		Severity:  "error",
		Component: "decision",
		Message:   formatErrorNotification(ctx, err),
	}
	notifyCtx := context.Background()
	if ctx != nil {
		notifyCtx = context.WithoutCancel(ctx)
	}
	sendCtx, cancel := context.WithTimeout(notifyCtx, 5*time.Second)
	defer cancel()
	if notifyErr := p.Notifier.SendError(sendCtx, notice); notifyErr != nil {
		logging.FromContext(ctx).Named("pipeline").Error("notify error failed", zap.Error(notifyErr))
	}
}

func formatErrorNotification(ctx context.Context, err error) string {
	if err == nil {
		return ""
	}
	lines := make([]string, 0, 4)
	lines = append(lines, "decision pipeline failed")
	if roundID, ok := llm.SessionRoundIDFromContext(ctx); ok {
		lines = append(lines, fmt.Sprintf("round_id=%s", roundID.String()))
	}
	if flow, ok := llm.SessionFlowFromContext(ctx); ok {
		lines = append(lines, fmt.Sprintf("flow=%s", flow.String()))
	}
	lines = append(lines, fmt.Sprintf("error=%s", strings.TrimSpace(err.Error())))
	return strings.Join(lines, "\n")
}

func (p *Pipeline) validate() error {
	if p.Runner == nil || p.store() == nil || p.positioner() == nil || p.riskPlans() == nil || p.stateProvider() == nil || p.planCache() == nil {
		return fmt.Errorf("pipeline dependencies missing")
	}
	if p.ExitConfirmCache == nil {
		p.ExitConfirmCache = NewExitConfirmCache()
	}
	if p.EntryCooldownCache == nil {
		p.EntryCooldownCache = NewEntryCooldownCache()
	}
	if p.EntryCooldownRounds <= 0 {
		p.EntryCooldownRounds = defaultEntryCooldownRoundsAfterExit
	}
	if p.ExecutionSystem == "" {
		return fmt.Errorf("execution_system is required")
	}
	return nil
}

func (p *Pipeline) getBinding(symbol string) (strategy.StrategyBinding, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	bind, ok := p.Bindings[symbol]
	if !ok {
		return strategy.StrategyBinding{}, fmt.Errorf("binding not found")
	}
	return bind, nil
}

func (p *Pipeline) loadState(ctx context.Context, symbol string) (fsm.PositionState, string, error) {
	state, posID, err := p.stateProvider().Load(ctx, symbol)
	if err != nil {
		return "", "", err
	}
	if state == "" {
		state = fsm.StateFlat
	}
	return state, posID, nil
}

func (p *Pipeline) applyReportMarkPrice(ctx context.Context, res *SymbolResult) {
	if p == nil || res == nil || p.priceSource() == nil {
		return
	}
	quote, err := p.priceSource().MarkPrice(ctx, res.Symbol)
	if err != nil || quote.Price <= 0 {
		return
	}
	if res.Gate.Derived == nil {
		res.Gate.Derived = map[string]any{}
	}
	res.Gate.Derived["current_price"] = quote.Price
}

func (p *Pipeline) store() store.Store {
	if p == nil {
		return nil
	}
	return p.Core.Store
}

func (p *Pipeline) positioner() *position.PositionService {
	if p == nil {
		return nil
	}
	return p.Core.Positioner
}

func (p *Pipeline) riskPlans() *position.RiskPlanService {
	if p == nil {
		return nil
	}
	return p.Core.RiskPlans
}

func (p *Pipeline) priceSource() market.PriceSource {
	if p == nil {
		return nil
	}
	return p.Core.PriceSource
}

func (p *Pipeline) stateProvider() StateProvider {
	if p == nil {
		return nil
	}
	return p.Core.States
}

func (p *Pipeline) planCache() *position.PlanCache {
	if p == nil {
		return nil
	}
	if p.Core.PlanCache != nil {
		return p.Core.PlanCache
	}
	if pos := p.positioner(); pos != nil {
		return pos.PlanCache
	}
	return nil
}
