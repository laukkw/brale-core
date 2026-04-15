-- 001_init.up.sql: PostgreSQL/TimescaleDB schema for brale-core.
-- Replaces the former SQLite/GORM auto-migration.

-- ─── agent_events ────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS agent_events (
    id              BIGSERIAL PRIMARY KEY,
    snapshot_id     BIGINT       NOT NULL,
    round_id        TEXT,
    symbol          TEXT         NOT NULL,
    timestamp       BIGINT       NOT NULL,
    stage           TEXT         NOT NULL,
    input_json      JSONB,
    system_prompt   TEXT,
    user_prompt     TEXT,
    output_json     JSONB,
    raw_output      TEXT,
    fingerprint     TEXT,
    system_config_hash   TEXT,
    strategy_config_hash TEXT,
    source_version  TEXT,
    model           TEXT,
    prompt_version  TEXT,
    latency_ms      INT,
    token_in        INT,
    token_out       INT,
    error           TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_events_symbol_ts   ON agent_events (symbol, timestamp DESC);
CREATE INDEX idx_agent_events_snapshot    ON agent_events (symbol, snapshot_id);
CREATE INDEX idx_agent_events_fingerprint ON agent_events (fingerprint);
CREATE INDEX idx_agent_events_round       ON agent_events (round_id) WHERE round_id IS NOT NULL;

-- ─── provider_events ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS provider_events (
    id              BIGSERIAL PRIMARY KEY,
    snapshot_id     BIGINT       NOT NULL,
    round_id        TEXT,
    symbol          TEXT         NOT NULL,
    timestamp       BIGINT       NOT NULL,
    provider_id     TEXT,
    role            TEXT         NOT NULL,
    data_context_json JSONB,
    system_prompt   TEXT,
    user_prompt     TEXT,
    output_json     JSONB,
    raw_output      TEXT,
    tradeable       BOOLEAN      NOT NULL DEFAULT false,
    fingerprint     TEXT,
    system_config_hash   TEXT,
    strategy_config_hash TEXT,
    source_version  TEXT,
    model           TEXT,
    prompt_version  TEXT,
    latency_ms      INT,
    token_in        INT,
    token_out       INT,
    error           TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_provider_events_symbol_ts   ON provider_events (symbol, timestamp DESC);
CREATE INDEX idx_provider_events_snapshot    ON provider_events (symbol, snapshot_id);
CREATE INDEX idx_provider_events_fingerprint ON provider_events (fingerprint);
CREATE INDEX idx_provider_events_round       ON provider_events (round_id) WHERE round_id IS NOT NULL;

-- ─── gate_events ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS gate_events (
    id              BIGSERIAL PRIMARY KEY,
    snapshot_id     BIGINT       NOT NULL,
    round_id        TEXT,
    symbol          TEXT         NOT NULL,
    timestamp       BIGINT       NOT NULL,
    global_tradeable BOOLEAN     NOT NULL DEFAULT false,
    decision_action TEXT,
    grade           INT          NOT NULL DEFAULT 0,
    gate_reason     TEXT,
    direction       TEXT,
    provider_refs_json JSONB,
    rule_hit_json   JSONB,
    derived_json    JSONB,
    fingerprint     TEXT,
    system_config_hash   TEXT,
    strategy_config_hash TEXT,
    source_version  TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_gate_events_symbol_ts   ON gate_events (symbol, timestamp DESC);
CREATE INDEX idx_gate_events_snapshot    ON gate_events (symbol, snapshot_id);
CREATE INDEX idx_gate_events_fingerprint ON gate_events (fingerprint);

-- ─── risk_plan_history ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS risk_plan_history (
    id           BIGSERIAL PRIMARY KEY,
    position_id  TEXT      NOT NULL,
    version      INT       NOT NULL,
    source       TEXT,
    payload_json JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_risk_plan_position ON risk_plan_history (position_id, version DESC);

-- ─── positions ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS positions (
    id                   BIGSERIAL PRIMARY KEY,
    position_id          TEXT      NOT NULL UNIQUE,
    symbol               TEXT      NOT NULL,
    side                 TEXT,
    initial_stake        DOUBLE PRECISION NOT NULL DEFAULT 0,
    qty                  DOUBLE PRECISION NOT NULL DEFAULT 0,
    avg_entry            DOUBLE PRECISION NOT NULL DEFAULT 0,
    risk_pct             DOUBLE PRECISION NOT NULL DEFAULT 0,
    leverage             DOUBLE PRECISION NOT NULL DEFAULT 0,
    status               TEXT      NOT NULL DEFAULT '',
    open_intent_id       TEXT,
    close_intent_id      TEXT,
    abort_reason         TEXT,
    source               TEXT,
    stop_reason          TEXT,
    abort_started_at     BIGINT    NOT NULL DEFAULT 0,
    abort_finalized_at   BIGINT    NOT NULL DEFAULT 0,
    close_submitted_at   BIGINT    NOT NULL DEFAULT 0,
    risk_json            JSONB,
    executor_name        TEXT,
    executor_position_id TEXT,
    version              INT       NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_positions_symbol_status ON positions (symbol, status);
CREATE INDEX idx_positions_open_intent   ON positions (open_intent_id);
CREATE INDEX idx_positions_close_intent  ON positions (close_intent_id);

CREATE UNIQUE INDEX idx_position_symbol_open ON positions (symbol)
WHERE status IN (
    'OPEN_SUBMITTING', 'OPEN_PENDING', 'OPEN_ABORTING', 'OPEN_ACTIVE',
    'CLOSE_ARMED', 'CLOSE_SUBMITTING', 'CLOSE_PENDING'
);

-- ─── episodic_memories ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS episodic_memories (
    id             BIGSERIAL PRIMARY KEY,
    symbol         TEXT      NOT NULL,
    position_id    TEXT      NOT NULL UNIQUE,
    direction      TEXT,
    entry_price    TEXT,
    exit_price     TEXT,
    pnl_percent    TEXT,
    duration       TEXT,
    reflection     TEXT,
    key_lessons    TEXT,
    market_context TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_episodic_symbol_time ON episodic_memories (symbol, created_at DESC);

-- ─── semantic_memories ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS semantic_memories (
    id         BIGSERIAL PRIMARY KEY,
    symbol     TEXT      NOT NULL,
    rule_text  TEXT,
    source     TEXT,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    active     BOOLEAN   NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_semantic_symbol ON semantic_memories (symbol);

-- ─── llm_rounds (task 66) ────────────────────────────────────────
CREATE TABLE IF NOT EXISTS llm_rounds (
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
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_llm_rounds_symbol   ON llm_rounds (symbol, started_at DESC);
CREATE INDEX idx_llm_rounds_snapshot ON llm_rounds (snapshot_id);

-- ─── prompt_registry (task 66) ───────────────────────────────────
CREATE TABLE IF NOT EXISTS prompt_registry (
    id            BIGSERIAL PRIMARY KEY,
    role          TEXT      NOT NULL,
    stage         TEXT      NOT NULL,
    version       TEXT      NOT NULL,
    system_prompt TEXT      NOT NULL,
    description   TEXT,
    active        BOOLEAN   NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(role, stage, version)
);

CREATE INDEX idx_prompt_registry_active ON prompt_registry (role, stage) WHERE active = true;
