//go:build integration

package media

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestGoldenExtractMetadata(t *testing.T) {
	requireFFmpegTools(t)
	video := makeSampleVideo(t)
	outDir := filepath.Join(t.TempDir(), "run-golden")
	res, err := ExtractMedia(ExtractMediaOptions{
		VideoPath:    video,
		FPS:          2,
		OutDir:       outDir,
		FrameFormat:  "png",
		Preset:       "safe",
		HWAccel:      "none",
		ExtractAudio: true,
	})
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	md, err := ReadRunMetadata(res.OutDir)
	if err != nil {
		t.Fatalf("read metadata failed: %v", err)
	}

	got := map[string]any{
		"fps":             round2(md.FPS),
		"frame_format":    md.FrameFormat,
		"extracted_audio": md.ExtractedAudio,
		"hwaccel":         md.HWAccel,
		"preset":          md.Preset,
		"width":           md.Width,
		"height":          md.Height,
		"source_fps":      round2(md.SourceFPS),
		"duration_sec":    round2(md.DurationSec),
		"start_time":      md.StartTime,
		"end_time":        md.EndTime,
		"every_n_frames":  md.EveryNFrames,
	}
	want := loadGoldenJSON(t, filepath.Join("testdata", "golden", "extract_metadata.json"))
	assertJSONEqual(t, want, got)
}

func TestGoldenTranscribeOutputs(t *testing.T) {
	requireFFmpegTools(t)
	video := makeSampleVideo(t)
	outDir := filepath.Join(t.TempDir(), "voice")
	audioPath, err := ExtractAudioFromVideoWithOptions(ExtractAudioOptions{
		VideoPath: video,
		OutDir:    outDir,
		Format:    "wav",
		Bitrate:   "128k",
	})
	if err != nil {
		t.Fatalf("audio extract failed: %v", err)
	}

	whisperBin := filepath.Join(t.TempDir(), "whisper-fake.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
audio="$1"
shift || true
out_dir=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output_dir" ] && [ "$#" -ge 2 ]; then
    out_dir="$2"
    shift 2
    continue
  fi
  shift
done
if [ -z "$out_dir" ]; then
  echo "missing --output_dir" >&2
  exit 1
fi
base="$(basename "$audio")"
base="${base%.*}"
cat > "$out_dir/$base.json" <<'JSON'
{"text":"golden transcript line","segments":[{"start":0.0,"end":0.8,"text":"golden transcript line"}]}
JSON
`
	if err := os.WriteFile(whisperBin, []byte(script), 0o755); err != nil {
		t.Fatalf("failed writing fake whisper: %v", err)
	}

	txtPath, jsonPath, err := TranscribeAudioWithOptions(TranscribeOptions{
		AudioPath: audioPath,
		OutDir:    outDir,
		Bin:       whisperBin,
		Model:     "base",
	})
	if err != nil {
		t.Fatalf("transcribe failed: %v", err)
	}
	gotTxtRaw, err := os.ReadFile(txtPath)
	if err != nil {
		t.Fatalf("read transcript txt failed: %v", err)
	}
	wantTxtRaw, err := os.ReadFile(filepath.Join("testdata", "golden", "transcript.txt"))
	if err != nil {
		t.Fatalf("read golden transcript txt failed: %v", err)
	}
	if string(gotTxtRaw) != string(wantTxtRaw) {
		t.Fatalf("transcript txt mismatch\nwant: %q\ngot:  %q", string(wantTxtRaw), string(gotTxtRaw))
	}

	gotJSONRaw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read transcript json failed: %v", err)
	}
	wantJSONRaw, err := os.ReadFile(filepath.Join("testdata", "golden", "transcript.json"))
	if err != nil {
		t.Fatalf("read golden transcript json failed: %v", err)
	}
	var gotObj any
	var wantObj any
	if err := json.Unmarshal(gotJSONRaw, &gotObj); err != nil {
		t.Fatalf("parse transcript json failed: %v", err)
	}
	if err := json.Unmarshal(wantJSONRaw, &wantObj); err != nil {
		t.Fatalf("parse golden transcript json failed: %v", err)
	}
	assertJSONEqual(t, wantObj, gotObj)
}

func loadGoldenJSON(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden json failed: %v", err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parse golden json failed: %v", err)
	}
	return out
}

func assertJSONEqual(t *testing.T, want any, got any) {
	t.Helper()
	wantRaw, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal want failed: %v", err)
	}
	gotRaw, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal got failed: %v", err)
	}
	if string(wantRaw) != string(gotRaw) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", string(wantRaw), string(gotRaw))
	}
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
