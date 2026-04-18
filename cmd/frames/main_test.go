package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/wraelen/framescli/internal/config"
	"github.com/wraelen/framescli/internal/media"
)

func TestSplitCSVPaths(t *testing.T) {
	in := "/A/B,/A/B,  /X/Y  "
	out := splitCSVPaths(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 unique paths got %d", len(out))
	}
	if out[0] != "/A/B" || out[1] != "/X/Y" {
		t.Fatalf("unexpected paths: %#v", out)
	}
}

func TestSplitCSVExts(t *testing.T) {
	out := splitCSVExts(".MP4, mov,mp4")
	if len(out) != 2 {
		t.Fatalf("expected 2 unique exts got %d", len(out))
	}
	if out[0] != "mp4" || out[1] != "mov" {
		t.Fatalf("unexpected exts: %#v", out)
	}
}

func TestBuildBenchmarkCasesCPUMode(t *testing.T) {
	cases := buildBenchmarkCases(true, false)
	if len(cases) != 2 {
		t.Fatalf("expected 2 cpu cases, got %d", len(cases))
	}
}

func TestApplyBenchmarkRecommendationMissing(t *testing.T) {
	err := applyBenchmarkRecommendation(media.BenchmarkCase{})
	if err == nil {
		t.Fatalf("expected error for missing recommendation")
	}
}

func TestCompareAgainstHistory(t *testing.T) {
	entries := []media.BenchmarkHistoryEntry{
		{Results: []media.BenchmarkResult{{Success: true, RealtimeFactor: 2.0}}},
		{Results: []media.BenchmarkResult{{Success: true, RealtimeFactor: 3.0}}},
		{Results: []media.BenchmarkResult{{Success: true, RealtimeFactor: 2.5}}},
	}
	out := compareAgainstHistory(entries, 2.0, 2)
	if out == "" {
		t.Fatalf("expected comparison output")
	}
}

func TestBenchmarkRecommendationPreview(t *testing.T) {
	cfg := appconfig.Config{HWAccel: "none", PerformanceMode: "balanced"}
	out := benchmarkRecommendationPreview(cfg, media.BenchmarkCase{HWAccel: "cuda", Preset: "fast"})
	if !strings.Contains(out, "none -> cuda") {
		t.Fatalf("missing hwaccel diff: %s", out)
	}
	if !strings.Contains(out, "balanced -> fast") {
		t.Fatalf("missing preset diff: %s", out)
	}
	if !strings.Contains(out, "No changes written") {
		t.Fatalf("missing dry-run note: %s", out)
	}
}

func TestDefaultDoctorReportPath(t *testing.T) {
	ts := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	got := defaultDoctorReportPath("frames-root", ts)
	if got != filepath.Join("frames-root", "doctor-report-20260102-030405.json") {
		t.Fatalf("unexpected report path: %s", got)
	}
}

func TestSaveDoctorReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diag", "report.json")
	r := doctorReport{
		GeneratedAt: "2026-01-01T00:00:00Z",
		GOOS:        "linux",
		GOARCH:      "amd64",
		Config:      appconfig.Default(),
		Environment: map[string]string{"WHISPER_MODEL": "base"},
	}
	if err := saveDoctorReport(path, r); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"generated_at\": \"2026-01-01T00:00:00Z\"") {
		t.Fatalf("report missing generated_at: %s", string(raw))
	}
}

func TestCollectDoctorReportCoreFields(t *testing.T) {
	r := collectDoctorReport(appconfig.Default())
	if strings.TrimSpace(r.GeneratedAt) == "" {
		t.Fatalf("expected generated_at")
	}
	if strings.TrimSpace(r.GOOS) == "" || strings.TrimSpace(r.GOARCH) == "" {
		t.Fatalf("expected runtime fields")
	}
	if len(r.Tools) == 0 {
		t.Fatalf("expected tool checks")
	}
	if strings.TrimSpace(r.TranscribeRuntimeClass) == "" || strings.TrimSpace(r.TranscribeCostHint) == "" {
		t.Fatalf("expected transcribe hint fields, got %+v", r)
	}
}

func TestNormalizeDroppedPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: `"/tmp/My File.mp4"`, want: "/tmp/My File.mp4"},
		{in: `/tmp/My\ File.mp4`, want: "/tmp/My File.mp4"},
		{in: " file:///tmp/My%20Clip.mov ", want: "/tmp/My Clip.mov"},
		{in: `C:\Videos\clip.mp4`, want: `C:\Videos\clip.mp4`},
	}
	for _, tc := range cases {
		if got := normalizeDroppedPath(tc.in); got != tc.want {
			t.Fatalf("normalizeDroppedPath(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestRotateChoice(t *testing.T) {
	choices := []string{"laptop-safe", "balanced", "high-fidelity"}
	if got := rotateChoice(choices, "balanced", 1); got != "high-fidelity" {
		t.Fatalf("expected high-fidelity, got %s", got)
	}
	if got := rotateChoice(choices, "laptop-safe", -1); got != "high-fidelity" {
		t.Fatalf("expected wrap to high-fidelity, got %s", got)
	}
}

func TestSetupWizardViewShowsGuidance(t *testing.T) {
	m := newSetupWizardModel(appconfig.Default())
	out := m.View()
	if !strings.Contains(out, "FramesCLI") || !strings.Contains(out, "Setup") {
		t.Fatalf("expected setup wizard title, got %q", out)
	}
	if !strings.Contains(out, "Quick Setup") {
		t.Fatalf("expected quick setup welcome copy, got %q", out)
	}
}

func TestSetupWizardValidatedConfigRejectsBadFPS(t *testing.T) {
	m := newSetupWizardModel(appconfig.Default())
	for i := range m.fields {
		if m.fields[i].label == "Primary recordings directory" {
			m.fields[i].input.SetValue("/tmp/recordings")
			break
		}
	}
	for i := range m.fields {
		if m.fields[i].label == "Default FPS" {
			m.fields[i].input.SetValue("0")
			break
		}
	}
	if _, err := m.validatedConfig(); err == nil || !strings.Contains(err.Error(), "positive number") {
		t.Fatalf("expected positive number error, got %v", err)
	}
}

func TestSetupWizardCustomWhisperModelSaved(t *testing.T) {
	cfg := appconfig.Default()
	cfg.WhisperModel = "ggml-large-v3"
	m := newSetupWizardModel(cfg)
	got := m.configFromFields()
	if got.WhisperModel != "ggml-large-v3" {
		t.Fatalf("expected custom whisper model to be preserved, got %q", got.WhisperModel)
	}
}

func TestResolveOpenPathPrefersExistingDirectory(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "recordings")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got := resolveOpenPath(nested + ", /missing")
	if got != nested {
		t.Fatalf("expected existing directory %q, got %q", nested, got)
	}
}

func TestResolveOpenPathFallsBackToParent(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "new-output", "frames")
	got := resolveOpenPath(target)
	want := tmp
	if got != want {
		t.Fatalf("expected nearest existing directory %q, got %q", want, got)
	}
}

func TestSetupWizardPickSelectedPathUpdatesField(t *testing.T) {
	orig := setupDirectoryPicker
	defer func() { setupDirectoryPicker = orig }()
	setupDirectoryPicker = func(initial string) (string, error) {
		return "/tmp/chosen", nil
	}

	m := newSetupWizardModel(appconfig.Default())
	m.step = 1
	if err := m.pickSelectedPath(); err != nil {
		t.Fatalf("expected picker success, got %v", err)
	}
	if got := m.fields[0].input.Value(); got != "/tmp/chosen" {
		t.Fatalf("expected chosen path to be written into field, got %q", got)
	}
}

func TestExpandVideoInputs(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.mp4")
	b := filepath.Join(tmp, "b.mp4")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := expandVideoInputs([]string{filepath.Join(tmp, "*.mp4"), a})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique inputs, got %d (%v)", len(got), got)
	}
}

func TestExpandVideoInputsUnmatchedGlobKept(t *testing.T) {
	tmp := t.TempDir()
	pattern := filepath.Join(tmp, "*.does-not-exist")
	got := expandVideoInputs([]string{pattern})
	if len(got) != 1 {
		t.Fatalf("expected unmatched glob to be kept as explicit input, got %v", got)
	}
	if got[0] != pattern {
		t.Fatalf("expected unmatched glob %q, got %q", pattern, got[0])
	}
}

func TestEstimateOutputs(t *testing.T) {
	appCfg = appconfig.Default()
	info := media.VideoInfo{DurationSec: 60, Width: 1920, Height: 1080}
	out := estimateOutputs(info, 4, "jpg", "both", buildTranscriptPlan(info.DurationSec, appCfg, false), 600)
	if out.FrameCount <= 0 || out.EstimatedMB <= 0 {
		t.Fatalf("unexpected estimate: %+v", out)
	}
	if len(out.DiskProfiles) == 0 {
		t.Fatalf("expected disk profiles, got %+v", out)
	}
	if out.DiskSummary == "" {
		t.Fatalf("expected disk summary, got %+v", out)
	}
	if out.Transcript.Backend == "" {
		t.Fatalf("expected transcript hint, got %+v", out.Transcript)
	}
}

func TestEstimateOutputsAutoFPS(t *testing.T) {
	appCfg = appconfig.Default()
	// 5s clip: auto fps caps at 8 → 40 frames.
	info := media.VideoInfo{DurationSec: 5, Width: 640, Height: 360}
	out := estimateOutputs(info, 0, "png", "frames", buildTranscriptPlan(info.DurationSec, appCfg, false), 0)
	if out.FrameCount != 40 {
		t.Fatalf("expected auto-fps 8 × 5s = 40 frames, got %+v", out)
	}
	if out.Transcript.Enabled {
		t.Fatalf("expected transcript estimate disabled for frames mode, got %+v", out.Transcript)
	}
}

func TestAutoFPSForDurationTiers(t *testing.T) {
	cases := []struct {
		duration float64
		want     float64
	}{
		{0, 1},         // invalid → safe floor
		{-1, 1},        // negative → safe floor
		{10, 8},        // short clip → capped at 8 fps
		{30, 8},        // 30s at 480 target → 16 clamped to 8
		{60, 8},        // 1 min → 8 fps exact
		{120, 4},       // 2 min → 4 fps
		{300, 1.5},     // 5 min → 1.5 fps (rounded)
		{600, 1},       // 10 min → floor 1 fps
		{3600, 1},      // 1 hr → floor 1 fps
	}
	for _, tc := range cases {
		got := autoFPSForDuration(tc.duration)
		if got != tc.want {
			t.Fatalf("autoFPSForDuration(%v) = %v, want %v", tc.duration, got, tc.want)
		}
	}
}

func TestEstimateOutputsWarnsForLongCPUTranscript(t *testing.T) {
	appCfg = appconfig.Default()
	appCfg.TranscribeBackend = "whisper"
	appCfg.WhisperModel = "large"
	info := media.VideoInfo{DurationSec: 45 * 60, Width: 1920, Height: 1080}
	out := estimateOutputs(info, 4, "png", "both", buildTranscriptPlan(info.DurationSec, appCfg, false), 0)
	if len(out.Warnings) == 0 {
		t.Fatalf("expected preview warnings, got %+v", out)
	}
	if out.Transcript.RuntimeClass == "" || out.Transcript.CostHint == "" {
		t.Fatalf("expected transcript runtime hint, got %+v", out.Transcript)
	}
	if !out.Guardrails.RequiresOverride {
		t.Fatalf("expected expensive workload override requirement, got %+v", out.Guardrails)
	}
}

func TestResolveWorkflowSettingsPresetDefaults(t *testing.T) {
	info := media.VideoInfo{DurationSec: 1800, Width: 1920, Height: 1080}
	got := resolveWorkflowSettings(info, 4, "png", "laptop-safe", false, false, true, true, 0, false)
	if got.Preset != "laptop-safe" || got.MediaPreset != "safe" {
		t.Fatalf("unexpected preset resolution: %+v", got)
	}
	if got.FPS != 1 {
		t.Fatalf("expected laptop-safe fps=1, got %+v", got)
	}
	if got.FrameFormat != "jpg" {
		t.Fatalf("expected laptop-safe format=jpg, got %+v", got)
	}
	if got.ChunkDurationSec != 300 {
		t.Fatalf("expected laptop-safe chunking=300, got %+v", got)
	}
}

func TestResolveWorkflowSettingsExplicitOverridesWin(t *testing.T) {
	info := media.VideoInfo{DurationSec: 120, Width: 1280, Height: 720}
	got := resolveWorkflowSettings(info, 6, "jpg", "high-fidelity", true, true, true, true, 1200, true)
	if got.FPS != 6 || got.FrameFormat != "jpg" || got.ChunkDurationSec != 1200 {
		t.Fatalf("expected explicit values to win, got %+v", got)
	}
	if got.MediaPreset != "fast" {
		t.Fatalf("expected high-fidelity media preset fast, got %+v", got)
	}
}

func TestResolveWorkflowSettingsImplicitConfigPresetUsesPresetDefaults(t *testing.T) {
	info := media.VideoInfo{DurationSec: 600, Width: 1280, Height: 720}
	defaultCfg := appconfig.Default()
	got := resolveWorkflowSettings(info, defaultCfg.DefaultFPS, defaultCfg.DefaultFormat, "high-fidelity", false, false, false, true, 0, false)
	if got.FPS != 8 || got.FrameFormat != "png" || got.ChunkDurationSec != 900 {
		t.Fatalf("expected implicit config preset defaults, got %+v", got)
	}
}

func TestResolveWorkflowSettingsCustomConfigDefaultsStillWin(t *testing.T) {
	info := media.VideoInfo{DurationSec: 600, Width: 1280, Height: 720}
	got := resolveWorkflowSettings(info, 6, "jpg", "high-fidelity", false, false, false, true, 0, false)
	if got.FPS != 6 || got.FrameFormat != "jpg" {
		t.Fatalf("expected custom config defaults to win, got %+v", got)
	}
	if got.ChunkDurationSec != 900 {
		t.Fatalf("expected chunking to still come from preset, got %+v", got)
	}
}

func TestImportDeprecatedCommandIsHiddenAndGuided(t *testing.T) {
	cmd := importDeprecatedCommand()
	if cmd.Use == "" || !strings.HasPrefix(cmd.Use, "import") {
		t.Fatalf("expected Use to start with 'import', got %q", cmd.Use)
	}
	if !cmd.Hidden {
		t.Fatalf("expected import command to be hidden from help")
	}
	if !strings.Contains(strings.ToLower(cmd.Short), "deprecated") {
		t.Fatalf("expected Short to mention deprecation, got %q", cmd.Short)
	}
}

func TestProbeTranscribeAccelRuleBased(t *testing.T) {
	cases := []struct {
		name      string
		backend   string
		hwGPU     bool
		wantGPU   bool
		reasonHas string
	}{
		{"no hw GPU → CPU regardless of backend", "faster-whisper", false, false, "no GPU detected"},
		{"whisper on GPU hw → CPU with guidance", "whisper", true, false, "faster-whisper for GPU acceleration"},
		{"faster-whisper on GPU hw → GPU", "faster-whisper", true, true, "faster-whisper with GPU hardware"},
		{"faster_whisper alias on GPU hw → GPU", "faster_whisper", true, true, "faster-whisper with GPU hardware"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := appconfig.Default()
			got := probeTranscribeAccel(cfg, tc.backend, tc.hwGPU)
			if got.UsesGPU != tc.wantGPU {
				t.Fatalf("UsesGPU: got %v, want %v (reason: %q)", got.UsesGPU, tc.wantGPU, got.Reason)
			}
			if !strings.Contains(got.Reason, tc.reasonHas) {
				t.Fatalf("Reason: got %q, want substring %q", got.Reason, tc.reasonHas)
			}
		})
	}
}

func TestTranscriptRealtimeRangeIsCPUFactorsWhenTranscribeGPUFalse(t *testing.T) {
	// Key honesty check: hw GPU does not imply GPU transcription speeds.
	// Caller passes transcribeGPU=false (e.g. backend=whisper on GPU hw)
	// and the factors should match the CPU branch, not the GPU branch.
	cpuLow, cpuHigh := transcriptRealtimeRange("whisper", "base", false)
	gpuLow, gpuHigh := transcriptRealtimeRange("whisper", "base", true)
	if cpuHigh >= gpuLow {
		t.Fatalf("CPU factors should be strictly below GPU: cpu=[%v,%v] gpu=[%v,%v]",
			cpuLow, cpuHigh, gpuLow, gpuHigh)
	}
}

func TestResolveWorkflowSettingsAutoFPSMarksMode(t *testing.T) {
	info := media.VideoInfo{DurationSec: 1068, Width: 640, Height: 360}
	got := resolveWorkflowSettings(info, 0, "png", "balanced", true, false, false, false, 0, false)
	if got.FPSMode != "auto" {
		t.Fatalf("expected FPSMode=auto when --fps 0/auto explicit, got %q", got.FPSMode)
	}
	if got.FPS <= 0 {
		t.Fatalf("expected auto fps to be > 0 after resolution, got %v", got.FPS)
	}
}

func TestResolveWorkflowSettingsExplicitNumericDoesNotSetAutoMode(t *testing.T) {
	info := media.VideoInfo{DurationSec: 120, Width: 1280, Height: 720}
	got := resolveWorkflowSettings(info, 4, "png", "balanced", true, false, false, false, 0, false)
	if got.FPSMode != "" {
		t.Fatalf("expected empty FPSMode for explicit numeric fps, got %q", got.FPSMode)
	}
}

func TestResolveVideoInputNormalizesWindowsPathOnNonWindows(t *testing.T) {
	got, note, err := resolveVideoInput(`C:\Users\wraelen\Videos\clip.mp4`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/mnt/c/Users/wraelen/Videos/clip.mp4" {
		t.Fatalf("unexpected normalized path: %q", got)
	}
	if note != "normalized from Windows path" {
		t.Fatalf("unexpected source note: %q", note)
	}
}

func TestEnforceGuardrailsRequiresOverride(t *testing.T) {
	report := workloadAssessment{
		RequiresOverride: true,
		OverrideFlag:     "--allow-expensive",
		Guardrails: []workloadGuardrail{
			{Severity: "block", Message: "Estimated frame count exceeds 40000."},
		},
	}
	if err := enforceGuardrails(report, false); err == nil || !strings.Contains(err.Error(), "--allow-expensive") {
		t.Fatalf("expected allow-expensive error, got %v", err)
	}
	if err := enforceGuardrails(report, true); err != nil {
		t.Fatalf("expected override to bypass guardrail, got %v", err)
	}
}

func TestLatestBenchmarkPreviewHint(t *testing.T) {
	root := t.TempDir()
	appCfg = appconfig.Default()
	appCfg.FramesRoot = root
	report := media.BenchmarkReport{
		VideoPath:   "video.mp4",
		DurationSec: 20,
		Results: []media.BenchmarkResult{
			{Success: true, RealtimeFactor: 2.5},
		},
		Recommended: media.BenchmarkCase{HWAccel: "none", Preset: "balanced"},
	}
	if err := media.AppendBenchmarkHistory(media.BenchmarkHistoryPath(root), report, 10); err != nil {
		t.Fatal(err)
	}
	hint := latestBenchmarkPreviewHint(600)
	if hint == nil {
		t.Fatalf("expected benchmark hint")
	}
	if hint.BestRealtimeFactor != 2.5 {
		t.Fatalf("unexpected benchmark hint: %+v", hint)
	}
}

func TestRunPreviewResultAutoFPS(t *testing.T) {
	requireFFmpegToolsMainTest(t)
	tmp := t.TempDir()
	video := filepath.Join(tmp, "sample.mp4")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=24", "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=16000", "-t", "5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", video)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed generating sample video: %v\n%s", err, string(out))
	}
	got, err := runPreviewResult(video, 0, "png", "both", "balanced", 0, true, false, false, false)
	if err != nil {
		t.Fatalf("runPreviewResult returned error: %v", err)
	}
	// 5s clip: auto fps caps at 8 (new tiered formula; previously would have been 12).
	if fps, ok := got["target_fps"].(float64); !ok || fps != 8 {
		t.Fatalf("expected target_fps=8 (auto cap), got %#v", got["target_fps"])
	}
	estimate, ok := got["estimate"].(outputEstimate)
	if !ok {
		t.Fatalf("expected outputEstimate payload, got %T", got["estimate"])
	}
	if estimate.DiskSummary == "" {
		t.Fatalf("expected preview disk summary, got %+v", estimate)
	}
}

func TestRunExtractWorkflowResultNoAudioVoiceFailsEarly(t *testing.T) {
	requireFFmpegToolsMainTest(t)
	video := makeNoAudioVideoMainTest(t)

	_, err := runExtractWorkflowResult(context.Background(), video, 2, extractWorkflowOptions{
		Voice:       true,
		FrameFormat: "png",
		HWAccel:     "none",
		Preset:      "safe",
		SheetCols:   6,
	}, false)
	if err == nil {
		t.Fatal("expected no-audio source to fail when voice extraction is requested")
	}
	msg := err.Error()
	if !strings.Contains(msg, "extraction failed: requested audio output, but video has no audio stream") {
		t.Fatalf("expected wrapped no-audio error, got %q", msg)
	}
	if !strings.Contains(msg, filepath.Base(video)) {
		t.Fatalf("expected source path in error, got %q", msg)
	}
}

func TestRunExtractWorkflowResultNoAudioFrameOnlyStillSucceeds(t *testing.T) {
	requireFFmpegToolsMainTest(t)
	video := makeNoAudioVideoMainTest(t)

	got, err := runExtractWorkflowResult(context.Background(), video, 2, extractWorkflowOptions{
		Voice:       false,
		FrameFormat: "png",
		HWAccel:     "none",
		Preset:      "safe",
		SheetCols:   6,
	}, false)
	if err != nil {
		t.Fatalf("expected frame-only extraction to succeed, got %v", err)
	}
	if got.FrameCount < 2 {
		t.Fatalf("expected extracted frames, got %d", got.FrameCount)
	}
	if got.AudioPath != "" {
		t.Fatalf("expected no audio artifact for frame-only path, got %q", got.AudioPath)
	}
}

func TestRunTranscribeRunResultTranscriptPresentWithIncompleteManifestResumes(t *testing.T) {
	runDir := makeTranscribeRunFixture(t)
	voiceDir := filepath.Join(runDir, "voice")
	audioPath := filepath.Join(voiceDir, "voice.wav")
	manifestPath := filepath.Join(voiceDir, "transcription-manifest.json")

	manifest := media.TranscriptionManifest{
		SchemaVersion:    "framescli.transcription_manifest.v1",
		SourceAudioPath:  filepath.Join(runDir, "different.wav"),
		ChunkDurationSec: 10,
		TotalDurationSec: 20,
		ChunkRootDir:     filepath.Join(voiceDir, "chunks"),
		Chunks: []media.TranscriptionManifestChunk{
			{
				Index:          0,
				StartSec:       0,
				EndSec:         10,
				Status:         "completed",
				AudioPath:      filepath.Join(voiceDir, "chunks", "chunk-0000", "audio.wav"),
				OutputDir:      filepath.Join(voiceDir, "chunks", "chunk-0000"),
				TranscriptTxt:  filepath.Join(voiceDir, "chunks", "chunk-0000", "transcript.txt"),
				TranscriptJSON: filepath.Join(voiceDir, "chunks", "chunk-0000", "transcript.json"),
			},
			{
				Index:     1,
				StartSec:  10,
				EndSec:    20,
				Status:    "pending",
				AudioPath: filepath.Join(voiceDir, "chunks", "chunk-0001", "audio.wav"),
				OutputDir: filepath.Join(voiceDir, "chunks", "chunk-0001"),
			},
		},
		Merge: media.TranscriptionManifestMerge{
			Status:         "pending",
			TranscriptTxt:  filepath.Join(voiceDir, "transcript.txt"),
			TranscriptJSON: filepath.Join(voiceDir, "transcript.json"),
		},
	}
	writeChunkFixtureFiles(t, manifest.Chunks[0])
	writeJSONFile(t, manifestPath, manifest)

	result, err := runTranscribeRunResult(context.Background(), runDir, media.TranscribeOptions{})
	if err == nil {
		t.Fatal("expected resume path to continue past transcript fast-path")
	}
	if !strings.Contains(err.Error(), audioPath) {
		t.Fatalf("expected manifest/source-audio resume error, got %v", err)
	}
	if result.AlreadyPresent {
		t.Fatalf("expected incomplete manifest to bypass AlreadyPresent fast path, got %+v", result)
	}
	if result.ManifestPath != manifestPath {
		t.Fatalf("expected manifest path %q, got %q", manifestPath, result.ManifestPath)
	}
}

func TestRunTranscribeRunResultTranscriptPresentWithCompleteManifestShortCircuits(t *testing.T) {
	runDir := makeTranscribeRunFixture(t)
	voiceDir := filepath.Join(runDir, "voice")
	audioPath := filepath.Join(voiceDir, "voice.wav")
	manifestPath := filepath.Join(voiceDir, "transcription-manifest.json")

	chunk := media.TranscriptionManifestChunk{
		Index:          0,
		StartSec:       0,
		EndSec:         10,
		Status:         "completed",
		AudioPath:      filepath.Join(voiceDir, "chunks", "chunk-0000", "audio.wav"),
		OutputDir:      filepath.Join(voiceDir, "chunks", "chunk-0000"),
		TranscriptTxt:  filepath.Join(voiceDir, "chunks", "chunk-0000", "transcript.txt"),
		TranscriptJSON: filepath.Join(voiceDir, "chunks", "chunk-0000", "transcript.json"),
	}
	manifest := media.TranscriptionManifest{
		SchemaVersion:    "framescli.transcription_manifest.v1",
		SourceAudioPath:  audioPath,
		ChunkDurationSec: 10,
		TotalDurationSec: 10,
		ChunkRootDir:     filepath.Join(voiceDir, "chunks"),
		Chunks:           []media.TranscriptionManifestChunk{chunk},
		Merge: media.TranscriptionManifestMerge{
			Status:         "completed",
			TranscriptTxt:  filepath.Join(voiceDir, "transcript.txt"),
			TranscriptJSON: filepath.Join(voiceDir, "transcript.json"),
		},
	}
	writeChunkFixtureFiles(t, chunk)
	writeJSONFile(t, manifestPath, manifest)

	result, err := runTranscribeRunResult(context.Background(), runDir, media.TranscribeOptions{})
	if err != nil {
		t.Fatalf("expected complete manifest to short-circuit, got %v", err)
	}
	if !result.AlreadyPresent {
		t.Fatalf("expected AlreadyPresent short-circuit, got %+v", result)
	}
	if result.ManifestPath != manifestPath {
		t.Fatalf("expected manifest path %q, got %q", manifestPath, result.ManifestPath)
	}
}

func TestStageTimingSuffixIncludesETA(t *testing.T) {
	started := time.Now().Add(-30 * time.Second)
	out := stageTimingSuffix(started, 50)
	if !strings.Contains(out, "elapsed") || !strings.Contains(out, "eta") {
		t.Fatalf("expected elapsed and eta, got %q", out)
	}
}

func requireFFmpegToolsMainTest(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed: %v", bin, err)
		}
	}
}

func makeTranscribeRunFixture(t *testing.T) string {
	t.Helper()
	runDir := t.TempDir()
	voiceDir := filepath.Join(runDir, "voice")
	if err := os.MkdirAll(voiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(voiceDir, "voice.wav")
	if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voiceDir, "transcript.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voiceDir, "transcript.json"), []byte(`{"text":"done"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	md := media.RunMetadata{
		VideoPath:      "video.mp4",
		ExtractedAudio: true,
		AudioPath:      audioPath,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	writeJSONFile(t, media.MetadataPathForRun(runDir), md)
	return runDir
}

func writeChunkFixtureFiles(t *testing.T, chunk media.TranscriptionManifestChunk) {
	t.Helper()
	if err := os.MkdirAll(chunk.OutputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunk.AudioPath, []byte("chunk-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunk.TranscriptTxt, []byte("chunk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(chunk.TranscriptJSON, []byte(`{"text":"chunk"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeNoAudioVideoMainTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "no-audio.mp4")
	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=12", "-t", "2", "-pix_fmt", "yuv420p", "-c:v", "libx264", "-an", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed generating no-audio video: %v\n%s", err, string(out))
	}
	return path
}

func TestMCPToolTimeoutClamp(t *testing.T) {
	state := newMCPServerState(os.Stdout)
	got := state.toolTimeout(int((3 * time.Hour) / time.Millisecond))
	if got != maxMCPToolTimeout {
		t.Fatalf("expected clamp to max timeout, got %s", got)
	}
}

func TestMCPCancelCall(t *testing.T) {
	state := newMCPServerState(os.Stdout)
	cancelled := false
	state.registerCall(json.RawMessage(`"1"`), func() { cancelled = true })
	if !state.cancelCall(json.RawMessage(`"1"`)) {
		t.Fatalf("expected cancelCall true")
	}
	if !cancelled {
		t.Fatalf("expected cancel func to run")
	}
}

func TestCallMCPToolCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := callMCPTool(ctx, "doctor", nil); err == nil || err != context.Canceled {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestIsCancellationError(t *testing.T) {
	if !isCancellationError(context.Canceled) {
		t.Fatalf("expected context.Canceled to be treated as cancellation")
	}
	if !isCancellationError(context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded to be treated as cancellation")
	}
	if isCancellationError(errors.New("other")) {
		t.Fatalf("expected non-cancellation error to return false")
	}
}

func TestNormalizeCancellationErrorFromContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := normalizeCancellationError(ctx, errors.New("ffmpeg exited"))
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("expected canceled context to normalize to context.Canceled, got %v", got)
	}
}

func TestAnnotateStorageError(t *testing.T) {
	cases := []error{
		errors.New("No space left on device"),
		errors.New("file too large"),
	}
	for _, in := range cases {
		out := annotateStorageError(in)
		if out == nil || !strings.Contains(strings.ToLower(out.Error()), "insufficient disk space") {
			t.Fatalf("expected storage annotation for %q, got %v", in, out)
		}
	}
	other := errors.New("some other error")
	if got := annotateStorageError(other); got == nil || got.Error() != other.Error() {
		t.Fatalf("unexpected non-storage annotation: %v", got)
	}
}

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantClass string
		retryable bool
	}{
		{name: "cancelled", err: context.Canceled, wantClass: "interrupted", retryable: true},
		{name: "disk", err: errors.New("no space left on device"), wantClass: "disk_space", retryable: true},
		{name: "dependency", err: errors.New(`whisper backend selected but binary "whisper" not found on PATH`), wantClass: "missing_dependency", retryable: true},
		{name: "unsupported", err: errors.New(`unsupported audio format "flac" (use wav|mp3|aac)`), wantClass: "unsupported_media", retryable: false},
		{name: "probe_failed", err: errors.New(`video probe failed: ffprobe failed for "/tmp/bad.mp4": exit status 1`), wantClass: "unsupported_media", retryable: false},
		{name: "invalid_video_input_wrapped_probe", err: errors.New(`invalid video input "/tmp/bad.mp4": ffprobe failed for "/tmp/bad.mp4": exit status 1`), wantClass: "unsupported_media", retryable: false},
		{name: "file_not_found", err: errors.New(`file not found: /tmp/missing.mp4`), wantClass: "file_not_found", retryable: false},
		{name: "path_is_directory", err: errors.New(`path is a directory, expected a video file: /tmp`), wantClass: "file_not_found", retryable: false},
		{name: "invalid", err: errors.New("transcribe_run requires run_dir"), wantClass: "invalid_input", retryable: false},
		{name: "path", err: errors.New("path outside allowed roots: /tmp/nope"), wantClass: "path_not_allowed", retryable: false},
		{name: "not_found", err: errors.New("no runs found in /tmp/frames"), wantClass: "not_found", retryable: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got.Class != tc.wantClass {
				t.Fatalf("classifyError(%q) class=%q want %q", tc.err, got.Class, tc.wantClass)
			}
			if got.Retryable != tc.retryable {
				t.Fatalf("classifyError(%q) retryable=%t want %t", tc.err, got.Retryable, tc.retryable)
			}
			if got.Message != tc.err.Error() {
				t.Fatalf("classifyError(%q) message=%q", tc.err, got.Message)
			}
			if got.Data["class"] != tc.wantClass {
				t.Fatalf("classifyError(%q) data.class=%#v", tc.err, got.Data["class"])
			}
		})
	}
}

func TestBuildAutomationEnvelopeIncludesErrorDetails(t *testing.T) {
	started := time.Now().Add(-time.Second)
	env := buildAutomationEnvelope("extract", started, "error", nil, errors.New("no space left on device"))
	if env.Error == nil {
		t.Fatal("expected error payload")
	}
	if env.Error.Code != "command_failed" {
		t.Fatalf("unexpected error code: %+v", env.Error)
	}
	if env.Error.Class != "disk_space" {
		t.Fatalf("unexpected error class: %+v", env.Error)
	}
	if !env.Error.Retryable {
		t.Fatalf("expected retryable disk error: %+v", env.Error)
	}
	if env.Error.Recovery == "" {
		t.Fatalf("expected recovery guidance: %+v", env.Error)
	}
}

func TestBuildMCPErrorIncludesStructuredData(t *testing.T) {
	got := buildMCPError(-32000, fmt.Errorf("path outside allowed roots: /tmp/nope"))
	if got == nil {
		t.Fatal("expected mcp error")
	}
	if got.Code != -32000 || got.Message == "" {
		t.Fatalf("unexpected mcp error: %+v", got)
	}
	if got.Data["class"] != "path_not_allowed" {
		t.Fatalf("unexpected mcp error data: %+v", got.Data)
	}
	if got.Data["recovery"] == "" {
		t.Fatalf("expected recovery in mcp error data: %+v", got.Data)
	}
}

func TestRenderUserFacingErrorIncludesRecovery(t *testing.T) {
	out := renderUserFacingError(errors.New("no clipboard tool found (pbcopy/clip/wl-copy/xclip/xsel)"))
	if !strings.Contains(out, "Recovery:") {
		t.Fatalf("expected recovery guidance, got %q", out)
	}
}

func TestRunPreviewResultUnsupportedMediaClassifiedCorrectly(t *testing.T) {
	requireFFmpegToolsMainTest(t)
	path := filepath.Join(t.TempDir(), "not-video.mp4")
	if err := os.WriteFile(path, []byte("definitely not a valid media file"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runPreviewResult(path, 2, "png", "both", "balanced", 0, true, true, false, false)
	if err == nil {
		t.Fatal("expected preview error for invalid media")
	}
	got := classifyError(err)
	if got.Class != "unsupported_media" {
		t.Fatalf("expected unsupported_media, got %+v", got)
	}
}

func TestEnsureMCPPathAllowed(t *testing.T) {
	tmp := t.TempDir()
	appCfg = appconfig.Default()
	appCfg.FramesRoot = tmp
	inside := filepath.Join(tmp, "a", "b.txt")
	if err := ensureMCPPathAllowed(inside); err != nil {
		t.Fatalf("expected inside path allowed, got %v", err)
	}
	if err := ensureMCPPathAllowed("https://example.com/file.mp4"); err == nil {
		t.Fatalf("expected remote path to be rejected")
	}
	outside := filepath.Join(string(filepath.Separator), "etc", "passwd")
	if err := ensureMCPPathAllowed(outside); err == nil {
		t.Fatalf("expected outside path to be rejected")
	}
}

func TestLatestRunArtifacts(t *testing.T) {
	root := t.TempDir()
	run := writeArtifactRunFixture(t, root, "Run_1", artifactFixtureOptions{withTranscript: true, withSheet: true, withAudio: true})
	if _, err := media.WriteRunsIndex(root, ""); err != nil {
		t.Fatal(err)
	}
	arts, err := latestRunArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	if arts["run"] != run || arts["sheet"] == "" || arts["transcript"] == "" || arts["metadata"] == "" {
		t.Fatalf("expected latest artifacts map, got %+v", arts)
	}
}

func TestFindLastRunArtifactUsesIndexForSpecificArtifacts(t *testing.T) {
	root := t.TempDir()
	run := writeArtifactRunFixture(t, root, "Run_1", artifactFixtureOptions{withTranscript: true, withAudio: true})
	if _, err := media.WriteRunsIndex(root, ""); err != nil {
		t.Fatal(err)
	}

	target, runPath, err := findLastRunArtifact(root, "transcript-json")
	if err != nil {
		t.Fatal(err)
	}
	if runPath != run {
		t.Fatalf("expected run path %q, got %q", run, runPath)
	}
	if !strings.HasSuffix(target, filepath.Join("voice", "transcript.json")) {
		t.Fatalf("expected transcript json path, got %q", target)
	}
}

func TestArtifactPathForRunSupportsExpandedArtifactSelectors(t *testing.T) {
	root := t.TempDir()
	runPath := writeArtifactRunFixture(t, root, "Run_1", artifactFixtureOptions{
		withTranscript:  true,
		withTimedText:   true,
		withManifest:    true,
		withMetadataCSV: true,
		withFramesZip:   true,
	})
	if _, err := media.WriteRunsIndex(root, ""); err != nil {
		t.Fatal(err)
	}
	run, err := media.SelectRun(root, runPath)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"transcript_json":        filepath.Join("voice", "transcript.json"),
		"transcript-srt":         filepath.Join("voice", "transcript.srt"),
		"transcript_vtt":         filepath.Join("voice", "transcript.vtt"),
		"transcription-manifest": filepath.Join("voice", "transcription-manifest.json"),
		"metadata_csv":           "frames.csv",
		"frames-zip":             "frames.zip",
	}
	for artifact, suffix := range cases {
		got, err := artifactPathForRun(run, artifact)
		if err != nil {
			t.Fatalf("artifactPathForRun(%q) returned error: %v", artifact, err)
		}
		if !strings.HasSuffix(got, suffix) {
			t.Fatalf("artifactPathForRun(%q)=%q, want suffix %q", artifact, got, suffix)
		}
	}
}

func TestLatestRunArtifactsIncludesExpandedIndexedArtifacts(t *testing.T) {
	root := t.TempDir()
	writeArtifactRunFixture(t, root, "Run_1", artifactFixtureOptions{
		withAudio:       true,
		withTranscript:  true,
		withTimedText:   true,
		withManifest:    true,
		withMetadataCSV: true,
		withFramesZip:   true,
	})
	if _, err := media.WriteRunsIndex(root, ""); err != nil {
		t.Fatal(err)
	}

	arts, err := latestRunArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"transcript_json", "transcript_srt", "transcript_vtt", "manifest", "metadata_csv", "frames_zip"} {
		if strings.TrimSpace(arts[key]) == "" {
			t.Fatalf("expected artifact key %q in %+v", key, arts)
		}
	}
}

func TestArtifactPathForRunMissingArtifact(t *testing.T) {
	root := t.TempDir()
	runPath := writeArtifactRunFixture(t, root, "Run_1", artifactFixtureOptions{})
	if _, err := media.WriteRunsIndex(root, ""); err != nil {
		t.Fatal(err)
	}
	run, err := media.SelectRun(root, runPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := artifactPathForRun(run, "transcript"); err == nil || !strings.Contains(err.Error(), "transcript not found") {
		t.Fatalf("expected missing transcript error, got %v", err)
	}
}

func TestPreviewCommandMarksFailureOnError(t *testing.T) {
	commandFailed = false
	appCfg = appconfig.Default()
	cmd := previewCommand()
	cmd.SetArgs([]string{"/definitely/missing.mp4"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected cobra error: %v", err)
	}
	if !commandFailed {
		t.Fatalf("expected commandFailed=true for preview error")
	}
}

func TestPrintDoctorReportRecoveryHints(t *testing.T) {
	r := doctorReport{
		GOOS:   "linux",
		GOARCH: "amd64",
		Environment: map[string]string{
			"OBS_VIDEO_DIR":    "-",
			"WHISPER_BIN":      "whisper",
			"WHISPER_MODEL":    "base",
			"WHISPER_LANGUAGE": "-",
		},
		Tools: []toolCheck{
			{Name: "ffmpeg", Required: true, Found: false},
			{Name: "ffprobe", Required: true, Found: false},
			{Name: "whisper", Required: false, Found: true},
		},
		RequiredFailed: true,
	}
	out := captureStdout(t, func() {
		printDoctorReport(r)
	})
	if !strings.Contains(out, "Recovery") || !strings.Contains(out, "framescli doctor") {
		t.Fatalf("expected recovery guidance in doctor output, got: %s", out)
	}
}

func TestEffectivePostHookUsesConfigFallback(t *testing.T) {
	appCfg = appconfig.Default()
	appCfg.PostExtractHook = "echo from-config"
	appCfg.PostHookTimeout = 12
	hook, timeout := effectivePostHook(extractWorkflowOptions{})
	if hook != "echo from-config" {
		t.Fatalf("expected config hook fallback, got %q", hook)
	}
	if timeout != 12*time.Second {
		t.Fatalf("expected config timeout fallback, got %s", timeout)
	}
}

func TestRunPostExtractHookSuccess(t *testing.T) {
	res := extractWorkflowResult{
		VideoInput:    "/tmp/in.mp4",
		ResolvedVideo: "/tmp/in.mp4",
		OutDir:        "/tmp/out",
		Artifacts: map[string]string{
			"out_dir": "/tmp/out",
		},
	}
	out, err := runPostExtractHook(context.Background(), "echo hook-ok", 2*time.Second, res)
	if err != nil {
		t.Fatalf("expected hook success, got %v", err)
	}
	if !strings.Contains(out, "hook-ok") {
		t.Fatalf("expected hook output, got %q", out)
	}
}

func TestAppendTelemetryEventEnabled(t *testing.T) {
	root := t.TempDir()
	appCfg = appconfig.Default()
	appCfg.FramesRoot = root
	appCfg.TelemetryEnabled = true
	if err := appendTelemetryEvent(telemetryEvent{
		Command: "extract",
		Status:  "success",
		Input:   "/tmp/in.mp4",
	}); err != nil {
		t.Fatalf("expected telemetry append success, got %v", err)
	}
	path := filepath.Join(root, "telemetry", "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"command\":\"extract\"") {
		t.Fatalf("expected telemetry event content, got %s", string(raw))
	}
}

func TestAppendTelemetryEventDisabledNoop(t *testing.T) {
	root := t.TempDir()
	appCfg = appconfig.Default()
	appCfg.FramesRoot = root
	appCfg.TelemetryEnabled = false
	if err := appendTelemetryEvent(telemetryEvent{Command: "extract", Status: "success"}); err != nil {
		t.Fatalf("expected noop telemetry success, got %v", err)
	}
	path := filepath.Join(root, "telemetry", "events.jsonl")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected no telemetry file when disabled")
	}
}

func TestReadLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	content := strings.Join([]string{"a", "b", "c", "d"}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readLastLines(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Fatalf("unexpected tail lines: %#v", got)
	}
}

func TestPruneToLastLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	content := strings.Join([]string{"1", "2", "3", "4", "5"}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	kept, err := pruneToLastLines(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 3 {
		t.Fatalf("expected kept=3, got %d", kept)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "3\n4\n5" {
		t.Fatalf("unexpected pruned file: %q", string(raw))
	}
}

type artifactFixtureOptions struct {
	withAudio       bool
	withTranscript  bool
	withTimedText   bool
	withManifest    bool
	withMetadataCSV bool
	withFramesZip   bool
	withSheet       bool
	createdAt       string
}

func writeArtifactRunFixture(t *testing.T, root string, name string, opts artifactFixtureOptions) string {
	t.Helper()

	run := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(run, "images", "sheets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(run, "voice"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "images", "frame-0001.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if opts.withSheet {
		if err := os.WriteFile(filepath.Join(run, "images", "sheets", "contact-sheet.png"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.withAudio {
		if err := os.WriteFile(filepath.Join(run, "voice", "voice.wav"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.withTranscript {
		if err := os.WriteFile(filepath.Join(run, "voice", "transcript.txt"), []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(run, "voice", "transcript.json"), []byte(`{"text":"hello","segments":[{"start":0,"end":1,"text":"hello"}]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if opts.withTimedText {
			if err := os.WriteFile(filepath.Join(run, "voice", "transcript.srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nhello\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(run, "voice", "transcript.vtt"), []byte("WEBVTT\n\n00:00.000 --> 00:01.000\nhello\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if opts.withManifest {
		if err := os.WriteFile(filepath.Join(run, "voice", "transcription-manifest.json"), []byte(`{"schema_version":"framescli.transcription_manifest.v1","chunks":[],"merge":{"status":"completed"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(run, "extract.log"), []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if opts.withMetadataCSV {
		if err := os.WriteFile(filepath.Join(run, "frames.csv"), []byte("file,index\nframe-0001.png,1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.withFramesZip {
		if err := os.WriteFile(filepath.Join(run, "frames.zip"), []byte("zip"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	createdAt := opts.createdAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	md := media.RunMetadata{
		VideoPath:      "/tmp/source.mp4",
		FPS:            4,
		FrameFormat:    "png",
		ExtractedAudio: opts.withAudio,
		AudioPath:      filepath.Join(run, "voice", "voice.wav"),
		Preset:         "balanced",
		CreatedAt:      createdAt,
	}
	raw, err := json.Marshal(md)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media.MetadataPathForRun(run), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return run
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPromptChoiceSelectsByNumber(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("2\n"))
	got := promptChoice(reader, "Format", "png", []string{"png", "jpg"})
	if got != "jpg" {
		t.Fatalf("expected jpg, got %q", got)
	}
}

func TestPromptChoiceKeepsDefaultOnBlank(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	got := promptChoice(reader, "Backend", "auto", []string{"auto", "whisper", "faster-whisper"})
	if got != "auto" {
		t.Fatalf("expected default auto, got %q", got)
	}
}

func TestPromptYesNoHonorsDefault(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\n"))
	if !promptYesNo(reader, "Save", true) {
		t.Fatalf("expected default yes")
	}
}

func TestFFmpegSupportsHWAccel(t *testing.T) {
	cases := []struct {
		name      string
		available []string
		want      string
		expected  bool
	}{
		{"empty want returns true", []string{"vdpau"}, "", true},
		{"none want returns true", []string{"vdpau"}, "none", true},
		{"auto want returns true", []string{"vdpau"}, "auto", true},
		{"match exact", []string{"vdpau", "cuda"}, "cuda", true},
		{"match case insensitive", []string{"CUDA"}, "cuda", true},
		{"no match — common static ffmpeg", []string{"vdpau"}, "cuda", false},
		{"empty available", []string{}, "cuda", false},
		{"vaapi present", []string{"vaapi", "qsv"}, "vaapi", true},
		{"qsv missing", []string{"vdpau"}, "qsv", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ffmpegSupportsHWAccel(tc.available, tc.want)
			if got != tc.expected {
				t.Fatalf("ffmpegSupportsHWAccel(%v, %q) = %v, want %v", tc.available, tc.want, got, tc.expected)
			}
		})
	}
}

func TestLooksLikeURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"https://youtu.be/abc", true},
		{"http://example.com/video.mp4", true},
		{"HTTPS://YOUTUBE.COM/watch?v=x", true},
		{"  https://leading-space.com  ", true},
		{"/local/path/video.mp4", false},
		{"", false},
		{"recent", false},
		{"C:\\Users\\foo\\bar.mp4", false},
		{"ftp://nope.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := looksLikeURL(tc.in); got != tc.want {
				t.Fatalf("looksLikeURL(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestExistingTranscriptArtifacts(t *testing.T) {
	dir := t.TempDir()
	if _, _, ok := existingTranscriptArtifacts(dir); ok {
		t.Fatal("empty dir should not report existing transcripts")
	}

	txt := filepath.Join(dir, "transcript.txt")
	jsn := filepath.Join(dir, "transcript.json")
	if err := os.WriteFile(txt, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := existingTranscriptArtifacts(dir); ok {
		t.Fatal("only txt present should not count as complete")
	}
	if err := os.WriteFile(jsn, []byte(`{"text":"hello"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gotTxt, gotJSON, ok := existingTranscriptArtifacts(dir)
	if !ok {
		t.Fatal("both files should report existing transcripts")
	}
	if gotTxt != txt || gotJSON != jsn {
		t.Fatalf("returned wrong paths: %q / %q", gotTxt, gotJSON)
	}

	// Empty file should not count
	if err := os.WriteFile(txt, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := existingTranscriptArtifacts(dir); ok {
		t.Fatal("empty txt should disqualify the pair")
	}
}

func TestParseDurationWithDays(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"30d", 30 * 24 * time.Hour, false},
		{"7d12h", 7*24*time.Hour + 12*time.Hour, false},
		{"2h", 2 * time.Hour, false},
		{"2h30m", 2*time.Hour + 30*time.Minute, false},
		{"", 0, true},
		{"abc", 0, true},
		{"d", 0, true}, // empty days prefix
	}
	for _, tc := range cases {
		got, err := parseDurationWithDays(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%q: err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
		if err == nil && got != tc.want {
			t.Fatalf("%q: got %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{2 * 1024 * 1024, "2.0 MB"},
		{int64(3.5 * 1024 * 1024 * 1024), "3.50 GB"},
	}
	for _, tc := range cases {
		if got := humanSize(tc.in); got != tc.want {
			t.Fatalf("humanSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
