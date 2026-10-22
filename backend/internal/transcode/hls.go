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

// RunHLS writes out/master.m3u8, out/stream_{name}.m3u8, out/init_{name}.mp4, out/seg_{name}_*.m4s
func RunHLS(ctx context.Context, src, out string, p *Plan) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", src,
		"-map", fmt.Sprintf("0:%d", p.VideoIndex)}
	for _, a := range p.Audio {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", a.N))
	}

	if p.VideoCopy {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
			"-pix_fmt", "yuv420p", "-profile:v", "high", "-sc_threshold", "0",
			"-force_key_frames", "expr:gte(t,n_forced*6)")
	}
	for i, a := range p.Audio {
		if a.Copy {
			args = append(args, fmt.Sprintf("-c:a:%d", i), "copy")
		} else {
			args = append(args, fmt.Sprintf("-c:a:%d", i), "aac",
				fmt.Sprintf("-b:a:%d", i), "192k", fmt.Sprintf("-ac:a:%d", i), "2")
		}
	}

	var vsm []string
	// ffmpeg only expands %v in the init filename when there are several
	// variants; a single variant needs the literal name.
	initName := "init_%v.mp4"
	switch len(p.Audio) {
	case 0:
		vsm = append(vsm, "v:0,name:video")
		initName = "init_video.mp4"
	case 1:
		// Mux the only audio track into the video segments: separate audio
		// renditions have to be re-synced by the player after every seek.
		vsm = append(vsm, "v:0,a:0,name:main")
		initName = "init_main.mp4"
	default:
		vsm = append(vsm, "v:0,agroup:aud,name:video")
		for i, a := range p.Audio {
			e := fmt.Sprintf("a:%d,agroup:aud,language:%s,name:%s", i, a.Lang, a.Name)
			if i == 0 {
				e += ",default:yes"
			}
			vsm = append(vsm, e)
		}
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
	return run(ctx, "ffmpeg", args...)
}

func Poster(ctx context.Context, src, dst string, duration float64) error {
	return run(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.0f", duration*0.1), "-i", src,
		"-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "3", dst)
}
