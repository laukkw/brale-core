package store

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type Store interface {
	SaveAgentEvent(ctx context.Context, rec *AgentEventRecord) error
	SaveProviderEvent(ctx context.Context, rec *ProviderEventRecord) error
	SaveGateEvent(ctx context.Context, rec *GateEventRecord) error
	SaveRiskPlanHistory(ctx context.Context, rec *RiskPlanHistoryRecord) error
	FindLatestRiskPlanHistory(ctx context.Context, positionID string) (RiskPlanHistoryRecord, bool, error)
	ListRiskPlanHistory(ctx context.Context, positionID string, limit int) ([]RiskPlanHistoryRecord, error)
	SavePosition(ctx context.Context, rec *PositionRecord) error
	UpdatePosition(ctx context.Context, positionID string, expectedVersion int, updates map[string]any) (bool, error)
	FindPositionByID(ctx context.Context, positionID string) (PositionRecord, bool, error)
	FindPositionBySymbol(ctx context.Context, symbol string, statuses []string) (PositionRecord, bool, error)
	ListPositionsByStatus(ctx context.Context, statuses []string) ([]PositionRecord, error)
	ListSymbols(ctx context.Context) ([]string, error)
	ListProviderEvents(ctx context.Context, symbol string, limit int) ([]ProviderEventRecord, error)
	ListProviderEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]ProviderEventRecord, error)
	ListAgentEvents(ctx context.Context, symbol string, limit int) ([]AgentEventRecord, error)
	ListAgentEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]AgentEventRecord, error)
	ListGateEvents(ctx context.Context, symbol string, limit int) ([]GateEventRecord, error)
}

type GormStore struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) SaveAgentEvent(ctx context.Context, rec *AgentEventRecord) error {
	return s.create(ctx, rec)
}

func (s *GormStore) SaveProviderEvent(ctx context.Context, rec *ProviderEventRecord) error {
	return s.create(ctx, rec)
}

func (s *GormStore) SaveGateEvent(ctx context.Context, rec *GateEventRecord) error {
	return s.create(ctx, rec)
}

func (s *GormStore) SaveRiskPlanHistory(ctx context.Context, rec *RiskPlanHistoryRecord) error {
	return s.create(ctx, rec)
}

func (s *GormStore) SavePosition(ctx context.Context, rec *PositionRecord) error {
	return s.create(ctx, rec)
}

func (s *GormStore) UpdatePosition(ctx context.Context, positionID string, expectedVersion int, updates map[string]any) (bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return false, fmt.Errorf("position_id is required")
	}
	if updates == nil {
		return false, fmt.Errorf("updates is required")
	}
	result := s.db.WithContext(ctx).Model(&PositionRecord{}).
		Where("position_id = ? AND version = ?", positionID, expectedVersion).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (s *GormStore) FindPositionByID(ctx context.Context, positionID string) (PositionRecord, bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return PositionRecord{}, false, fmt.Errorf("position_id is required")
	}
	var rec PositionRecord
	err := s.db.WithContext(ctx).Where("position_id = ?", positionID).Limit(1).Take(&rec).Error
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return PositionRecord{}, false, nil
		}
		return PositionRecord{}, false, err
	}
	return rec, true, nil
}

func (s *GormStore) FindPositionBySymbol(ctx context.Context, symbol string, statuses []string) (PositionRecord, bool, error) {
	if strings.TrimSpace(symbol) == "" {
		return PositionRecord{}, false, fmt.Errorf("symbol is required")
	}
	query := s.db.WithContext(ctx).Where("symbol = ?", symbol)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	var rec PositionRecord
	err := query.Order("updated_at desc").Limit(1).Take(&rec).Error
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return PositionRecord{}, false, nil
		}
		return PositionRecord{}, false, err
	}
	return rec, true, nil
}

func (s *GormStore) ListPositionsByStatus(ctx context.Context, statuses []string) ([]PositionRecord, error) {
	if len(statuses) == 0 {
		return nil, fmt.Errorf("statuses is required")
	}
	var out []PositionRecord
	if err := s.db.WithContext(ctx).Where("status IN ?", statuses).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) FindLatestRiskPlanHistory(ctx context.Context, positionID string) (RiskPlanHistoryRecord, bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return RiskPlanHistoryRecord{}, false, fmt.Errorf("position_id is required")
	}
	var rec RiskPlanHistoryRecord
	err := s.db.WithContext(ctx).Where("position_id = ?", positionID).Order("version desc").Limit(1).Take(&rec).Error
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "record not found") {
			return RiskPlanHistoryRecord{}, false, nil
		}
		return RiskPlanHistoryRecord{}, false, err
	}
	return rec, true, nil
}

func (s *GormStore) ListRiskPlanHistory(ctx context.Context, positionID string, limit int) ([]RiskPlanHistoryRecord, error) {
	if strings.TrimSpace(positionID) == "" {
		return nil, fmt.Errorf("position_id is required")
	}
	if limit <= 0 {
		limit = 200
	}
	var out []RiskPlanHistoryRecord
	if err := s.db.WithContext(ctx).
		Where("position_id = ?", positionID).
		Order("version desc").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) ListSymbols(ctx context.Context) ([]string, error) {
	merge := func(dst map[string]struct{}, src []string) {
		for _, item := range src {
			trim := strings.TrimSpace(item)
			if trim == "" {
				continue
			}
			dst[trim] = struct{}{}
		}
	}
	set := make(map[string]struct{})
	var tmp []string
	if err := s.db.WithContext(ctx).Model(&PositionRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	tmp = tmp[:0]
	if err := s.db.WithContext(ctx).Model(&AgentEventRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	tmp = tmp[:0]
	if err := s.db.WithContext(ctx).Model(&ProviderEventRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	tmp = tmp[:0]
	if err := s.db.WithContext(ctx).Model(&GateEventRecord{}).Distinct().Pluck("symbol", &tmp).Error; err != nil {
		return nil, err
	}
	merge(set, tmp)
	out := make([]string, 0, len(set))
	for sym := range set {
		out = append(out, sym)
	}
	sort.Strings(out)
	return out, nil
}

func (s *GormStore) ListProviderEvents(ctx context.Context, symbol string, limit int) ([]ProviderEventRecord, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if limit <= 0 {
		limit = 200
	}
	var out []ProviderEventRecord
	if err := s.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		Order("timestamp desc").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) ListProviderEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]ProviderEventRecord, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("snapshot_id is required")
	}
	var out []ProviderEventRecord
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND snapshot_id = ?", symbol, snapshotID).
		Order("timestamp asc").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) ListAgentEvents(ctx context.Context, symbol string, limit int) ([]AgentEventRecord, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if limit <= 0 {
		limit = 200
	}
	var out []AgentEventRecord
	if err := s.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		Order("timestamp desc").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) ListAgentEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]AgentEventRecord, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("snapshot_id is required")
	}
	var out []AgentEventRecord
	if err := s.db.WithContext(ctx).
		Where("symbol = ? AND snapshot_id = ?", symbol, snapshotID).
		Order("timestamp asc").
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) ListGateEvents(ctx context.Context, symbol string, limit int) ([]GateEventRecord, error) {
	if strings.TrimSpace(symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if limit <= 0 {
		limit = 200
	}
	var out []GateEventRecord
	if err := s.db.WithContext(ctx).
		Where("symbol = ?", symbol).
		Order("timestamp desc").
		Limit(limit).
		Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *GormStore) create(ctx context.Context, rec any) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	return s.db.WithContext(ctx).Create(rec).Error
}
