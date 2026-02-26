package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/transport"
	decisionview "brale-core/webui/decision-view"
)

type Options struct {
	SystemPath      string
	SymbolIndexPath string
}

func Run(baseCtx context.Context, opts Options) error {
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

	viewerHandler := decisionview.StartDecisionViewer(env.logger, env.sys, symbolIndexPath, env.index, deps.store)
	runtimes := buildRuntimeMap(env.ctx, env.logger, env.sys, symbolIndexPath, env.index, deps)
	runFreqtradeBalanceCheck(env.ctx, env.logger, deps)
	scheduler, err := startScheduler(env.ctx, env.logger, env.sys, deps, runtimes)
	if err != nil {
		return err
	}
	resolver := buildRuntimeResolver(env.ctx, env.logger, env.sys, symbolIndexPath, env.index, deps, runtimes)
	symbolConfigs := loadSymbolConfigs(env.logger, env.sys, symbolIndexPath, env.index)
	runtimeHandler, err := buildRuntimeHandler(env.sys, deps, scheduler, resolver, symbolConfigs)
	if err != nil {
		return fmt.Errorf("runtime api init failed: %w", err)
	}
	topMux := buildTopMux(viewerHandler, runtimeHandler)
	attachWebhookRoutes(env.ctx, env.logger, env.sys, deps, scheduler, topMux)

	addr := strings.TrimSpace(env.sys.Webhook.Addr)
	if addr == "" {
		return fmt.Errorf("http addr missing")
	}
	transport.StartHTTPServer(env.ctx, addr, topMux, env.logger)
	startTelegramBot(env.ctx, env.logger, env.sys, addr)

	<-env.ctx.Done()
	return nil
}
