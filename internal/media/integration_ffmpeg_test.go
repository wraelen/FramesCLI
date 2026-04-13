//go:build integration

package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntegrationExtractAndSheet(t *testing.T) {
	requireFFmpegTools(t)
	video := makeSampleVideo(t)
	outDir := filepath.Join(t.TempDir(), "run")

	res, err := ExtractMedia(ExtractMediaOptions{
		VideoPath:   video,
		FPS:         2,
		OutDir:      outDir,
		FrameFormat: "png",
		Preset:      "safe",
		HWAccel:     "none",
	})
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if res.ImagesDir == "" {
		t.Fatalf("images dir missing")
	}
	count, err := CountFrames(res.ImagesDir)
	if err != nil {
		t.Fatalf("count frames failed: %v", err)
	}
	if count < 2 {
		t.Fatalf("expected at least 2 frames, got %d", count)
	}
	if _, err := os.Stat(res.MetadataPath); err != nil {
		t.Fatalf("metadata missing: %v", err)
	}
	sheet, err := CreateContactSheet(res.ImagesDir, 3, filepath.Join(outDir, "images", "sheets", "sheet.png"), false)
	if err != nil {
		t.Fatalf("contact sheet failed: %v", err)
	}
	if _, err := os.Stat(sheet); err != nil {
		t.Fatalf("sheet missing: %v", err)
	}
}

func TestIntegrationBenchmarkExtraction(t *testing.T) {
	requireFFmpegTools(t)
	video := makeSampleVideo(t)
	report, err := BenchmarkExtraction(BenchmarkOptions{
		VideoPath:   video,
		DurationSec: 1,
		Cases: []BenchmarkCase{
			{HWAccel: "none", Preset: "safe"},
		},
	})
	if err != nil {
		t.Fatalf("benchmark failed: %v", err)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if !report.Results[0].Success {
		t.Fatalf("expected benchmark success, got error: %s", report.Results[0].Error)
	}
}
