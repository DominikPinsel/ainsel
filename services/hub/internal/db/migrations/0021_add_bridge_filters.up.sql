-- Bridge filters: a per-bridge transfer gate.
--
-- Until now a bridge transferred everything it reached: the channel graph had
-- no place to say "this edge only carries pull_request events" or "only a
-- comment that mentions this agent", so a grouping channel feeding three
-- inboxes gave each agent every event and the personas had to self-select.
-- Platform routing (forgejo → the developer/reviewer/refiner inboxes) needs
-- the gate on the edge: each agent's channel receives exactly the events
-- meant for it, and a filtered-out delivery never becomes a run.
--
-- Filters are stored as JSON — a disjunction of groups. The bridge delivers
-- when ANY group matches; a group matches when ALL of its filters match.
-- NULL keeps the pre-filter semantics: unconditional transfer.
--
-- The evaluation itself lives in the transfer path; this column only stores
-- what the walk already carries per edge.

ALTER TABLE channel_bridges ADD COLUMN IF NOT EXISTS filters jsonb NULL;