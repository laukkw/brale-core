package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/market/binance"
	"brale-core/internal/position"
	"brale-core/internal/reconcile"
	"brale-core/internal/store"
	"brale-core/internal/transport/notify"

	"go.uber.org/zap"
)

type coreDeps struct {
	store         store.Store
	stateProvider *reconcile.FSMStateProvider
	executor      *execution.FreqtradeAdapter
	notifier      notify.Notifier
	positionCache *position.PositionCache
	allowSymbol   func(string) bool
	riskPlanSvc   *position.RiskPlanService
	positioner    *position.PositionService
	recovery      *reconcile.RecoveryService
	reconciler    *reconcile.ReconcileService
	priceSource   *binance.MarkPriceStream
	freqtradeAcct func(context.Context, string) (execution.AccountState, error)
	riskMonitor   *position.RiskMonitor
	scheduled     bool
}

func buildCoreDeps(env appEnv) (coreDeps, error) {
	st, err := buildPersistence(env.sys)
	if err != nil {
		return coreDeps{}, err
	}

	stateProvider := reconcile.NewFSMStateProvider(st)
	executor := buildExecutionAdapter(env.sys)
	allowSymbol := buildAllowSymbol(env.index)

	positionCache, riskPlanSvc, positioner := buildPositionServices(st, executor, env.notifier)
	priceSource := buildPriceSource(env.index)

	recovery, reconciler := buildReconcileServices(env.sys, st, executor, env.notifier, positionCache, positioner.PlanCache, riskPlanSvc, allowSymbol, priceSource)

	freqtradeAccount := newFreqtradeAccountFetcher(executor)
	riskMonitor := buildRiskMonitor(st, priceSource, positioner, freqtradeAccount)

	return coreDeps{
		store:         st,
		stateProvider: stateProvider,
		executor:      executor,
		notifier:      env.notifier,
		positionCache: positionCache,
		allowSymbol:   allowSymbol,
		riskPlanSvc:   riskPlanSvc,
		positioner:    positioner,
		recovery:      recovery,
		reconciler:    reconciler,
		priceSource:   priceSource,
		freqtradeAcct: freqtradeAccount,
		riskMonitor:   riskMonitor,
		scheduled:     resolveScheduledDecision(env.sys),
	}, nil
}

func buildPersistence(sys config.SystemConfig) (store.Store, error) {
	db, err := store.OpenSQLite(sys.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", sys.DBPath, err)
	}
	if err := store.Migrate(db, store.MigrateOptions{Full: sys.PersistMode == "full"}); err != nil {
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return store.NewStore(db), nil
}

func buildExecutionAdapter(sys config.SystemConfig) *execution.FreqtradeAdapter {
	return &execution.FreqtradeAdapter{
		Client: &execution.FreqtradeClient{
			Endpoint:  sys.ExecEndpoint,
			APIKey:    sys.ExecAPIKey,
			APISecret: sys.ExecAPISecret,
			AuthType:  sys.ExecAuth,
		},
	}
}

func buildPositionServices(st store.Store, executor *execution.FreqtradeAdapter, notifier notify.Notifier) (*position.PositionCache, *position.RiskPlanService, *position.PositionService) {
	positionCache := position.NewPositionCache()
	riskPlanSvc := &position.RiskPlanService{Store: st}
	positioner := &position.PositionService{
		Store:     st,
		Executor:  executor,
		Notifier:  notifier,
		Cache:     positionCache,
		PlanCache: position.NewPlanCache(),
		RiskPlans: riskPlanSvc,
	}
	return positionCache, riskPlanSvc, positioner
}

func buildPriceSource(index config.SymbolIndexConfig) *binance.MarkPriceStream {
	return binance.NewMarkPriceStream(binance.MarkPriceStreamOptions{
		Symbols: config.SymbolsFromIndex(index),
		Rate:    time.Second,
	})
}

func buildReconcileServices(sys config.SystemConfig, st store.Store, executor *execution.FreqtradeAdapter, notifier notify.Notifier, positionCache *position.PositionCache, planCache *position.PlanCache, riskPlanSvc *position.RiskPlanService, allowSymbol func(string) bool, priceSource *binance.MarkPriceStream) (*reconcile.RecoveryService, *reconcile.ReconcileService) {
	recovery := &reconcile.RecoveryService{
		Store:       st,
		Executor:    executor,
		Notifier:    notifier,
		Cache:       positionCache,
		AllowSymbol: allowSymbol,
	}
	reconciler := &reconcile.ReconcileService{
		Store:       st,
		Executor:    executor,
		Notifier:    notifier,
		Cache:       positionCache,
		PlanCache:   planCache,
		RiskPlans:   riskPlanSvc,
		AllowSymbol: allowSymbol,
	}
	reconciler.OrderStatusFetcher = &execution.FreqtradeStatusFetcher{
		Endpoint:  sys.ExecEndpoint,
		APIKey:    sys.ExecAPIKey,
		APISecret: sys.ExecAPISecret,
		AuthType:  sys.ExecAuth,
	}
	reconciler.PriceSource = priceSource
	return recovery, reconciler
}

func buildRiskMonitor(st store.Store, priceSource *binance.MarkPriceStream, positioner *position.PositionService, accountFetcher func(context.Context, string) (execution.AccountState, error)) *position.RiskMonitor {
	return &position.RiskMonitor{
		Store:       st,
		PriceSource: priceSource,
		Positions:   positioner,
		PlanCache:   positioner.PlanCache,
		AccountFetcher: func(ctx context.Context, symbol string) (execution.AccountState, error) {
			return accountFetcher(ctx, symbol)
		},
	}
}

func resolveScheduledDecision(sys config.SystemConfig) bool {
	if sys.EnableScheduledDecision == nil {
		return true
	}
	return *sys.EnableScheduledDecision
}

func buildAllowSymbol(index config.SymbolIndexConfig) func(string) bool {
	allowed := make(map[string]struct{}, len(index.Symbols))
	for _, item := range index.Symbols {
		normalized := strings.ToUpper(strings.TrimSpace(item.Symbol))
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	return func(sym string) bool {
		_, ok := allowed[strings.ToUpper(strings.TrimSpace(sym))]
		return ok
	}
}

func newFreqtradeAccountFetcher(executor *execution.FreqtradeAdapter) func(context.Context, string) (execution.AccountState, error) {
	return func(ctx context.Context, symbol string) (execution.AccountState, error) {
		quote, err := executor.Client.Balance(ctx)
		if err != nil {
			return execution.AccountState{}, err
		}
		equity, ok := execution.ExtractUSDTBalance(quote)
		if !ok || equity <= 0 {
			return execution.AccountState{}, fmt.Errorf("balance not available")
		}
		available, ok := execution.ExtractUSDTAvailable(quote)
		if !ok || available <= 0 {
			available = equity
		}
		currency := execution.ResolveStakeCurrency(quote)
		return execution.AccountState{Equity: equity, Available: available, Currency: currency}, nil
	}
}

func runScheduledWarmup(ctx context.Context, logger *zap.Logger, deps coreDeps) {
	if !deps.scheduled {
		return
	}
	if err := deps.recovery.RunOnce(ctx, ""); err != nil {
		logger.Warn("recovery run failed", zap.Error(err))
	}
	if err := deps.reconciler.RunOnce(ctx, ""); err != nil {
		logger.Warn("reconcile warmup failed", zap.Error(err))
	}
}
