package transcode

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// runProgress runs ffmpeg like run, reporting the fraction of total
// (seconds of output) written so far. report may be nil; total <= 0 means
// the length is unknown and nothing is reported.
func runProgress(ctx context.Context, total float64, report func(float64), args ...string) error {
	if report == nil || total <= 0 {
		return run(ctx, "ffmpeg", args...)
	}
	args = append([]string{"-progress", "pipe:1", "-nostats"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		if f, ok := parseProgress(sc.Text(), total); ok {
			report(f)
		}
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, tail(stderr.String(), 1500))
	}
	return nil
}

// parseProgress reads an "out_time_us=…" line of ffmpeg's -progress output.
// (out_time_ms is microseconds too, an old ffmpeg naming bug; only _us is used.)
func parseProgress(line string, total float64) (float64, bool) {
	v, ok := strings.CutPrefix(line, "out_time_us=")
	if !ok {
		return 0, false
	}
	us, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || us < 0 {
		return 0, false
	}
	return min(float64(us)/1e6/total, 1), true
}
