package migrate_test

import (
	"context"
	"liotom/whateverserver/internal/migrate"
	"liotom/whateverserver/internal/testdb"
	"liotom/whateverserver/migrations"
	"testing"
)

func TestUpFreshAndIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Empty(t)
	for range 2 {
		if err := migrate.Up(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	list, err := migrate.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(list) {
		t.Fatalf("recorded %d migrations, want %d", n, len(list))
	}
	// a column from the latest migration exists
	if _, err := pool.Exec(ctx, `SELECT private, invite_code, controllers FROM rooms`); err != nil {
		t.Fatal(err)
	}
}

// A database Postgres initdb created from the SQL files (before the runner
// existed) has the schema but no schema_migrations: Up must adopt it.
func TestUpBaselinesLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Empty(t)
	list, err := migrate.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		if m.Tag == "0003_rooms_media_features" {
			break
		}
		for _, s := range m.Statements {
			if _, err := pool.Exec(ctx, s); err != nil {
				t.Fatalf("%s: %v", m.Tag, err)
			}
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO rooms (id, owner_name) VALUES ('r1', 'ann')`); err != nil {
		t.Fatal(err)
	}

	if err := migrate.Up(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var private bool
	if err := pool.QueryRow(ctx, `SELECT private FROM rooms WHERE id='r1'`).Scan(&private); err != nil {
		t.Fatalf("existing row after upgrade: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(list) {
		t.Fatalf("recorded %d migrations, want %d", n, len(list))
	}
}
