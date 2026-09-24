CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE job_stage AS ENUM ('queued', 'downloading', 'transcoding', 'packaging', 'done', 'failed');

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- version_id refers to a media_versions row owned by the Catalog service.
-- No cross-database FK; the Catalog service is the source of truth for
-- whether that version still exists.
CREATE TABLE processing_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  version_id UUID NOT NULL,
  stage job_stage NOT NULL DEFAULT 'queued',
  progress SMALLINT NOT NULL DEFAULT 0,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_processing_jobs_version_id ON processing_jobs (version_id);
CREATE TRIGGER trg_processing_jobs_updated_at BEFORE UPDATE ON processing_jobs
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();