package transcode

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExtractSubs returns the tracks that were extracted successfully.
func ExtractSubs(ctx context.Context, src, out string, subs []SubTrack) []SubTrack {
	_ = os.MkdirAll(filepath.Join(out, "subs"), 0o755)
	var ok []SubTrack
	for _, s := range subs {
		m := fmt.Sprintf("0:s:%d", s.N)
		err := run(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
			"-i", src, "-map", m, "-c:s", "webvtt", filepath.Join(out, s.VTT))
		if err == nil && s.IsASS {
			err = run(ctx, "ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
				"-i", src, "-map", m, "-c:s", "copy", filepath.Join(out, s.ASS))
		}
		if err != nil {
			log.Printf("subtitle %d failed: %v", s.N, err)
			continue
		}
		ok = append(ok, s)
	}
	return ok
}

// ExtractFonts dumps MKV attachments (fonts for ASS) to out/fonts and
// returns relative paths.
func ExtractFonts(ctx context.Context, src, out string) []string {
	dir := filepath.Join(out, "fonts")
	_ = os.MkdirAll(dir, 0o755)
	abs, _ := filepath.Abs(src)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-hide_banner",
		"-loglevel", "error", "-dump_attachment:t", "", "-i", abs)
	cmd.Dir = dir
	_ = cmd.Run() // ffmpeg exits non-zero because there is no output file; expected

	var fonts []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".ttf", ".otf", ".ttc", ".woff", ".woff2":
			fonts = append(fonts, "fonts/"+e.Name())
		}
	}
	return fonts
}
