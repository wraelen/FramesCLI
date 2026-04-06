package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChooseRecommended(t *testing.T) {
	results := []BenchmarkResult{
		{Case: BenchmarkCase{HWAccel: "none", Preset: "balanced"}, Success: true, RealtimeFactor: 1.0},
		{Case: BenchmarkCase{HWAccel: "cuda", Preset: "balanced"}, Success: true, RealtimeFactor: 2.5},
		{Case: BenchmarkCase{HWAccel: "auto", Preset: "balanced"}, Success: false},
	}
	got, ok := chooseRecommended(results)
	if !ok {
		t.Fatalf("expected recommendation")
	}
	if got.Case.HWAccel != "cuda" {
		t.Fatalf("expected cuda recommendation, got %+v", got.Case)
	}
}

func TestBenchmarkHistoryReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "benchmark-history.json")
	report := BenchmarkReport{
		VideoPath:   "video.mp4",
		DurationSec: 10,
		Results: []BenchmarkResult{
			{Case: BenchmarkCase{HWAccel: "none", Preset: "balanced"}, Success: true, RealtimeFactor: 1.2},
		},
		Recommended: BenchmarkCase{HWAccel: "none", Preset: "balanced"},
	}
	if err := AppendBenchmarkHistory(path, report, 100); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadBenchmarkHistory(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	if entries[0].Recommended.HWAccel != "none" {
		t.Fatalf("unexpected recommended value: %+v", entries[0].Recommended)
	}
}

func TestPruneBenchmarkHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "benchmark-history.json")
	base := BenchmarkReport{
		VideoPath:   "video.mp4",
		DurationSec: 10,
		Results: []BenchmarkResult{
			{Case: BenchmarkCase{HWAccel: "none", Preset: "balanced"}, Success: true, RealtimeFactor: 1.1},
		},
		Recommended: BenchmarkCase{HWAccel: "none", Preset: "balanced"},
	}
	_ = AppendBenchmarkHistory(path, base, 100)
	_ = AppendBenchmarkHistory(path, base, 100)
	_ = AppendBenchmarkHistory(path, base, 100)
	removed, err := PruneBenchmarkHistory(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("expected removed=1 got %d", removed)
	}
	entries, err := ReadBenchmarkHistory(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected len=2 got %d", len(entries))
	}
}

func TestExportBenchmarkHistoryCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.csv")
	entries := []BenchmarkHistoryEntry{
		{
			RecordedAt:  "2026-01-01T00:00:00Z",
			VideoPath:   "v.mp4",
			DurationSec: 10,
			Results: []BenchmarkResult{
				{Success: true, RealtimeFactor: 2.0},
			},
			Recommended: BenchmarkCase{HWAccel: "none", Preset: "balanced"},
		},
	}
	if err := ExportBenchmarkHistoryCSV(path, entries); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected csv output: %v", err)
	}
}
