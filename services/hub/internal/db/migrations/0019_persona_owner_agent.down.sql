-- NOTE: restoring the table-level UNIQUE constraint fails if owned
-- personas introduced duplicate names — delete or rename them first.

DROP INDEX IF EXISTS personas_name_templates_unique;

DROP INDEX IF EXISTS idx_personas_owner_agent;

ALTER TABLE personas DROP COLUMN IF EXISTS owner_agent;

ALTER TABLE personas ADD CONSTRAINT personas_name_key UNIQUE (name);
