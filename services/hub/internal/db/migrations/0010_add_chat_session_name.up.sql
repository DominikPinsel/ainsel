-- Add a display name column to chat_sessions.
-- Existing rows get name = id so the column can be NOT NULL.
ALTER TABLE chat_sessions ADD COLUMN name TEXT NOT NULL DEFAULT '';
UPDATE chat_sessions SET name = id WHERE name = '';
ALTER TABLE chat_sessions ALTER COLUMN name DROP DEFAULT;