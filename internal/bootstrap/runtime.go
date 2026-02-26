package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/runtime"
	"brale-core/internal/transport/runtimeapi"

	"go.uber.org/zap"
)

func buildRuntimeMap(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig, deps coreDeps) map[string]runtime.SymbolRuntime {
	runtimes := make(map[string]runtime.SymbolRuntime, len(index.Symbols))
	for _, item := range index.Symbols {
		rt, err := runtime.BuildSymbolRuntime(
			ctx,
			sys,
			symbolIndexPath,
			index,
			item.Symbol,
			runtime.SymbolRuntimeBuildDeps{
				Store:         deps.store,
				StateProvider: deps.stateProvider,
				Positioner:    deps.positioner,
				RiskPlanSvc:   deps.riskPlanSvc,
				PriceSource:   deps.priceSource,
			},
		)
		if err != nil {
			logger.Error("symbol runtime error", zap.Error(err), zap.String("symbol", item.Symbol))
			continue
		}
		runtimes[item.Symbol] = rt
	}
	return runtimes
}

func startScheduler(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, deps coreDeps, runtimes map[string]runtime.SymbolRuntime) (*runtime.RuntimeScheduler, error) {
	monitored := make([]string, 0, len(runtimes))
	for symbol := range runtimes {
		monitored = append(monitored, symbol)
	}
	newsUpdater, err := runtime.BuildNewsOverlayUpdater(sys, deps.notifier, monitored)
	if err != nil {
		return nil, fmt.Errorf("news overlay updater init failed: %w", err)
	}
	scheduler := &runtime.RuntimeScheduler{
		Symbols:             runtimes,
		Reconciler:          deps.reconciler,
		RiskMonitor:         deps.riskMonitor,
		AccountFetcher:      deps.freqtradeAcct,
		SyncOrderInterval:   time.Duration(sys.Webhook.FallbackOrderPollSec) * time.Second,
		ReconcileInterval:   time.Duration(sys.Webhook.FallbackReconcileSec) * time.Second,
		PriceTickInterval:   time.Second,
		NewsOverlayInterval: config.ParseDurationOrDefault(sys.NewsOverlay.Interval, time.Hour),
		Logger:              logger.Named("scheduler"),
		PriceStream:         deps.priceSource,
	}
	if newsUpdater != nil {
		scheduler.NewsOverlayUpdater = newsUpdater
	}
	scheduler.SetScheduledDecision(deps.scheduled)
	if err := scheduler.Start(ctx); err != nil {
		return nil, fmt.Errorf("scheduler start failed: %w", err)
	}
	return scheduler, nil
}

func buildRuntimeResolver(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig, deps coreDeps, runtimes map[string]runtime.SymbolRuntime) *runtimeapi.RuntimeSymbolResolver {
	defaultBuilder := func(sym string) (runtime.SymbolRuntime, error) {
		rt, err := runtime.BuildSymbolRuntime(
			ctx,
			sys,
			symbolIndexPath,
			index,
			sym,
			runtime.SymbolRuntimeBuildDeps{
				Store:         deps.store,
				StateProvider: deps.stateProvider,
				Positioner:    deps.positioner,
				RiskPlanSvc:   deps.riskPlanSvc,
				PriceSource:   deps.priceSource,
			},
		)
		if err != nil {
			logger.Warn("default runtime build failed", zap.Error(err), zap.String("symbol", sym))
		}
		return rt, err
	}
	defaultOnlyBuilder := func(sym string) (runtime.SymbolRuntime, error) {
		rt, err := runtime.BuildSymbolRuntime(
			ctx,
			sys,
			symbolIndexPath,
			config.SymbolIndexConfig{},
			sym,
			runtime.SymbolRuntimeBuildDeps{
				Store:         deps.store,
				StateProvider: deps.stateProvider,
				Positioner:    deps.positioner,
				RiskPlanSvc:   deps.riskPlanSvc,
				PriceSource:   deps.priceSource,
			},
		)
		if err != nil {
			logger.Warn("default-only runtime build failed", zap.Error(err), zap.String("symbol", sym))
		}
		return rt, err
	}
	const maxDynamicRuntimes = 16
	return runtimeapi.NewRuntimeSymbolResolver(runtimes, defaultBuilder, defaultOnlyBuilder, maxDynamicRuntimes)
}

func loadSymbolConfigs(logger *zap.Logger, sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig) map[string]runtimeapi.ConfigBundle {
	symbolConfigs := make(map[string]runtimeapi.ConfigBundle, len(index.Symbols))
	for _, item := range index.Symbols {
		symbolCfg, strategyCfg, _, err := runtime.LoadSymbolConfigs(sys, symbolIndexPath, item)
		if err != nil {
			logger.Warn("symbol config load failed", zap.Error(err), zap.String("symbol", item.Symbol))
			continue
		}
		symbolConfigs[item.Symbol] = runtimeapi.ConfigBundle{Symbol: symbolCfg, Strategy: strategyCfg}
	}
	return symbolConfigs
}

func buildRuntimeHandler(sys config.SystemConfig, deps coreDeps, scheduler *runtime.RuntimeScheduler, resolver *runtimeapi.RuntimeSymbolResolver, symbolConfigs map[string]runtimeapi.ConfigBundle) (http.Handler, error) {
	runtimeServer := runtimeapi.Server{
		Scheduler:             scheduler,
		Resolver:              resolver,
		PlanCache:             deps.positioner.PlanCache,
		PriceSource:           deps.priceSource,
		AllowSymbol:           deps.allowSymbol,
		Store:                 deps.store,
		ExecClient:            deps.executor.Client,
		PositionCache:         deps.positionCache,
		SymbolConfigs:         symbolConfigs,
		NewsOverlayStaleAfter: config.ParseDurationOrDefault(sys.NewsOverlay.SnapshotStaleAfter, 4*time.Hour),
	}
	return runtimeServer.Handler()
}

func resolveDurationWithDefault(raw string, fallback time.Duration) time.Duration {
	return config.ParseDurationOrDefault(raw, fallback)
}

func runFreqtradeBalanceCheck(ctx context.Context, logger *zap.Logger, deps coreDeps) {
	if !deps.scheduled {
		return
	}
	execution.CheckFreqtradeBalance(ctx, logger, deps.executor)
}
