package transcode

import (
	"context"
	"fmt"
	"log"
	"strconv"
)

// Encoder picks the H.264 encoder for re-encoded video variants.
type Encoder string

const (
	Software Encoder = "none"  // libx264
	NVENC    Encoder = "nvenc" // h264_nvenc on an NVIDIA GPU
)

// ParseEncoder maps HW_ENCODER to an Encoder; unknown values mean software.
func ParseEncoder(s string) Encoder {
	if Encoder(s) == NVENC {
		return NVENC
	}
	if s != "" && Encoder(s) != Software {
		log.Printf("HW_ENCODER=%q is not supported (use nvenc or none); encoding in software", s)
	}
	return Software
}

// Detect checks that enc works here (the ffmpeg build has it and the GPU is
// reachable) with a tiny test encode, falling back to Software.
func Detect(ctx context.Context, enc Encoder) Encoder {
	if enc == Software {
		return Software
	}
	err := run(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=black:s=256x144:d=0.2",
		"-c:v", "h264_nvenc", "-f", "null", "-")
	if err != nil {
		log.Printf("hardware encoder %s unavailable, encoding in software: %v", enc, err)
		return Software
	}
	log.Printf("hardware encoder: %s", enc)
	return enc
}

// videoArgs returns the codec options for re-encoded variant i. Scaling
// stays on the CPU (in the filter), so any source, 10-bit included, works.
func (e Encoder) videoArgs(i int, v variant, keyframes string) []string {
	o := func(name string) string { return fmt.Sprintf("-%s:v:%d", name, i) }
	var args []string
	filter := "format=yuv420p"
	if v.height > 0 {
		filter = fmt.Sprintf("scale=-2:%d,%s", v.height, filter)
	}
	args = append(args, o("filter"), filter)
	if v.maxRateK > 0 {
		args = append(args,
			o("maxrate"), fmt.Sprintf("%dk", v.maxRateK),
			o("bufsize"), fmt.Sprintf("%dk", 2*v.maxRateK))
	}
	// x264 CRF; NVENC's CQ needs about +4 for similar sizes (measured on a
	// 1080p test clip: CQ 24 ≈ CRF 20)
	quality := 20
	if v.maxRateK > 0 {
		quality = 23
	}
	switch e {
	case NVENC:
		args = append(args,
			o("c"), "h264_nvenc",
			o("preset"), "p5",
			o("tune"), "hq",
			o("rc"), "vbr",
			o("cq"), strconv.Itoa(quality+4),
			o("b"), "0",
			o("profile"), "high",
			// forced keyframes must be IDR frames so every segment starts clean
			o("forced-idr"), "1",
			o("no-scenecut"), "1")
	default:
		args = append(args,
			o("c"), "libx264",
			o("preset"), "veryfast",
			o("crf"), strconv.Itoa(quality),
			o("profile"), "high",
			o("sc_threshold"), "0")
	}
	return append(args, o("force_key_frames"), keyframes)
}
