package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/pgstore/queries"
	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ─── TimelineQueryStore ──────────────────────────────────────────

func (s *PGStore) ListAgentEvents(ctx context.Context, symbol string, limit int) ([]store.AgentEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp, stage,
		        input_json, system_prompt, user_prompt, output_json, raw_output,
		        fingerprint, system_config_hash, strategy_config_hash, source_version,
		        model, prompt_version, latency_ms, token_in, token_out, error, created_at
		 FROM agent_events WHERE symbol = $1 ORDER BY timestamp DESC LIMIT $2`, symbol, limit)
	if err != nil {
		return nil, err
	}
	return collectAgentRows(rows)
}

func (s *PGStore) ListAgentEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]store.AgentEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("snapshot_id is required")
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp, stage,
		        input_json, system_prompt, user_prompt, output_json, raw_output,
		        fingerprint, system_config_hash, strategy_config_hash, source_version,
		        model, prompt_version, latency_ms, token_in, token_out, error, created_at
		 FROM agent_events WHERE symbol = $1 AND snapshot_id = $2 ORDER BY timestamp ASC`, symbol, snapshotID)
	if err != nil {
		return nil, err
	}
	return collectAgentRows(rows)
}

func (s *PGStore) ListAgentEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]store.AgentEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp, stage,
		        input_json, system_prompt, user_prompt, output_json, raw_output,
		        fingerprint, system_config_hash, strategy_config_hash, source_version,
		        model, prompt_version, latency_ms, token_in, token_out, error, created_at
		 FROM agent_events WHERE symbol = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY timestamp ASC, stage ASC`, symbol, start, end)
	if err != nil {
		return nil, err
	}
	return collectAgentRows(rows)
}

func (s *PGStore) ListProviderEvents(ctx context.Context, symbol string, limit int) ([]store.ProviderEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp, provider_id, role,
		        data_context_json, system_prompt, user_prompt, output_json, raw_output,
		        tradeable, fingerprint, system_config_hash, strategy_config_hash, source_version,
		        model, prompt_version, latency_ms, token_in, token_out, error, created_at
		 FROM provider_events WHERE symbol = $1 ORDER BY timestamp DESC LIMIT $2`, symbol, limit)
	if err != nil {
		return nil, err
	}
	return collectProviderRows(rows)
}

func (s *PGStore) ListProviderEventsBySnapshot(ctx context.Context, symbol string, snapshotID uint) ([]store.ProviderEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	if snapshotID == 0 {
		return nil, fmt.Errorf("snapshot_id is required")
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp, provider_id, role,
		        data_context_json, system_prompt, user_prompt, output_json, raw_output,
		        tradeable, fingerprint, system_config_hash, strategy_config_hash, source_version,
		        model, prompt_version, latency_ms, token_in, token_out, error, created_at
		 FROM provider_events WHERE symbol = $1 AND snapshot_id = $2 ORDER BY timestamp ASC`, symbol, snapshotID)
	if err != nil {
		return nil, err
	}
	return collectProviderRows(rows)
}

func (s *PGStore) ListProviderEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]store.ProviderEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp, provider_id, role,
		        data_context_json, system_prompt, user_prompt, output_json, raw_output,
		        tradeable, fingerprint, system_config_hash, strategy_config_hash, source_version,
		        model, prompt_version, latency_ms, token_in, token_out, error, created_at
		 FROM provider_events WHERE symbol = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY timestamp ASC, role ASC`, symbol, start, end)
	if err != nil {
		return nil, err
	}
	return collectProviderRows(rows)
}

func (s *PGStore) ListGateEvents(ctx context.Context, symbol string, limit int) ([]store.GateEventRecord, error) {
	page, err := s.ListGateEventsPage(ctx, symbol, nil, limit)
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *PGStore) ListGateEventsPage(ctx context.Context, symbol string, cursor *store.GateEventCursor, limit int) (store.GateEventPage, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return store.GateEventPage{}, fmt.Errorf("symbol is required")
	}
	if limit <= 0 {
		limit = 200
	}
	var (
		rows []queries.GateEvent
		err  error
	)
	if cursor != nil && !cursor.CreatedAt.IsZero() && cursor.ID > 0 {
		ts := pgtype.Timestamptz{Time: cursor.CreatedAt.UTC(), Valid: true}
		rows, err = s.sqlc(ctx).ListGateEventsBefore(ctx, queries.ListGateEventsBeforeParams{
			Symbol:    symbol,
			CreatedAt: ts,
			ID:        int64(cursor.ID),
			Limit:     int32(limit + 1),
		})
	} else {
		rows, err = s.sqlc(ctx).ListGateEventsLatest(ctx, queries.ListGateEventsLatestParams{
			Symbol: symbol,
			Limit:  int32(limit + 1),
		})
	}
	if err != nil {
		return store.GateEventPage{}, err
	}
	items := make([]store.GateEventRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapGateEvent(row))
	}
	page := store.GateEventPage{Items: items}
	if len(items) <= limit {
		return page, nil
	}
	last := items[limit]
	page.Next = &store.GateEventCursor{CreatedAt: last.CreatedAt.UTC(), ID: uint64(last.ID)}
	page.Items = items[:limit]
	return page, nil
}

func (s *PGStore) ListGateEventsByTimeRange(ctx context.Context, symbol string, start, end int64) ([]store.GateEventRecord, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	rows, err := s.query(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp,
		        global_tradeable, decision_action, grade, gate_reason, direction,
		        provider_refs_json, rule_hit_json, derived_json,
		        fingerprint, system_config_hash, strategy_config_hash, source_version, created_at
		 FROM gate_events WHERE symbol = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY timestamp ASC`, symbol, start, end)
	if err != nil {
		return nil, err
	}
	return collectGateRows(rows)
}

func (s *PGStore) FindGateEventBySnapshot(ctx context.Context, symbol string, snapshotID uint) (store.GateEventRecord, bool, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return store.GateEventRecord{}, false, fmt.Errorf("symbol is required")
	}
	if snapshotID == 0 {
		return store.GateEventRecord{}, false, fmt.Errorf("snapshot_id is required")
	}
	row := s.queryRow(ctx,
		`SELECT id, snapshot_id, round_id, symbol, timestamp,
		        global_tradeable, decision_action, grade, gate_reason, direction,
		        provider_refs_json, rule_hit_json, derived_json,
		        fingerprint, system_config_hash, strategy_config_hash, source_version, created_at
		 FROM gate_events WHERE symbol = $1 AND snapshot_id = $2
		 ORDER BY timestamp DESC LIMIT 1`, symbol, snapshotID)
	var rec store.GateEventRecord
	if err := scanGateRow(row, &rec); err != nil {
		if err == pgx.ErrNoRows {
			return store.GateEventRecord{}, false, nil
		}
		return store.GateEventRecord{}, false, err
	}
	return rec, true, nil
}

func (s *PGStore) ListDistinctSnapshotIDs(ctx context.Context, symbol string, start, end int64) ([]uint, error) {
	symbol = decisionutil.NormalizeSymbol(symbol)
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	rows, err := s.query(ctx,
		`SELECT DISTINCT snapshot_id FROM gate_events
		 WHERE symbol = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY snapshot_id ASC`, symbol, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint
	for rows.Next() {
		var id uint
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ─── row scanners ────────────────────────────────────────────────

func collectAgentRows(rows pgx.Rows) ([]store.AgentEventRecord, error) {
	defer rows.Close()
	var out []store.AgentEventRecord
	for rows.Next() {
		var r store.AgentEventRecord
		var roundID, model, pv, rawOut, errStr *string
		var latMS, tokIn, tokOut *int
		var inputRaw, outputRaw []byte
		if err := rows.Scan(
			&r.ID, &r.SnapshotID, &roundID, &r.Symbol, &r.Timestamp, &r.Stage,
			&inputRaw, &r.SystemPrompt, &r.UserPrompt, &outputRaw, &rawOut,
			&r.Fingerprint, &r.SystemConfigHash, &r.StrategyConfigHash, &r.SourceVersion,
			&model, &pv, &latMS, &tokIn, &tokOut, &errStr, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.InputJSON = json.RawMessage(inputRaw)
		r.OutputJSON = json.RawMessage(outputRaw)
		r.RoundID = deref(roundID)
		r.Model = deref(model)
		r.PromptVersion = deref(pv)
		r.RawOutput = deref(rawOut)
		r.Error = deref(errStr)
		r.LatencyMS = derefInt(latMS)
		r.TokenIn = derefInt(tokIn)
		r.TokenOut = derefInt(tokOut)
		out = append(out, r)
	}
	return out, rows.Err()
}

func collectProviderRows(rows pgx.Rows) ([]store.ProviderEventRecord, error) {
	defer rows.Close()
	var out []store.ProviderEventRecord
	for rows.Next() {
		var r store.ProviderEventRecord
		var roundID, model, pv, rawOut, errStr *string
		var latMS, tokIn, tokOut *int
		var dcRaw, outputRaw []byte
		if err := rows.Scan(
			&r.ID, &r.SnapshotID, &roundID, &r.Symbol, &r.Timestamp, &r.ProviderID, &r.Role,
			&dcRaw, &r.SystemPrompt, &r.UserPrompt, &outputRaw, &rawOut,
			&r.Tradeable, &r.Fingerprint, &r.SystemConfigHash, &r.StrategyConfigHash, &r.SourceVersion,
			&model, &pv, &latMS, &tokIn, &tokOut, &errStr, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.DataContextJSON = json.RawMessage(dcRaw)
		r.OutputJSON = json.RawMessage(outputRaw)
		r.RoundID = deref(roundID)
		r.Model = deref(model)
		r.PromptVersion = deref(pv)
		r.RawOutput = deref(rawOut)
		r.Error = deref(errStr)
		r.LatencyMS = derefInt(latMS)
		r.TokenIn = derefInt(tokIn)
		r.TokenOut = derefInt(tokOut)
		out = append(out, r)
	}
	return out, rows.Err()
}

func collectGateRows(rows pgx.Rows) ([]store.GateEventRecord, error) {
	defer rows.Close()
	var out []store.GateEventRecord
	for rows.Next() {
		var r store.GateEventRecord
		if err := scanGateRow(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanGateRow(row scannable, r *store.GateEventRecord) error {
	var roundID *string
	var refsRaw, ruleRaw, derivedRaw []byte
	if err := row.Scan(
		&r.ID, &r.SnapshotID, &roundID, &r.Symbol, &r.Timestamp,
		&r.GlobalTradeable, &r.DecisionAction, &r.Grade, &r.GateReason, &r.Direction,
		&refsRaw, &ruleRaw, &derivedRaw,
		&r.Fingerprint, &r.SystemConfigHash, &r.StrategyConfigHash, &r.SourceVersion, &r.CreatedAt,
	); err != nil {
		return err
	}
	r.RoundID = deref(roundID)
	r.ProviderRefsJSON = json.RawMessage(refsRaw)
	r.RuleHitJSON = json.RawMessage(ruleRaw)
	r.DerivedJSON = json.RawMessage(derivedRaw)
	return nil
}

// scanGateRow for collectGateRows uses rows which also implements scannable.
// rows.Next() is called in the parent loop.
func scanGateFromRow(rows pgx.Rows, r *store.GateEventRecord) error {
	return scanGateRow(rows, r)
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefTime(p *time.Time) time.Time {
	if p == nil {
		return time.Time{}
	}
	return *p
}
