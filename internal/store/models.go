// 本文件主要内容：定义 brale-core 持久化模型结构体。

package store

import (
	"time"

	"gorm.io/datatypes"
)

type AgentEventRecord struct {
	ID                 uint           `gorm:"primaryKey"`
	SnapshotID         uint           `gorm:"index:idx_agent_symbol_stage_time,priority:3"`
	Symbol             string         `gorm:"index:idx_agent_symbol_stage_time,priority:1"`
	Timestamp          int64          `gorm:"index:idx_agent_symbol_stage_time,priority:2,sort:desc"`
	Stage              string         `gorm:"index:idx_agent_symbol_stage_time,priority:4"`
	SystemPrompt       string         `gorm:"type:text"`
	UserPrompt         string         `gorm:"type:text"`
	OutputJSON         datatypes.JSON `gorm:"type:json"`
	Fingerprint        string         `gorm:"index:idx_agent_fingerprint"`
	SystemConfigHash   string         `gorm:"index:idx_agent_sys_strat,priority:1"`
	StrategyConfigHash string         `gorm:"index:idx_agent_sys_strat,priority:2"`
	SourceVersion      string         `gorm:"index:idx_agent_source_ver"`
	CreatedAt          time.Time
}

type ProviderEventRecord struct {
	ID                 uint `gorm:"primaryKey"`
	SnapshotID         uint
	Symbol             string `gorm:"index:idx_prov_symbol_role_time,priority:1"`
	Timestamp          int64  `gorm:"index:idx_prov_symbol_role_time,priority:2,sort:desc"`
	ProviderID         string
	Role               string         `gorm:"index:idx_prov_symbol_role_time,priority:3"`
	SystemPrompt       string         `gorm:"type:text"`
	UserPrompt         string         `gorm:"type:text"`
	OutputJSON         datatypes.JSON `gorm:"type:json"`
	Tradeable          bool
	Fingerprint        string `gorm:"index:idx_prov_fingerprint"`
	SystemConfigHash   string `gorm:"index:idx_prov_sys_strat,priority:1"`
	StrategyConfigHash string `gorm:"index:idx_prov_sys_strat,priority:2"`
	SourceVersion      string `gorm:"index:idx_prov_source_ver"`
	CreatedAt          time.Time
}

type GateEventRecord struct {
	ID                 uint `gorm:"primaryKey"`
	SnapshotID         uint
	Symbol             string `gorm:"index:idx_gate_symbol_time,priority:1"`
	Timestamp          int64  `gorm:"index:idx_gate_symbol_time,priority:2,sort:desc"`
	GlobalTradeable    bool
	DecisionAction     string
	Grade              int
	GateReason         string
	Direction          string
	ProviderRefsJSON   datatypes.JSON `gorm:"type:json"`
	RuleHitJSON        datatypes.JSON `gorm:"type:json"`
	DerivedJSON        datatypes.JSON `gorm:"type:json"`
	Fingerprint        string         `gorm:"index:idx_gate_fingerprint"`
	SystemConfigHash   string         `gorm:"index:idx_gate_sys_strat,priority:1"`
	StrategyConfigHash string         `gorm:"index:idx_gate_sys_strat,priority:2"`
	SourceVersion      string         `gorm:"index:idx_gate_source_ver"`
	CreatedAt          time.Time
}

type RiskPlanHistoryRecord struct {
	ID          uint   `gorm:"primaryKey"`
	PositionID  string `gorm:"index:idx_risk_plan_position,priority:1"`
	Version     int    `gorm:"index:idx_risk_plan_position,priority:2"`
	Source      string
	PayloadJSON datatypes.JSON `gorm:"type:json"`
	CreatedAt   time.Time
}

type PositionRecord struct {
	ID                 uint   `gorm:"primaryKey"`
	PositionID         string `gorm:"uniqueIndex:idx_position_id"`
	Symbol             string `gorm:"index:idx_position_symbol_status,priority:1"`
	Side               string
	InitialStake       float64
	Qty                float64
	AvgEntry           float64
	RiskPct            float64
	Leverage           float64
	Status             string `gorm:"index:idx_position_symbol_status,priority:2"`
	OpenIntentID       string `gorm:"index:idx_position_open_intent"`
	CloseIntentID      string `gorm:"index:idx_position_close_intent"`
	AbortReason        string
	Source             string
	AbortStartedAt     int64
	AbortFinalizedAt   int64
	CloseSubmittedAt   int64
	PeakUnrealizedR    float64        `gorm:"-"`
	RiskJSON           datatypes.JSON `gorm:"type:json"`
	LastPrice          float64        `gorm:"-"`
	LastPriceTimestamp int64          `gorm:"-"`
	ExecutorName       string
	ExecutorPositionID string
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
