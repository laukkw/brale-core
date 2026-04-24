CREATE TABLE gate_events (
    id                   BIGSERIAL    NOT NULL,
    snapshot_id          BIGINT       NOT NULL,
    round_id             TEXT,
    symbol               TEXT         NOT NULL,
    timestamp            BIGINT       NOT NULL,
    global_tradeable     BOOLEAN      NOT NULL DEFAULT false,
    decision_action      TEXT,
    grade                INT          NOT NULL DEFAULT 0,
    gate_reason          TEXT,
    direction            TEXT,
    provider_refs_json   JSONB,
    rule_hit_json        JSONB,
    derived_json         JSONB,
    fingerprint          TEXT,
    system_config_hash   TEXT,
    strategy_config_hash TEXT,
    source_version       TEXT,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE llm_rounds (
    id               TEXT PRIMARY KEY,
    snapshot_id      BIGINT       NOT NULL,
    symbol           TEXT         NOT NULL,
    round_type       TEXT         NOT NULL DEFAULT '',
    started_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    finished_at      TIMESTAMPTZ,
    total_latency_ms INT,
    total_token_in   INT,
    total_token_out  INT,
    call_count       INT          NOT NULL DEFAULT 0,
    outcome          TEXT,
    prompt_version   TEXT         NOT NULL DEFAULT '',
    error            TEXT,
    agent_count      INT          NOT NULL DEFAULT 0,
    provider_count   INT          NOT NULL DEFAULT 0,
    gate_action      TEXT,
    request_id       TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE prompt_registry (
    id            BIGSERIAL PRIMARY KEY,
    role          TEXT         NOT NULL,
    stage         TEXT         NOT NULL,
    version       TEXT         NOT NULL,
    system_prompt TEXT         NOT NULL,
    description   TEXT,
    active        BOOLEAN      NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);
