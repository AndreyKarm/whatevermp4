-- 0002_series_and_media_versions.down.sql
CREATE TABLE movies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL,
  year INT,
  description TEXT,
  poster_key TEXT,
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO movies (id, title, year, description, poster_key, created_by, created_at, updated_at)
SELECT id, name, year, description, poster_key, created_by, created_at, updated_at
FROM titles WHERE type = 'movie';

ALTER TABLE watch_progress RENAME COLUMN media_version_id TO version_id;
ALTER TABLE rooms RENAME COLUMN media_version_id TO movie_version_id;

ALTER TABLE media_versions DROP CONSTRAINT media_versions_exactly_one_parent;
ALTER TABLE media_versions DROP COLUMN episode_id;
ALTER TABLE media_versions ALTER COLUMN title_id SET NOT NULL;
ALTER TABLE media_versions RENAME COLUMN title_id TO movie_id;
ALTER TABLE media_versions RENAME TO movie_versions;

DROP TABLE episodes;
DROP TABLE seasons;
DROP TABLE titles;
DROP TYPE title_type;