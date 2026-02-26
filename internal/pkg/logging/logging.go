// Package logging provides a shared zap logger and context helpers.

package logging

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey struct{}

var global = zap.NewNop()

func NewLogger(format, level, logPath string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "json"
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.DisableStacktrace = true
	cfg.DisableCaller = true
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text", "console":
		cfg.Encoding = "console"
	}
	if lvl := strings.ToLower(strings.TrimSpace(level)); lvl != "" {
		if parsed, err := zapcore.ParseLevel(lvl); err == nil {
			cfg.Level = zap.NewAtomicLevelAt(parsed)
		}
	}
	if path := strings.TrimSpace(logPath); path != "" && path != "stdout" && path != "stderr" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		cfg.OutputPaths = append(cfg.OutputPaths, path)
		cfg.ErrorOutputPaths = append(cfg.ErrorOutputPaths, path)
	}

	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return logger.With(zap.String("svc", "brale-core")), nil
}

func SetLogger(l *zap.Logger) {
	if l == nil {
		global = zap.NewNop()
		return
	}
	global = l
}

func L() *zap.Logger {
	return global
}

func WithLogger(ctx context.Context, l *zap.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		l = L()
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

func With(ctx context.Context, fields ...zap.Field) context.Context {
	return WithLogger(ctx, FromContext(ctx).With(fields...))
}

func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return L()
	}
	if val := ctx.Value(ctxKey{}); val != nil {
		if l, ok := val.(*zap.Logger); ok && l != nil {
			return l
		}
	}
	return L()
}
