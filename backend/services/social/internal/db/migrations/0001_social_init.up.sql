CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE watchlist_status AS ENUM ('planned', 'watching', 'completed', 'dropped');

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- user_id, title_id, episode_id are plain UUIDs referencing rows owned by
-- the Auth and Catalog services respectively; no cross-database FKs.
CREATE TABLE comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL,
  title_id UUID,
  episode_id UUID,
  parent_comment_id UUID REFERENCES comments(id) ON DELETE CASCADE,
  body TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  CHECK ((title_id IS NOT NULL)::int + (episode_id IS NOT NULL)::int = 1)
);
CREATE INDEX idx_comments_title_id ON comments (title_id);
CREATE INDEX idx_comments_episode_id ON comments (episode_id);
CREATE INDEX idx_comments_parent_id ON comments (parent_comment_id);
CREATE TRIGGER trg_comments_updated_at BEFORE UPDATE ON comments
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE ratings (
  user_id UUID NOT NULL,
  title_id UUID NOT NULL,
  score SMALLINT NOT NULL CHECK (score BETWEEN 1 AND 10),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, title_id)
);
CREATE TRIGGER trg_ratings_updated_at BEFORE UPDATE ON ratings
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE watchlist_items (
  user_id UUID NOT NULL,
  title_id UUID NOT NULL,
  status watchlist_status NOT NULL DEFAULT 'planned',
  added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, title_id)
);