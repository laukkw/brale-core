// 本文件主要职责：封装 Provider 阶段的 LLM 调用与输出解析。
// 本文件主要内容：调用 provider 模型并汇总持仓判断。

package llmapp

import (
	"context"
	"encoding/json"
	"fmt"

	"brale-core/internal/decision"
	"brale-core/internal/decision/agent"
	"brale-core/internal/decision/provider"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/logging"
	"brale-core/internal/pkg/parallel"
	"brale-core/internal/prompt/positionprompt"

	"go.uber.org/zap"
)

type LLMProviderService struct {
	Runner  *provider.Runner
	Prompts LLMPromptBuilder
	Cache   *LLMStageCache
	Tracker *LLMRunTracker
}

func (s LLMProviderService) Judge(ctx context.Context, symbol string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, enabled decision.AgentEnabled, dataCtx decision.ProviderDataContext) (provider.IndicatorProviderOut, provider.StructureProviderOut, provider.MechanicsProviderOut, decision.ProviderPromptSet, error) {
	if s.Runner == nil {
		logging.FromContext(ctx).Named("decision").Error("provider judge failed", zap.String("stage", "init"), zap.Error(fmt.Errorf("runner is required")))
		return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, decision.ProviderPromptSet{}, wrapLLMStageError("provider", symbol, "init", fmt.Errorf("runner is required"))
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, decision.ProviderPromptSet{}, ctxErr
	}
	prompts, err := s.Prompts.ProviderPrompts(ind, st, mech, enabled, dataCtx)
	if err != nil {
		logging.FromContext(ctx).Named("decision").Error("provider judge failed", zap.String("stage", "prompts"), zap.Error(err))
		return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, decision.ProviderPromptSet{}, wrapLLMStageError("provider", symbol, "prompts", err)
	}
	var indOut provider.IndicatorProviderOut
	var stOut provider.StructureProviderOut
	var mechOut provider.MechanicsProviderOut
	var indPrompt decision.LLMStagePrompt
	var stPrompt decision.LLMStagePrompt
	var mechPrompt decision.LLMStagePrompt
	promptSet := decision.ProviderPromptSet{}
	tasks := make([]func(context.Context) error, 0, 3)
	if enabled.Indicator {
		tasks = append(tasks, func(runCtx context.Context) error {
			indicatorInput := prompts.IndicatorUser
			runCtx = llm.WithSessionSymbol(runCtx, symbol)
			indicatorSys, indicatorUser := applyMemoryPromptContext(runCtx, s.Prompts.UserFormat, prompts.IndicatorSys, indicatorInput)
			cacheInput := promptCacheInput(indicatorSys, indicatorUser)
			indicatorUser = s.providerUserPrompt(runCtx, indicatorUser, symbol, "provider_indicator", llm.LLMStageIndicator)
			indPrompt = decision.LLMStagePrompt{System: indicatorSys, User: indicatorUser, PromptVersion: s.Prompts.ProviderIndicatorVersion}
			if out, ok := loadProviderIndicatorCache(s.Cache, symbol, "provider_indicator", cacheInput); ok {
				indOut = out
				return nil
			}
			var collector stageCallCollector
			callCtx := withPromptCallContext(runCtx, symbol, "provider", llm.LLMStageIndicator, s.Prompts.ProviderIndicatorVersion, &collector)
			stageOut, finalUser, stageErr := runProviderWithLaneSession(callCtx, s, symbol, llm.LLMStageIndicator, indicatorUser, func(callCtx context.Context, _ string, user string) (provider.IndicatorProviderOut, error) {
				return s.Runner.JudgeIndicator(callCtx, indicatorSys, user)
			})
			indPrompt.User = finalUser
			applyStageCallStats(&indPrompt, collector)
			if stageErr != nil {
				logging.FromContext(runCtx).Named("decision").Error("provider judge failed", zap.String("stage", "indicator"), zap.Error(stageErr))
				indPrompt.Error = stageErr.Error()
				return wrapLLMStageError("provider", symbol, "indicator", stageErr)
			}
			indOut = stageOut
			if s.Tracker != nil {
				s.Tracker.MarkProvider()
			}
			cacheProviderOutput(s.Cache, symbol, "provider_indicator", indOut, cacheInput)
			return nil
		})
	}
	if enabled.Structure {
		tasks = append(tasks, func(runCtx context.Context) error {
			structureInput := prompts.StructureUser
			runCtx = llm.WithSessionSymbol(runCtx, symbol)
			structureSys, structureUser := applyMemoryPromptContext(runCtx, s.Prompts.UserFormat, prompts.StructureSys, structureInput)
			cacheInput := promptCacheInput(structureSys, structureUser)
			structureUser = s.providerUserPrompt(runCtx, structureUser, symbol, "provider_structure", llm.LLMStageStructure)
			stPrompt = decision.LLMStagePrompt{System: structureSys, User: structureUser, PromptVersion: s.Prompts.ProviderStructureVersion}
			if out, ok := loadProviderStructureCache(s.Cache, symbol, "provider_structure", cacheInput); ok {
				stOut = out
				return nil
			}
			var collector stageCallCollector
			callCtx := withPromptCallContext(runCtx, symbol, "provider", llm.LLMStageStructure, s.Prompts.ProviderStructureVersion, &collector)
			stageOut, finalUser, stageErr := runProviderWithLaneSession(callCtx, s, symbol, llm.LLMStageStructure, structureUser, func(callCtx context.Context, _ string, user string) (provider.StructureProviderOut, error) {
				return s.Runner.JudgeStructure(callCtx, structureSys, user)
			})
			stPrompt.User = finalUser
			applyStageCallStats(&stPrompt, collector)
			if stageErr != nil {
				logging.FromContext(runCtx).Named("decision").Error("provider judge failed", zap.String("stage", "structure"), zap.Error(stageErr))
				stPrompt.Error = stageErr.Error()
				return wrapLLMStageError("provider", symbol, "structure", stageErr)
			}
			stOut = stageOut
			if s.Tracker != nil {
				s.Tracker.MarkProvider()
			}
			cacheProviderOutput(s.Cache, symbol, "provider_structure", stOut, cacheInput)
			return nil
		})
	}
	if enabled.Mechanics {
		tasks = append(tasks, func(runCtx context.Context) error {
			mechanicsInput := prompts.MechanicsUser
			runCtx = llm.WithSessionSymbol(runCtx, symbol)
			mechanicsSys, mechanicsUser := applyMemoryPromptContext(runCtx, s.Prompts.UserFormat, prompts.MechanicsSys, mechanicsInput)
			cacheInput := promptCacheInput(mechanicsSys, mechanicsUser)
			mechanicsUser = s.providerUserPrompt(runCtx, mechanicsUser, symbol, "provider_mechanics", llm.LLMStageMechanics)
			mechPrompt = decision.LLMStagePrompt{System: mechanicsSys, User: mechanicsUser, PromptVersion: s.Prompts.ProviderMechanicsVersion}
			if out, ok := loadProviderMechanicsCache(s.Cache, symbol, "provider_mechanics", cacheInput); ok {
				mechOut = out
				return nil
			}
			var collector stageCallCollector
			callCtx := withPromptCallContext(runCtx, symbol, "provider", llm.LLMStageMechanics, s.Prompts.ProviderMechanicsVersion, &collector)
			stageOut, finalUser, stageErr := runProviderWithLaneSession(callCtx, s, symbol, llm.LLMStageMechanics, mechanicsUser, func(callCtx context.Context, _ string, user string) (provider.MechanicsProviderOut, error) {
				return s.Runner.JudgeMechanics(callCtx, mechanicsSys, user)
			})
			mechPrompt.User = finalUser
			applyStageCallStats(&mechPrompt, collector)
			if stageErr != nil {
				logging.FromContext(runCtx).Named("decision").Error("provider judge failed", zap.String("stage", "mechanics"), zap.Error(stageErr))
				mechPrompt.Error = stageErr.Error()
				return wrapLLMStageError("provider", symbol, "mechanics", stageErr)
			}
			mechOut = stageOut
			if s.Tracker != nil {
				s.Tracker.MarkProvider()
			}
			cacheProviderOutput(s.Cache, symbol, "provider_mechanics", mechOut, cacheInput)
			return nil
		})
	}
	if err := parallel.RunFailFast(ctx, tasks...); err != nil {
		return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, decision.ProviderPromptSet{}, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return provider.IndicatorProviderOut{}, provider.StructureProviderOut{}, provider.MechanicsProviderOut{}, decision.ProviderPromptSet{}, ctxErr
	}
	if enabled.Indicator {
		promptSet.Indicator = indPrompt
	}
	if enabled.Structure {
		promptSet.Structure = stPrompt
	}
	if enabled.Mechanics {
		promptSet.Mechanics = mechPrompt
	}
	logProviderDecision(ctx, symbol, enabled, indOut, stOut, mechOut)
	return indOut, stOut, mechOut, promptSet, nil
}

func (s LLMProviderService) JudgeInPosition(ctx context.Context, symbol string, ind agent.IndicatorSummary, st agent.StructureSummary, mech agent.MechanicsSummary, summary positionprompt.Summary, enabled decision.AgentEnabled, dataCtx decision.ProviderDataContext) (provider.InPositionIndicatorOut, provider.InPositionStructureOut, provider.InPositionMechanicsOut, decision.ProviderPromptSet, error) {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", symbol))
	if s.Runner == nil {
		logger.Error("provider judge failed", zap.String("stage", "init"), zap.Error(fmt.Errorf("runner is required")))
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, decision.ProviderPromptSet{}, wrapLLMStageError("provider", symbol, "init", fmt.Errorf("runner is required"))
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, decision.ProviderPromptSet{}, ctxErr
	}
	prompts, err := s.Prompts.InPositionProviderPrompts(ind, st, mech, summary, enabled, dataCtx)
	if err != nil {
		logger.Error("provider judge failed", zap.String("stage", "prompts"), zap.Error(err))
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, decision.ProviderPromptSet{}, wrapLLMStageError("provider", symbol, "prompts_in_position", err)
	}
	var indOut provider.InPositionIndicatorOut
	var stOut provider.InPositionStructureOut
	var mechOut provider.InPositionMechanicsOut
	var indPrompt decision.LLMStagePrompt
	var stPrompt decision.LLMStagePrompt
	var mechPrompt decision.LLMStagePrompt
	promptSet := decision.ProviderPromptSet{}
	tasks := make([]func(context.Context) error, 0, 3)
	if enabled.Indicator {
		tasks = append(tasks, func(runCtx context.Context) error {
			indicatorInput := prompts.IndicatorUser
			runCtx = llm.WithSessionSymbol(runCtx, symbol)
			indicatorSys, indicatorUser := applyMemoryPromptContext(runCtx, s.Prompts.UserFormat, prompts.IndicatorSys, indicatorInput)
			cacheInput := promptCacheInput(indicatorSys, indicatorUser)
			indicatorUser = s.providerUserPrompt(runCtx, indicatorUser, symbol, "provider_indicator_in_position", llm.LLMStageIndicator)
			indPrompt = decision.LLMStagePrompt{System: indicatorSys, User: indicatorUser, PromptVersion: s.Prompts.ProviderInPosIndicatorVer}
			if out, ok := loadProviderIndicatorInPositionCache(s.Cache, symbol, "provider_indicator_in_position", cacheInput); ok {
				indOut = out
				return nil
			}
			var collector stageCallCollector
			callCtx := withPromptCallContext(runCtx, symbol, "provider_in_position", llm.LLMStageIndicator, s.Prompts.ProviderInPosIndicatorVer, &collector)
			stageOut, finalUser, stageErr := runProviderWithLaneSession(callCtx, s, symbol, llm.LLMStageIndicator, indicatorUser, func(callCtx context.Context, _ string, user string) (provider.InPositionIndicatorOut, error) {
				return s.Runner.JudgeIndicatorInPosition(callCtx, indicatorSys, user)
			})
			indPrompt.User = finalUser
			applyStageCallStats(&indPrompt, collector)
			if stageErr != nil {
				logger.Error("provider judge failed", zap.String("stage", "indicator_in_position"), zap.Error(stageErr))
				indPrompt.Error = stageErr.Error()
				return wrapLLMStageError("provider", symbol, "indicator_in_position", stageErr)
			}
			indOut = stageOut
			if s.Tracker != nil {
				s.Tracker.MarkProvider()
			}
			cacheProviderOutput(s.Cache, symbol, "provider_indicator_in_position", indOut, cacheInput)
			return nil
		})
	}
	if enabled.Structure {
		tasks = append(tasks, func(runCtx context.Context) error {
			structureInput := prompts.StructureUser
			runCtx = llm.WithSessionSymbol(runCtx, symbol)
			structureSys, structureUser := applyMemoryPromptContext(runCtx, s.Prompts.UserFormat, prompts.StructureSys, structureInput)
			cacheInput := promptCacheInput(structureSys, structureUser)
			structureUser = s.providerUserPrompt(runCtx, structureUser, symbol, "provider_structure_in_position", llm.LLMStageStructure)
			stPrompt = decision.LLMStagePrompt{System: structureSys, User: structureUser, PromptVersion: s.Prompts.ProviderInPosStructureVer}
			if out, ok := loadProviderStructureInPositionCache(s.Cache, symbol, "provider_structure_in_position", cacheInput); ok {
				stOut = out
				return nil
			}
			var collector stageCallCollector
			callCtx := withPromptCallContext(runCtx, symbol, "provider_in_position", llm.LLMStageStructure, s.Prompts.ProviderInPosStructureVer, &collector)
			stageOut, finalUser, stageErr := runProviderWithLaneSession(callCtx, s, symbol, llm.LLMStageStructure, structureUser, func(callCtx context.Context, _ string, user string) (provider.InPositionStructureOut, error) {
				return s.Runner.JudgeStructureInPosition(callCtx, structureSys, user)
			})
			stPrompt.User = finalUser
			applyStageCallStats(&stPrompt, collector)
			if stageErr != nil {
				logger.Error("provider judge failed", zap.String("stage", "structure_in_position"), zap.Error(stageErr))
				stPrompt.Error = stageErr.Error()
				return wrapLLMStageError("provider", symbol, "structure_in_position", stageErr)
			}
			stOut = stageOut
			if s.Tracker != nil {
				s.Tracker.MarkProvider()
			}
			cacheProviderOutput(s.Cache, symbol, "provider_structure_in_position", stOut, cacheInput)
			return nil
		})
	}
	if enabled.Mechanics {
		tasks = append(tasks, func(runCtx context.Context) error {
			mechanicsInput := prompts.MechanicsUser
			runCtx = llm.WithSessionSymbol(runCtx, symbol)
			mechanicsSys, mechanicsUser := applyMemoryPromptContext(runCtx, s.Prompts.UserFormat, prompts.MechanicsSys, mechanicsInput)
			cacheInput := promptCacheInput(mechanicsSys, mechanicsUser)
			mechanicsUser = s.providerUserPrompt(runCtx, mechanicsUser, symbol, "provider_mechanics_in_position", llm.LLMStageMechanics)
			mechPrompt = decision.LLMStagePrompt{System: mechanicsSys, User: mechanicsUser, PromptVersion: s.Prompts.ProviderInPosMechanicsVer}
			if out, ok := loadProviderMechanicsInPositionCache(s.Cache, symbol, "provider_mechanics_in_position", cacheInput); ok {
				mechOut = out
				return nil
			}
			var collector stageCallCollector
			callCtx := withPromptCallContext(runCtx, symbol, "provider_in_position", llm.LLMStageMechanics, s.Prompts.ProviderInPosMechanicsVer, &collector)
			stageOut, finalUser, stageErr := runProviderWithLaneSession(callCtx, s, symbol, llm.LLMStageMechanics, mechanicsUser, func(callCtx context.Context, _ string, user string) (provider.InPositionMechanicsOut, error) {
				return s.Runner.JudgeMechanicsInPosition(callCtx, mechanicsSys, user)
			})
			mechPrompt.User = finalUser
			applyStageCallStats(&mechPrompt, collector)
			if stageErr != nil {
				logger.Error("provider judge failed", zap.String("stage", "mechanics_in_position"), zap.Error(stageErr))
				mechPrompt.Error = stageErr.Error()
				return wrapLLMStageError("provider", symbol, "mechanics_in_position", stageErr)
			}
			mechOut = stageOut
			if s.Tracker != nil {
				s.Tracker.MarkProvider()
			}
			cacheProviderOutput(s.Cache, symbol, "provider_mechanics_in_position", mechOut, cacheInput)
			return nil
		})
	}
	if err := parallel.RunFailFast(ctx, tasks...); err != nil {
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, decision.ProviderPromptSet{}, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return provider.InPositionIndicatorOut{}, provider.InPositionStructureOut{}, provider.InPositionMechanicsOut{}, decision.ProviderPromptSet{}, ctxErr
	}
	if enabled.Indicator {
		promptSet.Indicator = indPrompt
	}
	if enabled.Structure {
		promptSet.Structure = stPrompt
	}
	if enabled.Mechanics {
		promptSet.Mechanics = mechPrompt
	}
	logProviderInPositionDecision(ctx, symbol, enabled, indOut, stOut, mechOut)
	return indOut, stOut, mechOut, promptSet, nil
}

func (s LLMProviderService) providerUserPrompt(ctx context.Context, user string, symbol string, cacheStage string, stage llm.LLMStage) string {
	return appendLastOutput(user, s.Cache, symbol, cacheStage)
}

func runProviderWithLaneSession[T any](ctx context.Context, service LLMProviderService, symbol string, stage llm.LLMStage, user string, invoke func(context.Context, string, string) (T, error)) (T, string, error) {
	callCtx := llm.WithSessionSymbol(ctx, symbol)
	service.logLaneCall(callCtx, stage, "stateless", "", false, "")
	out, err := invoke(callCtx, "", user)
	return out, user, err
}

func (s LLMProviderService) logLaneCall(ctx context.Context, stage llm.LLMStage, mode string, sessionID string, reused bool, fallbackReason string) {
	logLLMLaneCall(ctx, "provider", stage, mode, sessionID, reused, fallbackReason)
}

func cacheProviderOutput(cache *LLMStageCache, symbol string, stage string, out any, input []byte) {
	cacheStageOutput(cache, symbol, stage, out, input)
}

func loadProviderIndicatorCache(cache *LLMStageCache, symbol string, stage string, input []byte) (provider.IndicatorProviderOut, bool) {
	if cache == nil {
		return provider.IndicatorProviderOut{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return provider.IndicatorProviderOut{}, false
	}
	var out provider.IndicatorProviderOut
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return provider.IndicatorProviderOut{}, false
	}
	return out, true
}

func loadProviderStructureCache(cache *LLMStageCache, symbol string, stage string, input []byte) (provider.StructureProviderOut, bool) {
	if cache == nil {
		return provider.StructureProviderOut{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return provider.StructureProviderOut{}, false
	}
	var out provider.StructureProviderOut
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return provider.StructureProviderOut{}, false
	}
	return out, true
}

func loadProviderMechanicsCache(cache *LLMStageCache, symbol string, stage string, input []byte) (provider.MechanicsProviderOut, bool) {
	if cache == nil {
		return provider.MechanicsProviderOut{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return provider.MechanicsProviderOut{}, false
	}
	var out provider.MechanicsProviderOut
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return provider.MechanicsProviderOut{}, false
	}
	return out, true
}

func loadProviderIndicatorInPositionCache(cache *LLMStageCache, symbol string, stage string, input []byte) (provider.InPositionIndicatorOut, bool) {
	if cache == nil {
		return provider.InPositionIndicatorOut{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return provider.InPositionIndicatorOut{}, false
	}
	var out provider.InPositionIndicatorOut
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return provider.InPositionIndicatorOut{}, false
	}
	return out, true
}

func loadProviderStructureInPositionCache(cache *LLMStageCache, symbol string, stage string, input []byte) (provider.InPositionStructureOut, bool) {
	if cache == nil {
		return provider.InPositionStructureOut{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return provider.InPositionStructureOut{}, false
	}
	var out provider.InPositionStructureOut
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return provider.InPositionStructureOut{}, false
	}
	return out, true
}

func loadProviderMechanicsInPositionCache(cache *LLMStageCache, symbol string, stage string, input []byte) (provider.InPositionMechanicsOut, bool) {
	if cache == nil {
		return provider.InPositionMechanicsOut{}, false
	}
	item, ok := cache.Load(symbol, stage, input)
	if !ok {
		return provider.InPositionMechanicsOut{}, false
	}
	var out provider.InPositionMechanicsOut
	if err := json.Unmarshal(item.OutputJSON, &out); err != nil {
		return provider.InPositionMechanicsOut{}, false
	}
	return out, true
}

func logProviderDecision(ctx context.Context, symbol string, enabled decision.AgentEnabled, ind provider.IndicatorProviderOut, st provider.StructureProviderOut, mech provider.MechanicsProviderOut) {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", symbol))
	fields := make([]zap.Field, 0, 3)
	if enabled.Indicator {
		fields = append(fields, zap.String("指标", describeIndicator(ind)))
	} else {
		fields = append(fields, zap.String("指标", "禁用"))
	}
	if enabled.Structure {
		fields = append(fields, zap.String("结构", describeStructure(st)))
	} else {
		fields = append(fields, zap.String("结构", "禁用"))
	}
	if enabled.Mechanics {
		fields = append(fields, zap.String("机制", describeMechanics(mech)))
	} else {
		fields = append(fields, zap.String("机制", "禁用"))
	}
	logger.Info("provider LLM 决策", fields...)
}

func logProviderInPositionDecision(ctx context.Context, symbol string, enabled decision.AgentEnabled, ind provider.InPositionIndicatorOut, st provider.InPositionStructureOut, mech provider.InPositionMechanicsOut) {
	logger := logging.FromContext(ctx).Named("decision").With(zap.String("symbol", symbol))
	fields := make([]zap.Field, 0, 3)
	if enabled.Indicator {
		fields = append(fields, zap.String("指标", describeInPositionIndicator(ind)))
	} else {
		fields = append(fields, zap.String("指标", "禁用"))
	}
	if enabled.Structure {
		fields = append(fields, zap.String("结构", describeInPositionStructure(st)))
	} else {
		fields = append(fields, zap.String("结构", "禁用"))
	}
	if enabled.Mechanics {
		fields = append(fields, zap.String("机制", describeInPositionMechanics(mech)))
	} else {
		fields = append(fields, zap.String("机制", "禁用"))
	}
	logger.Info("provider LLM 决策(持仓)", fields...)
}

func describeIndicator(out provider.IndicatorProviderOut) string {
	return fmt.Sprintf(
		"momentum_expansion 动量扩张: %t; alignment 趋势一致: %t; mean_rev_noise 均值回归噪音: %t",
		out.MomentumExpansion,
		out.Alignment,
		out.MeanRevNoise,
	)
}

func describeStructure(out provider.StructureProviderOut) string {
	return fmt.Sprintf(
		"clear_structure 结构清晰: %t; integrity 叙事有效: %t; reason 理由: %s",
		out.ClearStructure,
		out.Integrity,
		out.Reason,
	)
}

func describeMechanics(out provider.MechanicsProviderOut) string {
	return fmt.Sprintf(
		"liquidation_stress 清算压力: %t; confidence 置信度: %s; reason 理由: %s",
		out.LiquidationStress.Value,
		string(out.LiquidationStress.Confidence),
		out.LiquidationStress.Reason,
	)
}

func describeInPositionIndicator(out provider.InPositionIndicatorOut) string {
	return fmt.Sprintf(
		"momentum_sustaining 动能维持: %t; divergence_detected 背离: %t; reason 理由: %s",
		out.MomentumSustaining,
		out.DivergenceDetected,
		out.Reason,
	)
}

func describeInPositionStructure(out provider.InPositionStructureOut) string {
	return fmt.Sprintf(
		"integrity 叙事有效: %t; threat_level 威胁等级: %s; reason 理由: %s",
		out.Integrity,
		string(out.ThreatLevel),
		out.Reason,
	)
}

func describeInPositionMechanics(out provider.InPositionMechanicsOut) string {
	return fmt.Sprintf(
		"adverse_liquidation 反向清算: %t; crowding_reversal 拥挤反转: %t; reason 理由: %s",
		out.AdverseLiquidation,
		out.CrowdingReversal,
		out.Reason,
	)
}
