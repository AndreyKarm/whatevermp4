package transcode

import (
	"context"
	"encoding/json"
	"errors"
	"liotom/whateverserver/internal/storage"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Processor struct {
	DB      *pgxpool.Pool
	St      *storage.Storage
	WorkDir string
}

func (p *Processor) Process(ctx context.Context, id string) error {
	dir := filepath.Join(p.WorkDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	var rawKey string
	err := p.DB.QueryRow(ctx, `SELECT raw_key FROM media_files WHERE id=$1`, id).Scan(&rawKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // deleted before the job ran
	}
	if err != nil {
		return err
	}
	_, _ = p.DB.Exec(ctx, `UPDATE media_files SET status='processing' WHERE id=$1`, id)

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
	if err := RunHLS(ctx, src, out, plan); err != nil {
		return err
	}
	subs := ExtractSubs(ctx, src, out, plan.Subs)
	fonts := ExtractFonts(ctx, src, out)

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
	// fonts/ lives inside out/, so it is uploaded with everything else
	if err := p.St.UploadDir(ctx, prefix, out); err != nil {
		return err
	}

	audio, _ := json.Marshal(plan.Audio)
	sj, _ := json.Marshal(subs)
	fj, _ := json.Marshal(fonts)
	tag, err := p.DB.Exec(ctx, `
		UPDATE media_files SET status='ready', hls_key=$2, poster_key=$3,
			duration_sec=$4, audio_tracks=$5, subtitle_tracks=$6, fonts=$7, error=NULL
		WHERE id=$1`, id, prefix, poster, plan.Duration, audio, sj, fj)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// deleted while processing: don't leave the output behind
		return p.St.DeletePrefix(ctx, prefix)
	}
	return nil
}
