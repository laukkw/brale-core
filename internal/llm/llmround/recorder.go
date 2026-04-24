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

	tokenBudget       int
	tokenBudgetWarnAt int
	budgetWarnFn      func(roundID string, totalTokens, warnThreshold, budget int)
	budgetWarned      bool
	budgetExceedFn    func(roundID string, totalTokens, budget int)
	budgetExceed      bool
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

// SetTokenBudget configures optional per-round token warnings.
// warnFn fires once when total tokens reach the warn threshold.
// exceedFn fires once when total tokens exceed the full budget.
func (r *Recorder) SetTokenBudget(budget int, warnPct int, warnFn func(roundID string, totalTokens, warnThreshold, budget int), exceedFn func(roundID string, totalTokens, budget int)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenBudget = budget
	r.tokenBudgetWarnAt = resolveTokenBudgetWarnThreshold(budget, warnPct)
	r.budgetWarnFn = warnFn
	r.budgetWarned = false
	r.budgetExceedFn = exceedFn
	r.budgetExceed = false
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
	if total > r.tokenBudget {
		if !r.budgetExceed {
			r.budgetExceed = true
			r.budgetWarned = true
			if r.budgetExceedFn != nil {
				r.budgetExceedFn(r.roundID, total, r.tokenBudget)
			}
		}
		return
	}
	if r.tokenBudgetWarnAt > 0 && total >= r.tokenBudgetWarnAt && !r.budgetWarned {
		r.budgetWarned = true
		if r.budgetWarnFn != nil {
			r.budgetWarnFn(r.roundID, total, r.tokenBudgetWarnAt, r.tokenBudget)
		}
	}
}

func resolveTokenBudgetWarnThreshold(budget int, warnPct int) int {
	if budget <= 0 || warnPct <= 0 {
		return 0
	}
	threshold := (budget*warnPct + 99) / 100
	if threshold <= 0 {
		return 0
	}
	return threshold
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
		RequestID:      sessionRequestID(ctx),
		CreatedAt:      finishedAt.UTC(),
	}
	return r.store.SaveLLMRound(ctx, rec)
}

func sessionRequestID(ctx context.Context) string {
	if requestID, ok := llm.SessionRequestIDFromContext(ctx); ok {
		return requestID
	}
	return ""
}
