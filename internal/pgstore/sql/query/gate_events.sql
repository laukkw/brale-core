-- name: ListGateEventsLatest :many
SELECT id, snapshot_id, round_id, symbol, timestamp,
       global_tradeable, decision_action, grade, gate_reason, direction,
       provider_refs_json, rule_hit_json, derived_json,
       fingerprint, system_config_hash, strategy_config_hash, source_version, created_at
FROM gate_events
WHERE symbol = $1
ORDER BY created_at DESC, id DESC
LIMIT $2;

-- name: ListGateEventsBefore :many
SELECT id, snapshot_id, round_id, symbol, timestamp,
       global_tradeable, decision_action, grade, gate_reason, direction,
       provider_refs_json, rule_hit_json, derived_json,
       fingerprint, system_config_hash, strategy_config_hash, source_version, created_at
FROM gate_events
WHERE symbol = $1
  AND (created_at < $2 OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;
