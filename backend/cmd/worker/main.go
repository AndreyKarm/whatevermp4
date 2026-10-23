package main

import (
	"context"
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/config"
	"liotom/whateverserver/internal/db"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/migrate"
	"liotom/whateverserver/internal/storage"
	"liotom/whateverserver/internal/torrent"
	"liotom/whateverserver/internal/transcode"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck(cfg.WorkerAddr)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := cfg.Check(); err != nil {
		log.Fatal(err)
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Up(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	st, err := storage.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	enc := transcode.Detect(ctx, transcode.ParseEncoder(cfg.HWEncoder))
	hwEncoder.WithLabelValues(string(enc)).Set(1)
	proc := &transcode.Processor{DB: pool, St: st, WorkDir: filepath.Join(cfg.WorkDir, "transcode"),
		Encoder: enc}
	dl, err := torrent.New(pool, st, filepath.Join(cfg.WorkDir, "torrents"))
	if err != nil {
		log.Fatal(err)
	}
	defer dl.Close()

	// single worker instance: anything still "running" was interrupted
	_, _ = pool.Exec(ctx, `UPDATE jobs SET status='queued' WHERE status='running'`)

	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		serveHealth(ctx, cfg.WorkerAddr, pool)
	}()
	go janitor(ctx, &wg, pool, st)
	go loop(ctx, &wg, pool, []string{"process"}, func(ctx context.Context, p json.RawMessage) error {
		var in struct {
			MediaID string `json:"mediaId"`
		}
		if err := json.Unmarshal(p, &in); err != nil {
			return err
		}
		err := proc.Process(ctx, in.MediaID)
		if errors.Is(err, transcode.ErrCancelled) {
			log.Printf("process %s: cancelled", in.MediaID)
			return nil // done, not failed: nothing to retry
		}
		if err != nil {
			_, _ = pool.Exec(context.Background(),
				`UPDATE media_files SET status='failed', error=$2, progress=NULL WHERE id=$1`,
				in.MediaID, err.Error())
		}
		return err
	})
	go loop(ctx, &wg, pool, []string{"torrent"}, func(ctx context.Context, p json.RawMessage) error {
		var in struct {
			TorrentID string `json:"torrentId"`
		}
		if err := json.Unmarshal(p, &in); err != nil {
			return err
		}
		err := dl.Run(ctx, in.TorrentID)
		if err != nil && ctx.Err() == nil {
			_, _ = pool.Exec(context.Background(),
				`UPDATE torrents SET status='failed', error=$2 WHERE id=$1`, in.TorrentID, err.Error())
		}
		return err
	})
	go loop(ctx, &wg, pool, []string{"import"}, func(ctx context.Context, p json.RawMessage) error {
		var in struct {
			MediaID string `json:"mediaId"`
			URL     string `json:"url"`
			Referer string `json:"referer"`
		}
		if err := json.Unmarshal(p, &in); err != nil {
			return err
		}
		err := proc.Import(ctx, in.MediaID, in.URL, in.Referer)
		if errors.Is(err, transcode.ErrCancelled) {
			log.Printf("import %s: cancelled", in.MediaID)
			return nil
		}
		if err != nil && ctx.Err() == nil {
			_, _ = pool.Exec(context.Background(),
				`UPDATE media_files SET status='failed', error=$2, progress=NULL WHERE id=$1`,
				in.MediaID, err.Error())
		}
		return err
	})
	wg.Wait()
}

func loop(ctx context.Context, wg *sync.WaitGroup, pool *pgxpool.Pool, types []string,
	fn func(context.Context, json.RawMessage) error) {
	defer wg.Done()
	for ctx.Err() == nil {
		j, err := jobs.Claim(ctx, pool, types)
		if err != nil {
			log.Printf("claim: %v", err)
			sleep(ctx, 5*time.Second)
			continue
		}
		if j == nil {
			sleep(ctx, 3*time.Second)
			continue
		}
		log.Printf("job %d (%s) attempt %d", j.ID, j.Type, j.Attempts)
		jobsRunning.WithLabelValues(j.Type).Inc()
		start := time.Now()
		err = fn(ctx, j.Payload)
		jobsRunning.WithLabelValues(j.Type).Dec()
		result := "done"
		if err != nil {
			result = "failed"
		}
		jobDuration.WithLabelValues(j.Type, result).Observe(time.Since(start).Seconds())
		if err != nil {
			log.Printf("job %d failed: %v", j.ID, err)
			_ = jobs.Fail(context.Background(), pool, j.ID, err.Error(), j.Attempts >= 3)
			continue
		}
		_ = jobs.Done(context.Background(), pool, j.ID)
	}
}

// janitor runs at startup and then hourly: it removes uploads abandoned for
// a day (with their stored parts) and finished jobs older than a week, and
// measures ready media processed before sizes were recorded.
func janitor(ctx context.Context, wg *sync.WaitGroup, pool *pgxpool.Pool, st *storage.Storage) {
	defer wg.Done()
	for ctx.Err() == nil {
		rows, err := pool.Query(ctx, `
			DELETE FROM media_files
			WHERE status='uploading' AND created_at < now() - interval '1 day'
			RETURNING id, raw_key`)
		if err == nil {
			var stale [][2]string
			for rows.Next() {
				var id, rawKey string
				if rows.Scan(&id, &rawKey) == nil {
					stale = append(stale, [2]string{id, rawKey})
				}
			}
			rows.Close()
			for _, s := range stale {
				log.Printf("janitor: removing abandoned upload %s", s[0])
				st.DeleteMedia(ctx, s[0], s[1])
			}
		} else if ctx.Err() == nil {
			log.Printf("janitor: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			DELETE FROM jobs
			WHERE status IN ('done','failed') AND created_at < now() - interval '7 days'`); err != nil && ctx.Err() == nil {
			log.Printf("janitor: %v", err)
		}
		measureSizes(ctx, pool, st)
		sleep(ctx, time.Hour)
	}
}

// measureSizes fills raw_bytes / hls_bytes for ready media that lack them.
func measureSizes(ctx context.Context, pool *pgxpool.Pool, st *storage.Storage) {
	rows, err := pool.Query(ctx, `
		SELECT id, raw_key, raw_deleted FROM media_files
		WHERE status='ready' AND (hls_bytes IS NULL OR (raw_bytes IS NULL AND NOT raw_deleted))`)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("janitor: sizes: %v", err)
		}
		return
	}
	type row struct {
		id, rawKey string
		rawDeleted bool
	}
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.rawKey, &r.rawDeleted) == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	var ids []string
	var raws, hlss []int64
	for _, r := range list {
		hls, err := st.PrefixSize(ctx, "hls/"+r.id)
		if err != nil {
			log.Printf("janitor: size of %s: %v", r.id, err)
			continue
		}
		var raw int64
		if !r.rawDeleted {
			if raw, err = st.ObjectSize(ctx, r.rawKey); err != nil {
				log.Printf("janitor: size of %s: %v", r.rawKey, err)
				continue
			}
		}
		ids, raws, hlss = append(ids, r.id), append(raws, raw), append(hlss, hls)
	}
	if len(ids) == 0 {
		return
	}
	// one statement, so open pages refetch once
	if _, err := pool.Exec(ctx, `
		UPDATE media_files m SET raw_bytes = v.raw, hls_bytes = v.hls
		FROM unnest($1::text[], $2::bigint[], $3::bigint[]) AS v(id, raw, hls)
		WHERE m.id = v.id`, ids, raws, hlss); err != nil {
		log.Printf("janitor: sizes: %v", err)
		return
	}
	log.Printf("janitor: measured %d media", len(ids))
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
