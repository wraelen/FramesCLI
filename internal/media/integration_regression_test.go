//go:build integration

package media

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationProbeFixtureMatrix(t *testing.T) {
	requireFFmpegTools(t)

	cases := []struct {
		name        string
		makeVideo   func(*testing.T) string
		wantWidth   int
		wantHeight  int
		wantAudio   bool
		minDuration float64
	}{
		{name: "sample-h264-aac", makeVideo: makeSampleVideo, wantWidth: 320, wantHeight: 240, wantAudio: true, minDuration: 1.9},
		{name: "variable-frame-rate", makeVideo: makeVFRVideo, wantWidth: 320, wantHeight: 240, wantAudio: true, minDuration: 1.2},
		{name: "no-audio", makeVideo: makeNoAudioVideo, wantWidth: 320, wantHeight: 240, wantAudio: false, minDuration: 1.9},
		{name: "odd-codec", makeVideo: makeOddCodecVideo, wantWidth: 352, wantHeight: 288, wantAudio: true, minDuration: 1.9},
		{name: "high-resolution", makeVideo: makeHighResolutionVideo, wantWidth: 2560, wantHeight: 1440, wantAudio: true, minDuration: 1.4},
		{name: "long-duration", makeVideo: makeLongDurationVideo, wantWidth: 160, wantHeight: 90, wantAudio: true, minDuration: 60},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			video := tc.makeVideo(t)
			info, err := ProbeVideoInfo(video)
			if err != nil {
				t.Fatalf("probe failed: %v", err)
			}
			if info.Width != tc.wantWidth || info.Height != tc.wantHeight {
				t.Fatalf("unexpected resolution: got %dx%d want %dx%d", info.Width, info.Height, tc.wantWidth, tc.wantHeight)
			}
			if info.HasAudio != tc.wantAudio {
				t.Fatalf("unexpected audio flag: got %v want %v", info.HasAudio, tc.wantAudio)
			}
			if info.DurationSec < tc.minDuration {
				t.Fatalf("unexpected duration: got %.2fs want >= %.2fs", info.DurationSec, tc.minDuration)
			}
			if info.FrameRate <= 0 {
				t.Fatalf("expected positive frame rate, got %.4f", info.FrameRate)
			}
		})
	}
}

func TestIntegrationExtractNoAudioFailsClearly(t *testing.T) {
	requireFFmpegTools(t)
	video := makeNoAudioVideo(t)
	outDir := filepath.Join(t.TempDir(), "run")

	_, err := ExtractMedia(ExtractMediaOptions{
		VideoPath:    video,
		FPS:          2,
		OutDir:       outDir,
		FrameFormat:  "png",
		Preset:       "safe",
		HWAccel:      "none",
		ExtractAudio: true,
	})
	if err == nil {
		t.Fatal("expected extract to fail for no-audio input when audio was requested")
	}
	assertNoAudioMessage(t, err, video)
}

func TestIntegrationExtractAudioNoAudioFailsClearly(t *testing.T) {
	requireFFmpegTools(t)
	video := makeNoAudioVideo(t)
	outDir := filepath.Join(t.TempDir(), "voice")

	_, err := ExtractAudioFromVideoWithOptions(ExtractAudioOptions{
		VideoPath: video,
		OutDir:    outDir,
		Format:    "wav",
	})
	if err == nil {
		t.Fatal("expected audio extraction to fail for no-audio input")
	}
	assertNoAudioMessage(t, err, video)
}

func TestIntegrationExtractRegressionFixtures(t *testing.T) {
	requireFFmpegTools(t)

	cases := []struct {
		name       string
		makeVideo  func(*testing.T) string
		fps        float64
		minFrames  int
		wantWidth  int
		wantHeight int
	}{
		{name: "variable-frame-rate", makeVideo: makeVFRVideo, fps: 3, minFrames: 2, wantWidth: 320, wantHeight: 240},
		{name: "odd-codec", makeVideo: makeOddCodecVideo, fps: 2, minFrames: 3, wantWidth: 352, wantHeight: 288},
		{name: "high-resolution", makeVideo: makeHighResolutionVideo, fps: 2, minFrames: 2, wantWidth: 2560, wantHeight: 1440},
		{name: "long-duration", makeVideo: makeLongDurationVideo, fps: 0.2, minFrames: 10, wantWidth: 160, wantHeight: 90},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			video := tc.makeVideo(t)
			outDir := filepath.Join(t.TempDir(), "run")
			res, err := ExtractMedia(ExtractMediaOptions{
				VideoPath:   video,
				FPS:         tc.fps,
				OutDir:      outDir,
				FrameFormat: "png",
				Preset:      "safe",
				HWAccel:     "none",
			})
			if err != nil {
				t.Fatalf("extract failed: %v", err)
			}
			count, err := CountFrames(res.ImagesDir)
			if err != nil {
				t.Fatalf("count frames failed: %v", err)
			}
			if count < tc.minFrames {
				t.Fatalf("expected at least %d frames, got %d", tc.minFrames, count)
			}
			md, err := ReadRunMetadata(res.OutDir)
			if err != nil {
				t.Fatalf("read metadata failed: %v", err)
			}
			if md.Width != tc.wantWidth || md.Height != tc.wantHeight {
				t.Fatalf("unexpected metadata resolution: got %dx%d want %dx%d", md.Width, md.Height, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

func TestIntegrationCorruptedInputErrorsIncludeContext(t *testing.T) {
	requireFFmpegTools(t)
	video := makeCorruptedVideo(t)

	_, err := ProbeVideoInfo(video)
	if err == nil {
		t.Fatal("expected probe to fail for corrupted input")
	}

	_, err = ExtractMedia(ExtractMediaOptions{
		VideoPath:   video,
		FPS:         1,
		OutDir:      filepath.Join(t.TempDir(), "run"),
		FrameFormat: "png",
		Preset:      "safe",
		HWAccel:     "none",
	})
	if err == nil {
		t.Fatal("expected extract to fail for corrupted input")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ffprobe failed for") {
		t.Fatalf("expected ffprobe context, got %q", msg)
	}
	if !strings.Contains(msg, filepath.Base(video)) {
		t.Fatalf("expected message to include fixture path, got %q", msg)
	}
}

func assertNoAudioMessage(t *testing.T, err error, video string) {
	t.Helper()
	msg := err.Error()
	if !strings.Contains(msg, "requested audio output, but video has no audio stream") {
		t.Fatalf("expected stable no-audio message, got %q", msg)
	}
	if !strings.Contains(msg, filepath.Base(video)) {
		t.Fatalf("expected message to include fixture path, got %q", msg)
	}
}
