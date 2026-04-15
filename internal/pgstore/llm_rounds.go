package pgstore

import (
	"context"
	"fmt"
	"time"

	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
)

// ─── LLM Rounds ─────────────────────────────────────────────────

func (s *PGStore) SaveLLMRound(ctx context.Context, rec *store.LLMRoundRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO llm_rounds (
		id, snapshot_id, symbol, round_type, started_at, finished_at,
		total_latency_ms, total_token_in, total_token_out, call_count,
		outcome, prompt_version, error, agent_count, provider_count, gate_action
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	ON CONFLICT (id) DO UPDATE SET
		finished_at = EXCLUDED.finished_at,
		total_latency_ms = EXCLUDED.total_latency_ms,
		total_token_in = EXCLUDED.total_token_in,
		total_token_out = EXCLUDED.total_token_out,
		call_count = EXCLUDED.call_count,
		outcome = EXCLUDED.outcome,
		prompt_version = EXCLUDED.prompt_version,
		error = EXCLUDED.error,
		agent_count = EXCLUDED.agent_count,
		provider_count = EXCLUDED.provider_count,
		gate_action = EXCLUDED.gate_action
	RETURNING created_at`
	var finishedAt *time.Time
	if !rec.FinishedAt.IsZero() {
		finishedAt = &rec.FinishedAt
	}
	return s.queryRow(ctx, q,
		rec.ID, rec.SnapshotID, rec.Symbol, rec.RoundType, rec.StartedAt, finishedAt,
		nilIfZero(rec.TotalLatencyMS), nilIfZero(rec.TotalTokenIn), nilIfZero(rec.TotalTokenOut), rec.CallCount,
		nilIfEmpty(rec.Outcome), nilIfEmpty(rec.PromptVersion), nilIfEmpty(rec.Error),
		rec.AgentCount, rec.ProviderCount, nilIfEmpty(rec.GateAction),
	).Scan(&rec.CreatedAt)
}

func (s *PGStore) FindLLMRound(ctx context.Context, id string) (store.LLMRoundRecord, bool, error) {
	row := s.queryRow(ctx, llmRoundSelectSQL+` WHERE id = $1`, id)
	rec, err := scanLLMRoundRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.LLMRoundRecord{}, false, nil
		}
		return store.LLMRoundRecord{}, false, err
	}
	return rec, true, nil
}

func (s *PGStore) ListLLMRounds(ctx context.Context, symbol string, limit int) ([]store.LLMRoundRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows pgx.Rows
	var err error
	if symbol != "" {
		rows, err = s.query(ctx, llmRoundSelectSQL+` WHERE symbol = $1 ORDER BY started_at DESC LIMIT $2`, symbol, limit)
	} else {
		rows, err = s.query(ctx, llmRoundSelectSQL+` ORDER BY started_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.LLMRoundRecord
	for rows.Next() {
		rec, err := scanLLMRoundRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

const llmRoundSelectSQL = `SELECT id, snapshot_id, symbol, round_type, started_at, finished_at,
	total_latency_ms, total_token_in, total_token_out, call_count,
	outcome, prompt_version, error, agent_count, provider_count, gate_action, created_at
FROM llm_rounds`

func scanLLMRoundRow(row scannable) (store.LLMRoundRecord, error) {
	var r store.LLMRoundRecord
	var finishedAt *time.Time
	var latMS, tokIn, tokOut *int
	var outcome, promptVersion, errStr, gateAction *string
	if err := row.Scan(
		&r.ID, &r.SnapshotID, &r.Symbol, &r.RoundType, &r.StartedAt, &finishedAt,
		&latMS, &tokIn, &tokOut, &r.CallCount,
		&outcome, &promptVersion, &errStr, &r.AgentCount, &r.ProviderCount, &gateAction, &r.CreatedAt,
	); err != nil {
		return store.LLMRoundRecord{}, err
	}
	r.FinishedAt = derefTime(finishedAt)
	r.TotalLatencyMS = derefInt(latMS)
	r.TotalTokenIn = derefInt(tokIn)
	r.TotalTokenOut = derefInt(tokOut)
	r.Outcome = deref(outcome)
	r.PromptVersion = deref(promptVersion)
	r.Error = deref(errStr)
	r.GateAction = deref(gateAction)
	return r, nil
}
