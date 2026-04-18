package decision

import (
	"context"
	"sort"
	"strings"
	"time"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/llm"
	"brale-core/internal/llm/llmround"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

const detachedRoundRecorderTimeout = 5 * time.Second

func (p *Pipeline) attachRoundRecorder(ctx context.Context, roundID llm.RoundID, roundType string, symbols []string) (context.Context, *llmround.Recorder) {
	if p == nil || p.store() == nil || len(symbols) == 0 {
		return ctx, nil
	}
	recorder := llmround.NewRecorder(p.store(), roundID.String(), summarizeRoundSymbols(symbols), roundType)
	if p.LLMTokenBudget > 0 {
		recorder.SetTokenBudget(p.LLMTokenBudget, func(roundID string, totalTokens, budget int) {
			logging.L().Error("LLM token budget exceeded",
				zap.String("round_id", roundID),
				zap.Int("total_tokens", totalTokens),
				zap.Int("budget", budget),
			)
		})
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

func finishRoundRecorder(ctx context.Context, recorder *llmround.Recorder, outcome string) error {
	if recorder == nil {
		return nil
	}
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	finishCtx, cancel := context.WithTimeout(baseCtx, detachedRoundRecorderTimeout)
	defer cancel()
	return recorder.Finish(finishCtx, outcome)
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
