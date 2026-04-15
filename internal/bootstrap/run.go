package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/config"
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
	deps, err := buildCoreDeps(env)
	if err != nil {
		return err
	}
	runScheduledWarmup(env.ctx, env.logger, deps)

	viewerHandler := decisionview.StartDecisionViewer(env.logger, env.sys, symbolIndexPath, env.index, deps.persistence.store)
	dashboardHandler := dashboard.Start()
	runtimes := buildRuntimeMap(env.ctx, env.logger, env.sys, symbolIndexPath, env.index, deps)
	runFreqtradeBalanceCheck(env.ctx, env.logger, deps)
	scheduler, err := startScheduler(env.ctx, env.logger, env.sys, deps, runtimes)
	if err != nil {
		return err
	}

	sendStartupNotify(env.ctx, env.logger, env.sys, env.index, runtimes, env.notifier)

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

func sendStartupNotify(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, index config.SymbolIndexConfig, runtimes map[string]runtime.SymbolRuntime, notifier startupNotifier) {
	if !sys.Notification.StartupNotifyEnabled {
		return
	}
	symbols := make([]string, 0, len(index.Symbols))
	for _, entry := range index.Symbols {
		symbols = append(symbols, strings.TrimSpace(entry.Symbol))
	}
	intervals := collectIntervals(runtimes)
	barInterval := collectBarInterval(runtimes)

	info := notify.StartupInfo{
		Symbols:      symbols,
		Intervals:    intervals,
		BarInterval:  barInterval,
		ScheduleMode: "自动调度",
	}
	if err := notifier.SendStartup(ctx, info); err != nil {
		logger.Error("startup notify failed", zap.Error(err))
		return
	}
	logger.Info("startup notify sent")
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
