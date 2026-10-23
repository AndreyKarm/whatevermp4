package jobs_test

import (
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/testdb"
	"sync"
	"testing"
)

func TestClaimOrderAndTypes(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	for _, typ := range []string{"process", "torrent", "process"} {
		if err := jobs.Enqueue(ctx, db, typ, map[string]string{"t": typ}); err != nil {
			t.Fatal(err)
		}
	}

	j, err := jobs.Claim(ctx, db, []string{"process"})
	if err != nil || j == nil {
		t.Fatalf("claim: %v %v", j, err)
	}
	if j.Type != "process" || j.Attempts != 1 {
		t.Fatalf("got %+v", j)
	}
	var p map[string]string
	if err := json.Unmarshal(j.Payload, &p); err != nil || p["t"] != "process" {
		t.Fatalf("payload %s", j.Payload)
	}
	second, _ := jobs.Claim(ctx, db, []string{"process"})
	if second == nil || second.ID <= j.ID {
		t.Fatalf("second process job: %+v (first %d)", second, j.ID)
	}
	if none, err := jobs.Claim(ctx, db, []string{"process"}); err != nil || none != nil {
		t.Fatalf("queue should be empty for process: %+v %v", none, err)
	}
	tor, _ := jobs.Claim(ctx, db, []string{"torrent", "import"})
	if tor == nil || tor.Type != "torrent" {
		t.Fatalf("torrent job: %+v", tor)
	}
}

// Concurrent workers never get the same job (FOR UPDATE SKIP LOCKED).
func TestConcurrentClaimsAreDistinct(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	const n = 20
	for i := range n {
		if err := jobs.Enqueue(ctx, db, "process", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	seen := map[int64]bool{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for {
				j, err := jobs.Claim(ctx, db, []string{"process"})
				if err != nil {
					t.Error(err)
					return
				}
				if j == nil {
					return
				}
				mu.Lock()
				if seen[j.ID] {
					t.Errorf("job %d claimed twice", j.ID)
				}
				seen[j.ID] = true
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(seen) != n {
		t.Fatalf("claimed %d jobs, want %d", len(seen), n)
	}
}

func TestFailBacksOffThenGivesUp(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	if err := jobs.Enqueue(ctx, db, "process", nil); err != nil {
		t.Fatal(err)
	}
	j, _ := jobs.Claim(ctx, db, []string{"process"})
	if err := jobs.Fail(ctx, db, j.ID, "boom", false); err != nil {
		t.Fatal(err)
	}
	// requeued, but not claimable until the 30s backoff passes
	if again, _ := jobs.Claim(ctx, db, []string{"process"}); again != nil {
		t.Fatalf("claimed during backoff: %+v", again)
	}
	var status, lastErr string
	_ = db.QueryRow(ctx, `SELECT status, last_error FROM jobs WHERE id=$1`, j.ID).Scan(&status, &lastErr)
	if status != "queued" || lastErr != "boom" {
		t.Fatalf("after retryable failure: %s %q", status, lastErr)
	}

	_, _ = db.Exec(ctx, `UPDATE jobs SET run_at = now() WHERE id=$1`, j.ID)
	j2, _ := jobs.Claim(ctx, db, []string{"process"})
	if j2 == nil || j2.Attempts != 2 {
		t.Fatalf("retry: %+v", j2)
	}
	if err := jobs.Fail(ctx, db, j2.ID, "boom again", true); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(ctx, `SELECT status FROM jobs WHERE id=$1`, j.ID).Scan(&status)
	if status != "failed" {
		t.Fatalf("permanent failure left status %s", status)
	}
}

func TestDone(t *testing.T) {
	ctx := context.Background()
	db := testdb.New(t)
	_ = jobs.Enqueue(ctx, db, "import", nil)
	j, _ := jobs.Claim(ctx, db, []string{"import"})
	if err := jobs.Done(ctx, db, j.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = db.QueryRow(ctx, `SELECT status FROM jobs WHERE id=$1`, j.ID).Scan(&status)
	if status != "done" {
		t.Fatalf("status %s", status)
	}
}
