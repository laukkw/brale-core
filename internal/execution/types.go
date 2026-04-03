package execution

// 本文件主要内容：定义执行计划与风险标注结构。
import "time"

const (
	PlanSourceGo  = "go"
	PlanSourceLLM = "llm"
)

type RiskAnnotations struct {
	StopSource   string
	StopReason   string
	RiskDistance float64
	ATR          float64
	BufferATR    float64
	MaxInvestPct float64
	MaxInvestAmt float64
	MaxLeverage  float64
	LiqPrice     float64
	MMR          float64
	Fee          float64
}

type LLMRiskTrace struct {
	Stage        string
	Flow         string
	SystemPrompt string
	UserPrompt   string
	RawOutput    string
	ParsedOutput any
}

type ExecutionPlan struct {
	Symbol             string
	Valid              bool
	InvalidReason      string
	Direction          string
	Entry              float64
	StopLoss           float64
	TakeProfits        []float64
	TakeProfitRatios   []float64
	RiskPct            float64
	PositionSize       float64
	Leverage           float64
	RMultiple          float64
	Template           string
	PlanSource         string
	StrategyID         string
	SystemConfigHash   string
	StrategyConfigHash string
	PositionID         string
	LLMRiskTrace       *LLMRiskTrace
	RiskAnnotations    RiskAnnotations
	CreatedAt          time.Time
	ExpiresAt          time.Time
}

type AccountState struct {
	Equity    float64
	Available float64
	Currency  string
}

type RiskParams struct {
	RiskPerTradePct float64
}

type OrderStatus struct {
	Status       string
	Filled       float64
	Price        float64
	Fee          float64
	Timestamp    int64
	Reason       string
	RawStatus    string
	CancelReason string
}
