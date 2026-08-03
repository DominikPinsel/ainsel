-- Restore old tables
CREATE TABLE resource_ownership (
    resource_type TEXT NOT NULL,
    resource_name TEXT NOT NULL,
    owner_id      TEXT NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_type, resource_name)
);
CREATE INDEX idx_resource_ownership_owner ON resource_ownership(owner_id);

CREATE TABLE resource_access (
    id             TEXT PRIMARY KEY,
    resource_type  TEXT NOT NULL,
    resource_name  TEXT NOT NULL,
    principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'group')),
    principal_id   TEXT NOT NULL,
    role           TEXT NOT NULL CHECK (role IN ('writer', 'reader')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (resource_type, resource_name)
        REFERENCES resource_ownership(resource_type, resource_name) ON DELETE CASCADE,
    UNIQUE (resource_type, resource_name, principal_type, principal_id, role)
);
CREATE INDEX idx_resource_access_principal ON resource_access(principal_type, principal_id);
CREATE INDEX idx_resource_access_resource ON resource_access(resource_type, resource_name);

-- Restore parent_id on groups
ALTER TABLE groups ADD COLUMN parent_id TEXT REFERENCES groups(id) ON DELETE CASCADE;
CREATE INDEX idx_groups_parent_id ON groups(parent_id);

-- Remove role from group_members
ALTER TABLE group_members DROP COLUMN IF EXISTS role;

-- Drop new table
DROP TABLE IF EXISTS resource_groups;
