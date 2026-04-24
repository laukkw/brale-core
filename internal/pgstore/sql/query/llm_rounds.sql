-- name: FindLLMRound :one
SELECT id, snapshot_id, symbol, round_type, started_at, finished_at,
       total_latency_ms, total_token_in, total_token_out, call_count,
       outcome, prompt_version, error, agent_count, provider_count, gate_action, request_id, created_at
FROM llm_rounds
WHERE id = $1;

-- name: ListLLMRoundsLatest :many
SELECT id, snapshot_id, symbol, round_type, started_at, finished_at,
       total_latency_ms, total_token_in, total_token_out, call_count,
       outcome, prompt_version, error, agent_count, provider_count, gate_action, request_id, created_at
FROM llm_rounds
WHERE CASE WHEN $1::text = '' THEN true ELSE symbol = $1 END
ORDER BY started_at DESC, id DESC
LIMIT $2;

-- name: ListLLMRoundsBefore :many
SELECT id, snapshot_id, symbol, round_type, started_at, finished_at,
       total_latency_ms, total_token_in, total_token_out, call_count,
       outcome, prompt_version, error, agent_count, provider_count, gate_action, request_id, created_at
FROM llm_rounds
WHERE CASE WHEN $1::text = '' THEN true ELSE symbol = $1 END
  AND (started_at < $2 OR (started_at = $2 AND id < $3))
ORDER BY started_at DESC, id DESC
LIMIT $4;
