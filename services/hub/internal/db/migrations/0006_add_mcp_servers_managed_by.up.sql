ALTER TABLE mcp_servers ADD COLUMN managed_by TEXT NOT NULL DEFAULT 'user';
CREATE INDEX idx_mcp_servers_managed_by ON mcp_servers(managed_by);
