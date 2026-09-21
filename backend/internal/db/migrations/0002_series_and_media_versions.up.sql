-- 0002_series_and_media_versions.up.sql
CREATE TYPE title_type AS ENUM ('movie', 'series');

CREATE TABLE titles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type title_type NOT NULL,
  name TEXT NOT NULL,
  year INT,
  description TEXT,
  poster_key TEXT,
  created_by UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_titles_updated_at BEFORE UPDATE ON titles
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- migrate existing movies into titles
INSERT INTO titles (id, type, name, year, description, poster_key, created_by, created_at, updated_at)
SELECT id, 'movie', title, year, description, poster_key, created_by, created_at, updated_at
FROM movies;

CREATE TABLE seasons (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  series_id UUID NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
  season_number INT NOT NULL,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (series_id, season_number)
);

CREATE TABLE episodes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  season_id UUID NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
  episode_number INT NOT NULL,
  name TEXT,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (season_id, episode_number)
);

-- rename movie_versions -> media_versions, allow it to point at a title (movie)
-- OR an episode, never both, never neither
ALTER TABLE movie_versions RENAME TO media_versions;
ALTER TABLE media_versions RENAME COLUMN movie_id TO title_id;
ALTER TABLE media_versions DROP CONSTRAINT IF EXISTS movie_versions_movie_id_fkey;
ALTER TABLE media_versions ADD COLUMN episode_id UUID REFERENCES episodes(id) ON DELETE CASCADE;
ALTER TABLE media_versions ALTER COLUMN title_id DROP NOT NULL;
ALTER TABLE media_versions ADD CONSTRAINT media_versions_exactly_one_parent
  CHECK ((title_id IS NOT NULL)::int + (episode_id IS NOT NULL)::int = 1);

CREATE INDEX idx_media_versions_episode_id ON media_versions (episode_id);

ALTER TABLE rooms RENAME COLUMN movie_version_id TO media_version_id;
ALTER TABLE watch_progress RENAME COLUMN version_id TO media_version_id;

DROP TABLE movies;