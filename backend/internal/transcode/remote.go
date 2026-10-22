package transcode

import (
	"context"
	"errors"
	"liotom/whateverserver/internal/jobs"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// DownloadHLS remuxes a remote HLS stream into a local MKV without re-encoding.
// For master playlists ffmpeg's default selection picks the best video and audio.
func DownloadHLS(ctx context.Context, src, referer, dst string) error {
	args := []string{"-y", "-hide_banner", "-loglevel", "error",
		// network only: a playlist must not be able to make ffmpeg read local files
		"-protocol_whitelist", "http,https,tls,tcp,crypto",
		"-rw_timeout", "30000000", // µs; fail instead of hanging on a dead server
		"-user_agent", browserUA}
	if referer != "" {
		args = append(args, "-referer", referer)
	}
	args = append(args, "-i", src, "-c", "copy", "-dn", "-f", "matroska", dst)
	return run(ctx, "ffmpeg", args...)
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
	_, _ = p.DB.Exec(ctx, `UPDATE media_files SET status='downloading', error=NULL WHERE id=$1`, id)

	dir := filepath.Join(p.WorkDir, "import-"+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	out := filepath.Join(dir, "src.mkv")
	if err := DownloadHLS(ctx, src, referer, out); err != nil {
		return err
	}
	if err := p.St.UploadFile(ctx, rawKey, out); err != nil {
		return err
	}

	tag, err := p.DB.Exec(ctx, `UPDATE media_files SET status='processing' WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// deleted while downloading
		return p.St.DeleteKey(ctx, rawKey)
	}
	return jobs.Enqueue(ctx, p.DB, "process", map[string]string{"mediaId": id})
}
