CREATE TABLE "folders" (
	"id" text PRIMARY KEY NOT NULL,
	"name" text NOT NULL,
	"parent_id" text,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
--> statement-breakpoint
ALTER TABLE "media_files" ADD COLUMN "folder_id" text;--> statement-breakpoint
ALTER TABLE "torrents" ADD COLUMN "folder_id" text;--> statement-breakpoint
ALTER TABLE "folders" ADD CONSTRAINT "folders_parent_id_folders_id_fk" FOREIGN KEY ("parent_id") REFERENCES "public"."folders"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "media_files" ADD CONSTRAINT "media_files_folder_id_folders_id_fk" FOREIGN KEY ("folder_id") REFERENCES "public"."folders"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "torrents" ADD CONSTRAINT "torrents_folder_id_folders_id_fk" FOREIGN KEY ("folder_id") REFERENCES "public"."folders"("id") ON DELETE set null ON UPDATE no action;--> statement-breakpoint
-- hand-added: live updates for folder changes (notify_change() is from 0001)
CREATE OR REPLACE TRIGGER folders_notify AFTER INSERT OR UPDATE OR DELETE ON folders
	FOR EACH STATEMENT EXECUTE FUNCTION notify_change();