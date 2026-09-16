-- Invocations record one dispatch of an event to an agent. Previously kept
-- only in an in-memory ring buffer (lost on hub restart); persisted here so
-- conversation transcripts remain reachable via their invocation records
-- across restarts. Retention is time-based (see invocations.Retention),
-- aligned with task_conversations pruning.
CREATE TABLE invocations (
    id           TEXT PRIMARY KEY,
    agent_name   TEXT NOT NULL,
    trigger_name TEXT NOT NULL DEFAULT '',
    event_id     TEXT NOT NULL DEFAULT '',
    connector    TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'running',
    error        TEXT NOT NULL DEFAULT '',
    start_time   TIMESTAMPTZ NOT NULL,
    end_time     TIMESTAMPTZ,
    duration_ms  BIGINT
);

CREATE INDEX idx_invocations_agent ON invocations(agent_name, start_time DESC);
CREATE INDEX idx_invocations_event ON invocations(event_id) WHERE event_id != '';
CREATE INDEX idx_invocations_status ON invocations(status) WHERE status = 'running';
