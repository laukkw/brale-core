package config

import "brale-core/internal/risk/initexit"

type InitialExitPolicyValidator func(policyName string, params map[string]any) error

var initialExitPolicyValidator InitialExitPolicyValidator = initexit.ValidatePolicyConfig

func SetInitialExitPolicyValidator(v InitialExitPolicyValidator) {
	if v == nil {
		initialExitPolicyValidator = initexit.ValidatePolicyConfig
		return
	}
	initialExitPolicyValidator = v
}
