// 本文件主要内容：实现 Agent 调用与摘要解析。
package agent

import (
	"context"
	"encoding/json"
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
// 示例：{"expansion":"expanded","alignment":"aligned","noise":"low"}
// =====================

type Runner struct {
	Indicator llm.Provider
	Structure llm.Provider
	Mechanics llm.Provider
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
	case "indicator":
		if r.Indicator == nil {
			return nil, fmt.Errorf("indicator provider is required")
		}
		return r.Indicator, nil
	case "structure":
		if r.Structure == nil {
			return nil, fmt.Errorf("structure provider is required")
		}
		return r.Structure, nil
	case "mechanics":
		if r.Mechanics == nil {
			return nil, fmt.Errorf("mechanics provider is required")
		}
		return r.Mechanics, nil
	default:
		return nil, fmt.Errorf("unknown stage: %s", stage)
	}
}
func (r *Runner) RunIndicator(ctx context.Context, system, user string) (IndicatorSummary, error) {
	return runAndParse(ctx, r, "indicator", system, user, decodeIndicatorSummary)
}

func (r *Runner) RunStructure(ctx context.Context, system, user string) (StructureSummary, error) {
	return runAndParse(ctx, r, "structure", system, user, decodeStructureSummary)
}

func (r *Runner) RunMechanics(ctx context.Context, system, user string) (MechanicsSummary, error) {
	return runAndParse(ctx, r, "mechanics", system, user, decodeMechanicsSummary)
}

func runAndParse[T any](ctx context.Context, r *Runner, stage, system, user string, decode func(string) (T, error)) (T, error) {
	return decisionutil.RunAndParse(ctx, r.providerFor, stage, system, user, decode, func(raw string, err error) {
		logging.FromContext(ctx).Named("agent").Error("agent parse failed",
			zap.String("stage", stage),
			zap.Error(err),
			zap.String("output", trimForLog(raw, 1200)),
		)
	})
}

func decodeIndicatorSummary(raw string) (IndicatorSummary, error) {
	var out IndicatorSummary
	if err := decodeStrict(raw, &out); err != nil {
		return IndicatorSummary{}, err
	}
	return out, nil
}

func decodeStructureSummary(raw string) (StructureSummary, error) {
	var out StructureSummary
	if err := decodeStrict(raw, &out); err != nil {
		return StructureSummary{}, err
	}
	return out, nil
}

func decodeMechanicsSummary(raw string) (MechanicsSummary, error) {
	var out MechanicsSummary
	if err := decodeStrict(raw, &out); err != nil {
		return MechanicsSummary{}, err
	}
	return out, nil
}

func decodeStrict(raw string, target any) error {
	raw = llmclean.StripCodeFences(raw)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty response")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
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
