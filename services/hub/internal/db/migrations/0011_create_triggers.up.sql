CREATE TABLE triggers (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL DEFAULT '',
    agent_ref     TEXT NOT NULL,
    connector_ref TEXT NOT NULL,
    filters       JSONB NOT NULL DEFAULT '[]'::jsonb,
    agent_valid   BOOLEAN NOT NULL DEFAULT false,
    connector_valid BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_triggers_agent_ref ON triggers(agent_ref);
CREATE INDEX idx_triggers_connector_ref ON triggers(connector_ref);

CREATE TABLE cron_triggers (
    id            TEXT PRIMARY KEY,
    display_name  TEXT NOT NULL DEFAULT '',
    agent_ref     TEXT NOT NULL,
    schedule      TEXT NOT NULL,
    prompt        TEXT NOT NULL DEFAULT '',
    enabled       BOOLEAN NOT NULL DEFAULT true,
    agent_valid   BOOLEAN NOT NULL DEFAULT false,
    schedule_valid BOOLEAN NOT NULL DEFAULT false,
    last_run      TIMESTAMPTZ,
    next_run      TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_cron_triggers_agent_ref ON cron_triggers(agent_ref);
