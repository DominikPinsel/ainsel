-- Channels: the named streams events live in.
--
-- Until now a channel existed only implicitly: `events.connector` said where
-- an event was born, `agent_tasks.agent_name` was the inbox it was delivered
-- to, and a trigger's (connector_ref, agent_ref) pair was the transfer between
-- them. Nobody could address a stream by identity, so every consumer — the
-- console's Channels pages, the MCP tools, an agent asking "what is in my
-- inbox" — re-derived the picture from those three tables and hoped the labels
-- lined up. This table makes the stream a resource with an id.
--
-- A channel has no role: it neither produces nor consumes. Events are born in
-- one channel and move to another when a subscription on the destination
-- transfers them.
--
--   kind        lifecycle
--   ----------  -----------------------------------------------------------
--   connector   provisioned per WebhookConnector CR, permanent
--   agent       provisioned per Agent CR — it IS that agent's inbox
--   custom      created by users to group subscriptions; deletable
--
-- `entity_ref` is the registry id a channel was provisioned for: the
-- WebhookConnector CR name (also the value `events.connector` carries) or the
-- Agent CR name. It is nullable only for custom channels, and the partial
-- unique index below is what makes provisioning idempotent.
--
-- `name` is a rename-able display label and deliberately NOT unique: a
-- connector and an agent may both be called "forgejo" and are then two
-- distinct channels. Identity is always `id`.
--
-- `orphaned` marks a channel whose registry entity is gone. Rows are never
-- deleted when an agent or connector is — `events.channel_id` points here, and
-- the history of a removed stream is still worth reading.

CREATE TABLE channels (
    id          TEXT PRIMARY KEY,
    kind        TEXT NOT NULL CHECK (kind IN ('connector', 'agent', 'custom')),
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    entity_ref  TEXT,
    orphaned    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_channels_entity ON channels (kind, entity_ref)
    WHERE entity_ref IS NOT NULL;
CREATE INDEX idx_channels_kind ON channels (kind);

-- Bridges: subscriptions between channels that are not triggers. A trigger
-- transfers one connector channel into one agent inbox; a bridge attaches a
-- channel to a custom grouping channel (or a grouping channel to an inbox),
-- so N sources can be bundled once and consumed by M inboxes without N*M
-- triggers.
--
-- At least one endpoint must be custom — enforced in the store, not here: a
-- connector→agent edge is a trigger's job, and letting both exist for the same
-- pair would put event routing in two places. The graph must stay acyclic,
-- also enforced in the store, and self-edges are excluded below.

CREATE TABLE channel_bridges (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL DEFAULT '',
    from_channel TEXT NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    to_channel   TEXT NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_channel <> to_channel),
    UNIQUE (from_channel, to_channel)
);

CREATE INDEX idx_channel_bridges_from ON channel_bridges (from_channel);
CREATE INDEX idx_channel_bridges_to ON channel_bridges (to_channel);

-- The birth channel of an event. Nullable: rows written before channels
-- existed, and events whose stream was never provisioned, carry no id. The
-- hub stamps new events at ingest and backfills old ones once it knows the
-- registry, so the column fills in rather than staying empty.
--
-- Deliberately not merged with `connector`: cron ticks and chat messages are
-- born directly in an agent's inbox, so their `connector` is a source label
-- ("cron", "chat") while their channel is the inbox.

ALTER TABLE events ADD COLUMN channel_id TEXT REFERENCES channels (id) ON DELETE SET NULL;

CREATE INDEX idx_events_channel ON events (channel_id)
    WHERE channel_id IS NOT NULL;
