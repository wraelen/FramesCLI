package media

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildRunsIndexIncludesArtifactsAndManifestMetadata(t *testing.T) {
	root := t.TempDir()
	run := createIndexedRunFixture(t, root, "Run_1", indexedRunFixtureOptions{
		withTranscript: true,
		withAudio:      true,
		withSheet:      true,
		withManifest:   true,
	})

	idx, err := BuildRunsIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if idx.SchemaVersion == "" {
		t.Fatalf("expected schema version")
	}
	if idx.RunCount != 1 {
		t.Fatalf("expected 1 run, got %d", idx.RunCount)
	}
	got := idx.Runs[0]
	if got.Artifacts.Run != run {
		t.Fatalf("expected run artifact %q, got %q", run, got.Artifacts.Run)
	}
	if got.Artifacts.TranscriptTxt == "" || got.Artifacts.ContactSheet == "" || got.Artifacts.TranscriptionManifest == "" {
		t.Fatalf("expected indexed artifacts, got %+v", got.Artifacts)
	}
	if !got.ManifestComplete || !got.ChunkedTranscript {
		t.Fatalf("expected completed chunk metadata, got %+v", got)
	}
	if got.TotalChunks != 1 || got.CompletedChunks != 1 {
		t.Fatalf("unexpected chunk counts: %+v", got)
	}
	if got.SourceVideoPath != "/tmp/source.mp4" || got.Format != "png" || got.Preset != "balanced" {
		t.Fatalf("expected run metadata to be indexed, got %+v", got)
	}
	if got.Status != "complete" {
		t.Fatalf("expected complete status, got %q", got.Status)
	}
}

func TestBuildRunsIndexMarksPartialMissingArtifacts(t *testing.T) {
	root := t.TempDir()
	createIndexedRunFixture(t, root, "Run_1", indexedRunFixtureOptions{
		withAudio: true,
	})

	idx, err := BuildRunsIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	got := idx.Runs[0]
	if got.Status != "partial" {
		t.Fatalf("expected partial status, got %q", got.Status)
	}
	if len(got.Warnings) == 0 {
		t.Fatalf("expected warnings for missing artifacts, got %+v", got)
	}
	if got.Artifacts.TranscriptTxt != "" {
		t.Fatalf("expected no transcript artifact, got %+v", got.Artifacts)
	}
}

func TestRecentRunsAndSelectRun(t *testing.T) {
	root := t.TempDir()
	runOlder := createIndexedRunFixture(t, root, "Run_old", indexedRunFixtureOptions{createdAt: "2026-01-01T00:00:00Z"})
	createIndexedRunFixture(t, root, "Run_new", indexedRunFixtureOptions{createdAt: "2026-01-02T00:00:00Z", withTranscript: true})

	if _, err := WriteRunsIndex(root, ""); err != nil {
		t.Fatal(err)
	}

	runs, err := RecentRuns(root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Name != "Run_new" {
		t.Fatalf("expected newest run, got %+v", runs)
	}

	got, err := SelectRun(root, runOlder)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Run_old" {
		t.Fatalf("expected Run_old, got %+v", got)
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
		SchemaVersion: runsIndexSchemaVersion,
		Root:          root,
		GeneratedAt:   "2026-01-01T00:00:00Z",
		RunCount:      1,
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

type indexedRunFixtureOptions struct {
	createdAt      string
	withAudio      bool
	withTranscript bool
	withSheet      bool
	withManifest   bool
}

func createIndexedRunFixture(t *testing.T, root string, name string, opts indexedRunFixtureOptions) string {
	t.Helper()

	run := filepath.Join(root, name)
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
	if opts.withSheet {
		if err := os.WriteFile(filepath.Join(images, "sheets", "contact-sheet.png"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.withAudio {
		if err := os.WriteFile(filepath.Join(voice, "voice.wav"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.withTranscript {
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
	}
	if opts.withManifest {
		manifest := TranscriptionManifest{
			SchemaVersion:    "framescli.transcription_manifest.v1",
			SourceAudioPath:  filepath.Join(voice, "voice.wav"),
			ChunkDurationSec: 300,
			Chunks: []TranscriptionManifestChunk{
				{
					Index:          0,
					Status:         transcriptionChunkCompleted,
					OutputDir:      filepath.Join(voice, "chunks", "chunk-0000"),
					TranscriptTxt:  filepath.Join(voice, "chunks", "chunk-0000", "transcript.txt"),
					TranscriptJSON: filepath.Join(voice, "chunks", "chunk-0000", "transcript.json"),
				},
			},
			Merge: TranscriptionManifestMerge{
				Status:         transcriptionMergeCompleted,
				TranscriptTxt:  filepath.Join(voice, "transcript.txt"),
				TranscriptJSON: filepath.Join(voice, "transcript.json"),
			},
		}
		if err := os.MkdirAll(manifest.Chunks[0].OutputDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest.Chunks[0].TranscriptTxt, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest.Chunks[0].TranscriptJSON, []byte(`{"text":"hello"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(voice, "transcription-manifest.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	createdAt := opts.createdAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	md := RunMetadata{
		VideoPath:      "/tmp/source.mp4",
		FPS:            4,
		FrameFormat:    "png",
		ExtractedAudio: opts.withAudio,
		AudioPath:      filepath.Join(voice, "voice.wav"),
		Preset:         "balanced",
		DurationSec:    42,
		CreatedAt:      createdAt,
	}
	raw, err := json.Marshal(md)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(MetadataPathForRun(run), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return run
}
