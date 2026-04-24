package runtime

import (
	"context"
	"path/filepath"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	llmapp "brale-core/internal/llm/app"
	"brale-core/internal/market"
	"brale-core/internal/memory"
	"brale-core/internal/position"
	"brale-core/internal/reconcile"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"
	"brale-core/internal/strategy"
	"brale-core/internal/transport/notify"

	"go.uber.org/zap"
)

func buildRunner(ctx context.Context, sys config.SystemConfig, promptStore store.PromptRegistryStore, fetcher *snapshot.Fetcher, compressor decision.Compressor, agentSvc decision.AgentService, providerSvc decision.ProviderService, runtimeCfg symbolRuntimeConfig, workingMemory memory.Store, episodicMemory memory.EpisodicStore, semanticMemory memory.SemanticStore) decision.Runner {
	riskPrompts, err := loadPromptBuilder(ctx, promptStore, zap.NewNop())
	if err != nil {
		riskPrompts = fallbackPromptBuilder()
	}
	riskSvc := llmapp.LLMRiskService{
		Provider: newLLMClient(sys, runtimeCfg.Symbol.LLM.Provider.Structure),
		Prompts:  riskPrompts,
	}
	return decision.Runner{
		Snapshotter:     fetcher,
		Compressor:      compressor,
		Agent:           agentSvc,
		Provider:        providerSvc,
		FlatRiskInitLLM: riskSvc.FlatRiskInitLLM(),
		TightenRiskLLM:  riskSvc.TightenRiskLLM(),
		Bindings:        map[string]strategy.StrategyBinding{runtimeCfg.Symbol.Symbol: runtimeCfg.Binding},
		Configs:         map[string]config.SymbolConfig{runtimeCfg.Symbol.Symbol: runtimeCfg.Symbol},
		Enabled:         runtimeCfg.EnabledMap,
		WorkingMemory:   workingMemory,
		EpisodicMemory:  episodicMemory,
		SemanticMemory:  semanticMemory,
	}
}

func buildPipeline(sys config.SystemConfig, st store.Store, stateProvider *reconcile.FSMStateProvider, positioner *position.PositionService, riskPlanSvc *position.RiskPlanService, priceSource market.PriceSource, barInterval time.Duration, symbol string, bind strategy.StrategyBinding, symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig, runner *decision.Runner, exitConfirmCache *decision.ExitConfirmCache, workingMemory memory.Store, episodicMemory memory.EpisodicStore, semanticMemory memory.SemanticStore) (*decision.Pipeline, error) {
	runtimeCfg := symbolRuntimeConfig{
		Symbol:      symbolCfg,
		Strategy:    stratCfg,
		Binding:     bind,
		BarInterval: barInterval,
	}
	deps := NewSymbolRuntimeBuildDeps(st, stateProvider, positioner, riskPlanSvc, priceSource, nil, nil, nil)
	return buildPipelineFromRuntimeConfig(sys, deps, runtimeCfg, runner, exitConfirmCache, workingMemory, episodicMemory, semanticMemory)
}

func buildPipelineFromRuntimeConfig(sys config.SystemConfig, deps SymbolRuntimeBuildDeps, runtimeCfg symbolRuntimeConfig, runner *decision.Runner, exitConfirmCache *decision.ExitConfirmCache, workingMemory memory.Store, episodicMemory memory.EpisodicStore, semanticMemory memory.SemanticStore) (*decision.Pipeline, error) {
	formatter := decision.NewFormatter()
	notifier := deps.Notifier
	if notifier == nil {
		var err error
		notifier, err = notify.NewManager(notify.FromConfig(sys.Notification), formatter)
		if err != nil {
			return nil, err
		}
	}
	roundRecorderTimeout, roundRecorderTimeoutSet := resolveRoundRecorderTimeout(sys)
	roundRecorderRetries, roundRecorderRetriesSet := resolveRoundRecorderRetries(sys)
	hooks := decision.StoreHooks{
		Store:         deps.Store,
		SystemHash:    sys.Hash,
		StrategyHash:  config.CombineHashes(runtimeCfg.Symbol.Hash, runtimeCfg.Strategy.Hash),
		SourceVersion: "v0",
		Notifier:      notifier,
		TraceDir:      filepath.Join("data", "llm-traces"),
		TraceLogPath:  config.ResolveLogPath(sys),
		TraceEnabled:  true,
		TraceRedacted: false,
	}
	return &decision.Pipeline{
		Runner: runner,
		Core: decision.PipelineCoreDeps{
			Store:       deps.Store,
			Positioner:  deps.Positioner,
			RiskPlans:   deps.RiskPlanSvc,
			PriceSource: deps.PriceSource,
			States:      deps.StateProvider,
			PlanCache:   deps.Positioner.PlanCache,
		},
		BarInterval:             runtimeCfg.BarInterval,
		ExecutionSystem:         sys.ExecutionSystem,
		Bindings:                map[string]strategy.StrategyBinding{runtimeCfg.Symbol.Symbol: runtimeCfg.Binding},
		ExitConfirmCache:        exitConfirmCache,
		EntryCooldownCache:      decision.NewEntryCooldownCache(),
		EntryCooldownRounds:     2,
		AgentStore:              hooks.SaveAgent,
		ProviderStore:           hooks.SaveProvider,
		ProviderInPositionStore: hooks.SaveProviderInPosition,
		GateStore:               hooks.SaveGate,
		Notifier:                notifier,
		TightenRiskLLM:          runner.TightenRiskLLM,
		WorkingMemory:           workingMemory,
		EpisodicMemory:          episodicMemory,
		SemanticMemory:          semanticMemory,
		LLMTokenBudget:          sys.LLM.TokenBudgetPerRound,
		LLMTokenBudgetWarnPct:   sys.LLM.TokenBudgetWarnPct,
		RoundRecorderTimeout:    roundRecorderTimeout,
		RoundRecorderTimeoutSet: roundRecorderTimeoutSet,
		RoundRecorderRetries:    roundRecorderRetries,
		RoundRecorderRetriesSet: roundRecorderRetriesSet,
	}, nil
}

func resolveRoundRecorderTimeout(sys config.SystemConfig) (time.Duration, bool) {
	if sys.LLM.RoundRecorderTimeoutSec == nil {
		return 0, false
	}
	return time.Duration(*sys.LLM.RoundRecorderTimeoutSec) * time.Second, true
}

func resolveRoundRecorderRetries(sys config.SystemConfig) (int, bool) {
	if sys.LLM.RoundRecorderRetries == nil {
		return 0, false
	}
	return *sys.LLM.RoundRecorderRetries, true
}
