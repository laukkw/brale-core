package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/runtime"
	"brale-core/internal/transport"
	"brale-core/internal/transport/feishubot"
	"brale-core/internal/transport/telegrambot"
	"brale-core/internal/transport/webhook"

	"go.uber.org/zap"
)

type feishuBotRunner interface {
	Handler() http.Handler
	RunLongConnection(context.Context) error
}

var newFeishuBot = func(cfg feishubot.Config, logger *zap.Logger) (feishuBotRunner, error) {
	return feishubot.New(cfg, logger)
}

func buildTopMux(viewerHandler http.Handler, dashboardHandler http.Handler, runtimeHandler http.Handler) *http.ServeMux {
	topMux := http.NewServeMux()
	if viewerHandler != nil {
		topMux.Handle("/decision-view", viewerHandler)
		topMux.Handle("/decision-view/", viewerHandler)
	}
	if dashboardHandler != nil {
		topMux.Handle("/dashboard", dashboardHandler)
		topMux.Handle("/dashboard/", dashboardHandler)
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

func startFeishuBot(ctx context.Context, logger *zap.Logger, sys config.SystemConfig, addr string, topMux *http.ServeMux) {
	if !sys.Notification.Enabled || !sys.Notification.Feishu.BotEnabled {
		return
	}
	mode := resolveFeishuBotMode(sys.Notification.Feishu.BotMode)
	runtimeBaseURL := transport.BuildRuntimeBaseURL(addr)
	bot, err := newFeishuBot(feishubot.Config{
		AppID:             strings.TrimSpace(sys.Notification.Feishu.AppID),
		AppSecret:         strings.TrimSpace(sys.Notification.Feishu.AppSecret),
		Mode:              mode,
		VerificationToken: strings.TrimSpace(sys.Notification.Feishu.VerificationToken),
		EncryptKey:        strings.TrimSpace(sys.Notification.Feishu.EncryptKey),
		RuntimeBaseURL:    runtimeBaseURL,
		SessionTTL:        5 * time.Minute,
		IdempotencyTTL:    15 * time.Minute,
	}, logger.Named("feishu-bot"))
	if err != nil {
		logger.Warn("feishu bot init failed", zap.Error(err))
		return
	}
	if mode == feishubot.ModeCallback {
		topMux.Handle("/api/feishu/events", bot.Handler())
		logger.Info("feishu bot started", zap.String("mode", mode), zap.String("path", "/api/feishu/events"))
		return
	}
	logger.Info("feishu bot started", zap.String("mode", mode))
	go func() {
		if err := bot.RunLongConnection(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("feishu long connection stopped", zap.Error(err))
		}
	}()
}

func resolveFeishuBotMode(value string) string {
	mode := strings.TrimSpace(strings.ToLower(value))
	if mode == "" {
		return feishubot.ModeLongConnection
	}
	return mode
}
