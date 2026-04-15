package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
)

// ─── PositionCommandStore ────────────────────────────────────────

func (s *PGStore) SavePosition(ctx context.Context, rec *store.PositionRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO positions (
		position_id, symbol, side, initial_stake, qty, avg_entry,
		risk_pct, leverage, status, open_intent_id, close_intent_id,
		abort_reason, source, stop_reason, abort_started_at, abort_finalized_at,
		close_submitted_at, risk_json, executor_name, executor_position_id, version
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	RETURNING id, created_at, updated_at`
	return s.queryRow(ctx, q,
		rec.PositionID, rec.Symbol, rec.Side, rec.InitialStake, rec.Qty, rec.AvgEntry,
		rec.RiskPct, rec.Leverage, rec.Status, rec.OpenIntentID, rec.CloseIntentID,
		rec.AbortReason, rec.Source, rec.StopReason, rec.AbortStartedAt, rec.AbortFinalizedAt,
		rec.CloseSubmittedAt, jsonbOrNull(rec.RiskJSON), rec.ExecutorName, rec.ExecutorPositionID, rec.Version,
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
}

func (s *PGStore) UpdatePositionPatch(ctx context.Context, patch store.PositionPatch) (bool, error) {
	if err := patch.Validate(); err != nil {
		return false, err
	}
	updates := patch.Updates()
	updates["version"] = patch.NextVersion
	return s.updatePosition(ctx, patch.PositionID, patch.ExpectedVersion, updates)
}

func (s *PGStore) UpdatePosition(ctx context.Context, positionID string, expectedVersion int, updates map[string]any) (bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return false, fmt.Errorf("position_id is required")
	}
	if updates == nil {
		return false, fmt.Errorf("updates is required")
	}
	return s.updatePosition(ctx, positionID, expectedVersion, updates)
}

func (s *PGStore) updatePosition(ctx context.Context, positionID string, expectedVersion int, updates map[string]any) (bool, error) {
	if len(updates) == 0 {
		return false, nil
	}
	// Build dynamic SET clause.
	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+3)
	argIdx := 1

	for col, val := range updates {
		pgCol := goFieldToPGColumn(col)
		if pgCol == "" {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", pgCol, argIdx))
		args = append(args, val)
		argIdx++
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = now()"))

	sql := fmt.Sprintf(`UPDATE positions SET %s WHERE position_id = $%d AND version = $%d`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, positionID, expectedVersion)

	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// goFieldToPGColumn maps PositionPatch field names (and map keys from Updates())
// to the corresponding PostgreSQL column name.
func goFieldToPGColumn(field string) string {
	m := map[string]string{
		"version":              "version",
		"status":               "status",
		"close_intent_id":      "close_intent_id",
		"close_submitted_at":   "close_submitted_at",
		"executor_position_id": "executor_position_id",
		"qty":                  "qty",
		"avg_entry":            "avg_entry",
		"initial_stake":        "initial_stake",
		"source":               "source",
		"abort_reason":         "abort_reason",
		"abort_started_at":     "abort_started_at",
		"abort_finalized_at":   "abort_finalized_at",
		"risk_json":            "risk_json",
		"stop_reason":          "stop_reason",
		"open_intent_id":       "open_intent_id",
		"side":                 "side",
		"risk_pct":             "risk_pct",
		"leverage":             "leverage",
		"executor_name":        "executor_name",
	}
	if v, ok := m[field]; ok {
		return v
	}
	return ""
}

// ─── PositionQueryStore ──────────────────────────────────────────

func (s *PGStore) FindPositionByID(ctx context.Context, positionID string) (store.PositionRecord, bool, error) {
	if strings.TrimSpace(positionID) == "" {
		return store.PositionRecord{}, false, fmt.Errorf("position_id is required")
	}
	row := s.queryRow(ctx, positionSelectSQL+` WHERE position_id = $1 LIMIT 1`, positionID)
	rec, err := scanPositionRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.PositionRecord{}, false, nil
		}
		return store.PositionRecord{}, false, err
	}
	return rec, true, nil
}

func (s *PGStore) FindPositionBySymbol(ctx context.Context, symbol string, statuses []string) (store.PositionRecord, bool, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return store.PositionRecord{}, false, fmt.Errorf("symbol is required")
	}
	var row pgx.Row
	if len(statuses) > 0 {
		row = s.queryRow(ctx, positionSelectSQL+` WHERE symbol = $1 AND status = ANY($2) ORDER BY updated_at DESC LIMIT 1`, symbol, statuses)
	} else {
		row = s.queryRow(ctx, positionSelectSQL+` WHERE symbol = $1 ORDER BY updated_at DESC LIMIT 1`, symbol)
	}
	rec, err := scanPositionRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.PositionRecord{}, false, nil
		}
		return store.PositionRecord{}, false, err
	}
	return rec, true, nil
}

func (s *PGStore) ListPositionsByStatus(ctx context.Context, statuses []string) ([]store.PositionRecord, error) {
	if len(statuses) == 0 {
		return nil, fmt.Errorf("statuses is required")
	}
	rows, err := s.query(ctx, positionSelectSQL+` WHERE status = ANY($1)`, statuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.PositionRecord
	for rows.Next() {
		rec, err := scanPositionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

const positionSelectSQL = `SELECT id, position_id, symbol, side, initial_stake, qty, avg_entry,
	risk_pct, leverage, status, open_intent_id, close_intent_id,
	abort_reason, source, stop_reason, abort_started_at, abort_finalized_at,
	close_submitted_at, risk_json, executor_name, executor_position_id,
	version, created_at, updated_at
FROM positions`

func scanPositionRow(row scannable) (store.PositionRecord, error) {
	var r store.PositionRecord
	var riskRaw []byte
	if err := row.Scan(
		&r.ID, &r.PositionID, &r.Symbol, &r.Side, &r.InitialStake, &r.Qty, &r.AvgEntry,
		&r.RiskPct, &r.Leverage, &r.Status, &r.OpenIntentID, &r.CloseIntentID,
		&r.AbortReason, &r.Source, &r.StopReason, &r.AbortStartedAt, &r.AbortFinalizedAt,
		&r.CloseSubmittedAt, &riskRaw, &r.ExecutorName, &r.ExecutorPositionID,
		&r.Version, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return store.PositionRecord{}, err
	}
	r.RiskJSON = json.RawMessage(riskRaw)
	return r, nil
}
