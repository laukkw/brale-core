// Package store defines persistence interfaces and model types for brale-core.
// The concrete PostgreSQL implementation lives in internal/pgstore.

package store

import (
	"context"
	"time"
)

type EventCommandStore interface {
	SaveAgentEvent(ctx context.Context, rec *AgentEventRecord) error
	SaveProviderEvent(ctx context.Context, rec *ProviderEventRecord) error
	SaveGateEvent(ctx context.Context, rec *GateEventRecord) error
	SaveRiskPlanHistory(ctx context.Context, rec *RiskPlanHistoryRecord) error
}

type PositionCommandStore interface {
	SavePosition(ctx context.Context, rec *PositionRecord) error
	UpdatePosition(ctx context.Context, positionID string, expectedVersion int, updates map[string]any) (bool, error)
	UpdatePositionPatch(ctx context.Context, patch PositionPatch) (bool, error)
}

type PositionQueryStore interface {
	FindPositionByID(ctx context.Context, positionID string) (PositionRecord, bool, error)
	FindPositionBySymbol(ctx context.Context, symbol string, statuses []string) (PositionRecord, bool, error)
	ListPositionsByStatus(ctx context.Context, statuses []string) ([]PositionRecord, error)
}

type RiskPlanQueryStore interface {
	FindLatestRiskPlanHistory(ctx context.Context, positionID string) (RiskPlanHistoryRecord, bool, error)
	ListRiskPlanHistory(ctx context.Context, positionID string, limit int) ([]RiskPlanHistoryRecord, error)
}

type TimelineQueryStore interface {
	ListProviderEvents(ctx context.Context, symbol string, limit int) ([]ProviderEventRecord, error)
	ListProviderEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]ProviderEventRecord, error)
	ListProviderEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]ProviderEventRecord, error)
	ListAgentEvents(ctx context.Context, symbol string, limit int) ([]AgentEventRecord, error)
	ListAgentEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]AgentEventRecord, error)
	ListAgentEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]AgentEventRecord, error)
	ListGateEvents(ctx context.Context, symbol string, limit int) ([]GateEventRecord, error)
	ListGateEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]GateEventRecord, error)
	FindGateEventBySnapshot(ctx context.Context, symbol string, snapshotID uint) (GateEventRecord, bool, error)
	ListDistinctSnapshotIDs(ctx context.Context, symbol string, start, end int64) ([]uint, error)
}

type GateEventCursor struct {
	CreatedAt time.Time
	ID        uint64
}

type GateEventPage struct {
	Items []GateEventRecord
	Next  *GateEventCursor
}

type GateEventPageStore interface {
	ListGateEventsPage(ctx context.Context, symbol string, cursor *GateEventCursor, limit int) (GateEventPage, error)
}

type SymbolCatalogQueryStore interface {
	ListSymbols(ctx context.Context) ([]string, error)
}

type EpisodicMemoryStore interface {
	SaveEpisodicMemory(ctx context.Context, rec *EpisodicMemoryRecord) error
	ListEpisodicMemories(ctx context.Context, symbol string, limit int) ([]EpisodicMemoryRecord, error)
	FindEpisodicMemoryByPosition(ctx context.Context, positionID string) (EpisodicMemoryRecord, bool, error)
	DeleteEpisodicMemoriesOlderThan(ctx context.Context, symbol string, before time.Time) (int64, error)
}

type SemanticMemoryStore interface {
	SaveSemanticMemory(ctx context.Context, rec *SemanticMemoryRecord) error
	UpdateSemanticMemory(ctx context.Context, id uint, updates map[string]any) error
	DeleteSemanticMemory(ctx context.Context, id uint) error
	ListSemanticMemories(ctx context.Context, symbol string, activeOnly bool, limit int) ([]SemanticMemoryRecord, error)
	FindSemanticMemory(ctx context.Context, id uint) (SemanticMemoryRecord, bool, error)
}

type LLMRoundStore interface {
	SaveLLMRound(ctx context.Context, rec *LLMRoundRecord) error
	FindLLMRound(ctx context.Context, id string) (LLMRoundRecord, bool, error)
	ListLLMRounds(ctx context.Context, symbol string, limit int) ([]LLMRoundRecord, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(context.Context) error) error
}

type PromptRegistryStore interface {
	SavePromptEntry(ctx context.Context, rec *PromptRegistryEntry) error
	FindActivePrompt(ctx context.Context, role, stage string) (PromptRegistryEntry, bool, error)
	ListPromptEntries(ctx context.Context, role string, activeOnly bool) ([]PromptRegistryEntry, error)
}

type Store interface {
	EventCommandStore
	PositionCommandStore
	PositionQueryStore
	RiskPlanQueryStore
	TimelineQueryStore
	SymbolCatalogQueryStore
	EpisodicMemoryStore
	SemanticMemoryStore
	LLMRoundStore
	PromptRegistryStore
}
