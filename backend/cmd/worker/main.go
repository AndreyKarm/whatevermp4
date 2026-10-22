package main

import (
	"context"
	"encoding/json"
	"liotom/whateverserver/internal/config"
	"liotom/whateverserver/internal/db"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"liotom/whateverserver/internal/torrent"
	"liotom/whateverserver/internal/transcode"
	"log"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	st, err := storage.New(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	proc := &transcode.Processor{DB: pool, St: st, WorkDir: filepath.Join(cfg.WorkDir, "transcode")}
	dl, err := torrent.New(pool, st, filepath.Join(cfg.WorkDir, "torrents"))
	if err != nil {
		log.Fatal(err)
	}
	defer dl.Close()

	// single worker instance: anything still "running" was interrupted
	_, _ = pool.Exec(ctx, `UPDATE jobs SET status='queued' WHERE status='running'`)

	var wg sync.WaitGroup
	wg.Add(3)
	go loop(ctx, &wg, pool, []string{"process"}, func(ctx context.Context, p json.RawMessage) error {
		var in struct {
			MediaID string `json:"mediaId"`
		}
		if err := json.Unmarshal(p, &in); err != nil {
			return err
		}
		err := proc.Process(ctx, in.MediaID)
		if err != nil {
			_, _ = pool.Exec(context.Background(),
				`UPDATE media_files SET status='failed', error=$2 WHERE id=$1`, in.MediaID, err.Error())
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
		if err != nil && ctx.Err() == nil {
			_, _ = pool.Exec(context.Background(),
				`UPDATE media_files SET status='failed', error=$2 WHERE id=$1`, in.MediaID, err.Error())
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
		if err := fn(ctx, j.Payload); err != nil {
			log.Printf("job %d failed: %v", j.ID, err)
			_ = jobs.Fail(context.Background(), pool, j.ID, err.Error(), j.Attempts >= 3)
			continue
		}
		_ = jobs.Done(context.Background(), pool, j.ID)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
