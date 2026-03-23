package runtime

import (
	"path/filepath"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/llm"
	llmapp "brale-core/internal/llm/app"
	"brale-core/internal/market"
	"brale-core/internal/position"
	"brale-core/internal/reconcile"
	"brale-core/internal/snapshot"
	"brale-core/internal/store"
	"brale-core/internal/strategy"
	"brale-core/internal/transport/notify"
)

func buildRunner(sys config.SystemConfig, fetcher *snapshot.Fetcher, compressor *decision.FeatureCompressor, agentSvc decision.AgentService, providerSvc decision.ProviderService, runtimeCfg symbolRuntimeConfig, sessionManager *llm.RoundSessionManager, sessionMode llm.SessionMode) decision.Runner {
	defaults := config.DefaultPromptDefaults()
	riskPrompts := llmapp.LLMPromptBuilder{
		RiskFlatInitSystem: defaults.RiskFlatInit,
		UserFormat:         llmapp.UserPromptFormatBullet,
	}
	riskSvc := llmapp.LLMRiskService{
		Provider:       newLLMClient(sys, runtimeCfg.Symbol.LLM.Provider.Structure),
		Prompts:        riskPrompts,
		SessionManager: sessionManager,
		SessionMode:    sessionMode,
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
	}
}

func buildPipeline(sys config.SystemConfig, st store.Store, stateProvider *reconcile.FSMStateProvider, positioner *position.PositionService, riskPlanSvc *position.RiskPlanService, priceSource market.PriceSource, barInterval time.Duration, symbol string, bind strategy.StrategyBinding, symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig, runner *decision.Runner, exitConfirmCache *decision.ExitConfirmCache, sessionManager *llm.RoundSessionManager, sessionMode llm.SessionMode) (*decision.Pipeline, error) {
	runtimeCfg := symbolRuntimeConfig{
		Symbol:      symbolCfg,
		Strategy:    stratCfg,
		Binding:     bind,
		BarInterval: barInterval,
	}
	deps := SymbolRuntimeBuildDeps{Store: st, StateProvider: stateProvider, Positioner: positioner, RiskPlanSvc: riskPlanSvc, PriceSource: priceSource}
	return buildPipelineFromRuntimeConfig(sys, deps, runtimeCfg, runner, exitConfirmCache, sessionManager, sessionMode)
}

func buildPipelineFromRuntimeConfig(sys config.SystemConfig, deps SymbolRuntimeBuildDeps, runtimeCfg symbolRuntimeConfig, runner *decision.Runner, exitConfirmCache *decision.ExitConfirmCache, sessionManager *llm.RoundSessionManager, sessionMode llm.SessionMode) (*decision.Pipeline, error) {
	formatter := decision.NewFormatter()
	notifier, err := notify.NewManager(notify.FromConfig(sys.Notification), formatter)
	if err != nil {
		return nil, err
	}
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
		Runner:                  runner,
		Store:                   deps.Store,
		Positioner:              deps.Positioner,
		RiskPlans:               deps.RiskPlanSvc,
		PriceSource:             deps.PriceSource,
		BarInterval:             runtimeCfg.BarInterval,
		ExecutionSystem:         sys.ExecutionSystem,
		States:                  deps.StateProvider,
		Bindings:                map[string]strategy.StrategyBinding{runtimeCfg.Symbol.Symbol: runtimeCfg.Binding},
		PlanCache:               deps.Positioner.PlanCache,
		ExitConfirmCache:        exitConfirmCache,
		EntryCooldownCache:      decision.NewEntryCooldownCache(),
		EntryCooldownRounds:     2,
		AgentStore:              hooks.SaveAgent,
		ProviderStore:           hooks.SaveProvider,
		ProviderInPositionStore: hooks.SaveProviderInPosition,
		GateStore:               hooks.SaveGate,
		Notifier:                notifier,
		SessionManager:          sessionManager,
		SessionCleanup:          llm.CleanupOpenAISession,
		SessionMode:             sessionMode,
		TightenRiskLLM:          runner.TightenRiskLLM,
	}, nil
}
