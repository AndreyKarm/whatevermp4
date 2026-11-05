package transcode

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type AudioTrack struct {
	N        int    `json:"n"` // audio ordinal (0:a:N)
	Lang     string `json:"lang"`
	Title    string `json:"title"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Name     string `json:"name"` // HLS rendition dir, e.g. jpn_0
	Copy     bool   `json:"-"`
}

type SubTrack struct {
	N     int    `json:"n"` // subtitle ordinal (0:s:N)
	Lang  string `json:"lang"`
	Title string `json:"title"`
	IsASS bool   `json:"isAss"`
	VTT   string `json:"vtt"`           // relative path under hls/{id}/
	ASS   string `json:"ass,omitempty"` // relative path, when original is ASS/SSA
}

type Plan struct {
	VideoIndex  int
	VideoCopy   bool
	VideoWidth  int // as displayed (rotation applied); 0 if ffprobe didn't report it
	VideoHeight int
	Audio       []AudioTrack
	Subs        []SubTrack
	Duration    float64
}

var safe = regexp.MustCompile(`[^a-z0-9]+`)

func lang(s Stream) string {
	l := safe.ReplaceAllString(strings.ToLower(s.Tags["language"]), "")
	if l == "" {
		return "und"
	}
	return l
}

func BuildPlan(p *ProbeResult) (*Plan, error) {
	plan := &Plan{VideoIndex: -1}
	plan.Duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	var ai, si int
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if plan.VideoIndex >= 0 || s.Disposition["attached_pic"] == 1 {
				continue
			}
			plan.VideoIndex = s.Index
			plan.VideoWidth, plan.VideoHeight = s.displaySize()
			// Browser-safe: 8-bit H.264 only. Everything else gets re-encoded.
			plan.VideoCopy = s.CodecName == "h264" && (s.PixFmt == "yuv420p" || s.PixFmt == "yuvj420p")
		case "audio":
			l := lang(s)
			plan.Audio = append(plan.Audio, AudioTrack{
				N: ai, Lang: l, Title: s.Tags["title"], Codec: s.CodecName,
				Channels: s.Channels, Name: fmt.Sprintf("%s_%d", l, ai),
				Copy: s.CodecName == "aac" && s.Channels <= 2,
			})
			ai++
		case "subtitle":
			t := SubTrack{N: si, Lang: lang(s), Title: s.Tags["title"]}
			si++
			switch s.CodecName {
			case "ass", "ssa":
				t.IsASS = true
			case "subrip", "srt", "webvtt", "mov_text", "text":
			default:
				continue // bitmap subs (PGS/VobSub) are skipped
			}
			t.VTT = fmt.Sprintf("subs/s%d_%s.vtt", t.N, t.Lang)
			if t.IsASS {
				t.ASS = fmt.Sprintf("subs/s%d_%s.ass", t.N, t.Lang)
			}
			plan.Subs = append(plan.Subs, t)
		}
	}
	if plan.VideoIndex < 0 {
		return nil, errors.New("no video stream")
	}
	return plan, nil
}
