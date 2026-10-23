-- Custom SQL migration file, put your code below! --
-- Change notifications say what changed, so pages can patch one item instead
-- of refetching whole lists (see internal/events and src/lib/events.ts).
-- Payloads are JSON:
--   {"type":"media_files","op":"INSERT|UPDATE|DELETE","id":…}
--   {"type":"media_progress","id":…,"progress":…}      only progress changed
--   {"type":"torrents","op":…,"id":…}
--   {"type":"torrent_progress","id":…,"progress":…,"bytesDone":…,"bytesTotal":…}
--   {"type":"rooms","op":…,"id":…}                      not for state flushes
--   {"type":"folders"}
CREATE OR REPLACE FUNCTION notify_change() RETURNS trigger AS $$
BEGIN
	PERFORM pg_notify('events', json_build_object('type', TG_TABLE_NAME)::text);
	RETURN NULL;
END
$$ LANGUAGE plpgsql;
--> statement-breakpoint
CREATE OR REPLACE FUNCTION notify_media() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM pg_notify('events', json_build_object('type', 'media_files', 'op', 'DELETE', 'id', OLD.id)::text);
	ELSIF TG_OP = 'UPDATE' AND (to_jsonb(NEW) - 'progress') = (to_jsonb(OLD) - 'progress') THEN
		IF NEW.progress IS DISTINCT FROM OLD.progress THEN
			PERFORM pg_notify('events', json_build_object(
				'type', 'media_progress', 'id', NEW.id, 'progress', NEW.progress)::text);
		END IF;
	ELSE
		PERFORM pg_notify('events', json_build_object('type', 'media_files', 'op', TG_OP, 'id', NEW.id)::text);
	END IF;
	RETURN NULL;
END
$$ LANGUAGE plpgsql;
--> statement-breakpoint
CREATE OR REPLACE FUNCTION notify_torrent() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM pg_notify('events', json_build_object('type', 'torrents', 'op', 'DELETE', 'id', OLD.id)::text);
	ELSIF TG_OP = 'UPDATE'
		AND (to_jsonb(NEW) - 'progress' - 'bytes_done' - 'bytes_total')
			= (to_jsonb(OLD) - 'progress' - 'bytes_done' - 'bytes_total') THEN
		IF (NEW.progress, NEW.bytes_done, NEW.bytes_total)
			IS DISTINCT FROM (OLD.progress, OLD.bytes_done, OLD.bytes_total) THEN
			PERFORM pg_notify('events', json_build_object(
				'type', 'torrent_progress', 'id', NEW.id, 'progress', NEW.progress,
				'bytesDone', NEW.bytes_done, 'bytesTotal', NEW.bytes_total)::text);
		END IF;
	ELSE
		PERFORM pg_notify('events', json_build_object('type', 'torrents', 'op', TG_OP, 'id', NEW.id)::text);
	END IF;
	RETURN NULL;
END
$$ LANGUAGE plpgsql;
--> statement-breakpoint
-- The hub announces live room changes itself; its periodic state flushes and
-- updated_at touches would only duplicate them.
CREATE OR REPLACE FUNCTION notify_room() RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'DELETE' THEN
		PERFORM pg_notify('events', json_build_object('type', 'rooms', 'op', 'DELETE', 'id', OLD.id)::text);
	ELSIF TG_OP = 'UPDATE'
		AND (to_jsonb(NEW) - 'state' - 'updated_at') = (to_jsonb(OLD) - 'state' - 'updated_at') THEN
		NULL;
	ELSE
		PERFORM pg_notify('events', json_build_object('type', 'rooms', 'op', TG_OP, 'id', NEW.id)::text);
	END IF;
	RETURN NULL;
END
$$ LANGUAGE plpgsql;
--> statement-breakpoint
DROP TRIGGER IF EXISTS media_files_notify ON media_files;
--> statement-breakpoint
CREATE TRIGGER media_files_notify AFTER INSERT OR UPDATE OR DELETE ON media_files
	FOR EACH ROW EXECUTE FUNCTION notify_media();
--> statement-breakpoint
DROP TRIGGER IF EXISTS torrents_notify ON torrents;
--> statement-breakpoint
CREATE TRIGGER torrents_notify AFTER INSERT OR UPDATE OR DELETE ON torrents
	FOR EACH ROW EXECUTE FUNCTION notify_torrent();
--> statement-breakpoint
DROP TRIGGER IF EXISTS rooms_notify ON rooms;
--> statement-breakpoint
CREATE TRIGGER rooms_notify AFTER INSERT OR UPDATE OR DELETE ON rooms
	FOR EACH ROW EXECUTE FUNCTION notify_room();
--> statement-breakpoint
CREATE OR REPLACE TRIGGER folders_notify AFTER INSERT OR UPDATE OR DELETE ON folders
	FOR EACH STATEMENT EXECUTE FUNCTION notify_change();
