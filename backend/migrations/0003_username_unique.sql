ALTER TABLE users ALTER COLUMN display_name text UNIQUE;
-- backfill existing rows before making it NOT NULL, e.g.:
-- UPDATE users SET username = 'user_' || id WHERE username IS NULL;
ALTER TABLE users ALTER COLUMN display_name SET NOT NULL;