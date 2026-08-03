-- The schema has a circular foreign key:
--   personas.current_version_id -> persona_versions.id
--   persona_versions.persona_id -> personas.id
-- A plain DROP TABLE refuses because each table is the target of an FK on the
-- other. CASCADE drops the dependent constraints as part of the same step, so
-- both tables can be removed in either order.
DROP TABLE IF EXISTS persona_versions CASCADE;
DROP TABLE IF EXISTS personas CASCADE;
