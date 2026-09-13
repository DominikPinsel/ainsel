-- Rolling back re-exposes agent-owned personas in the library and restores
-- global name uniqueness. If an owned persona duplicates a template name the
-- ADD CONSTRAINT below fails.
--
-- The column is dropped LAST on purpose: a failed rollback must leave
-- owner_agent in place so the offending rows can still be identified and
-- cleaned up, then this migration re-run:
--
--   DELETE FROM personas WHERE owner_agent IS NOT NULL;  -- or rename them

DROP INDEX IF EXISTS personas_name_templates_unique;

DROP INDEX IF EXISTS idx_personas_owner_agent;

ALTER TABLE personas ADD CONSTRAINT personas_name_key UNIQUE (name);

ALTER TABLE personas DROP COLUMN IF EXISTS owner_agent;
