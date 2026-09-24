CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE title_type AS ENUM ('movie', 'series');
CREATE TYPE job_stage AS ENUM ('queued', 'downloading', 'transcoding', 'packaging', 'done', 'failed');
CREATE TYPE source_type AS ENUM ('upload', 'url', 'magnet');

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- created_by references a user in the Auth service's database; there is no
-- FK across services, so it's stored as a plain UUID.
CREATE TABLE titles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type title_type NOT NULL,
  name TEXT NOT NULL,
  year INT,
  description TEXT,
  poster_key TEXT,
  created_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_titles_updated_at BEFORE UPDATE ON titles
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

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

CREATE TABLE media_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title_id UUID REFERENCES titles(id) ON DELETE CASCADE,
  episode_id UUID REFERENCES episodes(id) ON DELETE CASCADE,
  label TEXT NOT NULL DEFAULT 'default',
  source_type source_type NOT NULL,
  original_key TEXT,
  master_manifest_key TEXT,
  status job_stage NOT NULL DEFAULT 'queued',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((title_id IS NOT NULL)::int + (episode_id IS NOT NULL)::int = 1)
);
CREATE INDEX idx_media_versions_title_id ON media_versions (title_id);
CREATE INDEX idx_media_versions_episode_id ON media_versions (episode_id);
CREATE TRIGGER trg_media_versions_updated_at BEFORE UPDATE ON media_versions
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE renditions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  version_id UUID NOT NULL REFERENCES media_versions(id) ON DELETE CASCADE,
  resolution TEXT NOT NULL,
  bitrate_kbps INT NOT NULL,
  manifest_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_renditions_version_id ON renditions (version_id);

CREATE TABLE subtitles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  version_id UUID NOT NULL REFERENCES media_versions(id) ON DELETE CASCADE,
  lang TEXT NOT NULL,
  label TEXT,
  format TEXT NOT NULL DEFAULT 'vtt',
  storage_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subtitles_version_id ON subtitles (version_id);