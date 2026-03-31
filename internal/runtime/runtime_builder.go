package runtime

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision"
	"brale-core/internal/market"
	"brale-core/internal/position"
	"brale-core/internal/reconcile"
	"brale-core/internal/store"
	"brale-core/internal/strategy"

	symbolpkg "brale-core/internal/pkg/symbol"
)

func NormalizeSymbol(symbol string) string {
	return symbolpkg.Normalize(symbol)
}

type SymbolRuntimeBuildDeps struct {
	Store         store.Store
	StateProvider *reconcile.FSMStateProvider
	Positioner    *position.PositionService
	RiskPlanSvc   *position.RiskPlanService
	PriceSource   market.PriceSource
}

func NewSymbolRuntimeBuildDeps(st store.Store, stateProvider *reconcile.FSMStateProvider, positioner *position.PositionService, riskPlanSvc *position.RiskPlanService, priceSource market.PriceSource) SymbolRuntimeBuildDeps {
	return SymbolRuntimeBuildDeps{
		Store:         st,
		StateProvider: stateProvider,
		Positioner:    positioner,
		RiskPlanSvc:   riskPlanSvc,
		PriceSource:   priceSource,
	}
}

func BuildSymbolRuntime(metricsCtx context.Context, sys config.SystemConfig, indexPath string, index config.SymbolIndexConfig, symbol string, deps SymbolRuntimeBuildDeps) (SymbolRuntime, error) {
	normalized := NormalizeSymbol(symbol)
	if normalized == "" {
		return SymbolRuntime{}, fmt.Errorf("symbol is required")
	}
	if entry, ok := findSymbolIndexEntry(index, normalized); ok {
		symbolCfg, stratCfg, bind, err := LoadSymbolConfigs(sys, indexPath, entry)
		if err != nil {
			return SymbolRuntime{}, err
		}
		runtimeCfg, err := buildRuntimeConfig(symbolCfg, stratCfg, bind)
		if err != nil {
			return SymbolRuntime{}, err
		}
		return buildSymbolRuntimeFromRuntimeConfig(metricsCtx, sys, runtimeCfg, deps)
	}
	base := filepath.Dir(indexPath)
	runtimeCfg, err := loadDefaultRuntimeConfig(sys, base, normalized)
	if err != nil {
		return SymbolRuntime{}, err
	}
	return buildSymbolRuntimeFromRuntimeConfig(metricsCtx, sys, runtimeCfg, deps)
}

func buildSymbolRuntimeFromConfig(metricsCtx context.Context, sys config.SystemConfig, symbolCfg config.SymbolConfig, stratCfg config.StrategyConfig, bind strategy.StrategyBinding, st store.Store, stateProvider *reconcile.FSMStateProvider, positioner *position.PositionService, riskPlanSvc *position.RiskPlanService, priceSource market.PriceSource, barInterval time.Duration, enabledCfg config.AgentEnabled, enabledApp decision.AgentEnabled, requireMechanics bool) (SymbolRuntime, error) {
	runtimeCfg := symbolRuntimeConfig{
		Symbol:           symbolCfg,
		Strategy:         stratCfg,
		Binding:          bind,
		EnabledConfig:    enabledCfg,
		EnabledApp:       enabledApp,
		EnabledMap:       map[string]decision.AgentEnabled{symbolCfg.Symbol: enabledApp},
		BarInterval:      barInterval,
		RequireMechanics: requireMechanics,
	}
	deps := NewSymbolRuntimeBuildDeps(st, stateProvider, positioner, riskPlanSvc, priceSource)
	return buildSymbolRuntimeFromRuntimeConfig(metricsCtx, sys, runtimeCfg, deps)
}

func buildSymbolRuntimeFromRuntimeConfig(metricsCtx context.Context, sys config.SystemConfig, runtimeCfg symbolRuntimeConfig, deps SymbolRuntimeBuildDeps) (SymbolRuntime, error) {
	agentSvc, providerSvc, tracker := buildSymbolAgents(sys, runtimeCfg.Symbol)
	fetcher := buildSnapshotFetcher(runtimeCfg.Symbol, runtimeCfg.RequireMechanics)
	compressor, services := buildCompressor(runtimeCfg.Symbol, runtimeCfg.EnabledConfig, runtimeCfg.EnabledMap)
	exitConfirmCache := decision.NewExitConfirmCache()
	runner := buildRunner(sys, fetcher, compressor, agentSvc, providerSvc, runtimeCfg)
	pipeline, err := buildPipelineFromRuntimeConfig(sys, deps, runtimeCfg, &runner, exitConfirmCache)
	if err != nil {
		return SymbolRuntime{}, err
	}
	return SymbolRuntime{
		Symbol:          runtimeCfg.Symbol.Symbol,
		Intervals:       runtimeCfg.Symbol.Intervals,
		KlineLimit:      runtimeCfg.Symbol.KlineLimit,
		BarInterval:     runtimeCfg.BarInterval,
		RiskPerTradePct: runtimeCfg.Strategy.RiskManagement.RiskPerTradePct,
		Enabled:         runtimeCfg.EnabledApp,
		LLMTracker:      tracker,
		Pipeline:        pipeline,
		Services:        services,
	}, nil
}

func findSymbolIndexEntry(index config.SymbolIndexConfig, symbol string) (config.SymbolIndexEntry, bool) {
	for _, entry := range index.Symbols {
		if strings.EqualFold(entry.Symbol, symbol) {
			return entry, true
		}
	}
	return config.SymbolIndexEntry{}, false
}
