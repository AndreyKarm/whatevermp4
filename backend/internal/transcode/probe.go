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
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	Channels    int               `json:"channels"`
	Tags        map[string]string `json:"tags"`
	Disposition map[string]int    `json:"disposition"`
	SideData    []struct {
		Rotation float64 `json:"rotation"`
	} `json:"side_data_list"`
}

// displaySize is the frame size as shown. Phones store portrait video as
// landscape plus a rotation, which ffmpeg applies when it encodes.
func (s Stream) displaySize() (w, h int) {
	for _, d := range s.SideData {
		if int(d.Rotation)%180 != 0 {
			return s.Height, s.Width
		}
	}
	return s.Width, s.Height
}

type ProbeResult struct {
	Streams []Stream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Probe runs ffprobe on path; inputOpts go before it (e.g. network options).
func Probe(ctx context.Context, path string, inputOpts ...string) (*ProbeResult, error) {
	args := append([]string{"-v", "error", "-print_format", "json", "-show_streams", "-show_format"},
		inputOpts...)
	out, err := exec.CommandContext(ctx, "ffprobe", append(args, path)...).Output()
	if err != nil {
		return nil, err
	}
	var r ProbeResult
	return &r, json.Unmarshal(out, &r)
}
