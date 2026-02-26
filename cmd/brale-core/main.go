package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"brale-core/internal/bootstrap"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	systemPath, symbolIndexPath := parseBootstrapPaths()
	setupFallbackLogger()
	defer func() { _ = logging.L().Sync() }()

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(baseCtx, bootstrap.Options{SystemPath: systemPath, SymbolIndexPath: symbolIndexPath}); err != nil {
		fatalIfErr(logging.L(), err, "fatal: run brale-core")
	}
}

func parseBootstrapPaths() (string, string) {
	systemPath := flag.String("system", "configs/system.toml", "system config path")
	symbolIndexPath := flag.String("symbols", "configs/symbols-index.toml", "symbols index config path")
	flag.Parse()
	return *systemPath, *symbolIndexPath
}

func setupFallbackLogger() {
	fallback, _ := logging.NewLogger("text", "debug", "")
	logging.SetLogger(fallback)
}

func fatalIfErr(logger *zap.Logger, err error, message string, fields ...zap.Field) {
	if err == nil {
		return
	}
	logger.Error(message, append(fields, zap.Error(err))...)
	os.Exit(1)
}
