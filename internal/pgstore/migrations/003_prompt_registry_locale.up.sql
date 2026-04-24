ALTER TABLE prompt_registry
ADD COLUMN IF NOT EXISTS locale TEXT NOT NULL DEFAULT 'zh';

DROP INDEX IF EXISTS idx_prompt_registry_active;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'prompt_registry_role_stage_version_key'
    ) THEN
        ALTER TABLE prompt_registry DROP CONSTRAINT prompt_registry_role_stage_version_key;
    END IF;
END $$;

ALTER TABLE prompt_registry
ADD CONSTRAINT prompt_registry_role_stage_locale_version_key UNIQUE (role, stage, locale, version);

CREATE INDEX IF NOT EXISTS idx_prompt_registry_active
ON prompt_registry (role, stage, locale)
WHERE active = true;
