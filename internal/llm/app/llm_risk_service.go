package llmapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision"
	"brale-core/internal/decision/fund"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/llmclean"
	"brale-core/internal/risk/initexit"

	"github.com/google/uuid"
)

type LLMRiskService struct {
	Provider llm.Provider
	Prompts  LLMPromptBuilder

	SessionManager *llm.RoundSessionManager
	SessionMode    llm.SessionMode
}

func (s LLMRiskService) FlatRiskInitLLM() decision.FlatRiskInitLLM {
	return func(ctx context.Context, input decision.FlatRiskInitInput) (*initexit.BuildPatch, error) {
		if s.Provider == nil {
			return nil, fmt.Errorf("risk provider is required")
		}
		promptInput, err := buildFlatRiskPromptInput(input)
		if err != nil {
			return nil, err
		}
		system, user, err := s.Prompts.FlatRiskInitPrompt(promptInput)
		if err != nil {
			return nil, err
		}
		raw, err := s.callRiskWithLaneSession(ctx, input.Symbol, llm.LLMFlowFlat, system, user)
		if err != nil {
			return nil, err
		}
		parsed, err := decodeFlatRiskPatch(raw)
		if err != nil {
			return nil, err
		}
		return &initexit.BuildPatch{
			Entry:            parsed.Entry,
			StopLoss:         parsed.StopLoss,
			TakeProfits:      append([]float64(nil), parsed.TakeProfits...),
			TakeProfitRatios: append([]float64(nil), parsed.TakeProfitRatios...),
		}, nil
	}
}

func (s LLMRiskService) TightenRiskLLM() decision.TightenRiskUpdateLLM {
	return func(ctx context.Context, input decision.TightenRiskUpdateInput) (*decision.TightenRiskUpdatePatch, error) {
		if s.Provider == nil {
			return nil, fmt.Errorf("risk provider is required")
		}
		promptInput, err := buildTightenRiskPromptInput(input)
		if err != nil {
			return nil, err
		}
		system, user, err := s.Prompts.TightenRiskUpdatePrompt(promptInput)
		if err != nil {
			return nil, err
		}
		raw, err := s.callRiskWithLaneSession(ctx, input.Symbol, llm.LLMFlowInPosition, system, user)
		if err != nil {
			return nil, err
		}
		parsed, err := decodeTightenRiskPatch(raw)
		if err != nil {
			return nil, err
		}
		return &decision.TightenRiskUpdatePatch{
			StopLoss:    parsed.StopLoss,
			TakeProfits: append([]float64(nil), parsed.TakeProfits...),
		}, nil
	}
}

func buildFlatRiskPromptInput(input decision.FlatRiskInitInput) (FlatRiskPromptInput, error) {
	if strings.TrimSpace(input.Symbol) == "" {
		return FlatRiskPromptInput{}, fmt.Errorf("symbol is required")
	}
	if input.Plan.Entry <= 0 {
		return FlatRiskPromptInput{}, fmt.Errorf("plan entry is required")
	}
	consensus, structureSummary, otherProviderSummary := deriveRiskPromptSummaries(input.Gate)
	return FlatRiskPromptInput{
		Symbol:               strings.ToUpper(strings.TrimSpace(input.Symbol)),
		Direction:            strings.ToLower(strings.TrimSpace(input.Plan.Direction)),
		Entry:                input.Plan.Entry,
		RiskPct:              input.Plan.RiskPct,
		Consensus:            consensus,
		Structure:            structureSummary,
		OtherProviderSummary: otherProviderSummary,
		RiskContext: FlatRiskContext{
			ATR:          input.Plan.RiskAnnotations.ATR,
			MaxInvestPct: input.Plan.RiskAnnotations.MaxInvestPct,
			MaxInvestAmt: input.Plan.RiskAnnotations.MaxInvestAmt,
			MaxLeverage:  input.Plan.RiskAnnotations.MaxLeverage,
		},
	}, nil
}

func buildTightenRiskPromptInput(input decision.TightenRiskUpdateInput) (TightenRiskPromptInput, error) {
	if strings.TrimSpace(input.Symbol) == "" {
		return TightenRiskPromptInput{}, fmt.Errorf("symbol is required")
	}
	if strings.TrimSpace(input.Side) == "" {
		return TightenRiskPromptInput{}, fmt.Errorf("side is required")
	}
	if input.Entry <= 0 {
		return TightenRiskPromptInput{}, fmt.Errorf("entry is required")
	}
	if input.MarkPrice <= 0 {
		return TightenRiskPromptInput{}, fmt.Errorf("mark_price is required")
	}
	if input.ATR <= 0 {
		return TightenRiskPromptInput{}, fmt.Errorf("atr is required")
	}
	if input.CurrentStopLoss <= 0 {
		return TightenRiskPromptInput{}, fmt.Errorf("current_stop_loss is required")
	}
	if len(input.CurrentTakeProfits) == 0 {
		return TightenRiskPromptInput{}, fmt.Errorf("current_take_profits is required")
	}
	return TightenRiskPromptInput{
		Symbol:              strings.ToUpper(strings.TrimSpace(input.Symbol)),
		Direction:           strings.ToLower(strings.TrimSpace(input.Side)),
		Entry:               input.Entry,
		MarkPrice:           input.MarkPrice,
		ATR:                 input.ATR,
		CurrentStopLoss:     input.CurrentStopLoss,
		CurrentTakeProfits:  append([]float64(nil), input.CurrentTakeProfits...),
		Gate:                input.Gate,
		InPositionIndicator: input.InPositionIndicator,
		InPositionStructure: input.InPositionStructure,
		InPositionMechanics: input.InPositionMechanics,
	}, nil
}

func deriveRiskPromptSummaries(gate fund.GateDecision) (map[string]any, map[string]any, map[string]any) {
	consensus := pickDerivedMap(gate.Derived, "direction_consensus", "consensus")
	if len(consensus) == 0 {
		consensus = map[string]any{
			"direction":   strings.ToLower(strings.TrimSpace(gate.Direction)),
			"grade":       gate.Grade,
			"gate_reason": strings.TrimSpace(gate.GateReason),
		}
	}
	structureSummary := pickDerivedMap(gate.Derived, "provider_structure", "structure_summary", "structure")
	if len(structureSummary) == 0 {
		structureSummary = map[string]any{"gate_reason": strings.TrimSpace(gate.GateReason)}
	}
	otherProviderSummary := pickDerivedMap(gate.Derived, "providers", "provider_summary", "other_provider_summary")
	if len(otherProviderSummary) == 0 {
		otherProviderSummary = map[string]any{
			"decision_action": strings.TrimSpace(gate.DecisionAction),
			"gate_reason":     strings.TrimSpace(gate.GateReason),
		}
	}
	return consensus, structureSummary, otherProviderSummary
}

func pickDerivedMap(derived map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		candidate, ok := mapFromAny(derived[key])
		if ok && len(candidate) > 0 {
			return candidate
		}
	}
	return map[string]any{}
}

func mapFromAny(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	m, ok := value.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, false
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out, true
}

type flatRiskPatchPayload struct {
	Entry            *float64  `json:"entry"`
	StopLoss         *float64  `json:"stop_loss"`
	TakeProfits      []float64 `json:"take_profits"`
	TakeProfitRatios []float64 `json:"take_profit_ratios"`
}

type tightenRiskPatchPayload struct {
	StopLoss    *float64  `json:"stop_loss"`
	TakeProfits []float64 `json:"take_profits"`
}

func decodeFlatRiskPatch(raw string) (flatRiskPatchPayload, error) {
	clean := strings.TrimSpace(llmclean.CleanJSON(raw))
	if clean == "" {
		return flatRiskPatchPayload{}, fmt.Errorf("empty response")
	}
	var payload flatRiskPatchPayload
	if err := decodeStrictJSON(clean, &payload); err != nil {
		return flatRiskPatchPayload{}, err
	}
	if payload.StopLoss == nil {
		return flatRiskPatchPayload{}, fmt.Errorf("stop_loss is required")
	}
	if payload.Entry == nil {
		return flatRiskPatchPayload{}, fmt.Errorf("entry is required")
	}
	if *payload.Entry <= 0 {
		return flatRiskPatchPayload{}, fmt.Errorf("entry must be > 0")
	}
	if len(payload.TakeProfits) == 0 {
		return flatRiskPatchPayload{}, fmt.Errorf("take_profits is required")
	}
	if len(payload.TakeProfitRatios) == 0 {
		return flatRiskPatchPayload{}, fmt.Errorf("take_profit_ratios is required")
	}
	return payload, nil
}

func decodeTightenRiskPatch(raw string) (tightenRiskPatchPayload, error) {
	clean := strings.TrimSpace(llmclean.CleanJSON(raw))
	if clean == "" {
		return tightenRiskPatchPayload{}, fmt.Errorf("empty response")
	}
	var payload tightenRiskPatchPayload
	if err := decodeStrictJSON(clean, &payload); err != nil {
		return tightenRiskPatchPayload{}, err
	}
	if payload.StopLoss == nil {
		return tightenRiskPatchPayload{}, fmt.Errorf("stop_loss is required")
	}
	if len(payload.TakeProfits) == 0 {
		return tightenRiskPatchPayload{}, fmt.Errorf("take_profits is required")
	}
	return payload, nil
}

func decodeStrictJSON(raw string, out any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func (s LLMRiskService) callRiskWithLaneSession(ctx context.Context, symbol string, flow llm.LLMFlow, system string, user string) (string, error) {
	callCtx := llm.WithSessionSymbol(ctx, symbol)
	callCtx = llm.WithSessionFlow(callCtx, flow)
	key, laneMode, err := s.resolveLaneMode(callCtx, llm.LLMStageStructure)
	if err != nil || laneMode != llm.SessionModeSession {
		return llm.CallWithOptionalSession(callCtx, s.Provider, "", system, user)
	}
	sessionID, reused, err := s.acquireLaneSession(key)
	if err != nil {
		s.markLaneFallback(key)
		return llm.CallWithOptionalSession(callCtx, s.Provider, "", system, user)
	}
	currentUser := user
	if reused {
		currentUser = reusedRiskSessionConstraintUserPrompt()
	}
	raw, callErr := llm.CallWithOptionalSession(callCtx, s.Provider, sessionID, system, currentUser)
	if callErr == nil {
		return raw, nil
	}
	if !llm.IsSessionCapabilityError(callErr) {
		return "", callErr
	}
	s.markLaneFallback(key)
	return llm.CallWithOptionalSession(callCtx, s.Provider, "", system, user)
}

func reusedRiskSessionConstraintUserPrompt() string {
	return "继续基于当前会话上下文完成本轮判断。只输出一个 JSON 对象；严格遵循 system 提示中的字段、枚举和值类型约束；不要输出解释、Markdown 或代码块。"
}

func (s LLMRiskService) resolveLaneMode(ctx context.Context, stage llm.LLMStage) (llm.RoundLaneKey, llm.SessionMode, error) {
	if s.SessionMode != llm.SessionModeSession || s.SessionManager == nil {
		return "", llm.SessionModeStateless, nil
	}
	key, err := llm.RoundLaneKeyFromContext(ctx, stage)
	if err != nil {
		return "", llm.SessionModeStateless, err
	}
	return key, s.SessionManager.Mode(key), nil
}

func (s LLMRiskService) acquireLaneSession(key llm.RoundLaneKey) (string, bool, error) {
	if s.SessionManager == nil {
		return "", false, nil
	}
	return s.SessionManager.AcquireOrCreate(key, func() (string, error) {
		return uuid.NewString(), nil
	})
}

func (s LLMRiskService) markLaneFallback(key llm.RoundLaneKey) {
	if s.SessionManager == nil {
		return
	}
	s.SessionManager.MarkFallback(key)
}
