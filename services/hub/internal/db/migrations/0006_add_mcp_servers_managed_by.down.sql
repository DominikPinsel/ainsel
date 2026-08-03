DROP INDEX IF EXISTS idx_mcp_servers_managed_by;
ALTER TABLE mcp_servers DROP COLUMN IF EXISTS managed_by;
