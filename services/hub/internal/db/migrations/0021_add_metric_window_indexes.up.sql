-- The observability metric panels aggregate the hub's own records by creation
-- time when there is no Prometheus to ask (see internal/telemetry). Both tables
-- carried only partial indexes, so every dashboard poll scanned the whole table.
-- These make a time window a range scan. task_logs needs no new index: the
-- partial index on (level, created_at) already covers the error series.
CREATE INDEX IF NOT EXISTS idx_events_received_at ON events (received_at);
CREATE INDEX IF NOT EXISTS idx_agent_tasks_created_at ON agent_tasks (created_at);
