package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
)

// ─── RiskPlanQueryStore ──────────────────────────────────────────

func (s *PGStore) FindLatestRiskPlanHistory(ctx context.Context, positionID string) (store.RiskPlanHistoryRecord, bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return store.RiskPlanHistoryRecord{}, false, fmt.Errorf("position_id is required")
	}
	row := s.queryRow(ctx,
		`SELECT id, position_id, version, source, payload_json, created_at
		 FROM risk_plan_history WHERE position_id = $1 ORDER BY version DESC LIMIT 1`, positionID)
	var r store.RiskPlanHistoryRecord
	var payRaw []byte
	err := row.Scan(&r.ID, &r.PositionID, &r.Version, &r.Source, &payRaw, &r.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.RiskPlanHistoryRecord{}, false, nil
		}
		return store.RiskPlanHistoryRecord{}, false, err
	}
	r.PayloadJSON = json.RawMessage(payRaw)
	return r, true, nil
}

func (s *PGStore) ListRiskPlanHistory(ctx context.Context, positionID string, limit int) ([]store.RiskPlanHistoryRecord, error) {
	if strings.TrimSpace(positionID) == "" {
		return nil, fmt.Errorf("position_id is required")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.query(ctx,
		`SELECT id, position_id, version, source, payload_json, created_at
		 FROM risk_plan_history WHERE position_id = $1 ORDER BY version DESC LIMIT $2`, positionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.RiskPlanHistoryRecord
	for rows.Next() {
		var r store.RiskPlanHistoryRecord
		var payRaw []byte
		if err := rows.Scan(&r.ID, &r.PositionID, &r.Version, &r.Source, &payRaw, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.PayloadJSON = json.RawMessage(payRaw)
		out = append(out, r)
	}
	return out, rows.Err()
}
