package llmapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision"
	"brale-core/internal/decision/fund"
	"brale-core/internal/execution"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/llmclean"
	"brale-core/internal/risk/initexit"
)

type LLMRiskService struct {
	Provider llm.Provider
	Prompts  LLMPromptBuilder
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
		raw, err := s.Provider.Call(ctx, system, user)
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
			Reason:           parsed.Reason,
			Trace: &execution.LLMRiskTrace{
				Stage:        llm.LLMStageRiskFlatInit.String(),
				Flow:         llm.LLMFlowFlat.String(),
				SystemPrompt: system,
				UserPrompt:   user,
				RawOutput:    raw,
				ParsedOutput: map[string]any{
					"entry":              optionalFloat(parsed.Entry),
					"stop_loss":          optionalFloat(parsed.StopLoss),
					"take_profits":       append([]float64(nil), parsed.TakeProfits...),
					"take_profit_ratios": append([]float64(nil), parsed.TakeProfitRatios...),
					"reason":             optionalString(parsed.Reason),
				},
			},
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
		raw, err := s.Provider.Call(ctx, system, user)
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
			Trace: &execution.LLMRiskTrace{
				Stage:        llm.LLMStageRiskTighten.String(),
				Flow:         llm.LLMFlowInPosition.String(),
				SystemPrompt: system,
				UserPrompt:   user,
				RawOutput:    raw,
				ParsedOutput: map[string]any{
					"stop_loss":    optionalFloat(parsed.StopLoss),
					"take_profits": append([]float64(nil), parsed.TakeProfits...),
				},
			},
		}, nil
	}
}

func optionalFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func buildFlatRiskPromptInput(input decision.FlatRiskInitInput) (FlatRiskPromptInput, error) {
	if strings.TrimSpace(input.Symbol) == "" {
		return FlatRiskPromptInput{}, fmt.Errorf("symbol is required")
	}
	if input.Plan.Entry <= 0 {
		return FlatRiskPromptInput{}, fmt.Errorf("plan entry is required")
	}
	consensus, structureSummary, otherProviderSummary := deriveRiskPromptSummaries(input.Gate)
	planSummary := map[string]any{
		"entry":          input.Plan.Entry,
		"risk_pct":       input.Plan.RiskPct,
		"stop_loss":      input.Plan.StopLoss,
		"take_profits":   append([]float64(nil), input.Plan.TakeProfits...),
		"atr":            input.Plan.RiskAnnotations.ATR,
		"max_invest_pct": input.Plan.RiskAnnotations.MaxInvestPct,
		"max_invest_amt": input.Plan.RiskAnnotations.MaxInvestAmt,
		"max_leverage":   input.Plan.RiskAnnotations.MaxLeverage,
		"liq_price":      input.Plan.RiskAnnotations.LiqPrice,
	}
	return FlatRiskPromptInput{
		Symbol:               strings.ToUpper(strings.TrimSpace(input.Symbol)),
		Direction:            strings.ToLower(strings.TrimSpace(input.Plan.Direction)),
		Entry:                input.Plan.Entry,
		RiskPct:              input.Plan.RiskPct,
		PlanSummary:          planSummary,
		Consensus:            consensus,
		Structure:            structureSummary,
		OtherProviderSummary: otherProviderSummary,
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
	Reason           *string   `json:"reason"`
}

type tightenRiskPatchPayload struct {
	StopLoss    *float64  `json:"stop_loss"`
	TakeProfits []float64 `json:"take_profits"`
}

var flatRiskPatchAllowedFields = map[string]struct{}{
	"entry":              {},
	"stop_loss":          {},
	"take_profits":       {},
	"take_profit_ratios": {},
	"reason":             {},
}

func decodeFlatRiskPatch(raw string) (flatRiskPatchPayload, error) {
	clean := strings.TrimSpace(llmclean.CleanJSON(raw))
	if clean == "" {
		return flatRiskPatchPayload{}, fmt.Errorf("empty response")
	}
	sanitized, err := filterJSONObjectFields(clean, flatRiskPatchAllowedFields)
	if err != nil {
		return flatRiskPatchPayload{}, err
	}
	var payload flatRiskPatchPayload
	if err := decodeStrictJSON(sanitized, &payload); err != nil {
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
	if payload.Reason == nil || strings.TrimSpace(*payload.Reason) == "" {
		return flatRiskPatchPayload{}, fmt.Errorf("reason is required")
	}
	return payload, nil
}

func filterJSONObjectFields(raw string, allowed map[string]struct{}) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", fmt.Errorf("invalid json: %w", err)
	}
	filtered := make(map[string]json.RawMessage, len(payload))
	for key, value := range payload {
		if _, ok := allowed[key]; ok {
			filtered[key] = value
		}
	}
	out, err := json.Marshal(filtered)
	if err != nil {
		return "", fmt.Errorf("marshal filtered json: %w", err)
	}
	return string(out), nil
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
