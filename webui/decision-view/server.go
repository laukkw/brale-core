// 决策查看模块，提供决策链 API、配置图 API，并内嵌 Roam 风格静态页面。
// Package decisionview 汇总符号决策链与配置，供审计前端使用。
package decisionview

import (
	"embed"

	"brale-core/internal/config"
	"brale-core/internal/store"
)

//go:embed index.html styles.css main.js favicon-mask.svg vendor/*.js
var content embed.FS

type Server struct {
	Store         decisionViewStore
	BasePath      string
	RoundLimit    int
	SystemConfig  config.SystemConfig
	SymbolConfigs map[string]ConfigBundle
}

type decisionViewStore interface {
	store.SymbolCatalogQueryStore
	store.TimelineQueryStore
	store.PositionQueryStore
}

type Response struct {
	Symbols []SymbolChain `json:"symbols"`
}

type SymbolMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type SymbolChain struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Rounds []Round `json:"rounds"`
}

type Round struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	SnapshotID uint   `json:"snapshot_id,omitempty"`
	Timestamp  int64  `json:"timestamp,omitempty"`
	Nodes      []Node `json:"nodes"`
}

type Node struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary,omitempty"`
	Stage       string         `json:"stage"`
	Type        string         `json:"type,omitempty"`
	AgentKey    string         `json:"agentKey,omitempty"`
	Refs        []string       `json:"refs,omitempty"`
	Input       any            `json:"input,omitempty"`
	Output      any            `json:"output,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	SymbolID    string         `json:"symbolId,omitempty"`
	SymbolLabel string         `json:"symbolLabel,omitempty"`
	RoundID     string         `json:"roundId,omitempty"`
	RoundLabel  string         `json:"roundLabel,omitempty"`
}

type gateDisplay struct {
	Overall   gateOverall          `json:"overall"`
	Providers []gateProviderStatus `json:"providers,omitempty"`
	Derived   map[string]any       `json:"derived,omitempty"`
	Report    string               `json:"report"`
}

type llmRiskTraceRecord struct {
	Role         string `json:"role"`
	Timestamp    int64  `json:"timestamp"`
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
	Output       any    `json:"output"`
}

type gateOverall struct {
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

type gateProviderStatus struct {
	Role          string       `json:"role"`
	Tradeable     bool         `json:"tradeable"`
	TradeableText string       `json:"tradeable_text"`
	Factors       []gateFactor `json:"factors,omitempty"`
}

type gateFactor struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Raw    any    `json:"raw,omitempty"`
}
