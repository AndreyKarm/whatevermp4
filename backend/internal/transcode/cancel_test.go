package transcode

import (
	"context"
	"errors"
	"liotom/whateverserver/internal/testdb"
	"testing"
	"time"
)

func TestWatchCancel(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	prev := cancelPoll
	cancelPoll = 50 * time.Millisecond
	t.Cleanup(func() { cancelPoll = prev })
	if _, err := db.Exec(ctx, `INSERT INTO media_files (id, uploaded_by, filename, status, raw_key) VALUES
		('a', 'x', 'a', 'processing', 'k'), ('b', 'x', 'b', 'processing', 'k')`); err != nil {
		t.Fatal(err)
	}
	notReady := func(s string) bool { return s != "ready" }

	waitCancelled := func(c context.Context) bool {
		select {
		case <-c.Done():
			return cancelled(c)
		case <-time.After(2 * time.Second):
			return false
		}
	}

	// still wanted: keeps running
	c, stop := watchCancel(ctx, db, "a", notReady)
	select {
	case <-c.Done():
		t.Fatal("cancelled while still wanted")
	case <-time.After(300 * time.Millisecond):
	}
	// a re-process put back to ready
	_, _ = db.Exec(ctx, `UPDATE media_files SET status='ready' WHERE id='a'`)
	if !waitCancelled(c) {
		t.Fatal("not cancelled after the status changed")
	}
	stop()

	// the row deleted
	c, stop = watchCancel(ctx, db, "b", notReady)
	defer stop()
	_, _ = db.Exec(ctx, `DELETE FROM media_files WHERE id='b'`)
	if !waitCancelled(c) {
		t.Fatal("not cancelled after the row was deleted")
	}

	// a worker shutdown is not a cancel
	parent, shutdown := context.WithCancel(ctx)
	_, _ = db.Exec(ctx, `UPDATE media_files SET status='processing' WHERE id='a'`)
	c, stop2 := watchCancel(parent, db, "a", notReady)
	defer stop2()
	shutdown()
	<-c.Done()
	if cancelled(c) {
		t.Fatal("shutdown reported as cancel")
	}
}

// A re-process cancelled while its job was still queued is back to ready:
// the job ends without touching anything.
func TestProcessSkipsCancelledJob(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if _, err := db.Exec(ctx, `INSERT INTO media_files (id, uploaded_by, filename, status, raw_key, hls_key)
		VALUES ('m', 'x', 'm', 'ready', 'raw/m/a.mkv', 'hls/m')`); err != nil {
		t.Fatal(err)
	}
	p := &Processor{DB: db, WorkDir: t.TempDir()}
	if err := p.Process(ctx, "m"); !errors.Is(err, ErrCancelled) {
		t.Fatalf("got %v, want ErrCancelled", err)
	}
	var status string
	_ = db.QueryRow(ctx, `SELECT status FROM media_files WHERE id='m'`).Scan(&status)
	if status != "ready" {
		t.Fatalf("status changed to %s", status)
	}
	if err := p.Process(ctx, "gone"); err != nil {
		t.Fatalf("deleted media: %v", err)
	}
}
