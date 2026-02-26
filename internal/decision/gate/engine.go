// 本文件主要内容：实现 Gate 入场与持仓决策逻辑。
package gate

import "strings"

func EvaluateHold(mon MonitorAtomic) Decision {
	if !mon.StructureIntegrity || strings.EqualFold(mon.ThreatLevel, "critical") {
		return Decision{
			Action:     ActionPanicClose,
			Grade:      0,
			Direction:  "none",
			ReasonCode: "STRUCT_BREAK",
		}
	}
	if mon.AdverseLiquidation {
		return Decision{
			Action:     ActionRiskOff,
			Grade:      0,
			Direction:  "none",
			ReasonCode: "MECH_RISK",
		}
	}
	if mon.CrowdingReversal || mon.DivergenceDetected || !mon.MomentumSustaining {
		return Decision{
			Action:     ActionRiskOff,
			Grade:      0,
			Direction:  "none",
			ReasonCode: "MOMENTUM_WEAK",
		}
	}
	return Decision{
		Action:     ActionKeep,
		Grade:      0,
		Direction:  "none",
		ReasonCode: "KEEP",
	}
}

func IndicatorTradeable(ind IndicatorAtomic) bool {
	return ind.MomentumExpansion && ind.Alignment && !ind.MeanRevNoise
}

func StructureTradeable(st StructureAtomic) bool {
	return st.ClearStructure && st.Integrity
}

func MechanicsTradeable(mech MechanicsAtomic) bool {
	return !mech.LiquidationStress
}
