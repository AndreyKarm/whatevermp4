ALTER TABLE "media_files" ADD COLUMN "thumbnails" jsonb;--> statement-breakpoint
ALTER TABLE "media_files" ADD COLUMN "progress" double precision;--> statement-breakpoint
ALTER TABLE "media_files" ADD COLUMN "raw_bytes" bigint;--> statement-breakpoint
ALTER TABLE "media_files" ADD COLUMN "hls_bytes" bigint;--> statement-breakpoint
ALTER TABLE "media_files" ADD COLUMN "raw_deleted" boolean DEFAULT false NOT NULL;--> statement-breakpoint
ALTER TABLE "rooms" ADD COLUMN "controllers" jsonb DEFAULT '[]'::jsonb NOT NULL;--> statement-breakpoint
ALTER TABLE "rooms" ADD COLUMN "private" boolean DEFAULT false NOT NULL;--> statement-breakpoint
ALTER TABLE "rooms" ADD COLUMN "password_hash" text;--> statement-breakpoint
ALTER TABLE "rooms" ADD COLUMN "invite_code" text;--> statement-breakpoint
-- hand-added: (re)create every NOTIFY trigger, so databases that were set up
-- with `pnpm db:push` (which skips 0001) get live updates too
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
--> statement-breakpoint
CREATE OR REPLACE TRIGGER folders_notify AFTER INSERT OR UPDATE OR DELETE ON folders
	FOR EACH STATEMENT EXECUTE FUNCTION notify_change();