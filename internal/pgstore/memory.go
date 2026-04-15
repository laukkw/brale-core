package pgstore

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
)

// ─── EpisodicMemoryStore ─────────────────────────────────────────

func (s *PGStore) SaveEpisodicMemory(ctx context.Context, rec *store.EpisodicMemoryRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO episodic_memories (
		symbol, position_id, direction, entry_price, exit_price,
		pnl_percent, duration, reflection, key_lessons, market_context
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	RETURNING id, created_at`
	return s.queryRow(ctx, q,
		rec.Symbol, rec.PositionID, rec.Direction, rec.EntryPrice, rec.ExitPrice,
		rec.PnLPercent, rec.Duration, rec.Reflection, rec.KeyLessons, rec.MarketContext,
	).Scan(&rec.ID, &rec.CreatedAt)
}

func (s *PGStore) ListEpisodicMemories(ctx context.Context, symbol string, limit int) ([]store.EpisodicMemoryRecord, error) {
	q := `SELECT id, symbol, position_id, direction, entry_price, exit_price,
	             pnl_percent, duration, reflection, key_lessons, market_context, created_at
	      FROM episodic_memories WHERE symbol = $1 ORDER BY created_at DESC`
	args := []any{symbol}
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.EpisodicMemoryRecord
	for rows.Next() {
		var r store.EpisodicMemoryRecord
		if err := rows.Scan(&r.ID, &r.Symbol, &r.PositionID, &r.Direction,
			&r.EntryPrice, &r.ExitPrice, &r.PnLPercent, &r.Duration,
			&r.Reflection, &r.KeyLessons, &r.MarketContext, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PGStore) FindEpisodicMemoryByPosition(ctx context.Context, positionID string) (store.EpisodicMemoryRecord, bool, error) {
	row := s.queryRow(ctx,
		`SELECT id, symbol, position_id, direction, entry_price, exit_price,
		        pnl_percent, duration, reflection, key_lessons, market_context, created_at
		 FROM episodic_memories WHERE position_id = $1`, positionID)
	var r store.EpisodicMemoryRecord
	err := row.Scan(&r.ID, &r.Symbol, &r.PositionID, &r.Direction,
		&r.EntryPrice, &r.ExitPrice, &r.PnLPercent, &r.Duration,
		&r.Reflection, &r.KeyLessons, &r.MarketContext, &r.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.EpisodicMemoryRecord{}, false, nil
		}
		return store.EpisodicMemoryRecord{}, false, err
	}
	return r, true, nil
}

func (s *PGStore) DeleteEpisodicMemoriesOlderThan(ctx context.Context, symbol string, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM episodic_memories WHERE symbol = $1 AND created_at < $2`, symbol, before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ─── SemanticMemoryStore ─────────────────────────────────────────

func (s *PGStore) SaveSemanticMemory(ctx context.Context, rec *store.SemanticMemoryRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO semantic_memories (symbol, rule_text, source, confidence, active)
	VALUES ($1,$2,$3,$4,$5)
	RETURNING id, created_at, updated_at`
	return s.queryRow(ctx, q,
		rec.Symbol, rec.RuleText, rec.Source, rec.Confidence, rec.Active,
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
}

func (s *PGStore) UpdateSemanticMemory(ctx context.Context, id uint, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(updates)+1)
	args := make([]any, 0, len(updates)+1)
	i := 1
	for k, v := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, i))
		args = append(args, v)
		i++
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = now()"))
	args = append(args, id)
	sql := fmt.Sprintf("UPDATE semantic_memories SET %s WHERE id = $%d", joinComma(setClauses), i)
	return s.exec(ctx, sql, args...)
}

func (s *PGStore) DeleteSemanticMemory(ctx context.Context, id uint) error {
	return s.exec(ctx, `DELETE FROM semantic_memories WHERE id = $1`, id)
}

func (s *PGStore) ListSemanticMemories(ctx context.Context, symbol string, activeOnly bool, limit int) ([]store.SemanticMemoryRecord, error) {
	q := `SELECT id, symbol, rule_text, source, confidence, active, created_at, updated_at
	      FROM semantic_memories WHERE 1=1`
	args := []any{}
	idx := 1
	if symbol != "" {
		q += fmt.Sprintf(" AND (symbol = $%d OR symbol = '')", idx)
		args = append(args, symbol)
		idx++
	}
	if activeOnly {
		q += fmt.Sprintf(" AND active = $%d", idx)
		args = append(args, true)
		idx++
	}
	q += " ORDER BY confidence DESC, created_at DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.SemanticMemoryRecord
	for rows.Next() {
		var r store.SemanticMemoryRecord
		if err := rows.Scan(&r.ID, &r.Symbol, &r.RuleText, &r.Source,
			&r.Confidence, &r.Active, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PGStore) FindSemanticMemory(ctx context.Context, id uint) (store.SemanticMemoryRecord, bool, error) {
	row := s.queryRow(ctx,
		`SELECT id, symbol, rule_text, source, confidence, active, created_at, updated_at
		 FROM semantic_memories WHERE id = $1`, id)
	var r store.SemanticMemoryRecord
	err := row.Scan(&r.ID, &r.Symbol, &r.RuleText, &r.Source,
		&r.Confidence, &r.Active, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.SemanticMemoryRecord{}, false, nil
		}
		return store.SemanticMemoryRecord{}, false, err
	}
	return r, true, nil
}

func joinComma(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
