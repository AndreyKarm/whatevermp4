package transcode

import (
	"context"
	"errors"
	"liotom/whateverserver/internal/jobs"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jackc/pgx/v5"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// netInput are the input options for reading a remote stream.
func netInput(referer string) []string {
	args := []string{
		// network only: a playlist must not be able to make ffmpeg read local files
		"-protocol_whitelist", "http,https,tls,tcp,crypto",
		"-rw_timeout", "30000000", // µs; fail instead of hanging on a dead server
		"-user_agent", browserUA}
	if referer != "" {
		args = append(args, "-referer", referer)
	}
	return args
}

// DownloadHLS remuxes a remote HLS stream into a local MKV without re-encoding.
// For master playlists ffmpeg's default selection picks the best video and audio.
// duration (0 = unknown) lets report (may be nil) get the fraction done.
func DownloadHLS(ctx context.Context, src, referer, dst string, duration float64, report func(float64)) error {
	args := append([]string{"-y", "-hide_banner", "-loglevel", "error"}, netInput(referer)...)
	args = append(args, "-i", src, "-c", "copy", "-dn", "-f", "matroska", dst)
	return runProgress(ctx, duration, report, args...)
}

// Import downloads an HLS URL into the media's raw key and queues the
// regular "process" job for it.
func (p *Processor) Import(ctx context.Context, id, src, referer string) error {
	var rawKey string
	err := p.DB.QueryRow(ctx, `SELECT raw_key FROM media_files WHERE id=$1`, id).Scan(&rawKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // deleted before the job ran
	}
	if err != nil {
		return err
	}
	_, _ = p.DB.Exec(ctx, `UPDATE media_files SET status='downloading', error=NULL, progress=NULL WHERE id=$1`, id)

	dir := filepath.Join(p.WorkDir, "import-"+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// The playlist's length, for a percentage; live or odd streams have none.
	var duration float64
	if pr, err := Probe(ctx, src, netInput(referer)...); err != nil {
		log.Printf("import %s: probe: %v", id, err)
	} else {
		duration, _ = strconv.ParseFloat(pr.Format.Duration, 64)
	}

	out := filepath.Join(dir, "src.mkv")
	if err := DownloadHLS(ctx, src, referer, out, duration, p.reporter(ctx, id)); err != nil {
		return err
	}
	if err := p.St.UploadFile(ctx, rawKey, out); err != nil {
		return err
	}

	tag, err := p.DB.Exec(ctx, `UPDATE media_files SET status='processing', progress=NULL WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// deleted while downloading
		return p.St.DeleteKey(ctx, rawKey)
	}
	return jobs.Enqueue(ctx, p.DB, "process", map[string]string{"mediaId": id})
}
