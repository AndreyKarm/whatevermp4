package torrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"liotom/whateverserver/internal/jobs"
	"liotom/whateverserver/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Downloader struct {
	db     *pgxpool.Pool
	st     *storage.Storage
	dir    string
	client *atorrent.Client
}

func New(db *pgxpool.Pool, st *storage.Storage, dir string) (*Downloader, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfg := atorrent.NewDefaultClientConfig()
	cfg.DataDir = dir
	cfg.NoUpload = true // never upload piece data
	cfg.Seed = false
	c, err := atorrent.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Downloader{db: db, st: st, dir: dir, client: c}, nil
}

func (d *Downloader) Close() { d.client.Close() }

var videoExt = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true,
	".webm": true, ".m4v": true, ".mov": true, ".ts": true,
}

func (d *Downloader) cancelled(ctx context.Context, id string) bool {
	var st string
	_ = d.db.QueryRow(ctx, `SELECT status FROM torrents WHERE id=$1`, id).Scan(&st)
	return st == "cancelled"
}

func (d *Downloader) Run(ctx context.Context, id string) error {
	var userID string
	var magnet *string
	var meta []byte
	if err := d.db.QueryRow(ctx,
		`SELECT user_id, magnet, metainfo FROM torrents WHERE id=$1`, id).
		Scan(&userID, &magnet, &meta); err != nil {
		return err
	}
	if d.cancelled(ctx, id) {
		return nil
	}

	var t *atorrent.Torrent
	var err error
	if len(meta) > 0 {
		mi, lerr := metainfo.Load(bytes.NewReader(meta))
		if lerr != nil {
			return lerr
		}
		t, err = d.client.AddTorrent(mi)
	} else if magnet != nil {
		t, err = d.client.AddMagnet(*magnet)
	} else {
		return errors.New("no magnet or metainfo")
	}
	if err != nil {
		return err
	}
	dataPath := ""
	dropped := false
	drop := func() {
		if !dropped {
			t.Drop()
			dropped = true
		}
	}
	defer func() {
		drop()
		if dataPath != "" {
			_ = os.RemoveAll(dataPath)
		}
	}()

	select {
	case <-t.GotInfo():
	case <-time.After(5 * time.Minute):
		return errors.New("timed out fetching metadata")
	case <-ctx.Done():
		return ctx.Err()
	}
	if n := t.Name(); n != "" && n != "." && n != ".." {
		dataPath = filepath.Join(d.dir, filepath.Base(n))
	}
	_, _ = d.db.Exec(ctx, `UPDATE torrents SET name=$2 WHERE id=$1`, id, t.Name())

	var picked []*atorrent.File
	var total int64
	for _, f := range t.Files() {
		p := strings.ToLower(f.Path())
		if videoExt[filepath.Ext(p)] && !strings.Contains(p, "sample") {
			f.Download()
			picked = append(picked, f)
			total += f.Length()
		}
	}
	if len(picked) == 0 {
		return errors.New("no video files in torrent")
	}

	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var last int64
	lastMove := time.Now()
	for {
		var done int64
		for _, f := range picked {
			done += f.BytesCompleted()
		}
		_, _ = d.db.Exec(ctx, `UPDATE torrents SET status='downloading',
			progress=$2, bytes_done=$3, bytes_total=$4 WHERE id=$1`,
			id, float64(done)/float64(total), done, total)
		if done >= total {
			break
		}
		if done > last {
			last, lastMove = done, time.Now()
		} else if time.Since(lastMove) > 30*time.Minute {
			return errors.New("download stalled")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
		if d.cancelled(ctx, id) {
			return nil
		}
	}

	drop()
	_, _ = d.db.Exec(ctx, `UPDATE torrents SET status='importing' WHERE id=$1`, id)

	for _, f := range picked {
		base := filepath.Base(f.Path())
		mid := uuid.NewString()
		key := "raw/" + mid + "/" + storage.SafeName(base)
		if err := d.st.UploadFile(ctx, key, filepath.Join(d.dir, f.Path())); err != nil {
			return fmt.Errorf("upload %s: %w", base, err)
		}
		s, e := storage.ParseSE(base)
		if _, err := d.db.Exec(ctx, `
			INSERT INTO media_files (id, uploader_id, filename, status, raw_key, season, episode, torrent_id)
			VALUES ($1,$2,$3,'processing',$4,$5,$6,$7)`,
			mid, userID, base, key, s, e, id); err != nil {
			return err
		}
		if err := jobs.Enqueue(ctx, d.db, "process", map[string]string{"mediaId": mid}); err != nil {
			return err
		}
	}
	_, err = d.db.Exec(ctx, `UPDATE torrents SET status='done', progress=1 WHERE id=$1`, id)
	return err
}
