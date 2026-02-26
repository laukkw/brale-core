// 本文件主要内容：执行 provider LLM 调用并解析输出。

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/llm"
	"brale-core/internal/pkg/llmclean"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

// =====================
// JSON 严格解析：要求响应为单个 JSON 对象，字段必须匹配结构体。
// 示例：{"tradeable":true,"alignment":true,"mean_rev_noise":false}
// =====================

type Runner struct {
	Indicator llm.Provider
	Structure llm.Provider
	Mechanics llm.Provider
}

type DecodeError struct {
	Err error
}

func (e DecodeError) Error() string {
	return fmt.Sprintf("decode failed: %v", e.Err)
}

func (e DecodeError) Unwrap() error {
	return e.Err
}

func IsDecodeError(err error) bool {
	if err == nil {
		return false
	}
	var decodeErr DecodeError
	return errors.As(err, &decodeErr)
}

func (r *Runner) Validate() error {
	if r == nil {
		return fmt.Errorf("provider is required")
	}
	if r.Indicator == nil && r.Structure == nil && r.Mechanics == nil {
		return fmt.Errorf("provider is required")
	}
	return nil
}

func (r *Runner) providerFor(stage string) (llm.Provider, error) {
	if r == nil {
		return nil, fmt.Errorf("provider is required")
	}
	switch stage {
	case "indicator", "indicator_in_position":
		if r.Indicator == nil {
			return nil, fmt.Errorf("indicator provider is required")
		}
		return r.Indicator, nil
	case "structure", "structure_in_position":
		if r.Structure == nil {
			return nil, fmt.Errorf("structure provider is required")
		}
		return r.Structure, nil
	case "mechanics", "mechanics_in_position":
		if r.Mechanics == nil {
			return nil, fmt.Errorf("mechanics provider is required")
		}
		return r.Mechanics, nil
	default:
		return nil, fmt.Errorf("unknown stage: %s", stage)
	}
}

func (r *Runner) JudgeIndicator(ctx context.Context, system, user string) (IndicatorProviderOut, error) {
	return runAndParse(ctx, r, "indicator", system, user, decodeIndicator)
}

func (r *Runner) JudgeStructure(ctx context.Context, system, user string) (StructureProviderOut, error) {
	return runAndParse(ctx, r, "structure", system, user, decodeStructure)
}

func (r *Runner) JudgeMechanics(ctx context.Context, system, user string) (MechanicsProviderOut, error) {
	return runAndParse(ctx, r, "mechanics", system, user, decodeMechanics)
}

func (r *Runner) JudgeIndicatorInPosition(ctx context.Context, system, user string) (InPositionIndicatorOut, error) {
	return runAndParse(ctx, r, "indicator_in_position", system, user, decodeInPositionIndicator)
}

func (r *Runner) JudgeStructureInPosition(ctx context.Context, system, user string) (InPositionStructureOut, error) {
	return runAndParse(ctx, r, "structure_in_position", system, user, decodeInPositionStructure)
}

func (r *Runner) JudgeMechanicsInPosition(ctx context.Context, system, user string) (InPositionMechanicsOut, error) {
	return runAndParse(ctx, r, "mechanics_in_position", system, user, decodeInPositionMechanics)
}

func runAndParse[T any](ctx context.Context, r *Runner, stage, system, user string, decode func(string) (T, error)) (T, error) {
	logger := logging.FromContext(ctx).Named("provider")
	return decisionutil.RunAndParse(ctx, r.providerFor, stage, system, user, func(raw string) (T, error) {
		logger.Info("provider raw output",
			zap.String("stage", stage),
			zap.String("output", trimForLog(raw, 1200)),
		)
		out, err := decode(raw)
		if err != nil {
			return out, DecodeError{Err: err}
		}
		return out, nil
	}, func(raw string, err error) {
		logger.Error("provider parse failed",
			zap.String("stage", stage),
			zap.Error(err),
			zap.String("output", trimForLog(raw, 1200)),
		)
	})
}

func decodeIndicator(raw string) (IndicatorProviderOut, error) {
	var out IndicatorProviderOut
	if err := decodeStrict(raw, &out, "momentum_expansion", "alignment", "mean_rev_noise", "signal_tag"); err != nil {
		return IndicatorProviderOut{}, err
	}
	return out, nil
}

func decodeStructure(raw string) (StructureProviderOut, error) {
	var out StructureProviderOut
	if err := decodeStrict(raw, &out, "clear_structure", "integrity", "reason", "signal_tag"); err != nil {
		return StructureProviderOut{}, err
	}
	return out, nil
}

func decodeMechanics(raw string) (MechanicsProviderOut, error) {
	var out MechanicsProviderOut
	if err := decodeStrict(raw, &out, "liquidation_stress.value", "liquidation_stress.confidence", "liquidation_stress.reason", "signal_tag"); err != nil {
		return MechanicsProviderOut{}, err
	}
	return out, nil
}

func decodeInPositionIndicator(raw string) (InPositionIndicatorOut, error) {
	var out InPositionIndicatorOut
	if err := decodeStrict(raw, &out, "momentum_sustaining", "divergence_detected", "reason", "monitor_tag"); err != nil {
		return InPositionIndicatorOut{}, err
	}
	return out, nil
}

func decodeInPositionStructure(raw string) (InPositionStructureOut, error) {
	var out InPositionStructureOut
	if err := decodeStrict(raw, &out, "integrity", "threat_level", "reason", "monitor_tag"); err != nil {
		return InPositionStructureOut{}, err
	}
	return out, nil
}

func decodeInPositionMechanics(raw string) (InPositionMechanicsOut, error) {
	var out InPositionMechanicsOut
	if err := decodeStrict(raw, &out, "adverse_liquidation", "crowding_reversal", "reason", "monitor_tag"); err != nil {
		return InPositionMechanicsOut{}, err
	}
	return out, nil
}

func decodeStrict(raw string, target any, required ...string) error {
	raw = llmclean.CleanJSON(raw)
	if raw == "" {
		return fmt.Errorf("empty response")
	}
	if len(required) > 0 {
		var data map[string]any
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return fmt.Errorf("invalid json: %w", err)
		}
		for _, path := range required {
			if !jsonHasPath(data, path) {
				return fmt.Errorf("missing field: %s", path)
			}
		}
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func jsonHasPath(data map[string]any, path string) bool {
	if len(data) == 0 {
		return false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	parts := strings.Split(path, ".")
	var current any = data
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return false
		}
		obj, ok := current.(map[string]any)
		if !ok {
			return false
		}
		val, ok := obj[part]
		if !ok {
			return false
		}
		current = val
	}
	return current != nil
}

func trimForLog(raw string, limit int) string {
	if limit <= 0 {
		return ""
	}
	s := strings.TrimSpace(raw)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}
