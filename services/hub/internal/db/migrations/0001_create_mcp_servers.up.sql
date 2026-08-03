CREATE TABLE mcp_servers (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    image_repo    TEXT NOT NULL,
    image_tag     TEXT NOT NULL,
    transport     TEXT NOT NULL DEFAULT 'streamable-http',
    port          INTEGER NOT NULL DEFAULT 8080,
    path          TEXT NOT NULL DEFAULT '/mcp',
    env           JSONB NOT NULL DEFAULT '[]'::jsonb,
    env_from      JSONB NOT NULL DEFAULT '[]'::jsonb,
    resources     JSONB NOT NULL DEFAULT '{}'::jsonb,
    managed_by    TEXT NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mcp_servers_managed_by ON mcp_servers(managed_by);
