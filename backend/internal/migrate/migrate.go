// Package migrate applies the embedded drizzle-kit migrations at startup, so
// existing databases upgrade without a manual step.
package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"liotom/whateverserver/migrations"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockKey serializes migrations between the API and the worker.
const lockKey = 0x77686174 // "what"

type journal struct {
	Entries []struct {
		Tag string `json:"tag"`
	} `json:"entries"`
}

// Migration is one SQL file from the journal.
type Migration struct {
	Tag        string
	Statements []string
}

// Load reads the migrations in journal order.
func Load(fsys fs.FS) ([]Migration, error) {
	b, err := fs.ReadFile(fsys, "meta/_journal.json")
	if err != nil {
		return nil, err
	}
	var j journal
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	var out []Migration
	for _, e := range j.Entries {
		sql, err := fs.ReadFile(fsys, e.Tag+".sql")
		if err != nil {
			return nil, err
		}
		m := Migration{Tag: e.Tag}
		for _, s := range strings.Split(string(sql), "--> statement-breakpoint") {
			if s = strings.TrimSpace(s); s != "" {
				m.Statements = append(m.Statements, s)
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// Up applies every migration not yet recorded in schema_migrations.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	list, err := Load(migrations.FS)
	if err != nil {
		return err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			tag text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return err
	}
	applied, err := appliedTags(ctx, conn.Conn())
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		if applied, err = baseline(ctx, conn.Conn()); err != nil {
			return fmt.Errorf("baseline: %w", err)
		}
	}
	for _, m := range list {
		if applied[m.Tag] {
			continue
		}
		if err := apply(ctx, conn.Conn(), m); err != nil {
			return fmt.Errorf("migration %s: %w", m.Tag, err)
		}
		log.Printf("migrate: applied %s", m.Tag)
	}
	return nil
}

func appliedTags(ctx context.Context, c *pgx.Conn) (map[string]bool, error) {
	rows, err := c.Query(ctx, `SELECT tag FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	tags, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(tags))
	for _, t := range tags {
		out[t] = true
	}
	return out, nil
}

func apply(ctx context.Context, c *pgx.Conn, m Migration) error {
	return pgx.BeginFunc(ctx, c, func(tx pgx.Tx) error {
		for _, s := range m.Statements {
			if _, err := tx.Exec(ctx, s); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (tag) VALUES ($1)`, m.Tag)
		return err
	})
}

// baseline records the migrations an untracked database already has: ones
// created by Postgres initdb (which ran the files before this runner existed)
// or by `pnpm db:push`. 0001 only creates triggers with CREATE OR REPLACE, so
// it is left unrecorded and simply runs again.
func baseline(ctx context.Context, c *pgx.Conn) (map[string]bool, error) {
	exists := func(table string) (bool, error) {
		var ok bool
		err := c.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&ok)
		return ok, err
	}
	out := map[string]bool{}
	for tag, table := range map[string]string{"0000_init": "media_files", "0002_folders": "folders"} {
		ok, err := exists(table)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if _, err := c.Exec(ctx, `INSERT INTO schema_migrations (tag) VALUES ($1)`, tag); err != nil {
			return nil, err
		}
		log.Printf("migrate: existing database already has %s", tag)
		out[tag] = true
	}
	return out, nil
}
