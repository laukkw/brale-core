// Package llmround provides round-level aggregation for LLM pipeline calls.
// Each pipeline round (observe or decide) records total latency, tokens, outcome, and prompt versions.
package llmround

import (
	"context"
	"time"

	"brale-core/internal/store"
)

// Recorder accumulates LLM call stats within a single pipeline round
// and persists the aggregate to llm_rounds on completion.
type Recorder struct {
	store     store.LLMRoundStore
	roundID   string
	symbol    string
	roundType string // "observe" | "decide" | "risk"
	startTime time.Time

	totalTokenIn  int
	totalTokenOut int
	callCount     int
	promptVersion string
}

// NewRecorder starts tracking a new round.
func NewRecorder(s store.LLMRoundStore, roundID, symbol, roundType string) *Recorder {
	return &Recorder{
		store:     s,
		roundID:   roundID,
		symbol:    symbol,
		roundType: roundType,
		startTime: time.Now(),
	}
}

// RecordCall accumulates stats from an individual LLM call.
func (r *Recorder) RecordCall(tokenIn, tokenOut int, promptVersion string) {
	r.totalTokenIn += tokenIn
	r.totalTokenOut += tokenOut
	r.callCount++
	if promptVersion != "" {
		r.promptVersion = promptVersion
	}
}

// Finish persists the round summary to the database.
func (r *Recorder) Finish(ctx context.Context, outcome string) error {
	if r.store == nil {
		return nil
	}

	latencyMs := time.Since(r.startTime).Milliseconds()

	rec := &store.LLMRoundRecord{
		ID:             r.roundID,
		Symbol:         r.symbol,
		RoundType:      r.roundType,
		TotalLatencyMS: int(latencyMs),
		TotalTokenIn:   r.totalTokenIn,
		TotalTokenOut:  r.totalTokenOut,
		CallCount:      r.callCount,
		Outcome:        outcome,
		PromptVersion:  r.promptVersion,
		CreatedAt:      time.Now(),
	}
	return r.store.SaveLLMRound(ctx, rec)
}
