-- backfill existing rows before enforcing NOT NULL
UPDATE users SET username = 'user_' || id WHERE username IS NULL;

ALTER TABLE users ALTER COLUMN username SET NOT NULL;

-- case-insensitive uniqueness; name must match the one your repo
-- code checks for (pgErr.ConstraintName == "users_username_lower_idx")
CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));