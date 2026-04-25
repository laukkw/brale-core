package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"brale-core/internal/bootstrap"
	"brale-core/internal/config"
	"brale-core/internal/pgstore"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

func main() {
	opts, err := parseCLIOptions(os.Args[1:])
	setupFallbackLogger()
	defer func() { _ = logging.L().Sync() }()
	fatalIfErr(logging.L(), err, "fatal: parse cli options")

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if opts.Migration != migrationNone {
		fatalIfErr(logging.L(), runMigrationCommand(baseCtx, opts.SystemPath, opts.Migration), "fatal: migrate brale-core")
		return
	}

	if err := bootstrap.Run(baseCtx, bootstrap.Options{SystemPath: opts.SystemPath, SymbolIndexPath: opts.SymbolIndexPath}); err != nil {
		fatalIfErr(logging.L(), err, "fatal: run brale-core")
	}
}

type migrationMode string

const (
	migrationNone migrationMode = ""
	migrationUp   migrationMode = "up"
)

type cliOptions struct {
	SystemPath      string
	SymbolIndexPath string
	Migration       migrationMode
}

func parseCLIOptions(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("brale-core", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	systemPath := fs.String("system", "configs/system.toml", "system config path")
	symbolIndexPath := fs.String("symbols", "configs/symbols-index.toml", "symbols index config path")
	migrateUp := fs.Bool("migrate-up", false, "apply pending database migrations and exit")
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	migration := migrationNone
	if *migrateUp {
		migration = migrationUp
	}
	return cliOptions{
		SystemPath:      *systemPath,
		SymbolIndexPath: *symbolIndexPath,
		Migration:       migration,
	}, nil
}

func runMigrationCommand(ctx context.Context, systemPath string, mode migrationMode) error {
	sys, err := config.LoadSystemConfig(systemPath)
	if err != nil {
		return fmt.Errorf("load system config: %w", err)
	}
	switch mode {
	case migrationUp:
		return pgstore.RunMigrations(sys.Database.DSN, logging.L())
	default:
		return nil
	}
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
