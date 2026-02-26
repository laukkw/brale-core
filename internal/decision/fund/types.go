// 本文件主要内容：定义资金决策与 Provider 汇总结构。
package fund

import "brale-core/internal/decision/provider"

type GateRuleHit struct {
	Name      string
	Priority  int
	Action    string
	Reason    string
	Grade     int
	Direction string
	Default   bool
}

type GateDecision struct {
	GlobalTradeable bool
	DecisionAction  string
	GateReason      string
	Direction       string
	Grade           int
	RuleHit         *GateRuleHit
	Derived         map[string]any
}

type ProviderBundle struct {
	Indicator provider.IndicatorProviderOut
	Structure provider.StructureProviderOut
	Mechanics provider.MechanicsProviderOut
	Enabled   ProviderEnabled
}

type ProviderEnabled struct {
	Indicator bool
	Structure bool
	Mechanics bool
}

type StructureSignal struct {
	Direction string
}
