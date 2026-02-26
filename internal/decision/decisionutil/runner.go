package decisionutil

import (
	"context"

	"brale-core/internal/llm"
)

func RunAndParse[T any](
	ctx context.Context,
	providerFor func(stage string) (llm.Provider, error),
	stage string,
	system string,
	user string,
	decode func(string) (T, error),
	onParseError func(raw string, err error),
) (T, error) {
	var zero T
	provider, err := providerFor(stage)
	if err != nil {
		return zero, err
	}
	raw, err := provider.Call(ctx, system, user)
	if err != nil {
		return zero, err
	}
	out, err := decode(raw)
	if err != nil {
		if onParseError != nil {
			onParseError(raw, err)
		}
		return zero, err
	}
	return out, nil
}
