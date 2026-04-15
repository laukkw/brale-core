-- name: FindActivePrompt :one
SELECT id, role, stage, version, system_prompt, description, active, created_at, updated_at
FROM prompt_registry
WHERE role = $1 AND stage = $2 AND active = true
ORDER BY created_at DESC
LIMIT 1;

-- name: ListPromptEntries :many
SELECT id, role, stage, version, system_prompt, description, active, created_at, updated_at
FROM prompt_registry
WHERE CASE WHEN $1::text = '' THEN true ELSE role = $1 END
  AND CASE WHEN $2::boolean THEN active = true ELSE true END
ORDER BY role, stage, created_at DESC;
