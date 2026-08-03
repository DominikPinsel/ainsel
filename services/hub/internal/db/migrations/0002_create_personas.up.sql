CREATE TABLE personas (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    description         TEXT NOT NULL DEFAULT '',
    current_version_id  BIGINT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE persona_versions (
    id              BIGSERIAL PRIMARY KEY,
    persona_id      TEXT NOT NULL REFERENCES personas(id) ON DELETE CASCADE,
    version_number  INTEGER NOT NULL,
    text            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (persona_id, version_number)
);

CREATE INDEX idx_persona_versions_persona_id ON persona_versions (persona_id);

ALTER TABLE personas
    ADD CONSTRAINT fk_personas_current_version
    FOREIGN KEY (current_version_id) REFERENCES persona_versions(id) DEFERRABLE INITIALLY DEFERRED;
