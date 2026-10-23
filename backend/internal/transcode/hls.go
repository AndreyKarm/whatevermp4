package transcode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, tail(stderr.String(), 1500))
	}
	return nil
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// Lower renditions added under the source so each viewer can pick a quality.
var ladder = []struct{ height, maxRateK int }{
	{1080, 5000}, {720, 2800}, {480, 1400}, {360, 800},
}

const maxExtraRenditions = 2

type variant struct {
	name     string
	height   int // 0: keep the source size
	copy     bool
	maxRateK int // 0: no cap (CRF only)
}

// variants returns the source quality first, then up to maxExtraRenditions
// ladder steps below it.
func variants(p *Plan) []variant {
	name := "src"
	if p.VideoHeight > 0 {
		name = fmt.Sprintf("%dp", p.VideoHeight)
	}
	vs := []variant{{name: name, copy: p.VideoCopy}}
	for _, l := range ladder {
		if len(vs) > maxExtraRenditions || p.VideoHeight == 0 {
			break
		}
		if l.height < p.VideoHeight {
			vs = append(vs, variant{name: fmt.Sprintf("%dp", l.height), height: l.height, maxRateK: l.maxRateK})
		}
	}
	return vs
}

// RunHLS writes out/master.m3u8, out/stream_{name}.m3u8, out/init_{name}.mp4,
// out/seg_{name}_*.m4s. report (may be nil) gets the fraction done.
func RunHLS(ctx context.Context, src, out string, p *Plan, enc Encoder, report func(float64)) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return runProgress(ctx, p.Duration, report, hlsArgs(src, out, p, enc)...)
}

func hlsArgs(src, out string, p *Plan, enc Encoder) []string {
	vs := variants(p)

	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", src}
	for range vs {
		args = append(args, "-map", fmt.Sprintf("0:%d", p.VideoIndex))
	}
	// One audio track is muxed into every video variant (separate audio
	// renditions have to be re-synced by the player after every seek);
	// several tracks become alternate renditions shared by all variants.
	var audio []AudioTrack
	if len(p.Audio) == 1 {
		for range vs {
			audio = append(audio, p.Audio[0])
		}
	} else {
		audio = p.Audio
	}
	for _, a := range audio {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", a.N))
	}

	// Encoded variants must cut segments where the copied one does, so the
	// player can switch quality cleanly: reuse the source's keyframes.
	keyframes := "expr:gte(t,n_forced*6)"
	if vs[0].copy {
		keyframes = "source"
	}
	for i, v := range vs {
		if v.copy {
			args = append(args, fmt.Sprintf("-c:v:%d", i), "copy")
			continue
		}
		args = append(args, enc.videoArgs(i, v, keyframes)...)
	}
	for i, a := range audio {
		if a.Copy {
			args = append(args, fmt.Sprintf("-c:a:%d", i), "copy")
		} else {
			args = append(args, fmt.Sprintf("-c:a:%d", i), "aac",
				fmt.Sprintf("-b:a:%d", i), "192k", fmt.Sprintf("-ac:a:%d", i), "2")
		}
	}

	var vsm []string
	for i, v := range vs {
		switch len(p.Audio) {
		case 0:
			vsm = append(vsm, fmt.Sprintf("v:%d,name:%s", i, v.name))
		case 1:
			vsm = append(vsm, fmt.Sprintf("v:%d,a:%d,name:%s", i, i, v.name))
		default:
			vsm = append(vsm, fmt.Sprintf("v:%d,agroup:aud,name:%s", i, v.name))
		}
	}
	if len(p.Audio) > 1 {
		for i, a := range p.Audio {
			e := fmt.Sprintf("a:%d,agroup:aud,language:%s,name:%s", i, a.Lang, a.Name)
			if i == 0 {
				e += ",default:yes"
			}
			vsm = append(vsm, e)
		}
	}
	// ffmpeg only expands %v in the init filename when there are several
	// variants; a single variant needs the literal name.
	initName := "init_%v.mp4"
	if len(vsm) == 1 {
		initName = "init_" + vs[0].name + ".mp4"
	}

	args = append(args,
		"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4", "-hls_flags", "independent_segments",
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", strings.Join(vsm, " "),
		"-hls_fmp4_init_filename", initName,
		"-hls_segment_filename", filepath.Join(out, "seg_%v_%05d.m4s"),
		filepath.Join(out, "stream_%v.m3u8"),
	)
	return args
}

func Poster(ctx context.Context, src, dst string, duration float64) error {
	return run(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.0f", duration*0.1), "-i", src,
		"-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "3", dst)
}
