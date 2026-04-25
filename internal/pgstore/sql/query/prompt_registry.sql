-- name: FindActivePrompt :one
SELECT id, role, stage, locale, version, system_prompt, description, active, created_at, updated_at
FROM prompt_registry
WHERE role = sqlc.arg(role) AND stage = sqlc.arg(stage) AND locale = sqlc.arg(locale) AND active = true
ORDER BY created_at DESC
LIMIT 1;

-- name: ListPromptEntries :many
SELECT id, role, stage, locale, version, system_prompt, description, active, created_at, updated_at
FROM prompt_registry
WHERE CASE WHEN sqlc.arg(role)::text = '' THEN true ELSE role = sqlc.arg(role) END
  AND CASE WHEN sqlc.arg(active_only)::boolean THEN active = true ELSE true END
ORDER BY role, stage, locale, created_at DESC;
