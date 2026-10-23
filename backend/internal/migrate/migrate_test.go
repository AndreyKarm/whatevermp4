package migrate_test

import (
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/migrate"
	"liotom/whateverserver/internal/testdb"
	"liotom/whateverserver/migrations"
	"testing"
	"time"
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

// The triggers say what changed: progress-only updates carry the value,
// room state flushes stay quiet, and real changes name the row.
func TestNotifyPayloads(t *testing.T) {
	ctx := context.Background()
	pool := testdb.New(t)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN events"); err != nil {
		t.Fatal(err)
	}
	next := func() map[string]any {
		t.Helper()
		wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		n, err := conn.Conn().WaitForNotification(wctx)
		if err != nil {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(n.Payload), &m); err != nil {
			t.Fatalf("payload %q: %v", n.Payload, err)
		}
		return m
	}
	exec := func(sql string) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}

	exec(`INSERT INTO media_files (id, uploaded_by, filename, status, raw_key) VALUES ('m1', 'a', 'f', 'processing', 'k')`)
	if m := next(); m["type"] != "media_files" || m["op"] != "INSERT" || m["id"] != "m1" {
		t.Fatalf("insert: %v", m)
	}
	exec(`UPDATE media_files SET progress = 0.5 WHERE id = 'm1'`)
	if m := next(); m["type"] != "media_progress" || m["progress"] != 0.5 {
		t.Fatalf("progress: %v", m)
	}
	exec(`UPDATE media_files SET status = 'processing' WHERE id = 'm1'`) // no change at all
	exec(`UPDATE media_files SET status = 'ready', progress = NULL WHERE id = 'm1'`)
	if m := next(); m["type"] != "media_files" || m["op"] != "UPDATE" {
		t.Fatalf("a no-op update should be silent, then the real one: %v", m)
	}

	exec(`INSERT INTO torrents (id, added_by) VALUES ('t1', 'a')`)
	next()
	exec(`UPDATE torrents SET status = 'queued', progress = 0.25, bytes_done = 5, bytes_total = 20 WHERE id = 't1'`)
	if m := next(); m["type"] != "torrent_progress" || m["bytesDone"] != 5.0 || m["bytesTotal"] != 20.0 {
		t.Fatalf("torrent progress: %v", m)
	}

	exec(`INSERT INTO rooms (id, owner_name) VALUES ('r1', 'a')`)
	next()
	exec(`UPDATE rooms SET state = '{"mediaId":"m1"}', updated_at = now() WHERE id = 'r1'`)
	exec(`UPDATE rooms SET name = 'Party' WHERE id = 'r1'`)
	if m := next(); m["type"] != "rooms" || m["op"] != "UPDATE" || m["id"] != "r1" {
		t.Fatalf("state flushes should be silent, settings not: %v", m)
	}
	exec(`DELETE FROM media_files WHERE id = 'm1'`)
	if m := next(); m["type"] != "media_files" || m["op"] != "DELETE" {
		t.Fatalf("delete: %v", m)
	}

	exec(`INSERT INTO folders (id, name) VALUES ('f1', 'x')`)
	if m := next(); m["type"] != "folders" {
		t.Fatalf("folders: %v", m)
	}
	if m := next(); m != nil {
		t.Fatalf("unexpected extra event: %v", m)
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
