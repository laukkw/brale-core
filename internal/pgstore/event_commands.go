package pgstore

import (
	"context"
	"encoding/json"
	"fmt"

	"brale-core/internal/store"
)

// ─── EventCommandStore ───────────────────────────────────────────

func (s *PGStore) SaveAgentEvent(ctx context.Context, rec *store.AgentEventRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO agent_events (
		snapshot_id, round_id, symbol, timestamp, stage,
		input_json, system_prompt, user_prompt, output_json, raw_output,
		fingerprint, system_config_hash, strategy_config_hash, source_version,
		model, prompt_version, latency_ms, token_in, token_out, error
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
	RETURNING id, created_at`
	return s.queryRow(ctx, q,
		rec.SnapshotID, nilIfEmpty(rec.RoundID), rec.Symbol, rec.Timestamp, rec.Stage,
		jsonbOrNull(rec.InputJSON), rec.SystemPrompt, rec.UserPrompt, jsonbOrNull(rec.OutputJSON), rec.RawOutput,
		rec.Fingerprint, rec.SystemConfigHash, rec.StrategyConfigHash, rec.SourceVersion,
		nilIfEmpty(rec.Model), nilIfEmpty(rec.PromptVersion), nilIfZero(rec.LatencyMS), nilIfZero(rec.TokenIn), nilIfZero(rec.TokenOut), nilIfEmpty(rec.Error),
	).Scan(&rec.ID, &rec.CreatedAt)
}

func (s *PGStore) SaveProviderEvent(ctx context.Context, rec *store.ProviderEventRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO provider_events (
		snapshot_id, round_id, symbol, timestamp, provider_id, role,
		data_context_json, system_prompt, user_prompt, output_json, raw_output,
		tradeable, fingerprint, system_config_hash, strategy_config_hash, source_version,
		model, prompt_version, latency_ms, token_in, token_out, error
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
	RETURNING id, created_at`
	return s.queryRow(ctx, q,
		rec.SnapshotID, nilIfEmpty(rec.RoundID), rec.Symbol, rec.Timestamp, rec.ProviderID, rec.Role,
		jsonbOrNull(rec.DataContextJSON), rec.SystemPrompt, rec.UserPrompt, jsonbOrNull(rec.OutputJSON), rec.RawOutput,
		rec.Tradeable, rec.Fingerprint, rec.SystemConfigHash, rec.StrategyConfigHash, rec.SourceVersion,
		nilIfEmpty(rec.Model), nilIfEmpty(rec.PromptVersion), nilIfZero(rec.LatencyMS), nilIfZero(rec.TokenIn), nilIfZero(rec.TokenOut), nilIfEmpty(rec.Error),
	).Scan(&rec.ID, &rec.CreatedAt)
}

func (s *PGStore) SaveGateEvent(ctx context.Context, rec *store.GateEventRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO gate_events (
		snapshot_id, round_id, symbol, timestamp,
		global_tradeable, decision_action, grade, gate_reason, direction,
		provider_refs_json, rule_hit_json, derived_json,
		fingerprint, system_config_hash, strategy_config_hash, source_version
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	RETURNING id, created_at`
	return s.queryRow(ctx, q,
		rec.SnapshotID, nilIfEmpty(rec.RoundID), rec.Symbol, rec.Timestamp,
		rec.GlobalTradeable, rec.DecisionAction, rec.Grade, rec.GateReason, rec.Direction,
		jsonbOrNull(rec.ProviderRefsJSON), jsonbOrNull(rec.RuleHitJSON), jsonbOrNull(rec.DerivedJSON),
		rec.Fingerprint, rec.SystemConfigHash, rec.StrategyConfigHash, rec.SourceVersion,
	).Scan(&rec.ID, &rec.CreatedAt)
}

func (s *PGStore) SaveRiskPlanHistory(ctx context.Context, rec *store.RiskPlanHistoryRecord) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO risk_plan_history (position_id, version, source, payload_json)
	VALUES ($1,$2,$3,$4)
	RETURNING id, created_at`
	return s.queryRow(ctx, q,
		rec.PositionID, rec.Version, rec.Source, jsonbOrNull(rec.PayloadJSON),
	).Scan(&rec.ID, &rec.CreatedAt)
}

// ─── helpers ─────────────────────────────────────────────────────

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfZero(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func jsonbOrNull(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
