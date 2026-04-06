package media

import (
	"encoding/json"
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
