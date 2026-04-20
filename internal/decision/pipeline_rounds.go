package decision

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/llm"
	"brale-core/internal/llm/llmround"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

const (
	defaultRoundRecorderTimeout = 5 * time.Second
	defaultRoundRecorderRetries = 1
)

func (p *Pipeline) attachRoundRecorder(ctx context.Context, roundID llm.RoundID, roundType string, symbols []string) (context.Context, *llmround.Recorder) {
	if p == nil || p.store() == nil || len(symbols) == 0 {
		return ctx, nil
	}
	recorder := llmround.NewRecorder(p.store(), roundID.String(), summarizeRoundSymbols(symbols), roundType)
	if p.LLMTokenBudget > 0 {
		recorder.SetTokenBudget(
			p.LLMTokenBudget,
			p.LLMTokenBudgetWarnPct,
			func(roundID string, totalTokens, warnThreshold, budget int) {
				logging.L().Warn("LLM token budget warning",
					zap.String("round_id", roundID),
					zap.Int("total_tokens", totalTokens),
					zap.Int("warn_threshold", warnThreshold),
					zap.Int("budget", budget),
				)
			},
			func(roundID string, totalTokens, budget int) {
				logging.L().Warn("LLM token budget exceeded",
					zap.String("round_id", roundID),
					zap.Int("total_tokens", totalTokens),
					zap.Int("budget", budget),
				)
			},
		)
	}
	existing, _ := llm.CallObserverFromContext(ctx)
	return llm.WithCallObserver(ctx, llm.ChainCallObservers(existing, recorder)), recorder
}

func summarizeRoundSymbols(symbols []string) string {
	if len(symbols) == 0 {
		return ""
	}
	if len(symbols) == 1 {
		return decisionutil.NormalizeSymbol(symbols[0])
	}
	seen := make(map[string]struct{}, len(symbols))
	values := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		normalized := decisionutil.NormalizeSymbol(symbol)
		if normalized == "" {
			normalized = strings.TrimSpace(symbol)
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
	}
	sort.Strings(values)
	if len(values) <= 3 {
		return strings.Join(values, ",")
	}
	return "MULTI"
}

func applyRoundSummary(recorder *llmround.Recorder, snapID uint, results []SymbolResult) {
	if recorder == nil {
		return
	}
	if snapID > 0 {
		recorder.SetSnapshotID(snapID)
	}
	recorder.SetRoundSummary(countAgentPrompts(results), countProviderPrompts(results), summarizeGateActions(results))
}

func (p *Pipeline) finishRoundRecorder(ctx context.Context, recorder *llmround.Recorder, outcome string) error {
	if recorder == nil {
		return nil
	}
	return finishRoundRecorderWith(ctx, outcome, p.roundRecorderTimeout(), p.roundRecorderRetries(), func(callCtx context.Context, outcome string) error {
		return recorder.Finish(callCtx, outcome)
	})
}

func finishRoundRecorderWith(ctx context.Context, outcome string, timeout time.Duration, retries int, finish func(context.Context, string) error) error {
	if finish == nil {
		return nil
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	if timeout <= 0 {
		timeout = defaultRoundRecorderTimeout
	}
	if retries < 0 {
		retries = 0
	}
	attempts := retries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		finishCtx, cancel := context.WithTimeout(baseCtx, timeout)
		err := finish(finishCtx, outcome)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < attempts {
			logging.FromContext(ctx).Named("pipeline").Warn("llm round finish retrying",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", attempts),
				zap.Duration("timeout", timeout),
				zap.Error(err),
			)
		}
	}
	return fmt.Errorf("finish round recorder after %d attempts: %w", attempts, lastErr)
}

func (p *Pipeline) roundRecorderTimeout() time.Duration {
	if p != nil && p.RoundRecorderTimeoutSet && p.RoundRecorderTimeout > 0 {
		return p.RoundRecorderTimeout
	}
	return defaultRoundRecorderTimeout
}

func (p *Pipeline) roundRecorderRetries() int {
	if p != nil && p.RoundRecorderRetriesSet {
		return p.RoundRecorderRetries
	}
	return defaultRoundRecorderRetries
}

func countAgentPrompts(results []SymbolResult) int {
	total := 0
	for _, res := range results {
		total += countPromptSet(res.AgentPrompts)
	}
	return total
}

func countProviderPrompts(results []SymbolResult) int {
	total := 0
	for _, res := range results {
		total += countPromptSet(res.ProviderPrompts)
		total += countPromptSet(res.InPositionPrompts)
	}
	return total
}

func countPromptSet(set interface{}) int {
	switch typed := set.(type) {
	case AgentPromptSet:
		return countStagePrompt(typed.Indicator) + countStagePrompt(typed.Structure) + countStagePrompt(typed.Mechanics)
	case ProviderPromptSet:
		return countStagePrompt(typed.Indicator) + countStagePrompt(typed.Structure) + countStagePrompt(typed.Mechanics)
	default:
		return 0
	}
}

func countStagePrompt(prompt LLMStagePrompt) int {
	if strings.TrimSpace(prompt.System) == "" && strings.TrimSpace(prompt.User) == "" && strings.TrimSpace(prompt.Error) == "" {
		return 0
	}
	return 1
}

func summarizeGateActions(results []SymbolResult) string {
	if len(results) == 0 {
		return ""
	}
	seen := map[string]struct{}{}
	actions := make([]string, 0, len(results))
	for _, res := range results {
		action := strings.TrimSpace(res.Gate.DecisionAction)
		if action == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		actions = append(actions, action)
	}
	sort.Strings(actions)
	return strings.Join(actions, ",")
}
