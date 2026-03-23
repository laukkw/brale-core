package store

import (
	"context"

	"gorm.io/gorm"
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
	ListAgentEvents(ctx context.Context, symbol string, limit int) ([]AgentEventRecord, error)
	ListAgentEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]AgentEventRecord, error)
	ListGateEvents(ctx context.Context, symbol string, limit int) ([]GateEventRecord, error)
	FindGateEventBySnapshot(ctx context.Context, symbol string, snapshotID uint) (GateEventRecord, bool, error)
}

type SymbolCatalogQueryStore interface {
	ListSymbols(ctx context.Context) ([]string, error)
}

type Store interface {
	EventCommandStore
	PositionCommandStore
	PositionQueryStore
	RiskPlanQueryStore
	TimelineQueryStore
	SymbolCatalogQueryStore
}

type GormStore struct {
	db *gorm.DB
}

var _ EventCommandStore = (*GormStore)(nil)
var _ PositionCommandStore = (*GormStore)(nil)
var _ PositionQueryStore = (*GormStore)(nil)
var _ RiskPlanQueryStore = (*GormStore)(nil)
var _ TimelineQueryStore = (*GormStore)(nil)
var _ SymbolCatalogQueryStore = (*GormStore)(nil)
var _ Store = (*GormStore)(nil)

func NewStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}
