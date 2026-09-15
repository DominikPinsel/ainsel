-- Personas split into two classes: shared templates (the library) and
-- agent-owned personas the hub creates on an agent's behalf when its
-- persona is edited inline (copy-on-write). owner_agent marks the latter
-- with the owning Agent CR's name; owned personas never appear in the
-- persona library.
--
-- Name uniqueness becomes a partial index so it applies to templates only:
-- an owned copy may keep the name of the template it was forked from, and
-- two agents with the same display name may each own a same-named persona.

ALTER TABLE personas ADD COLUMN owner_agent TEXT;

CREATE INDEX idx_personas_owner_agent ON personas (owner_agent)
    WHERE owner_agent IS NOT NULL;

ALTER TABLE personas DROP CONSTRAINT personas_name_key;

CREATE UNIQUE INDEX personas_name_templates_unique
    ON personas (name) WHERE owner_agent IS NULL;
