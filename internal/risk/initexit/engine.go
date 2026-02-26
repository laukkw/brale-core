package initexit

import (
	"context"
	"fmt"
	"strings"
)

const DefaultPolicy = "atr_structure_v1"

func BuildInitial(ctx context.Context, policyName string, input BuildInput) (BuildOutput, error) {
	name := normalizePolicyName(policyName)
	if name == "" {
		name = DefaultPolicy
	}
	policy, err := MustGet(name)
	if err != nil {
		return BuildOutput{}, err
	}
	if err := policy.ValidateParams(input.Params); err != nil {
		return BuildOutput{}, fmt.Errorf("invalid params for policy %s: %w", name, err)
	}
	out, err := policy.Build(ctx, input)
	if err != nil {
		return BuildOutput{}, fmt.Errorf("build initial exit with policy %s: %w", name, err)
	}
	if strings.TrimSpace(out.StopSource) == "" {
		out.StopSource = name
	}
	if strings.TrimSpace(out.StopReason) == "" {
		out.StopReason = name
	}
	out, err = ValidateAndNormalize(input.Direction, input.Entry, out)
	if err != nil {
		return BuildOutput{}, fmt.Errorf("validate initial exit output: %w", err)
	}
	return out, nil
}

func ValidatePolicyConfig(policyName string, params map[string]any) error {
	name := normalizePolicyName(policyName)
	if name == "" {
		name = DefaultPolicy
	}
	policy, err := MustGet(name)
	if err != nil {
		return err
	}
	if err := policy.ValidateParams(params); err != nil {
		return fmt.Errorf("invalid params for policy %s: %w", name, err)
	}
	return nil
}
