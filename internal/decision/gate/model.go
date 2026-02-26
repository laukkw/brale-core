// 本文件主要内容：定义 Gate 判定的原子输入与决策结构。
package gate

const (
	ActionAllow      = "ALLOW"
	ActionWait       = "WAIT"
	ActionVeto       = "VETO"
	ActionKeep       = "KEEP"
	ActionRiskOff    = "RISK_OFF"
	ActionPanicClose = "PANIC_CLOSE"
)

type Decision struct {
	Action     string
	Grade      int
	Direction  string
	ReasonCode string
}

type IndicatorAtomic struct {
	MomentumExpansion bool
	Alignment         bool
	MeanRevNoise      bool
}

type StructureAtomic struct {
	ClearStructure bool
	Integrity      bool
}

type MechanicsAtomic struct {
	LiquidationStress bool
}

type MonitorAtomic struct {
	StructureIntegrity bool
	ThreatLevel        string
	AdverseLiquidation bool
	CrowdingReversal   bool
	MomentumSustaining bool
	DivergenceDetected bool
}
