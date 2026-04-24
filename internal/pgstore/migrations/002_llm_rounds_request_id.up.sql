ALTER TABLE llm_rounds
    ADD COLUMN IF NOT EXISTS request_id TEXT;
