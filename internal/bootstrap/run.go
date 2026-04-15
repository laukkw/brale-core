package bootstrap

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/jobs"
	braleOtel "brale-core/internal/otel"
	"brale-core/internal/runtime"
	"brale-core/internal/transport"
	"brale-core/internal/transport/notify"
	dashboard "brale-core/webui/dashboard"
	decisionview "brale-core/webui/decision-view"

	"go.uber.org/zap"
)

type startupNotifier interface {
	SendStartup(ctx context.Context, info notify.StartupInfo) error
	SendShutdown(ctx context.Context, info notify.ShutdownInfo) error
}

type Options struct {
	SystemPath      string
	SymbolIndexPath string
}

func Run(baseCtx context.Context, opts Options) error {
	startedAt := time.Now()
	systemPath := strings.TrimSpace(opts.SystemPath)
	if systemPath == "" {
		systemPath = "configs/system.toml"
	}
	symbolIndexPath := strings.TrimSpace(opts.SymbolIndexPath)
	if symbolIndexPath == "" {
		symbolIndexPath = "configs/symbols-index.toml"
	}

	env, err := bootstrapAppEnv(baseCtx, systemPath, symbolIndexPath)
	if err != nil {
		return err
	}
	deps, err := buildCoreDeps(env.ctx, env.logger, env)
	if err != nil {
		return err
	}
	if deps.closeDB != nil {
		defer deps.closeDB()
	}

	otelShutdown, otelErr := braleOtel.Init(env.ctx, env.sys.Telemetry, env.logger)
	if otelErr != nil {
		env.logger.Error("otel init failed, continuing without telemetry", zap.Error(otelErr))
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(shutdownCtx); err != nil {
				env.logger.Error("otel shutdown error", zap.Error(err))
			}
		}()
	}

	runScheduledWarmup(env.ctx, env.logger, deps)

	// Initialize River job client for async job processing.
	if migrateErr := jobs.RunMigrations(env.ctx, deps.persistence.pool); migrateErr != nil {
		env.logger.Error("river migration failed", zap.Error(migrateErr))
	}
	// Register workers with no-op executors for now; pipeline tasks still
	// run via the existing RuntimeScheduler.  River will process
	// notify render/deliver jobs once the notification pipeline is wired.
	noopExec := func(_ context.Context, _ string) error { return nil }
	riverWorkers := jobs.RegisterWorkers(noopExec, noopExec, noopExec, nil, nil)
	periodicJobs := jobs.BuildPeriodicJobs(nil, 0, 0, 0)
	riverClient, err := jobs.NewClient(env.ctx, deps.persistence.pool, riverWorkers, periodicJobs, env.logger)
	if err != nil {
		env.logger.Error("river client init failed", zap.Error(err))
	} else {
		if startErr := riverClient.Start(env.ctx); startErr != nil {
			env.logger.Error("river client start failed", zap.Error(startErr))
		} else {
			defer func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = riverClient.Stop(stopCtx)
			}()
		}
	}

	viewerHandler := decisionview.StartDecisionViewer(env.logger, env.sys, symbolIndexPath, env.index, deps.persistence.store)
	dashboardHandler := dashboard.Start()
	runtimes := buildRuntimeMap(env.ctx, env.logger, env.sys, symbolIndexPath, env.index, deps)
	runFreqtradeBalanceCheck(env.ctx, env.logger, deps)
	scheduler, err := startScheduler(env.ctx, env.logger, env.sys, deps, runtimes)
	if err != nil {
		return err
	}

	sendStartupNotify(env.ctx, env.logger, env.sys, env.index, runtimes, scheduler, deps, env.notifier)

	resolver := buildRuntimeResolver(env.ctx, env.logger, env.sys, symbolIndexPath, env.index, deps, runtimes)
	symbolConfigs := loadSymbolConfigs(env.logger, env.sys, symbolIndexPath, env.index)
	runtimeHandler, err := buildRuntimeHandler(env.sys, deps, scheduler, resolver, symbolConfigs)
	if err != nil {
		return fmt.Errorf("runtime api init failed: %w", err)
	}
	topMux := buildTopMux(viewerHandler, dashboardHandler, runtimeHandler)
	attachWebhookRoutes(env.ctx, env.logger, env.sys, deps, scheduler, topMux)

	addr := strings.TrimSpace(env.sys.Webhook.Addr)
	if addr == "" {
		return fmt.Errorf("http addr missing")
	}
	startFeishuBot(env.ctx, env.logger, env.sys, addr, topMux)
	transport.StartHTTPServer(env.ctx, addr, topMux, env.logger)
	startTelegramBot(env.ctx, env.logger, env.sys, addr)

	<-env.ctx.Done()
	sendShutdownNotify(env.logger, env.notifier, startedAt)
	return nil
}

func sendStartupNotify(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, index config.SymbolIndexConfig, runtimes map[string]runtime.SymbolRuntime, scheduler *runtime.RuntimeScheduler, deps coreDeps, notifier startupNotifier) {
	if !sys.Notification.StartupNotifyEnabled {
		return
	}
	info := buildStartupInfo(ctx, logger, index, runtimes, scheduler, deps)
	if err := notifier.SendStartup(ctx, info); err != nil {
		logger.Error("startup notify failed", zap.Error(err))
		return
	}
	logger.Info("startup notify sent")
}

func buildStartupInfo(ctx context.Context, logger *zap.Logger, index config.SymbolIndexConfig, runtimes map[string]runtime.SymbolRuntime, scheduler *runtime.RuntimeScheduler, deps coreDeps) notify.StartupInfo {
	symbols := make([]string, 0, len(index.Symbols))
	for _, entry := range index.Symbols {
		symbols = append(symbols, strings.TrimSpace(entry.Symbol))
	}
	intervals := collectIntervals(runtimes)
	barInterval := collectBarInterval(runtimes)
	info := notify.StartupInfo{
		Symbols:        symbols,
		Intervals:      intervals,
		BarInterval:    barInterval,
		ScheduleMode:   resolveStartupScheduleMode(deps.execution.scheduled),
		SymbolStatuses: collectStartupSymbolStatuses(index, runtimes, scheduler),
	}
	if deps.execution.freqtradeAcct != nil {
		balanceCtx := ctx
		if balanceCtx == nil {
			balanceCtx = context.Background()
		}
		accountCtx, cancel := context.WithTimeout(balanceCtx, 5*time.Second)
		defer cancel()
		symbolForAccount := ""
		if len(symbols) > 0 {
			symbolForAccount = symbols[0]
		}
		acct, err := deps.execution.freqtradeAcct(accountCtx, symbolForAccount)
		if err != nil {
			logger.Warn("startup balance fetch failed", zap.Error(err))
		} else {
			info.Balance = acct.Equity
			info.Currency = strings.TrimSpace(acct.Currency)
		}
	}
	return info
}

func resolveStartupScheduleMode(scheduled bool) string {
	if scheduled {
		return "定时调度"
	}
	return "手动/观察模式"
}

func collectStartupSymbolStatuses(index config.SymbolIndexConfig, runtimes map[string]runtime.SymbolRuntime, scheduler *runtime.RuntimeScheduler) []notify.StartupSymbolStatus {
	nextRunBySymbol := make(map[string]runtime.SymbolNextRun)
	if scheduler != nil {
		for _, item := range scheduler.GetScheduleStatus().NextRuns {
			nextRunBySymbol[strings.TrimSpace(item.Symbol)] = item
		}
	}
	statuses := make([]notify.StartupSymbolStatus, 0, len(index.Symbols))
	for _, entry := range index.Symbols {
		symbol := strings.TrimSpace(entry.Symbol)
		if symbol == "" {
			continue
		}
		rt, ok := runtimes[symbol]
		if !ok {
			continue
		}
		nextRun := nextRunBySymbol[symbol]
		statuses = append(statuses, notify.StartupSymbolStatus{
			Symbol:       symbol,
			Intervals:    slices.Clone(rt.Intervals),
			NextDecision: strings.TrimSpace(nextRun.NextExecution),
			Mode:         strings.TrimSpace(nextRun.Mode),
		})
	}
	return statuses
}

func sendShutdownNotify(logger *zap.Logger, notifier startupNotifier, startedAt time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info := notify.ShutdownInfo{
		Reason: "收到停止信号",
		Uptime: time.Since(startedAt),
	}
	if err := notifier.SendShutdown(ctx, info); err != nil {
		logger.Error("shutdown notify failed", zap.Error(err))
		return
	}
	logger.Info("shutdown notify sent")
}

func collectIntervals(runtimes map[string]runtime.SymbolRuntime) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, rt := range runtimes {
		for _, iv := range rt.Intervals {
			iv = strings.TrimSpace(iv)
			if iv == "" {
				continue
			}
			if _, ok := seen[iv]; ok {
				continue
			}
			seen[iv] = struct{}{}
			result = append(result, iv)
		}
	}
	return result
}

func collectBarInterval(runtimes map[string]runtime.SymbolRuntime) string {
	for _, rt := range runtimes {
		if rt.BarInterval > 0 {
			return rt.BarInterval.String()
		}
	}
	return ""
}
