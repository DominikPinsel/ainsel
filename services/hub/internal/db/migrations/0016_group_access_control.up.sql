-- Add role column to group_members
ALTER TABLE group_members ADD COLUMN role TEXT NOT NULL DEFAULT 'reader'
  CHECK (role IN ('reader', 'writer', 'owner'));

-- Remove parent_id from groups (flat groups)
ALTER TABLE groups DROP COLUMN IF EXISTS parent_id;
DROP INDEX IF EXISTS idx_groups_parent_id;

-- Resource-to-group mapping (replaces resource_ownership + resource_access)
CREATE TABLE resource_groups (
    resource_type TEXT NOT NULL,
    resource_name TEXT NOT NULL,
    group_id      TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    public        BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_type, resource_name)
);
CREATE INDEX idx_resource_groups_group ON resource_groups(group_id);

-- Migrate existing ownership data into resource_groups.
-- Resources without a group get assigned to a 'legacy' group.
-- The first admin becomes the legacy group owner.
DO $$
DECLARE
    legacy_gid TEXT;
    admin_id   TEXT;
BEGIN
    -- Create legacy group
    legacy_gid := 'legacy';
    INSERT INTO groups (id, name, description, created_at, updated_at)
    VALUES (legacy_gid, 'legacy', 'Auto-created during access control migration', now(), now())
    ON CONFLICT (id) DO NOTHING;

    -- Find first admin
    SELECT id INTO admin_id FROM users WHERE is_admin = true ORDER BY created_at LIMIT 1;
    IF admin_id IS NOT NULL THEN
        INSERT INTO group_members (group_id, user_id, role)
        VALUES (legacy_gid, admin_id, 'owner')
        ON CONFLICT DO NOTHING;
    END IF;

    -- Migrate ownership records
    INSERT INTO resource_groups (resource_type, resource_name, group_id, created_at)
    SELECT ro.resource_type, ro.resource_name, legacy_gid, ro.created_at
    FROM resource_ownership ro
    ON CONFLICT DO NOTHING;
END $$;

-- Drop old tables
DROP TABLE IF EXISTS resource_access;
DROP TABLE IF EXISTS resource_ownership;
