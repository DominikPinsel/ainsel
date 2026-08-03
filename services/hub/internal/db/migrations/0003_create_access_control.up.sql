CREATE TABLE users (
    id         TEXT PRIMARY KEY,
    email      TEXT NOT NULL,
    username   TEXT NOT NULL,
    is_admin   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE groups (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    parent_id   TEXT REFERENCES groups(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_groups_parent_id ON groups(parent_id);

CREATE TABLE group_members (
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX idx_group_members_user_id ON group_members(user_id);

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
