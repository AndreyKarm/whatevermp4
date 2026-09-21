-- 0003_comments.up.sql
CREATE TABLE comments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title_id UUID REFERENCES titles(id) ON DELETE CASCADE,
  episode_id UUID REFERENCES episodes(id) ON DELETE CASCADE,
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