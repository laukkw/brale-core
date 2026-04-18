// Package llmround provides round-level aggregation for LLM pipeline calls.
// Each pipeline round (observe or decide) records total latency, tokens, outcome, and prompt versions.
package llmround

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"brale-core/internal/llm"
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
	mu        sync.Mutex

	totalTokenIn  int
	totalTokenOut int
	callCount     int
	totalLatency  int64
	promptVersion map[string]struct{}
	snapshotID    uint
	agentCount    int
	providerCount int
	gateAction    string
	errMessage    string

	tokenBudget  int
	budgetWarnFn func(roundID string, totalTokens, budget int)
	budgetExceed bool
}

// NewRecorder starts tracking a new round.
func NewRecorder(s store.LLMRoundStore, roundID, symbol, roundType string) *Recorder {
	return &Recorder{
		store:         s,
		roundID:       roundID,
		symbol:        symbol,
		roundType:     roundType,
		startTime:     time.Now(),
		promptVersion: make(map[string]struct{}),
	}
}

// SetTokenBudget configures an optional per-round token budget.
// If budget > 0, the recorder will call warnFn when total tokens exceed the budget.
func (r *Recorder) SetTokenBudget(budget int, warnFn func(roundID string, totalTokens, budget int)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenBudget = budget
	r.budgetWarnFn = warnFn
}

// BudgetExceeded returns true if the round's token usage exceeded the budget.
func (r *Recorder) BudgetExceeded() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.budgetExceed
}

func (r *Recorder) checkBudget() {
	if r.tokenBudget <= 0 {
		return
	}
	total := r.totalTokenIn + r.totalTokenOut
	if total > r.tokenBudget && !r.budgetExceed {
		r.budgetExceed = true
		if r.budgetWarnFn != nil {
			r.budgetWarnFn(r.roundID, total, r.tokenBudget)
		}
	}
}

// RecordCall accumulates stats from an individual LLM call.
func (r *Recorder) RecordCall(tokenIn, tokenOut int, promptVersion string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalTokenIn += tokenIn
	r.totalTokenOut += tokenOut
	r.callCount++
	if promptVersion != "" {
		r.promptVersion[promptVersion] = struct{}{}
	}
	r.checkBudget()
}

func (r *Recorder) ObserveCall(_ context.Context, stats llm.CallStats) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalLatency += stats.LatencyMs
	r.totalTokenIn += stats.TokenIn
	r.totalTokenOut += stats.TokenOut
	r.callCount++
	if stats.PromptVersion != "" {
		r.promptVersion[stats.PromptVersion] = struct{}{}
	}
	if stats.Err != nil && r.errMessage == "" {
		r.errMessage = stats.Err.Error()
	}
	r.checkBudget()
}

func (r *Recorder) SetSnapshotID(snapshotID uint) {
	if r != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.snapshotID = snapshotID
	}
}

func (r *Recorder) SetRoundSummary(agentCount, providerCount int, gateAction string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agentCount = agentCount
	r.providerCount = providerCount
	r.gateAction = strings.TrimSpace(gateAction)
}

// TotalTokenIn returns accumulated input tokens (thread-safe).
func (r *Recorder) TotalTokenIn() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.totalTokenIn
}

// TotalTokenOut returns accumulated output tokens (thread-safe).
func (r *Recorder) TotalTokenOut() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.totalTokenOut
}

// Finish persists the round summary to the database.
func (r *Recorder) Finish(ctx context.Context, outcome string) error {
	if r == nil || r.store == nil {
		return nil
	}

	finishedAt := time.Now()
	r.mu.Lock()
	latencyMs := r.totalLatency
	if latencyMs <= 0 {
		latencyMs = finishedAt.Sub(r.startTime).Milliseconds()
	}
	versions := make([]string, 0, len(r.promptVersion))
	for version := range r.promptVersion {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	totalTokenIn := r.totalTokenIn
	totalTokenOut := r.totalTokenOut
	callCount := r.callCount
	snapshotID := r.snapshotID
	agentCount := r.agentCount
	providerCount := r.providerCount
	gateAction := r.gateAction
	errMessage := r.errMessage
	r.mu.Unlock()

	rec := &store.LLMRoundRecord{
		ID:             r.roundID,
		SnapshotID:     snapshotID,
		Symbol:         r.symbol,
		RoundType:      r.roundType,
		StartedAt:      r.startTime.UTC(),
		FinishedAt:     finishedAt.UTC(),
		TotalLatencyMS: int(latencyMs),
		TotalTokenIn:   totalTokenIn,
		TotalTokenOut:  totalTokenOut,
		CallCount:      callCount,
		Outcome:        outcome,
		PromptVersion:  strings.Join(versions, ","),
		Error:          errMessage,
		AgentCount:     agentCount,
		ProviderCount:  providerCount,
		GateAction:     gateAction,
		CreatedAt:      finishedAt.UTC(),
	}
	return r.store.SaveLLMRound(ctx, rec)
}
