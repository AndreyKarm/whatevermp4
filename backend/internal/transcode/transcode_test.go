package transcode

import (
	"slices"
	"strings"
	"testing"
)

func probeOf(streams ...Stream) *ProbeResult {
	p := &ProbeResult{Streams: streams}
	p.Format.Duration = "1450.5"
	return p
}

func TestBuildPlan(t *testing.T) {
	p, err := BuildPlan(probeOf(
		Stream{Index: 0, CodecType: "video", CodecName: "mjpeg", Disposition: map[string]int{"attached_pic": 1}},
		Stream{Index: 1, CodecType: "video", CodecName: "h264", PixFmt: "yuv420p", Width: 1920, Height: 1080},
		Stream{Index: 2, CodecType: "audio", CodecName: "aac", Channels: 2, Tags: map[string]string{"language": "JPN"}},
		Stream{Index: 3, CodecType: "audio", CodecName: "eac3", Channels: 6},
		Stream{Index: 4, CodecType: "subtitle", CodecName: "ass", Tags: map[string]string{"language": "eng"}},
		Stream{Index: 5, CodecType: "subtitle", CodecName: "hdmv_pgs_subtitle"},
		Stream{Index: 6, CodecType: "subtitle", CodecName: "subrip"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if p.VideoIndex != 1 || !p.VideoCopy || p.VideoWidth != 1920 || p.VideoHeight != 1080 || p.Duration != 1450.5 {
		t.Fatalf("video: %+v", p)
	}
	if len(p.Audio) != 2 || !p.Audio[0].Copy || p.Audio[1].Copy || p.Audio[0].Name != "jpn_0" || p.Audio[1].Lang != "und" {
		t.Fatalf("audio: %+v", p.Audio)
	}
	// bitmap subtitles are skipped; ordinals still count them
	if len(p.Subs) != 2 || !p.Subs[0].IsASS || p.Subs[0].ASS == "" || p.Subs[1].N != 2 {
		t.Fatalf("subs: %+v", p.Subs)
	}

	p, _ = BuildPlan(probeOf(Stream{Index: 0, CodecType: "video", CodecName: "h264", PixFmt: "yuv420p10le"}))
	if p.VideoCopy {
		t.Fatal("10-bit H.264 must be re-encoded")
	}
	if _, err := BuildPlan(probeOf(Stream{CodecType: "audio"})); err == nil {
		t.Fatal("no video should fail")
	}
}

func TestBuildPlanRotation(t *testing.T) {
	phone := Stream{CodecType: "video", CodecName: "h264", PixFmt: "yuv420p", Width: 1920, Height: 1080}
	for rotation, portrait := range map[float64]bool{0: false, 180: false, -180: false, 90: true, -90: true, 270: true} {
		s := phone
		s.SideData = []struct {
			Rotation float64 `json:"rotation"`
		}{{}, {Rotation: rotation}} // other side data has no rotation
		p, err := BuildPlan(probeOf(s))
		if err != nil {
			t.Fatal(err)
		}
		if got := p.VideoHeight > p.VideoWidth; got != portrait {
			t.Fatalf("rotation %v: %dx%d", rotation, p.VideoWidth, p.VideoHeight)
		}
	}
}

func TestQualityHeight(t *testing.T) {
	for _, c := range []struct{ w, h, want int }{
		{1920, 1080, 1080}, {1280, 720, 720}, {854, 480, 480}, {640, 360, 360},
		{1280, 546, 720}, {1920, 800, 1080}, {3840, 1600, 2160}, {640, 272, 360}, // 21:9
		{1904, 800, 1080}, {1920, 1088, 1080}, {4096, 2160, 2160}, {2048, 858, 1080}, // near a tier
		{720, 1280, 720}, {1080, 1920, 1080}, // portrait
		{1440, 1080, 1080}, {640, 480, 480}, {720, 576, 576}, // 4:3, PAL
		{960, 540, 540}, {1600, 900, 900}, // off the list: keep the real size
	} {
		if got := qualityHeight(c.w, c.h); got != c.want {
			t.Fatalf("qualityHeight(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestVariants(t *testing.T) {
	names := func(vs []variant) (out []string) {
		for _, v := range vs {
			out = append(out, v.name)
		}
		return
	}
	for _, c := range []struct {
		w, h int
		want []string
	}{
		{3840, 2160, []string{"2160p", "1080p", "720p"}},
		{1280, 720, []string{"720p", "480p", "360p"}},
		{1280, 546, []string{"720p", "480p", "360p"}},   // 21:9: not 546p
		{1920, 800, []string{"1080p", "720p", "480p"}},  // 21:9
		{1920, 1088, []string{"1080p", "720p", "480p"}}, // no second 1080p
		{1080, 1920, []string{"1080p", "720p", "480p"}}, // portrait: not 1920p
		{0, 0, []string{"src"}},
	} {
		if got := names(variants(&Plan{VideoWidth: c.w, VideoHeight: c.h})); !slices.Equal(got, c.want) {
			t.Fatalf("%dx%d: %v", c.w, c.h, got)
		}
	}

	// rungs are fitted into a box of the frame's own orientation
	if v := variants(&Plan{VideoWidth: 1080, VideoHeight: 1920})[1]; v.width != 720 || v.height != 1280 {
		t.Fatalf("portrait rung: %+v", v)
	}
	if v := variants(&Plan{VideoWidth: 1920, VideoHeight: 800})[1]; v.width != 1280 || v.height != 720 {
		t.Fatalf("21:9 rung: %+v", v)
	}
}

func TestEncoderArgs(t *testing.T) {
	v := variant{name: "720p", width: 1280, height: 720, maxRateK: 2800}
	sw := strings.Join(Software.videoArgs(1, v, "source"), " ")
	for _, want := range []string{"-c:v:1 libx264", "-crf:v:1 23", "scale=1280:720:force_original_aspect_ratio=decrease", "-force_key_frames:v:1 source", "-maxrate:v:1 2800k"} {
		if !strings.Contains(sw, want) {
			t.Fatalf("software args lack %q: %s", want, sw)
		}
	}
	hw := strings.Join(NVENC.videoArgs(0, variant{name: "1080p"}, "expr:gte(t,n_forced*6)"), " ")
	for _, want := range []string{"-c:v:0 h264_nvenc", "-cq:v:0 24", "-forced-idr:v:0 1", "crop=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p", "-force_key_frames:v:0 expr:"} {
		if !strings.Contains(hw, want) {
			t.Fatalf("nvenc args lack %q: %s", want, hw)
		}
	}
	if strings.Contains(hw, "libx264") || strings.Contains(hw, "maxrate") {
		t.Fatalf("nvenc source variant: %s", hw)
	}
}

func TestHLSArgsSharedAudio(t *testing.T) {
	p := &Plan{VideoIndex: 0, VideoWidth: 1920, VideoHeight: 1080, VideoCopy: true, Duration: 60,
		Audio: []AudioTrack{{N: 0, Lang: "eng", Name: "eng_0", Copy: true}, {N: 1, Lang: "jpn", Name: "jpn_1"}}}
	args := strings.Join(hlsArgs("in.mkv", "out", p, NVENC), " ")
	if !strings.Contains(args, "-c:v:0 copy") || !strings.Contains(args, "-c:v:1 h264_nvenc") {
		t.Fatalf("copy + encoded variants: %s", args)
	}
	if !strings.Contains(args, "agroup:aud,language:eng,name:eng_0,default:yes") {
		t.Fatalf("alternate audio renditions: %s", args)
	}
	if !strings.Contains(args, "-force_key_frames:v:1 source") {
		t.Fatalf("encoded variants must follow the copied keyframes: %s", args)
	}
}

func TestParseProgress(t *testing.T) {
	if f, ok := parseProgress("out_time_us=30000000", 120); !ok || f != 0.25 {
		t.Fatalf("got %v %v", f, ok)
	}
	if f, ok := parseProgress("out_time_us=999000000", 120); !ok || f != 1 {
		t.Fatalf("not capped: %v", f)
	}
	for _, line := range []string{"out_time_us=N/A", "frame=12", "out_time_ms=5000000"} {
		if _, ok := parseProgress(line, 120); ok {
			t.Fatalf("%q should be ignored", line)
		}
	}
}

func TestThumbInterval(t *testing.T) {
	for _, c := range []struct{ dur, want float64 }{{60, 2}, {1200, 4}, {7200, 10}, {30000, 10}} {
		if got := thumbInterval(c.dur); got != c.want {
			t.Fatalf("thumbInterval(%v) = %v, want %v", c.dur, got, c.want)
		}
	}
}
