package transcode

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrCancelled: the media was cancelled (POST /api/media/{id}/cancel) while
// its job waited or ran. The job is done, not failed.
var ErrCancelled = errors.New("cancelled")

// cancelPoll is how often a running job checks whether it was cancelled.
var cancelPoll = 2 * time.Second

// watchCancel returns a context that is cancelled once the media row is gone
// or wanted(status) turns false, checked every cancelPoll. Call stop when
// the job ends.
func watchCancel(ctx context.Context, db *pgxpool.Pool, id string,
	wanted func(status string) bool) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(cancelPoll)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				var status string
				err := db.QueryRow(ctx, `SELECT status FROM media_files WHERE id=$1`, id).Scan(&status)
				if errors.Is(err, pgx.ErrNoRows) {
					cancel(ErrCancelled)
					return
				}
				if err == nil && !wanted(status) {
					cancel(ErrCancelled)
					return
				}
			}
		}
	}()
	return ctx, func() {
		close(done)
		cancel(nil)
	}
}

// cancelled reports whether ctx was stopped by watchCancel (and not by a
// worker shutdown).
func cancelled(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrCancelled)
}
