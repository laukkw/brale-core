package dashboard

type RiskPlanTimelineItem struct {
	Source              string
	Label               string
	CreatedAt           string
	StopLoss            float64
	TakeProfits         []float64
	PreviousStopLoss    float64
	PreviousTakeProfits []float64
}

type RiskState struct {
	StopLoss    float64
	TakeProfits []float64
	Timeline    []RiskPlanTimelineItem
}

type TightenInfo struct {
	Triggered bool
	Reason    string
}

type FlowAnchor struct {
	Type       string
	SnapshotID uint
	Confidence string
	Reason     string
}

type FlowSelection struct {
	Anchor FlowAnchor
}

type ConsensusMetrics struct {
	Score               *float64
	Confidence          *float64
	ScoreThreshold      *float64
	ConfidenceThreshold *float64
	ScorePassed         *bool
	ConfidencePassed    *bool
	Passed              *bool
}

type DecisionTightenDetail struct {
	Action         string
	Evaluated      bool
	Eligible       bool
	Executed       bool
	TPTightened    bool
	BlockedBy      []string
	Score          float64
	ScoreThreshold float64
	ScoreParseOK   bool
	DisplayReason  string
}
