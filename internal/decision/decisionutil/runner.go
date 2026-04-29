package decisionutil

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type RetryPromptBuilder func(originalUser string, raw string, err error) string

type runConfig struct {
	retryOnParseFail bool
	retryPrompt      RetryPromptBuilder
	retryTimeout     time.Duration
}

type RunOption func(*runConfig)

func WithRetryOnParseFail(builder RetryPromptBuilder) RunOption {
	return func(cfg *runConfig) {
		cfg.retryOnParseFail = builder != nil
		cfg.retryPrompt = builder
		if cfg.retryTimeout <= 0 {
			cfg.retryTimeout = 30 * time.Second
		}
	}
}

func DefaultRetryPrompt(originalUser, raw string, err error) string {
	truncated := raw
	if len(truncated) > 500 {
		truncated = truncated[:500] + "..."
	}
	return fmt.Sprintf(
		"你上一次的输出解析失败。\n错误：%s\n你的输出（截断）：%s\n\n请严格按照 Schema 重新输出 JSON。\n\n原始输入：\n%s",
		err.Error(),
		truncated,
		originalUser,
	)
}

func RunAndParse[T any](
	ctx context.Context,
	providerFor func(stage string) (llm.Provider, error),
	stage string,
	system string,
	user string,
	decode func(string) (T, error),
	onParseError func(raw string, err error),
	opts ...RunOption,
) (T, error) {
	var zero T
	cfg := runConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.retryOnParseFail && cfg.retryTimeout <= 0 {
		cfg.retryTimeout = 30 * time.Second
	}

	provider, err := providerFor(stage)
	if err != nil {
		return zero, err
	}

	invoke := func(callCtx context.Context, reqSystem, reqUser string) (string, error) {
		return provider.Call(callCtx, reqSystem, reqUser)
	}
	parse := decode
	if sp, ok := provider.(llm.StructuredProvider); ok {
		schema := llm.SchemaFromType[T]()
		invoke = func(callCtx context.Context, reqSystem, reqUser string) (string, error) {
			return sp.CallStructured(callCtx, reqSystem, reqUser, schema)
		}
	}

	return runAndParseWithStrategy(ctx, stage, system, user, invoke, parse, onParseError, cfg)
}

func runAndParseWithStrategy[T any](
	ctx context.Context,
	stage string,
	system string,
	user string,
	invoke func(context.Context, string, string) (string, error),
	parse func(string) (T, error),
	onParseError func(raw string, err error),
	cfg runConfig,
) (T, error) {
	var zero T

	raw, err := invoke(ctx, system, user)
	if err != nil {
		return zero, err
	}

	out, err := parse(raw)
	if err == nil {
		return out, nil
	}
	if onParseError != nil {
		onParseError(raw, err)
	}
	if !cfg.retryOnParseFail || cfg.retryPrompt == nil {
		return zero, err
	}

	logging.FromContext(ctx).Warn("llm parse retry triggered",
		zap.String("stage", stage),
		zap.Int("attempt", 2),
		zap.String("first_err", err.Error()),
	)

	retryCtx, cancel := context.WithTimeout(ctx, cfg.retryTimeout)
	defer cancel()

	retryUser := cfg.retryPrompt(user, raw, err)
	rawRetry, retryErr := invoke(retryCtx, system, retryUser)
	if retryErr != nil {
		return zero, fmt.Errorf("retry call: %w", retryErr)
	}

	outRetry, retryErr := parse(rawRetry)
	if retryErr != nil {
		if onParseError != nil {
			onParseError(rawRetry, retryErr)
		}
		return zero, fmt.Errorf("retry parse: %w", retryErr)
	}
	return outRetry, nil
}
