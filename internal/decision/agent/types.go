// 本文件主要内容：定义 Agent 输出摘要与枚举类型。
package agent

import (
	"brale-core/internal/decision/decisionutil"
)

type Expansion string

type Alignment string

type Noise string

type Regime string

type LastBreak string

type Quality string

type Pattern string

type LeverageState string

type Crowding string

type RiskLevel string

const (
	ExpansionExpanding   Expansion = "expanding"
	ExpansionContracting Expansion = "contracting"
	ExpansionStable      Expansion = "stable"
	ExpansionMixed       Expansion = "mixed"
	ExpansionUnknown     Expansion = "unknown"
)

const (
	AlignmentAligned   Alignment = "aligned"
	AlignmentMixed     Alignment = "mixed"
	AlignmentDivergent Alignment = "divergent"
	AlignmentUnknown   Alignment = "unknown"
)

const (
	NoiseLow     Noise = "low"
	NoiseMedium  Noise = "medium"
	NoiseHigh    Noise = "high"
	NoiseMixed   Noise = "mixed"
	NoiseUnknown Noise = "unknown"
)

const (
	RegimeTrendUp   Regime = "trend_up"
	RegimeTrendDown Regime = "trend_down"
	RegimeRange     Regime = "range"
	RegimeMixed     Regime = "mixed"
	RegimeUnclear   Regime = "unclear"
)

const (
	LastBreakBosUp     LastBreak = "bos_up"
	LastBreakBosDown   LastBreak = "bos_down"
	LastBreakChochUp   LastBreak = "choch_up"
	LastBreakChochDown LastBreak = "choch_down"
	LastBreakNone      LastBreak = "none"
	LastBreakUnknown   LastBreak = "unknown"
)

const (
	QualityClean   Quality = "clean"
	QualityMessy   Quality = "messy"
	QualityMixed   Quality = "mixed"
	QualityUnclear Quality = "unclear"
)

const (
	PatternDoubleTop        Pattern = "double_top"
	PatternDoubleBottom     Pattern = "double_bottom"
	PatternHeadShoulders    Pattern = "head_shoulders"
	PatternInvHeadShoulders Pattern = "inv_head_shoulders"
	PatternTriangleSym      Pattern = "triangle_sym"
	PatternTriangleAsc      Pattern = "triangle_asc"
	PatternTriangleDesc     Pattern = "triangle_desc"
	PatternWedgeRising      Pattern = "wedge_rising"
	PatternWedgeFalling     Pattern = "wedge_falling"
	PatternFlag             Pattern = "flag"
	PatternPennant          Pattern = "pennant"
	PatternChannelUp        Pattern = "channel_up"
	PatternChannelDown      Pattern = "channel_down"
	PatternNone             Pattern = "none"
	PatternUnknown          Pattern = "unknown"
)

const (
	LeverageStateIncreasing LeverageState = "increasing"
	LeverageStateStable     LeverageState = "stable"
	LeverageStateOverheated LeverageState = "overheated"
	LeverageStateUnknown    LeverageState = "unknown"
)

const (
	CrowdingLong     Crowding = "long_crowded"
	CrowdingShort    Crowding = "short_crowded"
	CrowdingBalanced Crowding = "balanced"
	CrowdingUnknown  Crowding = "unknown"
)

const (
	RiskLevelLow     RiskLevel = "low"
	RiskLevelMedium  RiskLevel = "medium"
	RiskLevelHigh    RiskLevel = "high"
	RiskLevelUnknown RiskLevel = "unknown"
)

var expansionSet = map[string]struct{}{
	string(ExpansionExpanding):   {},
	string(ExpansionContracting): {},
	string(ExpansionStable):      {},
	string(ExpansionMixed):       {},
	string(ExpansionUnknown):     {},
}

var alignmentSet = map[string]struct{}{
	string(AlignmentAligned):   {},
	string(AlignmentMixed):     {},
	string(AlignmentDivergent): {},
	string(AlignmentUnknown):   {},
}

var noiseSet = map[string]struct{}{
	string(NoiseLow):     {},
	string(NoiseMedium):  {},
	string(NoiseHigh):    {},
	string(NoiseMixed):   {},
	string(NoiseUnknown): {},
}

var regimeSet = map[string]struct{}{
	string(RegimeTrendUp):   {},
	string(RegimeTrendDown): {},
	string(RegimeRange):     {},
	string(RegimeMixed):     {},
	string(RegimeUnclear):   {},
}

var lastBreakSet = map[string]struct{}{
	string(LastBreakBosUp):     {},
	string(LastBreakBosDown):   {},
	string(LastBreakChochUp):   {},
	string(LastBreakChochDown): {},
	string(LastBreakNone):      {},
	string(LastBreakUnknown):   {},
}

var qualitySet = map[string]struct{}{
	string(QualityClean):   {},
	string(QualityMessy):   {},
	string(QualityMixed):   {},
	string(QualityUnclear): {},
}

var patternSet = map[string]struct{}{
	string(PatternDoubleTop):        {},
	string(PatternDoubleBottom):     {},
	string(PatternHeadShoulders):    {},
	string(PatternInvHeadShoulders): {},
	string(PatternTriangleSym):      {},
	string(PatternTriangleAsc):      {},
	string(PatternTriangleDesc):     {},
	string(PatternWedgeRising):      {},
	string(PatternWedgeFalling):     {},
	string(PatternFlag):             {},
	string(PatternPennant):          {},
	string(PatternChannelUp):        {},
	string(PatternChannelDown):      {},
	string(PatternNone):             {},
	string(PatternUnknown):          {},
}

var leverageStateSet = map[string]struct{}{
	string(LeverageStateIncreasing): {},
	string(LeverageStateStable):     {},
	string(LeverageStateOverheated): {},
	string(LeverageStateUnknown):    {},
}

var crowdingSet = map[string]struct{}{
	string(CrowdingLong):     {},
	string(CrowdingShort):    {},
	string(CrowdingBalanced): {},
	string(CrowdingUnknown):  {},
}

var riskLevelSet = map[string]struct{}{
	string(RiskLevelLow):     {},
	string(RiskLevelMedium):  {},
	string(RiskLevelHigh):    {},
	string(RiskLevelUnknown): {},
}

func (e *Expansion) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, expansionSet, "expansion")
	if err != nil {
		return err
	}
	*e = Expansion(value)
	return nil
}

func (a *Alignment) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, alignmentSet, "alignment")
	if err != nil {
		return err
	}
	*a = Alignment(value)
	return nil
}

func (n *Noise) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, noiseSet, "noise")
	if err != nil {
		return err
	}
	*n = Noise(value)
	return nil
}

func (r *Regime) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, regimeSet, "regime")
	if err != nil {
		return err
	}
	*r = Regime(value)
	return nil
}

func (b *LastBreak) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, lastBreakSet, "last_break")
	if err != nil {
		value, err = parseEnum(data, lastBreakAliasSet, "last_break")
		if err != nil {
			return err
		}
	}
	value = normalizeLastBreak(value)
	*b = LastBreak(value)
	return nil
}

var lastBreakAliasSet = map[string]struct{}{
	"break_up":   {},
	"break_down": {},
}

func normalizeLastBreak(value string) string {
	switch value {
	case "break_up":
		return string(LastBreakBosUp)
	case "break_down":
		return string(LastBreakBosDown)
	default:
		return value
	}
}

func (q *Quality) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, qualitySet, "quality")
	if err != nil {
		return err
	}
	*q = Quality(value)
	return nil
}

func (p *Pattern) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, patternSet, "pattern")
	if err != nil {
		return err
	}
	*p = Pattern(value)
	return nil
}

func (s *LeverageState) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, leverageStateSet, "leverage_state")
	if err != nil {
		return err
	}
	*s = LeverageState(value)
	return nil
}

func (c *Crowding) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, crowdingSet, "crowding")
	if err != nil {
		return err
	}
	*c = Crowding(value)
	return nil
}

func (r *RiskLevel) UnmarshalJSON(data []byte) error {
	value, err := parseEnum(data, riskLevelSet, "risk_level")
	if err != nil {
		return err
	}
	*r = RiskLevel(value)
	return nil
}

func parseEnum(data []byte, allowed map[string]struct{}, name string) (string, error) {
	return decisionutil.ParseEnumJSON(data, allowed, name)
}

type IndicatorSummary struct {
	Expansion          Expansion `json:"expansion"`
	Alignment          Alignment `json:"alignment"`
	Noise              Noise     `json:"noise"`
	MomentumDetail     string    `json:"momentum_detail"`
	ConflictDetail     string    `json:"conflict_detail"`
	MovementScore      float64   `json:"movement_score"`
	MovementConfidence float64   `json:"movement_confidence"`
}

type StructureSummary struct {
	Regime             Regime    `json:"regime"`
	LastBreak          LastBreak `json:"last_break"`
	Quality            Quality   `json:"quality"`
	Pattern            Pattern   `json:"pattern"`
	VolumeAction       string    `json:"volume_action"`
	CandleReaction     string    `json:"candle_reaction"`
	MovementScore      float64   `json:"movement_score"`
	MovementConfidence float64   `json:"movement_confidence"`
}

type MechanicsSummary struct {
	LeverageState       LeverageState `json:"leverage_state"`
	Crowding            Crowding      `json:"crowding"`
	RiskLevel           RiskLevel     `json:"risk_level"`
	OpenInterestContext string        `json:"open_interest_context"`
	AnomalyDetail       string        `json:"anomaly_detail"`
	MovementScore       float64       `json:"movement_score"`
	MovementConfidence  float64       `json:"movement_confidence"`
}
