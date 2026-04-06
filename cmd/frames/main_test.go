package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
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
	choices := []string{"safe", "balanced", "fast"}
	if got := rotateChoice(choices, "balanced", 1); got != "fast" {
		t.Fatalf("expected fast, got %s", got)
	}
	if got := rotateChoice(choices, "safe", -1); got != "fast" {
		t.Fatalf("expected wrap to fast, got %s", got)
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
	info := media.VideoInfo{DurationSec: 60, Width: 1920, Height: 1080}
	out := estimateOutputs(info, 4, "jpg", "both")
	if out.FrameCount <= 0 || out.EstimatedMB <= 0 {
		t.Fatalf("unexpected estimate: %+v", out)
	}
}

func TestStageTimingSuffixIncludesETA(t *testing.T) {
	started := time.Now().Add(-30 * time.Second)
	out := stageTimingSuffix(started, 50)
	if !strings.Contains(out, "elapsed") || !strings.Contains(out, "eta") {
		t.Fatalf("expected elapsed and eta, got %q", out)
	}
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
	run := filepath.Join(root, "Run_1")
	if err := os.MkdirAll(filepath.Join(run, "images", "sheets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(run, "voice"), 0o755); err != nil {
		t.Fatal(err)
	}
	for p := range map[string]string{
		filepath.Join(run, "images", "sheets", "contact-sheet.png"): "x",
		filepath.Join(run, "voice", "transcript.txt"):               "x",
		filepath.Join(run, "extract.log"):                           "x",
		filepath.Join(run, "voice", "voice.wav"):                    "x",
		media.MetadataPathForRun(run):                               "{}",
	} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	arts, err := latestRunArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	if arts["run"] == "" || arts["sheet"] == "" || arts["transcript"] == "" {
		t.Fatalf("expected latest artifacts map, got %+v", arts)
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
