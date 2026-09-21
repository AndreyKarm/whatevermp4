package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InitDB opens a connection pool and ensures the schema exists.
func InitDB(dbpath string) (*pgxpool.Pool, error) {
	ctx := context.Background()

	db, err := pgxpool.New(ctx, dbpath)
	if err != nil {
		return nil, err
	}

	if err := createSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func createSchema(ctx context.Context, db *pgxpool.Pool) error {
	statements := []string{
		`create extension if not exists pgcrypto;`,

		`do $$ begin
			create type user_role as enum ('admin', 'user');
		exception when duplicate_object then null;
		end $$;`,
		`do $$ begin
			create type job_stage as enum ('queued', 'downloading', 'transcoding', 'packaging', 'done', 'failed');
		exception when duplicate_object then null;
		end $$;`,
		`do $$ begin
			create type source_type as enum ('upload', 'url', 'magnet');
		exception when duplicate_object then null;
		end $$;`,

		`create table if not exists users (
			id uuid primary key default gen_random_uuid(),
			email text not null unique,
			password_hash text not null,
			role user_role not null default 'user',
			username text not null,
			avatar_key text,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);`,
		`create unique index if not exists users_username_lower_idx on users (lower(username));`,

		`create table if not exists sessions (
			id uuid primary key default gen_random_uuid(),
			user_id uuid not null references users (id) on delete cascade,
			refresh_token_hash text not null unique,
			user_agent text,
			ip_address text,
			expires_at timestamptz not null,
			revoked_at timestamptz,
			created_at timestamptz not null default now()
		);`,
		`create index if not exists idx_sessions_user_id on sessions (user_id);`,
		`create index if not exists idx_sessions_expires_at on sessions (expires_at);`,

		`create table if not exists movies (
			id uuid primary key default gen_random_uuid(),
			title text not null,
			year int,
			description text,
			poster_key text,
			created_by uuid references users (id),
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);`,

		`create table if not exists movie_versions (
			id uuid primary key default gen_random_uuid(),
			movie_id uuid not null references movies (id) on delete cascade,
			label text not null default 'default',
			source_type source_type not null,
			original_key text,
			master_manifest_key text,
			status job_stage not null default 'queued',
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);`,
		`create index if not exists idx_movie_versions_movie_id on movie_versions (movie_id);`,

		`create table if not exists renditions (
			id uuid primary key default gen_random_uuid(),
			version_id uuid not null references movie_versions (id) on delete cascade,
			resolution text not null,
			bitrate_kbps int not null,
			manifest_key text not null,
			created_at timestamptz not null default now()
		);`,
		`create index if not exists idx_renditions_version_id on renditions (version_id);`,

		`create table if not exists subtitles (
			id uuid primary key default gen_random_uuid(),
			version_id uuid not null references movie_versions (id) on delete cascade,
			lang text not null,
			label text,
			format text not null default 'vtt',
			storage_key text not null,
			created_at timestamptz not null default now()
		);`,
		`create index if not exists idx_subtitles_version_id on subtitles (version_id);`,

		`create table if not exists processing_jobs (
			id uuid primary key default gen_random_uuid(),
			version_id uuid not null references movie_versions (id) on delete cascade,
			stage job_stage not null default 'queued',
			progress smallint not null default 0,
			error text,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);`,
		`create index if not exists idx_processing_jobs_version_id on processing_jobs (version_id);`,

		`create table if not exists rooms (
			id uuid primary key default gen_random_uuid(),
			code text unique not null,
			host_user_id uuid not null references users (id),
			movie_version_id uuid not null references movie_versions (id),
			position_seconds numeric not null default 0,
			is_playing boolean not null default false,
			created_at timestamptz not null default now(),
			updated_at timestamptz not null default now()
		);`,

		`create table if not exists room_members (
			room_id uuid not null references rooms (id) on delete cascade,
			user_id uuid not null references users (id) on delete cascade,
			joined_at timestamptz not null default now(),
			primary key (room_id, user_id)
		);`,

		`create table if not exists watch_progress (
			user_id uuid not null references users (id) on delete cascade,
			version_id uuid not null references movie_versions (id) on delete cascade,
			position_seconds numeric not null default 0,
			updated_at timestamptz not null default now(),
			primary key (user_id, version_id)
		);`,

		`create or replace function set_updated_at()
		returns trigger as $$
		begin
			new.updated_at = now();
			return new;
		end;
		$$ language plpgsql;`,
	}

	triggerTables := []string{"users", "movies", "movie_versions", "processing_jobs", "rooms"}
	for _, table := range triggerTables {
		trigger := "trg_" + table + "_updated_at"
		statements = append(statements,
			`drop trigger if exists `+trigger+` on `+table+`;`,
			`create trigger `+trigger+` before update on `+table+`
			 for each row execute function set_updated_at();`,
		)
	}

	for _, stmt := range statements {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
