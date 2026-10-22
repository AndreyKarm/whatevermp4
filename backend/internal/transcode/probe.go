package transcode

import (
	"context"
	"encoding/json"
	"os/exec"
)

type Stream struct {
	Index       int               `json:"index"`
	CodecName   string            `json:"codec_name"`
	CodecType   string            `json:"codec_type"`
	Profile     string            `json:"profile"`
	PixFmt      string            `json:"pix_fmt"`
	Height      int               `json:"height"`
	Channels    int               `json:"channels"`
	Tags        map[string]string `json:"tags"`
	Disposition map[string]int    `json:"disposition"`
}

type ProbeResult struct {
	Streams []Stream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func Probe(ctx context.Context, path string) (*ProbeResult, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error",
		"-print_format", "json", "-show_streams", "-show_format", path).Output()
	if err != nil {
		return nil, err
	}
	var r ProbeResult
	return &r, json.Unmarshal(out, &r)
}
