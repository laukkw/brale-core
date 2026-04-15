package llmapp

import (
	"context"
	"strings"

	"brale-core/internal/decision"
	"brale-core/internal/llm"
)

type stageCallCollector struct {
	stats llm.CallStats
	ok    bool
}

func (c *stageCallCollector) ObserveCall(_ context.Context, stats llm.CallStats) {
	c.stats = stats
	c.ok = true
}

func withPromptCallContext(ctx context.Context, symbol, role string, stage llm.LLMStage, promptVersion string, collector *stageCallCollector) context.Context {
	meta := llm.CallMetadata{
		Role:          strings.TrimSpace(role),
		Stage:         stage.String(),
		Symbol:        strings.TrimSpace(symbol),
		PromptVersion: strings.TrimSpace(promptVersion),
	}
	ctx = llm.WithCallMetadata(ctx, meta)
	if collector == nil {
		return ctx
	}
	existing, _ := llm.CallObserverFromContext(ctx)
	return llm.WithCallObserver(ctx, llm.ChainCallObservers(existing, collector))
}

func applyStageCallStats(prompt *decision.LLMStagePrompt, collector stageCallCollector) {
	if prompt == nil || !collector.ok {
		return
	}
	prompt.Model = collector.stats.Model
	prompt.LatencyMS = int(collector.stats.LatencyMs)
	prompt.TokenIn = collector.stats.TokenIn
	prompt.TokenOut = collector.stats.TokenOut
}
