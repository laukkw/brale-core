package bootstrap

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/market/binance"
	"brale-core/internal/runtime"
	"brale-core/internal/transport/runtimeapi"

	"go.uber.org/zap"
)

type symbolRuntimeBuilder struct {
	ctx             context.Context
	sys             config.SystemConfig
	symbolIndexPath string
	deps            runtime.SymbolRuntimeBuildDeps
}

func newSymbolRuntimeBuilder(ctx context.Context, sys config.SystemConfig, symbolIndexPath string, deps coreDeps) symbolRuntimeBuilder {
	return symbolRuntimeBuilder{
		ctx:             ctx,
		sys:             sys,
		symbolIndexPath: symbolIndexPath,
		deps:            runtime.NewSymbolRuntimeBuildDeps(deps.persistence.store, deps.persistence.stateProvider, deps.position.positioner, deps.position.riskPlanSvc, deps.position.priceSource, deps.execution.notifier),
	}
}

func (b symbolRuntimeBuilder) Build(index config.SymbolIndexConfig, symbol string) (runtime.SymbolRuntime, error) {
	return runtime.BuildSymbolRuntime(b.ctx, b.sys, b.symbolIndexPath, index, symbol, b.deps)
}

func newSymbolRuntimeBuildFunc(builder symbolRuntimeBuilder, index config.SymbolIndexConfig, logger *zap.Logger, warnMsg string) func(string) (runtime.SymbolRuntime, error) {
	return func(sym string) (runtime.SymbolRuntime, error) {
		rt, err := builder.Build(index, sym)
		if err != nil {
			logger.Warn(warnMsg, zap.Error(err), zap.String("symbol", sym))
		}
		return rt, err
	}
}

func buildRuntimeMap(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig, deps coreDeps) map[string]runtime.SymbolRuntime {
	builder := newSymbolRuntimeBuilder(ctx, sys, symbolIndexPath, deps)
	runtimes := make(map[string]runtime.SymbolRuntime, len(index.Symbols))
	for _, item := range index.Symbols {
		symbolKey := canonicalSymbolFromIndexEntry(item)
		if symbolKey == "" {
			continue
		}
		rt, err := builder.Build(index, symbolKey)
		if err != nil {
			logger.Error("symbol runtime error", zap.Error(err), zap.String("symbol", symbolKey))
			continue
		}
		runtimes[symbolKey] = rt
	}
	return runtimes
}

func startScheduler(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, deps coreDeps, runtimes map[string]runtime.SymbolRuntime) (*runtime.RuntimeScheduler, error) {
	scheduler := &runtime.RuntimeScheduler{
		Symbols:           runtimes,
		Reconciler:        deps.reconcile.reconciler,
		RiskMonitor:       deps.position.riskMonitor,
		AccountFetcher:    deps.execution.freqtradeAcct,
		SyncOrderInterval: time.Duration(sys.Webhook.FallbackOrderPollSec) * time.Second,
		ReconcileInterval: time.Duration(sys.Webhook.FallbackReconcileSec) * time.Second,
		PriceTickInterval: time.Second,
		DisableTickerLoops: strings.EqualFold(sys.Scheduler.Backend, "river"),
		Logger:            logger.Named("scheduler"),
		PriceStream:       deps.position.priceSource,
	}
	scheduler.SetScheduledDecision(deps.execution.scheduled)
	if err := scheduler.Start(ctx); err != nil {
		return nil, fmt.Errorf("scheduler start failed: %w", err)
	}
	return scheduler, nil
}

func buildRuntimeResolver(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig, deps coreDeps, runtimes map[string]runtime.SymbolRuntime) *runtimeapi.RuntimeSymbolResolver {
	builder := newSymbolRuntimeBuilder(ctx, sys, symbolIndexPath, deps)
	defaultBuilder := newSymbolRuntimeBuildFunc(builder, index, logger, "default runtime build failed")
	defaultOnlyBuilder := newSymbolRuntimeBuildFunc(builder, config.SymbolIndexConfig{}, logger, "default-only runtime build failed")
	const maxDynamicRuntimes = 16
	return runtimeapi.NewRuntimeSymbolResolver(runtimes, defaultBuilder, defaultOnlyBuilder, maxDynamicRuntimes)
}

func loadSymbolConfigs(logger *zap.Logger, sys config.SystemConfig, symbolIndexPath string, index config.SymbolIndexConfig) map[string]runtimeapi.ConfigBundle {
	symbolConfigs := make(map[string]runtimeapi.ConfigBundle, len(index.Symbols))
	for _, item := range index.Symbols {
		symbolKey := canonicalSymbolFromIndexEntry(item)
		if symbolKey == "" {
			continue
		}
		symbolCfg, strategyCfg, _, err := runtime.LoadSymbolConfigs(sys, symbolIndexPath, item)
		if err != nil {
			logger.Warn("symbol config load failed", zap.Error(err), zap.String("symbol", symbolKey))
			continue
		}
		symbolConfigs[symbolKey] = runtimeapi.ConfigBundle{Symbol: symbolCfg, Strategy: strategyCfg}
	}
	return symbolConfigs
}

func buildRuntimeHandler(sys config.SystemConfig, deps coreDeps, scheduler *runtime.RuntimeScheduler, resolver *runtimeapi.RuntimeSymbolResolver, symbolConfigs map[string]runtimeapi.ConfigBundle) (http.Handler, error) {
	runtimeServer := runtimeapi.Server{
		Scheduler:     scheduler,
		Resolver:      resolver,
		PlanCache:     deps.position.positioner.PlanCache,
		PriceSource:   deps.position.priceSource,
		KlineProvider: binance.NewFuturesMarket(),
		AllowSymbol:   deps.execution.allowSymbol,
		Store:         deps.persistence.store,
		ExecClient:    deps.execution.executor.Client,
		PositionCache: deps.position.positionCache,
		SymbolConfigs: symbolConfigs,
	}
	return runtimeServer.Handler()
}

func resolveDurationWithDefault(raw string, fallback time.Duration) time.Duration {
	return config.ParseDurationOrDefault(raw, fallback)
}

func runFreqtradeBalanceCheck(ctx context.Context, logger *zap.Logger, deps coreDeps) {
	if !deps.execution.scheduled {
		return
	}
	execution.CheckFreqtradeBalance(ctx, logger, deps.execution.executor)
}
