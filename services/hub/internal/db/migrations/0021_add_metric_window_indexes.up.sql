-- The observability metric panels aggregate the hub's own records by creation
-- time (see internal/telemetry), which is the backend they read by default. Both
-- tables carried only partial indexes, so every dashboard poll scanned the whole
-- table. These make a time window a range scan. task_logs needs no new index: the
-- partial index on (level, created_at) already covers the error series.
--
-- Plain CREATE INDEX, not CONCURRENTLY: the hub applies migrations with
-- golang-migrate, which submits each file as one implicit transaction, and
-- PostgreSQL refuses a concurrent index build inside one. The price is a SHARE
-- lock on both tables for the duration of the build, so an upgrade briefly
-- pauses writes on an install that already has a large queue.
CREATE INDEX IF NOT EXISTS idx_events_received_at ON events (received_at);
CREATE INDEX IF NOT EXISTS idx_agent_tasks_created_at ON agent_tasks (created_at);
