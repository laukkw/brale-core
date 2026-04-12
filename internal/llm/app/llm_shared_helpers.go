package llmapp

import (
	"context"
	"encoding/json"
	"strings"

	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

func cacheStageOutput(cache *LLMStageCache, symbol, stage string, out any, input []byte) {
	if cache == nil {
		return
	}
	cache.Store(symbol, stage, out, input)
}

func logLLMLaneCall(ctx context.Context, scope string, stage llm.LLMStage, mode string, sessionID string, reused bool, fallbackReason string) {
	fields := []zap.Field{
		zap.String("stage", stage.String()),
		zap.String("session_mode", strings.TrimSpace(mode)),
		zap.String("session_id", strings.TrimSpace(sessionID)),
		zap.Bool("session_reused", reused),
		zap.String("fallback_reason", strings.TrimSpace(fallbackReason)),
	}
	if roundID, ok := llm.SessionRoundIDFromContext(ctx); ok {
		fields = append(fields, zap.String("round_id", roundID.String()))
	}
	if symbol, ok := llm.SessionSymbolFromContext(ctx); ok {
		fields = append(fields, zap.String("symbol", symbol))
	}
	if flow, ok := llm.SessionFlowFromContext(ctx); ok {
		fields = append(fields, zap.String("flow", flow.String()))
	}
	logging.FromContext(ctx).Named("decision").Info(strings.TrimSpace(scope)+" lane session", fields...)
}

func marshalExample(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
