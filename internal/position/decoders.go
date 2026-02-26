// 本文件主要内容：提供风险计划与注解的解码辅助函数。

package position

import (
	"encoding/json"
	"fmt"

	"brale-core/internal/execution"
	"brale-core/internal/risk"
)

func DecodeRiskPlan(raw []byte) (risk.RiskPlan, error) {
	if len(raw) == 0 {
		return risk.RiskPlan{}, fmt.Errorf("risk plan empty")
	}
	var plan risk.RiskPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return risk.RiskPlan{}, err
	}
	return plan, nil
}

func DecodeRiskAnnotations(raw []byte) (execution.RiskAnnotations, error) {
	if len(raw) == 0 {
		return execution.RiskAnnotations{}, fmt.Errorf("risk annotations empty")
	}
	var ann execution.RiskAnnotations
	if err := json.Unmarshal(raw, &ann); err != nil {
		return execution.RiskAnnotations{}, err
	}
	return ann, nil
}
