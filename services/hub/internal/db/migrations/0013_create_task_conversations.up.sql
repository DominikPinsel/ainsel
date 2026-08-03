CREATE TABLE task_conversations (
    id              BIGSERIAL PRIMARY KEY,
    invocation_id   TEXT NOT NULL DEFAULT '',
    correlation_id  TEXT NOT NULL DEFAULT '',
    agent_name      TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'assistant',
    content         TEXT NOT NULL,
    model           TEXT NOT NULL DEFAULT '',
    input_tokens    INTEGER NOT NULL DEFAULT 0,
    output_tokens   INTEGER NOT NULL DEFAULT 0,
    stop_reason     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_task_conversations_agent ON task_conversations(agent_name, created_at DESC);
CREATE INDEX idx_task_conversations_invocation ON task_conversations(invocation_id) WHERE invocation_id != '';
CREATE INDEX idx_task_conversations_correlation ON task_conversations(correlation_id) WHERE correlation_id != '';
