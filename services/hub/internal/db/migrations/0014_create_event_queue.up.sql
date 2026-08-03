-- Incoming events from connectors (replaces NATS EVENTS stream).
CREATE TABLE events (
    id          TEXT PRIMARY KEY,
    connector   TEXT NOT NULL,
    headers     JSONB NOT NULL DEFAULT '{}',
    data        JSONB NOT NULL,
    raw         TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    routed_at   TIMESTAMPTZ  -- set when the router processes this event
);
CREATE INDEX idx_events_unrouted ON events (received_at) WHERE routed_at IS NULL;

-- Tasks dispatched to agents (replaces NATS AGENTS stream).
CREATE TABLE agent_tasks (
    id            BIGSERIAL PRIMARY KEY,
    event_id      TEXT NOT NULL REFERENCES events(id),
    agent_name    TEXT NOT NULL,
    trigger_name  TEXT NOT NULL DEFAULT '',
    invocation_id TEXT NOT NULL DEFAULT '',
    headers       JSONB NOT NULL DEFAULT '{}',
    payload       JSONB NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',  -- pending | claimed | completed | failed
    attempts      INT NOT NULL DEFAULT 0,
    max_attempts  INT NOT NULL DEFAULT 10,
    retry_after   TIMESTAMPTZ,
    claimed_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    error         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Deduplication: same event routed to same agent only once.
    UNIQUE (event_id, agent_name)
);
-- Partial index over pending tasks. The retry_after readiness check
-- (retry_after IS NULL OR retry_after <= now()) is applied at query time;
-- now() is not IMMUTABLE so it cannot appear in an index predicate.
CREATE INDEX idx_agent_tasks_pending ON agent_tasks (agent_name, created_at)
    WHERE status = 'pending';
CREATE INDEX idx_agent_tasks_status ON agent_tasks (status);
