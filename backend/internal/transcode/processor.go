package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"liotom/whateverserver/internal/storage"
	"log"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Processor struct {
	DB      *pgxpool.Pool
	St      *storage.Storage
	WorkDir string
	Encoder Encoder // for re-encoded variants; falls back to Software if it fails
}

// reporter returns a progress callback that stores media_files.progress,
// at most every few seconds and only on visible change, since each write
// makes every open library page refetch.
func (p *Processor) reporter(ctx context.Context, id string) func(float64) {
	var last float64 = -1
	var at time.Time
	return func(f float64) {
		if f-last < 0.01 || time.Since(at) < 3*time.Second {
			return
		}
		last, at = f, time.Now()
		_, _ = p.DB.Exec(ctx, `UPDATE media_files SET progress=$2 WHERE id=$1`, id, f)
	}
}

func (p *Processor) Process(ctx context.Context, id string) error {
	dir := filepath.Join(p.WorkDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	var rawKey string
	var rawDeleted bool
	err := p.DB.QueryRow(ctx, `SELECT raw_key, raw_deleted FROM media_files WHERE id=$1`, id).
		Scan(&rawKey, &rawDeleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // deleted before the job ran
	}
	if err != nil {
		return err
	}
	if rawDeleted {
		return errors.New("the original was deleted")
	}
	_, _ = p.DB.Exec(ctx, `UPDATE media_files SET status='processing', progress=NULL WHERE id=$1`, id)

	// the stored original's extension; the display name may have lost it in a rename
	src := filepath.Join(dir, "src"+path.Ext(rawKey))
	if err := p.St.Download(ctx, rawKey, src); err != nil {
		return err
	}
	probe, err := Probe(ctx, src)
	if err != nil {
		return err
	}
	plan, err := BuildPlan(probe)
	if err != nil {
		return err
	}

	out := filepath.Join(dir, "out")
	report := p.reporter(ctx, id)
	err = RunHLS(ctx, src, out, plan, p.Encoder, report)
	if err != nil && p.Encoder != Software && ctx.Err() == nil {
		log.Printf("process %s: %s encode failed, retrying in software: %v", id, p.Encoder, err)
		if err := os.RemoveAll(out); err != nil {
			return err
		}
		err = RunHLS(ctx, src, out, plan, Software, report)
	}
	if err != nil {
		return err
	}
	subs := ExtractSubs(ctx, src, out, plan.Subs)
	fonts := ExtractFonts(ctx, src, out)
	thumbs, err := Sprites(ctx, src, out, plan.VideoIndex, plan.Duration)
	if err != nil {
		log.Printf("thumbnails: %v", err)
	}

	prefix := "hls/" + id
	var poster *string
	if err := Poster(ctx, src, filepath.Join(out, "poster.jpg"), plan.Duration); err != nil {
		log.Printf("poster: %v", err)
	} else {
		k := prefix + "/poster.jpg"
		poster = &k
	}

	// Re-processing: drop the previous output, whose file names may differ.
	if err := p.St.DeletePrefix(ctx, prefix); err != nil {
		return err
	}
	// fonts/ and thumbs/ live inside out/, so they are uploaded with everything else
	if err := p.St.UploadDir(ctx, prefix, out); err != nil {
		return err
	}

	var rawBytes int64
	if fi, err := os.Stat(src); err == nil {
		rawBytes = fi.Size()
	}
	hlsBytes, err := dirSize(out)
	if err != nil {
		return err
	}
	audio, _ := json.Marshal(plan.Audio)
	sj, _ := json.Marshal(subs)
	fj, _ := json.Marshal(fonts)
	var tj []byte // NULL when there are no thumbnails
	if thumbs != nil {
		tj, _ = json.Marshal(thumbs)
	}
	tag, err := p.DB.Exec(ctx, `
		UPDATE media_files SET status='ready', hls_key=$2, poster_key=$3,
			duration_sec=$4, audio_tracks=$5, subtitle_tracks=$6, fonts=$7, thumbnails=$8,
			raw_bytes=$9, hls_bytes=$10, progress=NULL, error=NULL
		WHERE id=$1`, id, prefix, poster, plan.Duration, audio, sj, fj, tj, rawBytes, hlsBytes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// deleted while processing: don't leave the output behind
		return p.St.DeletePrefix(ctx, prefix)
	}
	return nil
}

func dirSize(dir string) (int64, error) {
	var n int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fi, err := d.Info()
		if err == nil {
			n += fi.Size()
		}
		return err
	})
	return n, err
}
