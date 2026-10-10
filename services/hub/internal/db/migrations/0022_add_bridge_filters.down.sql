-- Undo bridge filters: drop the column. Bridges created with filters revert to
-- unconditional transfer, which is how they behaved before this migration.

ALTER TABLE channel_bridges DROP COLUMN IF EXISTS filters;