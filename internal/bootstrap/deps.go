package bootstrap

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/execution"
	"brale-core/internal/market/binance"
	"brale-core/internal/pgstore"
	"brale-core/internal/position"
	"brale-core/internal/reconcile"
	"brale-core/internal/store"
	"brale-core/internal/transport/notify"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type persistenceDeps struct {
	store         store.Store
	pool          *pgxpool.Pool
	stateProvider *reconcile.FSMStateProvider
}

type executionDeps struct {
	executor      *execution.FreqtradeAdapter
	notifier      notify.Notifier
	allowSymbol   func(string) bool
	freqtradeAcct func(context.Context, string) (execution.AccountState, error)
	scheduled     bool
}

type positionDeps struct {
	positionCache *position.PositionCache
	riskPlanSvc   *position.RiskPlanService
	positioner    *position.PositionService
	priceSource   *binance.MarkPriceStream
	riskMonitor   *position.RiskMonitor
}

type reconcileDeps struct {
	recovery   *reconcile.RecoveryService
	reconciler *reconcile.ReconcileService
}

type positionServiceBuildDeps struct {
	store    store.Store
	executor *execution.FreqtradeAdapter
	notifier notify.Notifier
}

type reconcileServiceBuildDeps struct {
	sys             config.SystemConfig
	index           config.SymbolIndexConfig
	symbolIndexPath string
	store           store.Store
	executor        *execution.FreqtradeAdapter
	notifier        notify.Notifier
	positionCache   *position.PositionCache
	planCache       *position.PlanCache
	riskPlanSvc     *position.RiskPlanService
	allowSymbol     func(string) bool
	priceSource     *binance.MarkPriceStream
}

type riskMonitorBuildDeps struct {
	store          store.Store
	priceSource    *binance.MarkPriceStream
	positioner     *position.PositionService
	accountFetcher func(context.Context, string) (execution.AccountState, error)
	sys            config.SystemConfig
}

type coreDeps struct {
	persistence persistenceDeps
	execution   executionDeps
	position    positionDeps
	reconcile   reconcileDeps
	closeDB     func()
}

func buildCoreDeps(ctx context.Context, logger *zap.Logger, env appEnv) (coreDeps, error) {
	st, pool, closeDB, err := buildPersistence(ctx, env.sys, logger)
	if err != nil {
		return coreDeps{}, err
	}

	stateProvider := reconcile.NewFSMStateProvider(st)
	executor := buildExecutionAdapter(env.sys)
	allowSymbol := buildAllowSymbol(env.index)

	positionCache, riskPlanSvc, positioner := buildPositionServices(positionServiceBuildDeps{store: st, executor: executor, notifier: env.notifier})
	priceSource := buildPriceSource(env.index)

	recovery, reconciler, err := buildReconcileServices(reconcileServiceBuildDeps{
		sys:             env.sys,
		index:           env.index,
		symbolIndexPath: env.symbolIndexPath,
		store:           st,
		executor:        executor,
		notifier:        env.notifier,
		positionCache:   positionCache,
		planCache:       positioner.PlanCache,
		riskPlanSvc:     riskPlanSvc,
		allowSymbol:     allowSymbol,
		priceSource:     priceSource,
	})
	if err != nil {
		return coreDeps{}, err
	}

	freqtradeAccount := newFreqtradeAccountFetcher(executor)
	riskMonitor := buildRiskMonitor(riskMonitorBuildDeps{store: st, priceSource: priceSource, positioner: positioner, accountFetcher: freqtradeAccount, sys: env.sys})

	return coreDeps{
		persistence: persistenceDeps{store: st, pool: pool, stateProvider: stateProvider},
		execution: executionDeps{
			executor:      executor,
			notifier:      env.notifier,
			allowSymbol:   allowSymbol,
			freqtradeAcct: freqtradeAccount,
			scheduled:     resolveScheduledDecision(env.sys),
		},
		position: positionDeps{
			positionCache: positionCache,
			riskPlanSvc:   riskPlanSvc,
			positioner:    positioner,
			priceSource:   priceSource,
			riskMonitor:   riskMonitor,
		},
		reconcile: reconcileDeps{recovery: recovery, reconciler: reconciler},
		closeDB:   closeDB,
	}, nil
}

func buildPersistence(ctx context.Context, sys config.SystemConfig, logger *zap.Logger) (store.Store, *pgxpool.Pool, func(), error) {
	dsn := sys.Database.DSN
	if dsn == "" {
		return nil, nil, nil, fmt.Errorf("database.dsn is required in system config")
	}
	if err := pgstore.RunMigrations(dsn, logger); err != nil {
		return nil, nil, nil, fmt.Errorf("run migrations: %w", err)
	}
	pool, err := pgstore.OpenPool(ctx, sys.Database)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open pg pool: %w", err)
	}
	st := pgstore.New(pool, logger)
	if err := seedPromptRegistryDefaults(ctx, st); err != nil {
		st.Close()
		return nil, nil, nil, fmt.Errorf("seed prompt registry defaults: %w", err)
	}
	closeFn := func() { st.Close() }
	return st, pool, closeFn, nil
}

func seedPromptRegistryDefaults(ctx context.Context, st store.PromptRegistryStore) error {
	if st == nil {
		return nil
	}
	defaults := config.PromptRegistryDefaults()
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		role, stage, ok := strings.Cut(key, "/")
		if !ok || strings.TrimSpace(role) == "" || strings.TrimSpace(stage) == "" {
			return fmt.Errorf("invalid prompt registry key %q", key)
		}
		entry := store.PromptRegistryEntry{
			Role:         role,
			Stage:        stage,
			Version:      "builtin",
			SystemPrompt: defaults[key],
			Description:  "Seeded from compiled-in prompt defaults.",
			Active:       true,
		}
		if err := st.SavePromptEntry(ctx, &entry); err != nil {
			return fmt.Errorf("save prompt %s: %w", key, err)
		}
	}
	return nil
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

func buildPositionServices(deps positionServiceBuildDeps) (*position.PositionCache, *position.RiskPlanService, *position.PositionService) {
	positionCache := position.NewPositionCache()
	riskPlanSvc := &position.RiskPlanService{Store: deps.store}
	positioner := &position.PositionService{
		Store:     deps.store,
		Executor:  deps.executor,
		Notifier:  deps.notifier,
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

func buildReconcileServices(deps reconcileServiceBuildDeps) (*reconcile.RecoveryService, *reconcile.ReconcileService, error) {
	recovery := &reconcile.RecoveryService{
		Store:       deps.store,
		Executor:    deps.executor,
		Notifier:    deps.notifier,
		Cache:       deps.positionCache,
		AllowSymbol: deps.allowSymbol,
	}
	reconciler := &reconcile.ReconcileService{
		Store:             deps.store,
		Executor:          deps.executor,
		Notifier:          deps.notifier,
		Cache:             deps.positionCache,
		PlanCache:         deps.planCache,
		RiskPlans:         deps.riskPlanSvc,
		CloseRecoverAfter: config.ParseDurationOrDefault(deps.sys.Reconcile.CloseRecoverAfter, 10*time.Minute),
		AllowSymbol:       deps.allowSymbol,
	}
	reconciler.OrderStatusFetcher = &execution.FreqtradeStatusFetcher{
		Endpoint:  deps.sys.ExecEndpoint,
		APIKey:    deps.sys.ExecAPIKey,
		APISecret: deps.sys.ExecAPISecret,
		AuthType:  deps.sys.ExecAuth,
	}
	reconciler.PriceSource = deps.priceSource
	reflector, err := buildPositionReflector(deps.sys, deps.symbolIndexPath, deps.index, deps.store)
	if err != nil {
		return nil, nil, fmt.Errorf("build position reflector: %w", err)
	}
	reconciler.Reflector = reflector
	return recovery, reconciler, nil
}

func buildRiskMonitor(deps riskMonitorBuildDeps) *position.RiskMonitor {
	return &position.RiskMonitor{
		Store:          deps.store,
		PriceSource:    deps.priceSource,
		Positions:      deps.positioner,
		PlanCache:      deps.positioner.PlanCache,
		MaxDrawdownPct: deps.sys.RiskGuard.MaxDrawdownPct,
		AccountFetcher: func(ctx context.Context, symbol string) (execution.AccountState, error) {
			return deps.accountFetcher(ctx, symbol)
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
		normalized := canonicalSymbolFromIndexEntry(item)
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	return func(sym string) bool {
		_, ok := allowed[canonicalSymbol(sym)]
		return ok
	}
}

func newFreqtradeAccountFetcher(executor *execution.FreqtradeAdapter) func(context.Context, string) (execution.AccountState, error) {
	return func(ctx context.Context, symbol string) (execution.AccountState, error) {
		quote, err := executor.Client.Balance(ctx)
		if err != nil {
			return execution.AccountState{}, err
		}
		return execution.AccountStateFromBalance(quote)
	}
}

func runScheduledWarmup(ctx context.Context, logger *zap.Logger, deps coreDeps) {
	if !deps.execution.scheduled {
		return
	}
	if err := deps.reconcile.recovery.RunOnce(ctx, ""); err != nil {
		logger.Warn("recovery run failed", zap.Error(err))
	}
	if err := deps.reconcile.reconciler.RunOnce(ctx, ""); err != nil {
		logger.Warn("reconcile warmup failed", zap.Error(err))
	}
}
