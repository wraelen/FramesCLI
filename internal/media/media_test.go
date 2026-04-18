package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeVideoPath_WindowsPath(t *testing.T) {
	got := NormalizeVideoPath(`C:\Users\me\Videos\clip.mp4`)
	if got != "/mnt/c/Users/me/Videos/clip.mp4" {
		t.Fatalf("unexpected path: %s", got)
	}
}

func TestNormalizeFrameFormat(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"png", "png"},
		{"jpg", "jpg"},
		{"jpeg", "jpg"},
		{"", "png"},
	}
	for _, tc := range cases {
		got, err := normalizeFrameFormat(tc.in)
		if err != nil {
			t.Fatalf("format %q returned err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("format %q => %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestEnsureUniquePath(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, "run")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	got := ensureUniquePath(base)
	if got == base {
		t.Fatalf("expected unique suffix path, got %s", got)
	}
}

func TestReadRunMetadata(t *testing.T) {
	tmp := t.TempDir()
	md := RunMetadata{VideoPath: "video.mp4", FPS: 4, FrameFormat: "png", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(md)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(MetadataPathForRun(tmp), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadRunMetadata(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if got.VideoPath != md.VideoPath || got.FPS != md.FPS || got.FrameFormat != md.FrameFormat {
		t.Fatalf("unexpected metadata: %+v", got)
	}
}

func TestNormalizeHWAccelAndPreset(t *testing.T) {
	if got := normalizeHWAccel("CUDA"); got != "cuda" {
		t.Fatalf("expected cuda got %s", got)
	}
	if got := normalizeHWAccel("invalid"); got != "none" {
		t.Fatalf("expected none fallback got %s", got)
	}
	if got := normalizePreset("FAST"); got != "fast" {
		t.Fatalf("expected fast got %s", got)
	}
	if got := normalizePreset("bad"); got != "balanced" {
		t.Fatalf("expected balanced fallback got %s", got)
	}
}

func TestNormalizeAudioFormat(t *testing.T) {
	if got, err := normalizeAudioFormat("wav"); err != nil || got != "wav" {
		t.Fatalf("expected wav, got %q err=%v", got, err)
	}
	if got, err := normalizeAudioFormat("MP3"); err != nil || got != "mp3" {
		t.Fatalf("expected mp3, got %q err=%v", got, err)
	}
	if _, err := normalizeAudioFormat("flac"); err == nil {
		t.Fatalf("expected unsupported format error")
	}
}

func TestBuildVideoFilter(t *testing.T) {
	got := buildVideoFilter(2, 10, 100, 200)
	want := "select='not(mod(n\\,10))*between(n\\,100\\,200)',fps=2"
	if got != want {
		t.Fatalf("unexpected filter: %s", got)
	}
}

func TestNormalizeTranscribeBackend(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "auto"},
		{"auto", "auto"},
		{"whisper", "whisper"},
		{"faster_whisper", "faster-whisper"},
		{"fasterwhisper", "faster-whisper"},
		{"faster-whisper", "faster-whisper"},
		{"bad", ""},
	}
	for _, tc := range cases {
		if got := normalizeTranscribeBackend(tc.in); got != tc.want {
			t.Fatalf("normalizeTranscribeBackend(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveTranscribeBackendAndBinAutoPrefersFaster(t *testing.T) {
	tmp := t.TempDir()
	if err := writeFakeExe(filepath.Join(tmp, "whisper")); err != nil {
		t.Fatal(err)
	}
	if err := writeFakeExe(filepath.Join(tmp, "faster-whisper")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WHISPER_BIN", "whisper")
	t.Setenv("FASTER_WHISPER_BIN", "faster-whisper")

	backend, bin, err := resolveTranscribeBackendAndBin(TranscribeOptions{Backend: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	if backend != TranscribeBackendFasterWhisper {
		t.Fatalf("expected faster-whisper backend, got %s", backend)
	}
	if bin != "faster-whisper" {
		t.Fatalf("expected faster-whisper bin, got %s", bin)
	}
}

func TestResolveTranscriptJSONPathFallback(t *testing.T) {
	outDir := t.TempDir()
	audioPath := filepath.Join(outDir, "audio.wav")
	if err := os.WriteFile(audioPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	alt := filepath.Join(outDir, "result.json")
	if err := os.WriteFile(alt, []byte(`{"text":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveTranscriptJSONPath(outDir, audioPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "result.json") {
		t.Fatalf("expected fallback json path, got %s", got)
	}
}

func writeFakeExe(path string) error {
	content := "#!/usr/bin/env sh\nexit 0\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	return nil
}

func TestEvaluateResumeStateForceBypasses(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("frame-%04d.png", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := ExtractMediaOptions{Force: true, FPS: 1}
	frames, audio, skip := evaluateResumeState(opts, dir, "", "png", 5)
	if frames != 0 || audio || skip {
		t.Fatalf("force should disable resume, got frames=%d audio=%v skip=%v", frames, audio, skip)
	}
}

func TestEvaluateResumeStateMatchingFramesSkips(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("frame-%04d.png", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := ExtractMediaOptions{FPS: 1}
	frames, audio, skip := evaluateResumeState(opts, dir, "", "png", 10)
	if frames != 10 {
		t.Fatalf("want 10 existing frames, got %d", frames)
	}
	if audio {
		t.Fatalf("audio not requested, expected false")
	}
	if !skip {
		t.Fatalf("expected ffmpeg skip when frames match expected count")
	}
}

func TestEvaluateResumeStateMissingAudioRunsFFmpeg(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("frame-%04d.png", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := ExtractMediaOptions{FPS: 1, ExtractAudio: true}
	missingAudio := filepath.Join(dir, "voice.wav")
	frames, audio, skip := evaluateResumeState(opts, dir, missingAudio, "png", 10)
	if skip {
		t.Fatalf("audio required but missing — must run ffmpeg, got skip=true")
	}
	if frames != 10 {
		t.Fatalf("want 10 frames, got %d", frames)
	}
	if audio {
		t.Fatalf("audio file missing, expected audio=false")
	}
}

func TestEvaluateResumeStateFrameCountMismatchRunsFFmpeg(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 3; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("frame-%04d.png", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := ExtractMediaOptions{FPS: 1}
	_, _, skip := evaluateResumeState(opts, dir, "", "png", 10)
	if skip {
		t.Fatalf("3 frames vs expected 10 should not be considered complete")
	}
}

// ---- PruneFrames tests ----

func writePruneFixtureRun(t *testing.T, root, name string, modTime time.Time, sizeBytes int) string {
	t.Helper()
	runDir := filepath.Join(root, name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	runJSON := filepath.Join(runDir, "run.json")
	if err := os.WriteFile(runJSON, []byte(`{"fps":1}`), 0o644); err != nil {
		t.Fatalf("write run.json: %v", err)
	}
	if sizeBytes > 0 {
		payload := make([]byte, sizeBytes)
		if err := os.WriteFile(filepath.Join(runDir, "payload.bin"), payload, 0o644); err != nil {
			t.Fatalf("write payload: %v", err)
		}
	}
	if err := os.Chtimes(runDir, modTime, modTime); err != nil {
		t.Fatalf("chtimes run dir: %v", err)
	}
	return runDir
}

func TestPruneFramesOlderThan(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	recent := writePruneFixtureRun(t, root, "Run_recent", now.Add(-5*24*time.Hour), 100)
	old := writePruneFixtureRun(t, root, "Run_old", now.Add(-60*24*time.Hour), 200)

	result, err := PruneFrames(PruneFramesOptions{
		RootDir:   root,
		OlderThan: 30 * 24 * time.Hour,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if result.TotalRuns != 2 {
		t.Fatalf("expected 2 total runs, got %d", result.TotalRuns)
	}
	if len(result.Pruned) != 1 {
		t.Fatalf("expected 1 pruned, got %d", len(result.Pruned))
	}
	if result.Pruned[0].Path != old {
		t.Fatalf("expected old dir to be pruned, got %s", result.Pruned[0].Path)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent run must still exist: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old run must be gone, got err=%v", err)
	}
}

func TestPruneFramesKeepLast(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	paths := []string{
		writePruneFixtureRun(t, root, "Run_1", base.Add(-5*24*time.Hour), 10),
		writePruneFixtureRun(t, root, "Run_2", base.Add(-4*24*time.Hour), 10),
		writePruneFixtureRun(t, root, "Run_3", base.Add(-3*24*time.Hour), 10),
		writePruneFixtureRun(t, root, "Run_4", base.Add(-2*24*time.Hour), 10),
	}

	result, err := PruneFrames(PruneFramesOptions{RootDir: root, KeepLast: 2, Now: base})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(result.Pruned) != 2 {
		t.Fatalf("expected 2 pruned, got %d", len(result.Pruned))
	}
	// Two newest (Run_3, Run_4) must still exist; two oldest (Run_1, Run_2) gone.
	for _, p := range paths[:2] {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be pruned, got err=%v", p, err)
		}
	}
	for _, p := range paths[2:] {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s to survive keep-last=2, got err=%v", p, err)
		}
	}
}

func TestPruneFramesDryRunDoesNotDelete(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	old := writePruneFixtureRun(t, root, "Run_old", now.Add(-100*24*time.Hour), 50)

	result, err := PruneFrames(PruneFramesOptions{
		RootDir:   root,
		OlderThan: 30 * 24 * time.Hour,
		DryRun:    true,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if !result.DryRun || len(result.Pruned) != 1 {
		t.Fatalf("expected 1 dry-run candidate, got %+v", result)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("dry-run must not delete: %v", err)
	}
}

func TestPruneFramesSkipsNonRunSubdirs(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)

	// A subdir without run.json — should be ignored, not pruned.
	stray := filepath.Join(root, "diagnostics")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stray, "diag.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Chtimes(stray, now.Add(-100*24*time.Hour), now.Add(-100*24*time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	result, err := PruneFrames(PruneFramesOptions{
		RootDir:   root,
		OlderThan: 30 * 24 * time.Hour,
		Now:       now,
	})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if result.TotalRuns != 0 || len(result.Pruned) != 0 {
		t.Fatalf("expected 0 runs considered, got %+v", result)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatalf("non-run subdir must be untouched: %v", err)
	}
}
