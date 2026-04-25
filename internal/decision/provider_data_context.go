package decision

// ProviderDataContext carries code-computed quantitative summaries
// alongside Agent LLM summaries for Provider evaluation. Each Provider
// receives both its Agent's qualitative assessment and these grounding
// anchors, allowing it to cross-validate claims against computed reality.
type ProviderDataContext struct {
	IndicatorCrossTF   *IndicatorCrossTFContext `json:"indicator_cross_tf,omitempty"`
	StructureAnchorCtx *StructureAnchorContext  `json:"structure_anchor,omitempty"`
	MechanicsCtx       *MechanicsDataContext    `json:"mechanics,omitempty"`
}

// IndicatorCrossTFContext provides code-computed multi-timeframe alignment
// signals for the Indicator Provider to cross-validate Agent claims.
type IndicatorCrossTFContext struct {
	DecisionTFBias    string `json:"decision_tf_bias"`
	Alignment         string `json:"alignment"`
	ConflictCount     int    `json:"conflict_count"`
	LowerTFAgreement  bool   `json:"lower_tf_agreement"`
	HigherTFAgreement bool   `json:"higher_tf_agreement"`
}

// StructureAnchorContext provides code-computed structure event data
// for the Structure Provider to cross-validate Agent claims.
type StructureAnchorContext struct {
	LatestBreakType    string `json:"latest_break_type,omitempty"`
	LatestBreakBarAge  int    `json:"latest_break_bar_age,omitempty"`
	SupertrendState    string `json:"supertrend_state,omitempty"`
	SupertrendInterval string `json:"supertrend_interval,omitempty"`
}

// MechanicsDataContext provides code-computed mechanics conflict signals
// for the Mechanics Provider to cross-validate Agent claims.
type MechanicsDataContext struct {
	Conflicts         []string                           `json:"conflicts,omitempty"`
	ReversalRisk      string                             `json:"reversal_risk,omitempty"`
	LiquidationState  *MechanicsLiquidationContext       `json:"liquidation_state,omitempty"`
	LiquidationSource *MechanicsLiquidationSourceContext `json:"liquidation_source,omitempty"`
}

type MechanicsLiquidationContext struct {
	Stress   string `json:"stress,omitempty"`
	Status   string `json:"status,omitempty"`
	Window   string `json:"window,omitempty"`
	Complete bool   `json:"complete,omitempty"`
}

type MechanicsLiquidationSourceContext struct {
	Source          string `json:"source,omitempty"`
	Coverage        string `json:"coverage,omitempty"`
	Status          string `json:"status,omitempty"`
	StreamConnected bool   `json:"stream_connected,omitempty"`
	CoverageSec     int64  `json:"coverage_sec,omitempty"`
	SampleCount     int    `json:"sample_count,omitempty"`
	LastEventAgeSec int64  `json:"last_event_age_sec,omitempty"`
	Complete        bool   `json:"complete,omitempty"`
}
