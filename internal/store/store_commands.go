package store

import (
	"context"
	"fmt"
	"strings"
)

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

func (s *GormStore) create(ctx context.Context, rec any) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	return s.db.WithContext(ctx).Create(rec).Error
}
