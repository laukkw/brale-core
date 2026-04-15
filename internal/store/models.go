// Package store defines brale-core persistence model types and store interfaces.
// Models are backend-agnostic; the concrete implementation lives in internal/pgstore.

package store

import (
	"encoding/json"
	"time"
)

type AgentEventRecord struct {
	ID                 uint
	SnapshotID         uint
	RoundID            string
	Symbol             string
	Timestamp          int64
	Stage              string
	InputJSON          json.RawMessage
	SystemPrompt       string
	UserPrompt         string
	OutputJSON         json.RawMessage
	RawOutput          string
	Fingerprint        string
	SystemConfigHash   string
	StrategyConfigHash string
	SourceVersion      string
	Model              string
	PromptVersion      string
	LatencyMS          int
	TokenIn            int
	TokenOut           int
	Error              string
	CreatedAt          time.Time
}

type ProviderEventRecord struct {
	ID                 uint
	SnapshotID         uint
	RoundID            string
	Symbol             string
	Timestamp          int64
	ProviderID         string
	Role               string
	DataContextJSON    json.RawMessage
	SystemPrompt       string
	UserPrompt         string
	OutputJSON         json.RawMessage
	RawOutput          string
	Tradeable          bool
	Fingerprint        string
	SystemConfigHash   string
	StrategyConfigHash string
	SourceVersion      string
	Model              string
	PromptVersion      string
	LatencyMS          int
	TokenIn            int
	TokenOut           int
	Error              string
	CreatedAt          time.Time
}

type GateEventRecord struct {
	ID                 uint
	SnapshotID         uint
	RoundID            string
	Symbol             string
	Timestamp          int64
	GlobalTradeable    bool
	DecisionAction     string
	Grade              int
	GateReason         string
	Direction          string
	ProviderRefsJSON   json.RawMessage
	RuleHitJSON        json.RawMessage
	DerivedJSON        json.RawMessage
	Fingerprint        string
	SystemConfigHash   string
	StrategyConfigHash string
	SourceVersion      string
	CreatedAt          time.Time
}

type RiskPlanHistoryRecord struct {
	ID          uint
	PositionID  string
	Version     int
	Source      string
	PayloadJSON json.RawMessage
	CreatedAt   time.Time
}

type PositionRecord struct {
	ID                 uint
	PositionID         string
	Symbol             string
	Side               string
	InitialStake       float64
	Qty                float64
	AvgEntry           float64
	RiskPct            float64
	Leverage           float64
	Status             string
	OpenIntentID       string
	CloseIntentID      string
	AbortReason        string
	Source             string
	StopReason         string
	AbortStartedAt     int64
	AbortFinalizedAt   int64
	CloseSubmittedAt   int64
	PeakUnrealizedR    float64 // runtime-only, not persisted
	RiskJSON           json.RawMessage
	LastPrice          float64 // runtime-only, not persisted
	LastPriceTimestamp int64   // runtime-only, not persisted
	ExecutorName       string
	ExecutorPositionID string
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type EpisodicMemoryRecord struct {
	ID            uint
	Symbol        string
	PositionID    string
	Direction     string
	EntryPrice    string
	ExitPrice     string
	PnLPercent    string
	Duration      string
	Reflection    string
	KeyLessons    string
	MarketContext string
	CreatedAt     time.Time
}

type SemanticMemoryRecord struct {
	ID         uint
	Symbol     string
	RuleText   string
	Source     string
	Confidence float64
	Active     bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// LLMRoundRecord tracks one complete decision round (all agent + provider + gate calls).
type LLMRoundRecord struct {
	ID             string
	SnapshotID     uint
	Symbol         string
	RoundType      string // "observe" | "decide" | "risk"
	StartedAt      time.Time
	FinishedAt     time.Time
	TotalLatencyMS int
	TotalTokenIn   int
	TotalTokenOut  int
	CallCount      int
	Outcome        string
	PromptVersion  string
	Error          string
	AgentCount     int
	ProviderCount  int
	GateAction     string
	CreatedAt      time.Time
}

// PromptRegistryEntry stores a versioned system prompt for a specific role+stage.
type PromptRegistryEntry struct {
	ID           uint
	Role         string
	Stage        string
	Version      string
	SystemPrompt string
	Description  string
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
