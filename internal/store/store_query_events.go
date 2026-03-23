package store

import (
	"context"
	"fmt"
	"strings"
)

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

func (s *GormStore) FindGateEventBySnapshot(ctx context.Context, symbol string, snapshotID uint) (GateEventRecord, bool, error) {
	if strings.TrimSpace(symbol) == "" {
		return GateEventRecord{}, false, fmt.Errorf("symbol is required")
	}
	if snapshotID == 0 {
		return GateEventRecord{}, false, fmt.Errorf("snapshot_id is required")
	}
	var rec GateEventRecord
	err := s.db.WithContext(ctx).
		Where("symbol = ? AND snapshot_id = ?", symbol, snapshotID).
		Order("timestamp desc").
		Limit(1).
		Take(&rec).Error
	if err != nil {
		if isRecordNotFound(err) {
			return GateEventRecord{}, false, nil
		}
		return GateEventRecord{}, false, err
	}
	return rec, true, nil
}
