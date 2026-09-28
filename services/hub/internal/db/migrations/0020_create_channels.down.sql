-- Drops channels. Bridges go first (they reference channels), then the
-- `events.channel_id` column, then the channels table.
--
-- Event history survives the rollback: `connector` and `agent_tasks` still
-- describe every stream as they did before channels existed.

DROP INDEX IF EXISTS idx_events_channel;

ALTER TABLE events DROP COLUMN IF EXISTS channel_id;

DROP TABLE IF EXISTS channel_bridges;
DROP TABLE IF EXISTS channels;
