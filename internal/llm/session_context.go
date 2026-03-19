package llm

import (
	"context"
	"fmt"
	"strings"
)

type sessionRoundIDContextKey struct{}
type sessionSymbolContextKey struct{}
type sessionFlowContextKey struct{}

func WithSessionRoundID(ctx context.Context, roundID RoundID) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionRoundIDContextKey{}, roundID.String())
}

func SessionRoundIDFromContext(ctx context.Context) (RoundID, bool) {
	if ctx == nil {
		return "", false
	}
	raw, ok := ctx.Value(sessionRoundIDContextKey{}).(string)
	if !ok {
		return "", false
	}
	roundID, err := NewRoundID(raw)
	if err != nil {
		return "", false
	}
	return roundID, true
}

func WithSessionSymbol(ctx context.Context, symbol string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionSymbolContextKey{}, strings.TrimSpace(symbol))
}

func SessionSymbolFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	raw, ok := ctx.Value(sessionSymbolContextKey{}).(string)
	if !ok {
		return "", false
	}
	symbol, err := normalizeSessionField("symbol", raw)
	if err != nil {
		return "", false
	}
	return symbol, true
}

func WithSessionFlow(ctx context.Context, flow LLMFlow) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionFlowContextKey{}, flow.String())
}

func SessionFlowFromContext(ctx context.Context) (LLMFlow, bool) {
	if ctx == nil {
		return "", false
	}
	raw, ok := ctx.Value(sessionFlowContextKey{}).(string)
	if !ok {
		return "", false
	}
	flow, err := NewLLMFlow(raw)
	if err != nil {
		return "", false
	}
	return flow, true
}

func RoundLaneKeyFromContext(ctx context.Context, stage LLMStage) (RoundLaneKey, error) {
	roundID, ok := SessionRoundIDFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("round_id missing from context")
	}
	symbol, ok := SessionSymbolFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("symbol missing from context")
	}
	flow, ok := SessionFlowFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("flow missing from context")
	}
	return NewRoundLaneKey(roundID, symbol, flow, stage)
}
