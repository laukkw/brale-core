package store

import (
	"context"
	"fmt"
	"strings"
)

func (s *GormStore) FindLatestRiskPlanHistory(ctx context.Context, positionID string) (RiskPlanHistoryRecord, bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return RiskPlanHistoryRecord{}, false, fmt.Errorf("position_id is required")
	}
	var rec RiskPlanHistoryRecord
	err := s.db.WithContext(ctx).Where("position_id = ?", positionID).Order("version desc").Limit(1).Take(&rec).Error
	if err != nil {
		if isRecordNotFound(err) {
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
