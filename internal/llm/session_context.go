package llm

import (
	"context"
	"strings"
)

type sessionRoundIDContextKey struct{}
type sessionSymbolContextKey struct{}
type sessionFlowContextKey struct{}
type sessionRequestIDContextKey struct{}

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

func WithSessionRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, sessionRequestIDContextKey{}, strings.TrimSpace(requestID))
}

func SessionRequestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	raw, ok := ctx.Value(sessionRequestIDContextKey{}).(string)
	if !ok {
		return "", false
	}
	requestID, err := normalizeSessionField("request_id", raw)
	if err != nil {
		return "", false
	}
	return requestID, true
}
