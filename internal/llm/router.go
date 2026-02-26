// 本文件主要内容：按阶段路由 LLM 调用到不同的 Provider。

package llm

import (
	"context"
	"fmt"
	"strings"

	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

type StageDetector func(system string) string

type RoutedProvider struct {
	Indicator Provider
	Structure Provider
	Mechanics Provider
	Detect    StageDetector
}

func (r RoutedProvider) Call(ctx context.Context, system, user string) (string, error) {
	if r.Detect == nil {
		return "", fmt.Errorf("stage detector is required")
	}
	stage := r.Detect(system)
	logger := logging.FromContext(ctx).Named("llm").With(
		zap.String("stage", stage),
		zap.Int("system_len", len(system)),
		zap.Int("user_len", len(user)),
	)
	switch stage {
	case "indicator":
		if r.Indicator == nil {
			return "", fmt.Errorf("indicator provider is required")
		}
		return r.Indicator.Call(ctx, system, user)
	case "structure":
		if r.Structure == nil {
			return "", fmt.Errorf("structure provider is required")
		}
		return r.Structure.Call(ctx, system, user)
	case "mechanics":
		if r.Mechanics == nil {
			return "", fmt.Errorf("mechanics provider is required")
		}
		return r.Mechanics.Call(ctx, system, user)
	default:
		logger.Warn("unknown llm stage", zap.String("system_head", truncate(system, 120)))
		return "", fmt.Errorf("unknown stage: %s", stage)
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func DetectAgentStage(system string) string {
	lower := strings.ToLower(system)
	switch {
	case strings.Contains(lower, "indicator"):
		return "indicator"
	case strings.Contains(lower, "trend"):
		return "structure"
	case strings.Contains(lower, "mechanics"):
		return "mechanics"
	default:
		return "unknown"
	}
}

func DetectProviderStage(system string) string {
	lower := strings.ToLower(system)
	switch {
	case strings.Contains(lower, "llm-3"):
		return "indicator"
	case strings.Contains(lower, "llm-1"):
		return "structure"
	case strings.Contains(lower, "llm-2"):
		return "mechanics"
	default:
		return "unknown"
	}
}
