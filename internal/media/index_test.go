package media

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRunsIndex(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "Run_1")
	images := filepath.Join(run, "images")
	voice := filepath.Join(run, "voice")
	if err := os.MkdirAll(filepath.Join(images, "sheets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(voice, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(images, "frame-0001.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(images, "sheets", "contact-sheet.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voice, "voice.wav"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voice, "transcript.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	payload := WhisperJSON{Segments: []WhisperSegment{{Start: 0, End: 1, Text: "hello"}}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voice, "transcript.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildRunsIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if idx.RunCount != 1 {
		t.Fatalf("expected 1 run, got %d", idx.RunCount)
	}
	if idx.Runs[0].SegmentCount != 1 {
		t.Fatalf("expected segment count 1, got %d", idx.Runs[0].SegmentCount)
	}
	if !idx.Runs[0].HasContactSheet || !idx.Runs[0].HasTranscript {
		t.Fatalf("expected sheet+transcript true")
	}
}

func TestWriteRunsIndex(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "custom-index.json")
	_, err := WriteRunsIndex(root, out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected index file to exist: %v", err)
	}
}

func TestReadRunsIndex(t *testing.T) {
	root := t.TempDir()
	path := IndexFilePath(root)
	payload := RunsIndex{
		Root:        root,
		GeneratedAt: "2026-01-01T00:00:00Z",
		RunCount:    1,
		Runs: []IndexedRun{
			{Name: "A", Path: filepath.Join(root, "A"), Frames: 1, FrameExt: "png"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRunsIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunCount != 1 || len(got.Runs) != 1 || got.Runs[0].Name != "A" {
		t.Fatalf("unexpected index: %+v", got)
	}
}
