// 本文件主要内容：实现 LLM Agent 调用与缓存编排。
package llmapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"brale-core/internal/decision"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/features"
	"brale-core/internal/interval"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parallel"

	"go.uber.org/zap"
)

type LLMAgentService struct {
	Runner  *agent.Runner
	Prompts LLMPromptBuilder
	Cache   *LLMStageCache
	Tracker *LLMRunTracker
}

func (s LLMAgentService) Analyze(ctx context.Context, symbol string, data features.CompressionResult, enabled decision.AgentEnabled) (agent.IndicatorSummary, agent.StructureSummary, agent.MechanicsSummary, decision.AgentPromptSet, error) {
	if err := s.ensureRunner(ctx); err != nil {
		return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, decision.AgentPromptSet{}, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, decision.AgentPromptSet{}, ctxErr
	}
	inputs, stageErrs := s.pickInputs(ctx, data, symbol, enabled)
	var indOut agent.IndicatorSummary
	var stOut agent.StructureSummary
	var mechOut agent.MechanicsSummary
	var indPrompt decision.LLMStagePrompt
	var stPrompt decision.LLMStagePrompt
	var mechPrompt decision.LLMStagePrompt
	prompts := decision.AgentPromptSet{}
	tasks := make([]func(context.Context) error, 0, 3)
	if enabled.Indicator {
		if err := stageErrs["indicator"]; err != nil {
			indPrompt = decision.LLMStagePrompt{Error: err.Error()}
		} else {
			tasks = append(tasks, func(runCtx context.Context) error {
				var stageErr error
				indOut, indPrompt, stageErr = s.runIndicatorStage(runCtx, symbol, inputs.indicator)
				return stageErr
			})
		}
	}
	if enabled.Structure {
		if err := stageErrs["structure"]; err != nil {
			stPrompt = decision.LLMStagePrompt{Error: err.Error()}
		} else {
			tasks = append(tasks, func(runCtx context.Context) error {
				var stageErr error
				stOut, stPrompt, stageErr = s.runStructureStage(runCtx, symbol, inputs.trend)
				return stageErr
			})
		}
	}
	if enabled.Mechanics {
		if err := stageErrs["mechanics"]; err != nil {
			mechPrompt = decision.LLMStagePrompt{Error: err.Error()}
		} else {
			tasks = append(tasks, func(runCtx context.Context) error {
				var stageErr error
				mechOut, mechPrompt, stageErr = s.runMechanicsStage(runCtx, symbol, inputs.mechanics)
				return stageErr
			})
		}
	}
	parallel.RunBestEffort(ctx, tasks...)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return agent.IndicatorSummary{}, agent.StructureSummary{}, agent.MechanicsSummary{}, decision.AgentPromptSet{}, ctxErr
	}
	if enabled.Indicator {
		prompts.Indicator = indPrompt
	}
	if enabled.Structure {
		prompts.Structure = stPrompt
	}
	if enabled.Mechanics {
		prompts.Mechanics = mechPrompt
	}
	return indOut, stOut, mechOut, prompts, nil
}

type llmAgentInputs struct {
	indicator features.IndicatorJSON
	trend     features.TrendJSON
	mechanics features.MechanicsSnapshot
}

func (s LLMAgentService) ensureRunner(ctx context.Context) error {
	if s.Runner != nil {
		return nil
	}
	return s.logStageError(ctx, "init", fmt.Errorf("runner is required"))
}

func (s LLMAgentService) pickInputs(ctx context.Context, data features.CompressionResult, symbol string, enabled decision.AgentEnabled) (llmAgentInputs, map[string]error) {
	var inputs llmAgentInputs
	stageErrs := make(map[string]error)
	var ok bool
	if enabled.Indicator {
		inputs.indicator, ok = decision.PickIndicatorJSON(data, symbol)
		if !ok {
			stageErrs["indicator"] = s.logStageError(ctx, "indicator", fmt.Errorf("indicator json missing"))
		}
	}
	if enabled.Structure {
		inputs.trend, ok = pickTrendJSONMulti(data, symbol)
		if !ok {
			stageErrs["structure"] = s.logStageError(ctx, "structure", fmt.Errorf("trend json missing"))
		}
	}
	if enabled.Mechanics {
		inputs.mechanics, ok = decision.PickMechanicsJSON(data, symbol)
		if !ok {
			stageErrs["mechanics"] = s.logStageError(ctx, "mechanics", fmt.Errorf("mechanics json missing"))
		}
	}
	return inputs, stageErrs
}

func pickTrendJSONMulti(data features.CompressionResult, symbol string) (features.TrendJSON, bool) {
	byInterval, ok := data.Trends[symbol]
	if !ok || len(byInterval) == 0 {
		return features.TrendJSON{}, false
	}
	keys := sortedTrendIntervals(byInterval)
	intervalOrder := make(map[string]int, len(keys))
	for i, key := range keys {
		intervalOrder[key] = i
	}
	blocks := make([]json.RawMessage, 0, len(keys))
	var latestBreak *latestBreakAcrossBlocks
	for _, iv := range keys {
		raw := bytes.TrimSpace(byInterval[iv].RawJSON)
		if len(raw) == 0 {
			continue
		}
		if candidate, ok := extractLatestBreakAcrossBlocks(iv, raw); ok {
			latestBreak = selectLatestBreakAcrossBlocks(latestBreak, candidate, intervalOrder)
		}
		blocks = append(blocks, json.RawMessage(raw))
	}
	if len(blocks) == 0 {
		return features.TrendJSON{}, false
	}
	meta := trendMultiMeta{Symbol: strings.ToUpper(strings.TrimSpace(symbol))}
	payload := trendMultiInput{Meta: meta, Blocks: blocks, LatestBreakAcrossBlocks: latestBreak}
	raw, err := json.Marshal(payload)
	if err != nil {
		return features.TrendJSON{}, false
	}
	return features.TrendJSON{Symbol: symbol, Interval: "multi", RawJSON: raw}, true
}

type trendMultiMeta struct {
	Symbol string `json:"symbol"`
}

type trendMultiInput struct {
	Meta                    trendMultiMeta           `json:"meta"`
	Blocks                  []json.RawMessage        `json:"blocks"`
	LatestBreakAcrossBlocks *latestBreakAcrossBlocks `json:"latest_break_across_blocks,omitempty"`
}

type latestBreakAcrossBlocks struct {
	Interval   string   `json:"interval"`
	Type       string   `json:"type"`
	Age        int      `json:"age"`
	LevelPrice *float64 `json:"level_price,omitempty"`
	LevelIdx   *int     `json:"level_idx,omitempty"`
	BarIdx     *int     `json:"bar_idx,omitempty"`
}

func extractLatestBreakAcrossBlocks(interval string, raw []byte) (*latestBreakAcrossBlocks, bool) {
	var block features.TrendCompressedInput
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, false
	}
	if block.BreakSummary == nil {
		return nil, false
	}
	if block.BreakSummary.LatestEventType == "" || block.BreakSummary.LatestEventType == "none" {
		return nil, false
	}
	if block.BreakSummary.LatestEventAge == nil {
		return nil, false
	}
	candidate := &latestBreakAcrossBlocks{
		Interval: interval,
		Type:     block.BreakSummary.LatestEventType,
		Age:      *block.BreakSummary.LatestEventAge,
	}
	if block.BreakSummary.LatestEventLevelPrice != nil {
		candidate.LevelPrice = block.BreakSummary.LatestEventLevelPrice
	}
	if block.BreakSummary.LatestEventLevelIdx != nil {
		candidate.LevelIdx = block.BreakSummary.LatestEventLevelIdx
	}
	if block.BreakSummary.LatestEventBarIdx != nil {
		candidate.BarIdx = block.BreakSummary.LatestEventBarIdx
	}
	return candidate, true
}

func selectLatestBreakAcrossBlocks(current *latestBreakAcrossBlocks, candidate *latestBreakAcrossBlocks, order map[string]int) *latestBreakAcrossBlocks {
	if candidate == nil {
		return current
	}
	if current == nil {
		return candidate
	}
	if candidate.Age < current.Age {
		return candidate
	}
	if candidate.Age > current.Age {
		return current
	}
	currentOrder, okCurrent := order[current.Interval]
	candidateOrder, okCandidate := order[candidate.Interval]
	if !okCurrent || !okCandidate {
		return current
	}
	if candidateOrder < currentOrder {
		return candidate
	}
	return current
}

func sortedTrendIntervals(m map[string]features.TrendJSON) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		di, errI := interval.ParseInterval(keys[i])
		dj, errJ := interval.ParseInterval(keys[j])
		if errI != nil || errJ != nil {
			return keys[i] < keys[j]
		}
		return di < dj
	})
	return keys
}

func (s LLMAgentService) runIndicatorStage(ctx context.Context, symbol string, indJSON features.IndicatorJSON) (agent.IndicatorSummary, decision.LLMStagePrompt, error) {
	sysInd, userInd, err := s.Prompts.AgentIndicatorPrompt(indJSON)
	if err != nil {
		stageErr := s.logStageError(ctx, "indicator", err)
		return agent.IndicatorSummary{}, decision.LLMStagePrompt{Error: stageErr.Error()}, stageErr
	}
	prompt := decision.LLMStagePrompt{System: sysInd, User: userInd}
	if out, ok := loadAgentCache(s.Cache, symbol, "agent_indicator", indJSON.RawJSON); ok {
		return out, prompt, nil
	}
	indOut, err := s.Runner.RunIndicator(ctx, sysInd, userInd)
	if err != nil {
		stageErr := s.logStageError(ctx, "indicator", err)
		prompt.Error = stageErr.Error()
		return agent.IndicatorSummary{}, prompt, stageErr
	}
	if s.Tracker != nil {
		s.Tracker.MarkAgent()
	}
	cacheAgentOutput(s.Cache, symbol, "agent_indicator", indOut, indJSON.RawJSON)
	return indOut, prompt, nil
}

func (s LLMAgentService) runStructureStage(ctx context.Context, symbol string, trJSON features.TrendJSON) (agent.StructureSummary, decision.LLMStagePrompt, error) {
	trInput := normalizeTrendInput(trJSON.RawJSON)
	trPrompt := trJSON
	trPrompt.RawJSON = trInput
	sysSt, userSt, err := s.Prompts.AgentStructurePrompt(trPrompt)
	if err != nil {
		stageErr := s.logStageError(ctx, "structure", err)
		return agent.StructureSummary{}, decision.LLMStagePrompt{Error: stageErr.Error()}, stageErr
	}
	input := append([]byte{}, trInput...)
	prompt := decision.LLMStagePrompt{System: sysSt, User: userSt}
	if out, ok := loadStructureCache(s.Cache, symbol, "agent_structure", input); ok {
		return out, prompt, nil
	}
	stOut, err := s.Runner.RunStructure(ctx, sysSt, userSt)
	if err != nil {
		stageErr := s.logStageError(ctx, "structure", err)
		prompt.Error = stageErr.Error()
		return agent.StructureSummary{}, prompt, stageErr
	}
	if s.Tracker != nil {
		s.Tracker.MarkAgent()
	}
	cacheAgentOutput(s.Cache, symbol, "agent_structure", stOut, input)
	return stOut, prompt, nil
}

func normalizeTrendInput(raw []byte) []byte {
	payload, ok := decodeJSON(raw)
	if !ok {
		return raw
	}
	normalizeTrendPayload(payload)
	normalized, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return normalized
}

func decodeJSON(raw []byte) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func normalizeTrendPayload(payload any) {
	switch typed := payload.(type) {
	case map[string]any:
		if blocks, ok := typed["blocks"].([]any); ok {
			normalizeTrendBlocks(blocks)
			return
		}
		if _, ok := typed["global_context"]; ok {
			stripTrendFields(typed)
			return
		}
		for _, value := range typed {
			if item, ok := value.(map[string]any); ok {
				stripTrendFields(item)
			}
		}
	case []any:
		normalizeTrendBlocks(typed)
	}
}

func normalizeTrendBlocks(blocks []any) {
	for _, value := range blocks {
		if item, ok := value.(map[string]any); ok {
			stripTrendFields(item)
		}
	}
}

func stripTrendFields(obj map[string]any) {
	stripTrendMeta(obj)
	stripTrendPattern(obj)
	stripTrendGlobalContext(obj)
}

func stripTrendMeta(obj map[string]any) {
	meta, ok := obj["meta"].(map[string]any)
	if !ok {
		return
	}
	delete(meta, "symbol")
	if len(meta) == 0 {
		delete(obj, "meta")
	}
}

func stripTrendPattern(obj map[string]any) {
	pattern, ok := obj["pattern"].(map[string]any)
	if !ok {
		return
	}
	delete(pattern, "primary")
	if len(pattern) == 0 {
		delete(obj, "pattern")
	}
}

func stripTrendGlobalContext(obj map[string]any) {
	gc, ok := obj["global_context"].(map[string]any)
	if !ok {
		return
	}
	for _, key := range []string{"trend_slope", "normalized_slope", "ema20", "ema50", "ema200", "vol_ratio", "window"} {
		delete(gc, key)
	}
	if len(gc) == 0 {
		delete(obj, "global_context")
	}
}

func (s LLMAgentService) runMechanicsStage(ctx context.Context, symbol string, mechJSON features.MechanicsSnapshot) (agent.MechanicsSummary, decision.LLMStagePrompt, error) {
	mechInput := normalizeMechanicsInput(mechJSON.RawJSON)
	mechPrompt := mechJSON
	mechPrompt.RawJSON = mechInput
	sysMech, userMech, err := s.Prompts.AgentMechanicsPrompt(mechPrompt)
	if err != nil {
		stageErr := s.logStageError(ctx, "mechanics", err)
		return agent.MechanicsSummary{}, decision.LLMStagePrompt{Error: stageErr.Error()}, stageErr
	}
	prompt := decision.LLMStagePrompt{System: sysMech, User: userMech}
	if out, ok := loadMechanicsCache(s.Cache, symbol, "agent_mechanics", mechInput); ok {
		return out, prompt, nil
	}
	mechOut, err := s.Runner.RunMechanics(ctx, sysMech, userMech)
	if err != nil {
		stageErr := s.logStageError(ctx, "mechanics", err)
		prompt.Error = stageErr.Error()
		return agent.MechanicsSummary{}, prompt, stageErr
	}
	if s.Tracker != nil {
		s.Tracker.MarkAgent()
	}
	cacheAgentOutput(s.Cache, symbol, "agent_mechanics", mechOut, mechInput)
	return mechOut, prompt, nil
}

func (s LLMAgentService) logStageError(ctx context.Context, stage string, err error) error {
	logging.FromContext(ctx).Named("decision").Error("agent analyze failed", zap.String("stage", stage), zap.Error(err))
	return err
}

func cacheAgentOutput(cache *LLMStageCache, symbol string, stage string, out any, input []byte) {
	if cache == nil {
		return
	}
	cache.Store(symbol, stage, out, input)
}

func loadAgentCache(cache *LLMStageCache, symbol string, stage string, input []byte) (agent.IndicatorSummary, bool) {
	if cache == nil {
		return agent.IndicatorSummary{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return agent.IndicatorSummary{}, false
	}
	var out agent.IndicatorSummary
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return agent.IndicatorSummary{}, false
	}
	return out, true
}

func loadStructureCache(cache *LLMStageCache, symbol string, stage string, input []byte) (agent.StructureSummary, bool) {
	if cache == nil {
		return agent.StructureSummary{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return agent.StructureSummary{}, false
	}
	var out agent.StructureSummary
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return agent.StructureSummary{}, false
	}
	return out, true
}

func loadMechanicsCache(cache *LLMStageCache, symbol string, stage string, input []byte) (agent.MechanicsSummary, bool) {
	if cache == nil {
		return agent.MechanicsSummary{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return agent.MechanicsSummary{}, false
	}
	var out agent.MechanicsSummary
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return agent.MechanicsSummary{}, false
	}
	return out, true
}
