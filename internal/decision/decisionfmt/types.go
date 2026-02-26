// 本文件主要内容：定义决策展示与通知格式的接口与数据结构。
package decisionfmt

import (
	"encoding/json"
)

type Formatter interface {
	BuildGateReport(record GateEvent) (GateReport, error)
	BuildMissingGateReport(snapshotID uint) GateReport
	BuildDecisionReport(input DecisionInput) (DecisionReport, error)
	HumanizeLLMOutput(raw json.RawMessage) (any, any, error)
	RenderGateText(report GateReport) string
	RenderDecisionMarkdown(report DecisionReport) string
	RenderDecisionHTML(report DecisionReport) string
}

type GateEvent struct {
	ID               uint
	SnapshotID       uint
	GlobalTradeable  bool
	DecisionAction   string
	Grade            int
	GateReason       string
	Direction        string
	ProviderRefsJSON json.RawMessage
	RuleHitJSON      json.RawMessage
	DerivedJSON      json.RawMessage
}

type ProviderEvent struct {
	SnapshotID uint
	OutputJSON json.RawMessage
	Role       string
}

type AgentEvent struct {
	SnapshotID uint
	OutputJSON json.RawMessage
	Stage      string
}

type DecisionInput struct {
	Symbol     string
	SnapshotID uint
	Gate       GateEvent
	Providers  []ProviderEvent
	Agents     []AgentEvent
}

type DecisionReport struct {
	Symbol     string
	SnapshotID uint
	Gate       GateReport
	Providers  []StageOutput
	Agents     []StageOutput
}

type StageOutput struct {
	Role    string
	Summary string
}

type GateReport struct {
	Overall   GateOverall
	Providers []GateProviderStatus
	RuleHit   *GateRuleHit
	Derived   map[string]any
}

type GateOverall struct {
	Tradeable      bool   `json:"tradeable"`
	TradeableText  string `json:"tradeable_text"`
	DecisionAction string `json:"decision_action"`
	DecisionText   string `json:"decision_text"`
	Grade          int    `json:"grade"`
	Reason         string `json:"reason"`
	ReasonCode     string `json:"reason_code"`
	Direction      string `json:"direction"`
	ExpectedSnapID uint   `json:"expected_snapshot_id,omitempty"`
}

type GateProviderStatus struct {
	Role          string       `json:"role"`
	Tradeable     bool         `json:"tradeable"`
	TradeableText string       `json:"tradeable_text"`
	Factors       []GateFactor `json:"factors,omitempty"`
}

type GateRuleHit struct {
	Name      string `json:"name"`
	Priority  int    `json:"priority"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Grade     int    `json:"grade"`
	Direction string `json:"direction"`
	Default   bool   `json:"default"`
}

type GateFactor struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Raw    any    `json:"raw,omitempty"`
}
