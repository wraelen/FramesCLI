//go:build integration

package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireFFmpegTools(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed: %v", bin, err)
		}
	}
}

func makeSampleVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.mp4")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=320x240:rate=10",
		"-f", "lavfi",
		"-i", "sine=frequency=1000:sample_rate=16000",
		"-t", "2",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-c:a", "aac",
		path,
	})
	return path
}

func makeNoAudioVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "no-audio.mp4")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=320x240:rate=12",
		"-t", "2",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-an",
		path,
	})
	return path
}

func makeHighResolutionVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "high-res.mp4")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "lavfi",
		"-i", "testsrc2=size=2560x1440:rate=2",
		"-f", "lavfi",
		"-i", "sine=frequency=523:sample_rate=16000",
		"-t", "1.5",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "35",
		"-c:a", "aac",
		"-b:a", "64k",
		path,
	})
	return path
}

func makeLongDurationVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "long-duration.mp4")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "lavfi",
		"-i", "color=c=0x223344:size=160x90:rate=1",
		"-f", "lavfi",
		"-i", "sine=frequency=330:sample_rate=16000",
		"-t", "65",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "38",
		"-c:a", "aac",
		"-b:a", "48k",
		path,
	})
	return path
}

func makeOddCodecVideo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "odd-codec.mkv")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=352x288:rate=12",
		"-f", "lavfi",
		"-i", "sine=frequency=660:sample_rate=16000",
		"-t", "2",
		"-c:v", "ffv1",
		"-level", "3",
		"-g", "1",
		"-c:a", "pcm_s16le",
		path,
	})
	return path
}

func makeVFRVideo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	pattern := filepath.Join(tmp, "vfr-%02d.png")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=size=320x240:rate=1",
		"-frames:v", "4",
		pattern,
	})

	listPath := filepath.Join(tmp, "vfr.ffconcat")
	frames := []string{
		filepath.Join(tmp, "vfr-01.png"),
		filepath.Join(tmp, "vfr-02.png"),
		filepath.Join(tmp, "vfr-03.png"),
		filepath.Join(tmp, "vfr-04.png"),
	}
	durations := []string{"0.10", "0.35", "0.15", "0.80"}
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	for i, frame := range frames {
		b.WriteString(fmt.Sprintf("file '%s'\n", strings.ReplaceAll(frame, "'", "'\\''")))
		b.WriteString(fmt.Sprintf("duration %s\n", durations[i]))
	}
	b.WriteString(fmt.Sprintf("file '%s'\n", strings.ReplaceAll(frames[len(frames)-1], "'", "'\\''")))
	if err := os.WriteFile(listPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("failed writing VFR concat list: %v", err)
	}

	path := filepath.Join(tmp, "vfr.mp4")
	runFFmpegFixtureCommand(t, []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-f", "lavfi",
		"-i", "sine=frequency=880:sample_rate=16000:duration=1.4",
		"-vsync", "vfr",
		"-pix_fmt", "yuv420p",
		"-c:v", "libx264",
		"-c:a", "aac",
		"-shortest",
		path,
	})
	return path
}

func makeCorruptedVideo(t *testing.T) string {
	t.Helper()
	valid := makeSampleVideo(t)
	raw, err := os.ReadFile(valid)
	if err != nil {
		t.Fatalf("failed reading source fixture: %v", err)
	}
	if len(raw) < 128 {
		t.Fatalf("source fixture too small to corrupt: %d bytes", len(raw))
	}
	path := filepath.Join(t.TempDir(), "corrupted.mp4")
	cut := len(raw) / 5
	if err := os.WriteFile(path, raw[:cut], 0o644); err != nil {
		t.Fatalf("failed writing corrupted fixture: %v", err)
	}
	return path
}

func runFFmpegFixtureCommand(t *testing.T, args []string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed generating fixture with ffmpeg %q: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
