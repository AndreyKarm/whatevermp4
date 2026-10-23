package transcode

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// Thumbnails describes the timeline preview sprites in thumbs/: sheets
// sprite_001.jpg, sprite_002.jpg, … of Cols×Rows tiles, one tile every
// Interval seconds (tile n covers [n*Interval, (n+1)*Interval)).
type Thumbnails struct {
	Interval float64 `json:"interval"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Cols     int     `json:"cols"`
	Rows     int     `json:"rows"`
	Count    int     `json:"count"`
}

const (
	thumbW, thumbH       = 160, 90
	thumbCols, thumbRows = 10, 10
	maxThumbs            = 300 // per movie, before the interval grows past 2s
)

// thumbInterval spaces about maxThumbs tiles over the movie, 2-10s apart.
func thumbInterval(duration float64) float64 {
	return min(max(math.Ceil(duration/maxThumbs), 2), 10)
}

// Sprites renders the preview sheets into out/thumbs. Only keyframes are
// decoded, which keeps it fast; tiles land on the nearest keyframe.
func Sprites(ctx context.Context, src, out string, videoIndex int, duration float64) (*Thumbnails, error) {
	if duration <= 0 {
		return nil, fmt.Errorf("unknown duration")
	}
	dir := filepath.Join(out, "thumbs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	t := &Thumbnails{
		Interval: thumbInterval(duration),
		Width:    thumbW, Height: thumbH,
		Cols: thumbCols, Rows: thumbRows,
	}
	t.Count = int(math.Ceil(duration / t.Interval))
	vf := fmt.Sprintf("fps=1/%g,scale=%d:%d:force_original_aspect_ratio=decrease,"+
		"pad=%d:%d:(ow-iw)/2:(oh-ih)/2,tile=%dx%d",
		t.Interval, thumbW, thumbH, thumbW, thumbH, thumbCols, thumbRows)
	err := run(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-skip_frame", "nokey", "-i", src,
		"-map", fmt.Sprintf("0:%d", videoIndex), "-an", "-sn",
		"-vf", vf, "-fps_mode", "vfr", "-q:v", "5",
		filepath.Join(dir, "sprite_%03d.jpg"))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return t, nil
}
