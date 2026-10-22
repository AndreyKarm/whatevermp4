-- Custom SQL migration file, put your code below! --
-- Announce table changes on the "events" channel (payload = table name).
-- The API LISTENs and pushes them to browsers over /api/events.
CREATE OR REPLACE FUNCTION notify_change() RETURNS trigger AS $$
BEGIN
	PERFORM pg_notify('events', TG_TABLE_NAME);
	RETURN NULL;
END
$$ LANGUAGE plpgsql;
--> statement-breakpoint
CREATE OR REPLACE TRIGGER media_files_notify AFTER INSERT OR UPDATE OR DELETE ON media_files
	FOR EACH STATEMENT EXECUTE FUNCTION notify_change();
--> statement-breakpoint
CREATE OR REPLACE TRIGGER torrents_notify AFTER INSERT OR UPDATE OR DELETE ON torrents
	FOR EACH STATEMENT EXECUTE FUNCTION notify_change();
--> statement-breakpoint
CREATE OR REPLACE TRIGGER rooms_notify AFTER INSERT OR UPDATE OR DELETE ON rooms
	FOR EACH STATEMENT EXECUTE FUNCTION notify_change();
