package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/decision/decisionfmt"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/transport/notify"

	"go.uber.org/zap"
)

type appEnv struct {
	ctx             context.Context
	logger          *zap.Logger
	sys             config.SystemConfig
	index           config.SymbolIndexConfig
	symbolIndexPath string
	notifier        notify.Notifier
}

func bootstrapAppEnv(baseCtx context.Context, systemPath, symbolIndexPath string) (appEnv, error) {
	sys, err := config.LoadSystemConfig(systemPath)
	if err != nil {
		return appEnv{}, fmt.Errorf("load system config %s: %w", systemPath, err)
	}
	logPath := config.ResolveLogPath(sys)
	coreLogger, err := logging.NewLogger(sys.LogFormat, sys.LogLevel, logPath)
	if err != nil {
		return appEnv{}, fmt.Errorf("init logger: %w", err)
	}
	logging.SetLogger(coreLogger)
	logger := coreLogger.Named("bootstrap").With(
		zap.String("system_path", systemPath),
		zap.String("symbols_path", symbolIndexPath),
	)
	ctx := logging.WithLogger(baseCtx, logger)
	logger.Info("config loaded", zap.String("log_path", logPath), zap.Bool("database_configured", sys.Database.DSN != ""))
	if err := configureLLMConcurrency(sys, logger); err != nil {
		return appEnv{}, err
	}
	if err := configureLLMMinInterval(sys, logger); err != nil {
		return appEnv{}, err
	}
	notifier, err := notify.NewManager(notify.FromConfig(sys.Notification), decisionfmt.New())
	if err != nil {
		return appEnv{}, fmt.Errorf("init notifier: %w", err)
	}
	index, err := config.LoadSymbolIndexConfig(symbolIndexPath)
	if err != nil {
		return appEnv{}, fmt.Errorf("load symbols index config %s: %w", symbolIndexPath, err)
	}
	return appEnv{
		ctx:             ctx,
		logger:          logger,
		sys:             sys,
		index:           index,
		symbolIndexPath: symbolIndexPath,
		notifier:        notifier,
	}, nil
}

func configureLLMConcurrency(sys config.SystemConfig, logger *zap.Logger) error {
	const defaultLimit = 1
	modelLimits := make(map[string]int)
	for model, cfg := range sys.LLMModels {
		if cfg.Concurrency == nil {
			continue
		}
		modelLimits[model] = *cfg.Concurrency
	}
	llm.ConfigureModelConcurrency(defaultLimit, modelLimits)
	logger.Info("llm model concurrency configured", zap.Int("default_limit", defaultLimit), zap.Int("model_overrides", len(modelLimits)))
	return nil
}

func configureLLMMinInterval(sys config.SystemConfig, logger *zap.Logger) error {
	if strings.TrimSpace(sys.LLMMinInterval) == "" {
		return nil
	}
	interval, err := time.ParseDuration(strings.TrimSpace(sys.LLMMinInterval))
	if err != nil {
		return fmt.Errorf("invalid llm_min_interval: %w", err)
	}
	llm.ConfigureMinInterval(interval)
	logger.Info("llm min interval configured", zap.Duration("min_interval", interval))
	return nil
}
