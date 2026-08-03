CREATE TABLE task_logs (
    id              BIGSERIAL PRIMARY KEY,
    invocation_id   TEXT NOT NULL DEFAULT '',
    correlation_id  TEXT NOT NULL DEFAULT '',
    agent_name      TEXT NOT NULL,
    level           TEXT NOT NULL DEFAULT 'info',
    message         TEXT NOT NULL,
    fields          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_task_logs_agent ON task_logs(agent_name, created_at DESC);
CREATE INDEX idx_task_logs_invocation ON task_logs(invocation_id) WHERE invocation_id != '';
CREATE INDEX idx_task_logs_level ON task_logs(level, created_at DESC) WHERE level = 'error';
