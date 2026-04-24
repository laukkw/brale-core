DROP INDEX IF EXISTS idx_prompt_registry_active;

ALTER TABLE prompt_registry
DROP CONSTRAINT IF EXISTS prompt_registry_role_stage_locale_version_key;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'prompt_registry_role_stage_version_key'
    ) THEN
        ALTER TABLE prompt_registry
        ADD CONSTRAINT prompt_registry_role_stage_version_key UNIQUE (role, stage, version);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_prompt_registry_active
ON prompt_registry (role, stage)
WHERE active = true;

ALTER TABLE prompt_registry
DROP COLUMN IF EXISTS locale;
