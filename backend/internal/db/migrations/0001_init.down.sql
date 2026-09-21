-- 0001_init.down.sql
DROP TABLE IF EXISTS watch_progress;
DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS rooms;
DROP TABLE IF EXISTS processing_jobs;
DROP TABLE IF EXISTS subtitles;
DROP TABLE IF EXISTS renditions;
DROP TABLE IF EXISTS movie_versions;
DROP TABLE IF EXISTS movies;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS set_updated_at;

DROP TYPE IF EXISTS source_type;
DROP TYPE IF EXISTS job_stage;
DROP TYPE IF EXISTS user_role;