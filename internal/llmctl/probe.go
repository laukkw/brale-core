package llmctl

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/llm"
)

type ProbeTarget struct {
	Stage     string
	Model     string
	Endpoint  string
	APIKey    string
	Timeout   time.Duration
	RepoRoot  string
	ConfigKey string
}

type ProbeResult struct {
	Target    ProbeTarget
	Supported bool
	Error     error
}

func LoadProbeTargets(repoRoot, stage string) ([]ProbeTarget, error) {
	systemPath := filepath.Join(repoRoot, "configs", "system.toml")
	symbolPath := filepath.Join(repoRoot, "configs", "symbols", "default.toml")

	sys, err := config.LoadSystemConfig(systemPath)
	if err != nil {
		return nil, err
	}
	sym, err := config.LoadSymbolConfig(symbolPath)
	if err != nil {
		return nil, err
	}
	return BuildProbeTargets(sys, sym, stage)
}

func BuildProbeTargets(sys config.SystemConfig, sym config.SymbolConfig, stage string) ([]ProbeTarget, error) {
	stage = normalizeStage(stage)
	if stage != "" && !isValidStage(stage) {
		return nil, fmt.Errorf("invalid stage: %s", stage)
	}

	roles := []struct {
		stage string
		role  config.LLMRoleConfig
	}{
		{stage: "indicator", role: pickStageRole(sym, "indicator")},
		{stage: "structure", role: pickStageRole(sym, "structure")},
		{stage: "mechanics", role: pickStageRole(sym, "mechanics")},
	}

	targets := make([]ProbeTarget, 0, len(roles))
	for _, item := range roles {
		if stage != "" && item.stage != stage {
			continue
		}
		model := strings.TrimSpace(item.role.Model)
		if model == "" {
			return nil, fmt.Errorf("stage %s model is empty", item.stage)
		}
		modelCfg, ok := config.LookupLLMModelConfig(sys, model)
		if !ok {
			return nil, fmt.Errorf("stage %s model config not found: %s", item.stage, model)
		}
		timeout := 30 * time.Second
		if modelCfg.TimeoutSec != nil && *modelCfg.TimeoutSec > 0 {
			timeout = time.Duration(*modelCfg.TimeoutSec) * time.Second
		}
		targets = append(targets, ProbeTarget{
			Stage:     item.stage,
			Model:     model,
			Endpoint:  strings.TrimSpace(modelCfg.Endpoint),
			APIKey:    strings.TrimSpace(modelCfg.APIKey),
			Timeout:   timeout,
			ConfigKey: config.CanonicalLLMModelKey(model),
		})
	}
	return targets, nil
}

func ProbeStructuredSupport(ctx context.Context, target ProbeTarget) error {
	client := &llm.OpenAIClient{
		Endpoint:         target.Endpoint,
		Model:            target.Model,
		APIKey:           target.APIKey,
		Timeout:          target.Timeout,
		StructuredOutput: true,
	}
	return ProbeStructuredSupportWithClient(ctx, client)
}

func ProbeStructuredSupportWithClient(ctx context.Context, client *llm.OpenAIClient) error {
	if client == nil {
		return fmt.Errorf("client is required")
	}
	schema := &llm.JSONSchema{
		Name: "structured_support_probe",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ok": map[string]any{"type": "boolean"},
			},
			"required":             []string{"ok"},
			"additionalProperties": false,
		},
	}
	raw, err := client.CallStructured(ctx, "Return JSON only.", "Return {\"ok\": true}.", schema)
	if err != nil {
		return err
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return fmt.Errorf("probe decode failed: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("probe returned ok=false")
	}
	return nil
}

func ProbeTargets(ctx context.Context, targets []ProbeTarget) []ProbeResult {
	results := make([]ProbeResult, 0, len(targets))
	for _, target := range targets {
		callCtx := ctx
		var cancel context.CancelFunc
		if target.Timeout > 0 {
			callCtx, cancel = context.WithTimeout(ctx, target.Timeout)
		}
		err := ProbeStructuredSupport(callCtx, target)
		if cancel != nil {
			cancel()
		}
		results = append(results, ProbeResult{
			Target:    target,
			Supported: err == nil,
			Error:     err,
		})
	}
	return results
}

func normalizeStage(stage string) string {
	return strings.ToLower(strings.TrimSpace(stage))
}

func isValidStage(stage string) bool {
	switch stage {
	case "indicator", "structure", "mechanics":
		return true
	default:
		return false
	}
}

func pickStageRole(sym config.SymbolConfig, stage string) config.LLMRoleConfig {
	switch stage {
	case "indicator":
		if strings.TrimSpace(sym.LLM.Agent.Indicator.Model) != "" {
			return sym.LLM.Agent.Indicator
		}
		return sym.LLM.Provider.Indicator
	case "structure":
		if strings.TrimSpace(sym.LLM.Agent.Structure.Model) != "" {
			return sym.LLM.Agent.Structure
		}
		return sym.LLM.Provider.Structure
	case "mechanics":
		if strings.TrimSpace(sym.LLM.Agent.Mechanics.Model) != "" {
			return sym.LLM.Agent.Mechanics
		}
		return sym.LLM.Provider.Mechanics
	default:
		return config.LLMRoleConfig{}
	}
}
