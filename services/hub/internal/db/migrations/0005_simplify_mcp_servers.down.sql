ALTER TABLE mcp_servers
  DROP COLUMN IF EXISTS url,
  DROP COLUMN IF EXISTS bearer_token;

ALTER TABLE mcp_servers
  ADD COLUMN image_repo  TEXT NOT NULL DEFAULT '',
  ADD COLUMN image_tag   TEXT NOT NULL DEFAULT '',
  ADD COLUMN transport   TEXT NOT NULL DEFAULT 'streamable-http',
  ADD COLUMN port        INTEGER NOT NULL DEFAULT 8080,
  ADD COLUMN path        TEXT NOT NULL DEFAULT '/mcp',
  ADD COLUMN env         JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN env_from    JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN resources   JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN managed_by  TEXT NOT NULL DEFAULT 'user';

CREATE INDEX idx_mcp_servers_managed_by ON mcp_servers(managed_by);
