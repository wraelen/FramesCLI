package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestChunkedTranscriptionResumeSkipsCompletedChunks(t *testing.T) {
	tmp := t.TempDir()
	audioPath := writeTestAudioFile(t, tmp)
	outDir := filepath.Join(tmp, "voice")
	callCounts := map[int]int{}

	restore := stubChunkedTranscriptionHooks(t, 25, func(opts TranscribeOptions) (string, string, error) {
		index := mustChunkIndex(t, opts.OutDir)
		callCounts[index]++
		writeChunkTranscriptFiles(t, opts.OutDir, fmt.Sprintf("chunk-%d", index), 0)
		if index == 1 && callCounts[index] == 1 {
			return "", "", errors.New("forced chunk failure")
		}
		return filepath.Join(opts.OutDir, "transcript.txt"), filepath.Join(opts.OutDir, "transcript.json"), nil
	})
	defer restore()
	_, err := TranscribeAudioWithDetails(TranscribeOptions{
		AudioPath:        audioPath,
		OutDir:           outDir,
		ChunkDurationSec: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "manifest:") {
		t.Fatalf("expected manifest-wrapped failure, got %v", err)
	}

	manifestPath := transcriptionManifestPath(outDir)
	manifest, err := loadTranscriptionManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest failed: %v", err)
	}
	if manifest.SourceAudioPath != audioPath {
		t.Fatalf("unexpected source audio path: %q", manifest.SourceAudioPath)
	}
	if manifest.ChunkDurationSec != 10 {
		t.Fatalf("unexpected chunk duration: %d", manifest.ChunkDurationSec)
	}
	if len(manifest.Chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(manifest.Chunks))
	}
	if manifest.Chunks[0].Status != transcriptionChunkCompleted {
		t.Fatalf("expected chunk 0 completed, got %q", manifest.Chunks[0].Status)
	}
	if manifest.Chunks[1].Status != transcriptionChunkFailed {
		t.Fatalf("expected chunk 1 failed, got %q", manifest.Chunks[1].Status)
	}
	if manifest.Chunks[2].Status != transcriptionChunkPending {
		t.Fatalf("expected chunk 2 pending, got %q", manifest.Chunks[2].Status)
	}

	result, err := TranscribeAudioWithDetails(TranscribeOptions{
		AudioPath: audioPath,
		OutDir:    outDir,
	})
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if !result.Chunked || result.TotalChunks != 3 || result.CompletedChunks != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if callCounts[0] != 1 {
		t.Fatalf("expected chunk 0 to run once, got %d", callCounts[0])
	}
	if callCounts[1] != 2 {
		t.Fatalf("expected chunk 1 to run twice, got %d", callCounts[1])
	}
	if callCounts[2] != 1 {
		t.Fatalf("expected chunk 2 to run once, got %d", callCounts[2])
	}

	gotTxt, err := os.ReadFile(result.TranscriptTxt)
	if err != nil {
		t.Fatalf("read merged txt failed: %v", err)
	}
	if string(gotTxt) != "chunk-0\nchunk-1\nchunk-2\n" {
		t.Fatalf("unexpected merged transcript txt: %q", string(gotTxt))
	}

	var payload WhisperJSON
	raw, err := os.ReadFile(result.TranscriptJSON)
	if err != nil {
		t.Fatalf("read merged json failed: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse merged json failed: %v", err)
	}
	if len(payload.Segments) != 3 {
		t.Fatalf("expected 3 merged segments, got %d", len(payload.Segments))
	}
	if payload.Segments[0].Start != 0 || payload.Segments[1].Start != 10 || payload.Segments[2].Start != 20 {
		t.Fatalf("unexpected merged segment offsets: %+v", payload.Segments)
	}
}

func TestChunkedTranscriptionResumesInProgressChunk(t *testing.T) {
	tmp := t.TempDir()
	audioPath := writeTestAudioFile(t, tmp)
	outDir := filepath.Join(tmp, "voice")
	chunkRoot := filepath.Join(outDir, "chunks")
	manifest := &TranscriptionManifest{
		SchemaVersion:    "framescli.transcription_manifest.v1",
		SourceAudioPath:  audioPath,
		ChunkDurationSec: 10,
		TotalDurationSec: 25,
		ChunkRootDir:     chunkRoot,
		Chunks:           buildTranscriptionManifestChunks(chunkRoot, 25, 10),
		Merge: TranscriptionManifestMerge{
			Status: transcriptionMergePending,
		},
	}
	writeChunkTranscriptFiles(t, manifest.Chunks[0].OutputDir, "chunk-0", 0)
	manifest.Chunks[0].Status = transcriptionChunkCompleted
	manifest.Chunks[0].TranscriptTxt = filepath.Join(manifest.Chunks[0].OutputDir, "transcript.txt")
	manifest.Chunks[0].TranscriptJSON = filepath.Join(manifest.Chunks[0].OutputDir, "transcript.json")
	manifest.Chunks[1].Status = transcriptionChunkInProgress
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := saveTranscriptionManifest(transcriptionManifestPath(outDir), manifest); err != nil {
		t.Fatalf("save manifest failed: %v", err)
	}

	callCounts := map[int]int{}
	restore := stubChunkedTranscriptionHooks(t, 25, func(opts TranscribeOptions) (string, string, error) {
		index := mustChunkIndex(t, opts.OutDir)
		callCounts[index]++
		writeChunkTranscriptFiles(t, opts.OutDir, fmt.Sprintf("chunk-%d", index), 0)
		return filepath.Join(opts.OutDir, "transcript.txt"), filepath.Join(opts.OutDir, "transcript.json"), nil
	})
	defer restore()

	result, err := TranscribeAudioWithDetails(TranscribeOptions{
		AudioPath: audioPath,
		OutDir:    outDir,
	})
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if callCounts[0] != 0 {
		t.Fatalf("expected completed chunk 0 to be skipped, got %d calls", callCounts[0])
	}
	if callCounts[1] != 1 || callCounts[2] != 1 {
		t.Fatalf("expected chunks 1 and 2 to run once, got %+v", callCounts)
	}
	if !result.Chunked || result.CompletedChunks != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestMergeChunkTranscriptsAssemblesFinalArtifacts(t *testing.T) {
	tmp := t.TempDir()
	chunkRoot := filepath.Join(tmp, "chunks")
	manifest := &TranscriptionManifest{
		Chunks: buildTranscriptionManifestChunks(chunkRoot, 20, 10),
	}
	for i := range manifest.Chunks {
		writeChunkTranscriptFiles(t, manifest.Chunks[i].OutputDir, fmt.Sprintf("part-%d", i), float64(i))
		manifest.Chunks[i].Status = transcriptionChunkCompleted
		manifest.Chunks[i].TranscriptTxt = filepath.Join(manifest.Chunks[i].OutputDir, "transcript.txt")
		manifest.Chunks[i].TranscriptJSON = filepath.Join(manifest.Chunks[i].OutputDir, "transcript.json")
	}

	result, err := mergeChunkTranscripts(tmp, manifest)
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "transcript.srt")); err != nil {
		t.Fatalf("expected merged srt: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "transcript.vtt")); err != nil {
		t.Fatalf("expected merged vtt: %v", err)
	}
	if result.TranscriptTxt == "" || result.TranscriptJSON == "" {
		t.Fatalf("expected merged result paths, got %+v", result)
	}
}

func stubChunkedTranscriptionHooks(t *testing.T, duration float64, transcribeFn func(opts TranscribeOptions) (string, string, error)) func() {
	t.Helper()
	prevProbe := probeTranscriptionDuration
	prevSplit := splitTranscriptionChunk
	prevTranscribe := transcribeSingleAudio
	probeTranscriptionDuration = func(string) (float64, error) {
		return duration, nil
	}
	splitTranscriptionChunk = func(_ context.Context, _ string, outPath string, _ float64, _ float64) error {
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(outPath, []byte("chunk-audio"), 0o644)
	}
	transcribeSingleAudio = transcribeFn
	return func() {
		probeTranscriptionDuration = prevProbe
		splitTranscriptionChunk = prevSplit
		transcribeSingleAudio = prevTranscribe
	}
}

func writeChunkTranscriptFiles(t *testing.T, outDir string, text string, segStart float64) {
	t.Helper()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir chunk out failed: %v", err)
	}
	txtPath := filepath.Join(outDir, "transcript.txt")
	if err := os.WriteFile(txtPath, []byte(text+"\n"), 0o644); err != nil {
		t.Fatalf("write txt failed: %v", err)
	}
	payload := WhisperJSON{
		Text: text,
		Segments: []WhisperSegment{
			{Start: segStart, End: segStart + 1, Text: text},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "transcript.json"), raw, 0o644); err != nil {
		t.Fatalf("write json failed: %v", err)
	}
}

func writeTestAudioFile(t *testing.T, dir string) string {
	t.Helper()
	audioPath := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write audio failed: %v", err)
	}
	return audioPath
}

func mustChunkIndex(t *testing.T, outDir string) int {
	t.Helper()
	base := filepath.Base(outDir)
	index, err := strconv.Atoi(strings.TrimPrefix(base, "chunk-"))
	if err != nil {
		t.Fatalf("parse chunk index from %q failed: %v", base, err)
	}
	return index
}
