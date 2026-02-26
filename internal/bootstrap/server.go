package bootstrap

import (
	"context"
	"net/http"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/runtime"
	"brale-core/internal/transport"
	"brale-core/internal/transport/telegrambot"
	"brale-core/internal/transport/webhook"

	"go.uber.org/zap"
)

func buildTopMux(viewerHandler http.Handler, runtimeHandler http.Handler) *http.ServeMux {
	topMux := http.NewServeMux()
	if viewerHandler != nil {
		topMux.Handle("/decision-view", viewerHandler)
		topMux.Handle("/decision-view/", viewerHandler)
	}
	if runtimeHandler != nil {
		topMux.Handle("/", runtimeHandler)
	}
	return topMux
}

func attachWebhookRoutes(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, deps coreDeps, scheduler *runtime.RuntimeScheduler, topMux *http.ServeMux) {
	if !deps.scheduled || !sys.Webhook.Enabled {
		return
	}
	webhookSvc := runtime.NewWebhookSyncService(sys.Webhook, scheduler)
	webhookSvc.AllowSymbol = deps.allowSymbol
	webhookSvc.ExecClient = deps.executor.Client
	webhookSvc.Notifier = deps.notifier
	webhookSvc.PosCache = deps.positionCache
	webhookSvc.Start(ctx)
	server := &webhook.Server{
		Addr:        sys.Webhook.Addr,
		Handler:     webhookSvc,
		Secret:      sys.Webhook.Secret,
		IPAllowlist: sys.Webhook.IPAllowlist,
	}
	mux, err := server.Mux()
	if err != nil {
		logger.Warn("webhook mux init failed", zap.Error(err))
		return
	}
	topMux.Handle("/healthz", mux)
	topMux.Handle("/api/live/freqtrade/webhook", mux)
}

func startTelegramBot(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, addr string) {
	if !sys.Notification.Enabled || !sys.Notification.Telegram.Enabled {
		return
	}
	botURL := transport.BuildRuntimeBaseURL(addr)
	bot, err := telegrambot.New(telegrambot.Config{
		Token:          sys.Notification.Telegram.Token,
		RuntimeBaseURL: botURL,
		SessionTTL:     5 * time.Minute,
	}, logger.Named("telegram-bot"))
	if err != nil {
		logger.Warn("telegram bot init failed", zap.Error(err))
		return
	}
	go func() {
		if err := bot.Run(ctx); err != nil {
			logger.Warn("telegram bot stopped", zap.Error(err))
		}
	}()
}
