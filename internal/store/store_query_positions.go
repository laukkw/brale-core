package store

import (
	"context"
	"fmt"
	"strings"
)

func (s *GormStore) FindPositionByID(ctx context.Context, positionID string) (PositionRecord, bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return PositionRecord{}, false, fmt.Errorf("position_id is required")
	}
	var rec PositionRecord
	err := s.db.WithContext(ctx).Where("position_id = ?", positionID).Limit(1).Take(&rec).Error
	if err != nil {
		if isRecordNotFound(err) {
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
		if isRecordNotFound(err) {
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
