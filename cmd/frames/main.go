package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	appconfig "github.com/wraelen/framescli/internal/config"
	"github.com/wraelen/framescli/internal/media"
)

var appCfg appconfig.Config
var commandFailed bool
var commandInterrupted bool

func markCommandFailure() {
	commandFailed = true
}

func markCommandInterrupted() {
	commandInterrupted = true
	commandFailed = true
}

func failf(format string, args ...any) {
	markCommandFailure()
	fmt.Fprintf(os.Stderr, format, args...)
}

func failln(msg string) {
	markCommandFailure()
	fmt.Fprintln(os.Stderr, msg)
}

func renderProgressBar(label string, percent float64) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	const width = 28
	filled := int((percent / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	if isStdoutTTY() {
		fmt.Printf("\r%s [%s] %6.2f%%", label, bar, percent)
	} else {
		fmt.Printf("%s [%s] %6.2f%%\n", label, bar, percent)
	}
}

func renderStageProgress(stage string, percent float64, suffix string) {
	label := "  " + strings.TrimSpace(stage)
	if label == "  " {
		label = "  processing"
	}
	if strings.TrimSpace(suffix) != "" {
		label += " (" + strings.TrimSpace(suffix) + ")"
	}
	renderProgressBar(label, percent)
}

type stageProgressTracker struct {
	started time.Time
	// Non-TTY dedup: piped output can't use \r overwrite, so we must print each
	// change on its own line. Without dedup, ffmpeg-driven progressFn calls at
	// the same percent (plus explicit stage transitions that also land on
	// percent=70/85/etc.) flood stdout with duplicate lines. Emit only when
	// the (stage, bucketed-percent) pair changes.
	lastStage string
	lastPct   int
}

func newStageProgressTracker(started time.Time) *stageProgressTracker {
	if started.IsZero() {
		started = time.Now()
	}
	return &stageProgressTracker{started: started, lastPct: -1}
}

func (t *stageProgressTracker) render(stage string, percent float64) {
	if !isStdoutTTY() {
		bucket := int(percent)
		if stage == t.lastStage && bucket == t.lastPct {
			return
		}
		t.lastStage = stage
		t.lastPct = bucket
	}
	renderStageProgress(stage, percent, stageTimingSuffix(t.started, percent))
}

func stageTimingSuffix(started time.Time, percent float64) string {
	if started.IsZero() {
		return ""
	}
	elapsed := time.Since(started)
	if elapsed < 0 {
		elapsed = 0
	}
	if percent <= 0 || percent >= 100 {
		return "elapsed " + formatClockDuration(elapsed)
	}
	eta := time.Duration((float64(elapsed) * (100.0 - percent)) / percent)
	if eta < 0 {
		eta = 0
	}
	return fmt.Sprintf("elapsed %s, eta %s", formatClockDuration(elapsed), formatClockDuration(eta))
}

func formatClockDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(math.Round(d.Seconds()))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func printTranscribeHeader(backend, model string, chunkSec int, durationSec float64, accel transcribeAccel) {
	backend = strings.TrimSpace(backend)
	if backend == "" {
		backend = "auto"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "base"
	}
	speedHint := "CPU path — runtime can approach clip length"
	if accel.UsesGPU {
		speedHint = "GPU-accelerated transcription — typically several× realtime"
	}
	fmt.Printf("  transcribe: starting (%s)\n", speedHint)
	if reason := strings.TrimSpace(accel.Reason); reason != "" {
		fmt.Printf("    accel: %s\n", reason)
	}
	fmt.Printf("    backend=%s · model=%s", backend, model)
	if chunkSec > 0 && durationSec > 0 {
		if durationSec <= float64(chunkSec)*media.SingleShotChunkOverhangRatio {
			fmt.Printf(" · single-shot (clip %.0fs fits in one %ds chunk)", durationSec, chunkSec)
		} else {
			chunks := int(math.Ceil(durationSec / float64(chunkSec)))
			if chunks > 0 {
				fmt.Printf(" · chunks=%d (~%ds each)", chunks, chunkSec)
			}
		}
	}
	fmt.Println()
	if !accel.UsesGPU {
		whisperPath, _ := exec.LookPath("whisper")
		fasterWhisperPath, _ := exec.LookPath("faster-whisper")
		if whisperPath != "" && fasterWhisperPath == "" {
			fmt.Println("    tip: install faster-whisper for 3-5x speedup on CPU (pip install faster-whisper)")
		}
	}
}

type transcribeProgressRenderer struct {
	label string
	mu    sync.Mutex
	stage string
	pct   float64
}

func newTranscribeProgressRenderer(label string) *transcribeProgressRenderer {
	return &transcribeProgressRenderer{label: label, stage: "starting"}
}

// isStdoutTTY reports whether stdout is a terminal. Non-TTY (pipes, CI, file
// redirect) can't interpret carriage returns, so the renderer falls back to
// newline-per-update cadence to avoid spraying hundreds of unrendered CR lines.
func isStdoutTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// run executes fn while animating a spinner with progress updates from the
// callback. Only one goroutine writes to stdout, so spinner ticks and progress
// updates never collide. The callback is goroutine-safe and idempotent.
// On a TTY: carriage-return animated at 200ms. Off a TTY (pipe, log, CI): one
// newline update every 5s so piped output stays readable and informative.
func (r *transcribeProgressRenderer) run(fn func(progressFn func(stage string, pct float64)) error) error {
	tty := isStdoutTTY()
	tickInterval := 200 * time.Millisecond
	if !tty {
		tickInterval = 5 * time.Second
	}
	spinner := []rune{'|', '/', '-', '\\'}
	done := make(chan struct{})
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	progressFn := func(stage string, pct float64) {
		r.mu.Lock()
		r.stage = stage
		if pct >= 0 {
			r.pct = pct
		}
		r.mu.Unlock()
	}

	var runErr error
	go func() {
		runErr = fn(progressFn)
		close(done)
	}()

	i := 0
	var lastNonTTYStage string
	var lastNonTTYPct float64 = -1
	for {
		select {
		case <-done:
			r.mu.Lock()
			finalStage := r.stage
			r.mu.Unlock()
			if tty {
				if runErr == nil {
					fmt.Printf("\r%s [ok] %s                              \n", r.label, finalStage)
				} else {
					fmt.Printf("\r%s [x] failed                              \n", r.label)
				}
			} else {
				if runErr == nil {
					fmt.Printf("%s [ok] %s\n", r.label, finalStage)
				} else {
					fmt.Printf("%s [x] failed\n", r.label)
				}
			}
			return runErr
		case <-ticker.C:
			r.mu.Lock()
			stage, pct := r.stage, r.pct
			r.mu.Unlock()
			if tty {
				fmt.Printf("\r%s [%c] %s (%.0f%%)         ", r.label, spinner[i%len(spinner)], stage, pct*100)
				i++
			} else if stage != lastNonTTYStage || pct != lastNonTTYPct {
				fmt.Printf("%s %s (%.0f%%)\n", r.label, stage, pct*100)
				lastNonTTYStage = stage
				lastNonTTYPct = pct
			}
		}
	}
}

func runSpinner(label string, fn func() error) error {
	spinner := []rune{'|', '/', '-', '\\'}
	done := make(chan struct{})
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	var runErr error
	go func() {
		runErr = fn()
		close(done)
	}()

	i := 0
	for {
		select {
		case <-done:
			if runErr == nil {
				fmt.Printf("\r%s [ok] done\n", label)
			} else {
				fmt.Printf("\r%s [x] failed\n", label)
			}
			return runErr
		case <-ticker.C:
			fmt.Printf("\r%s [%c]", label, spinner[i%len(spinner)])
			i++
		}
	}
}

func main() {
	cfg, err := appconfig.LoadOrDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config load warning: %v\n", err)
		cfg = appconfig.Default()
	}
	appCfg = cfg
	appconfig.ApplyEnvDefaults(appCfg)

	// Auto-detect GPU and set hwaccel if not explicitly configured. Only
	// recommend a GPU accel that the local ffmpeg can actually use — many
	// static ffmpeg builds (including the johnvansickle build commonly used
	// on Linux) lack CUDA/NVENC, so blindly setting hwaccel=cuda would just
	// trigger the "cuda failed; falling back to CPU" warning on every run.
	if appCfg.HWAccel == "" || appCfg.HWAccel == "none" {
		gpu := detectGPU()
		if gpu.Available {
			ffmpegAccels, _ := media.FFmpegHWAccels()
			if ffmpegSupportsHWAccel(ffmpegAccels, gpu.RecommendedHWAccel) {
				appCfg.HWAccel = gpu.RecommendedHWAccel
			}
		}
	}

	root := &cobra.Command{
		Use:          "framescli",
		Short:        "FramesCLI - extract frame timelines from coding recordings",
		SilenceUsage: true,
		Version:      versionString(),
	}
	root.SetVersionTemplate("framescli {{.Version}}\n")

	root.AddCommand(extractCommand())
	root.AddCommand(extractBatchCommand())
	root.AddCommand(importDeprecatedCommand())
	root.AddCommand(previewCommand())
	root.AddCommand(openLastCommand())
	root.AddCommand(copyLastCommand())
	root.AddCommand(artifactsCommand())
	root.AddCommand(mcpCommand())
	root.AddCommand(sheetCommand())
	root.AddCommand(cleanCommand())
	root.AddCommand(transcribeCommand())
	root.AddCommand(transcribeRunCommand())
	root.AddCommand(doctorCommand())
	root.AddCommand(indexCommand())
	root.AddCommand(benchmarkCommand())
	root.AddCommand(telemetryCommand())
	root.AddCommand(setupCommand())
	root.AddCommand(configCommand())

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := root.ExecuteContext(signalCtx); err != nil {
		if isCancellationError(err) || errors.Is(signalCtx.Err(), context.Canceled) {
			fmt.Fprintln(os.Stderr, "Interrupted.")
			os.Exit(130)
		}
		os.Exit(1)
	}
	if commandInterrupted {
		os.Exit(130)
	}
	if commandFailed {
		os.Exit(1)
	}
}

type extractWorkflowOptions struct {
	URL                string
	NoCache            bool
	OutDir             string
	Voice              bool
	FrameFormat        string
	FormatExplicit     bool
	JPGQuality         int
	NoSheet            bool
	SheetCols          int
	Verbose            bool
	HWAccel            string
	Preset             string
	PresetExplicit     bool
	FPSExplicit        bool
	StartTime          string
	EndTime            string
	StartFrame         int
	EndFrame           int
	EveryN             int
	FrameName          string
	ZipFrames          bool
	LogFile            string
	MetadataCSV        bool
	AudioFormat        string
	AudioBitrate       string
	NormalizeAudio     bool
	AudioStart         string
	AudioEnd           string
	TranscribeModel    string
	TranscribeBackend  string
	TranscribeBin      string
	TranscribeLanguage string
	ChunkDurationSec   int
	ChunkExplicit      bool
	TranscribeTimeout  int
	AllowExpensive     bool
	PostHook           string
	PostHookTimeout    time.Duration
	Force              bool
}

type extractWorkflowResult struct {
	VideoInput       string              `json:"video_input"`
	ResolvedVideo    string              `json:"resolved_video"`
	SourceNote       string              `json:"source_note,omitempty"`
	VideoInfo        media.VideoInfo     `json:"video_info"`
	FPS              float64             `json:"fps"`
	FrameFormat      string              `json:"frame_format"`
	OutDir           string              `json:"out_dir"`
	FrameCount       int                 `json:"frame_count"`
	HWAccel          string              `json:"hwaccel"`
	Preset           string              `json:"preset"`
	MediaPreset      string              `json:"media_preset,omitempty"`
	ChunkDurationSec int                 `json:"chunk_duration_sec,omitempty"`
	MetadataPath     string              `json:"metadata_path,omitempty"`
	MetadataCSVPath  string              `json:"metadata_csv_path,omitempty"`
	FramesZipPath    string              `json:"frames_zip_path,omitempty"`
	ContactSheet     string              `json:"contact_sheet,omitempty"`
	TranscriptTxt    string              `json:"transcript_txt,omitempty"`
	TranscriptJSON   string              `json:"transcript_json,omitempty"`
	AudioPath        string              `json:"audio_path,omitempty"`
	Guardrails       []workloadGuardrail `json:"guardrails,omitempty"`
	Warnings         []string            `json:"warnings,omitempty"`
	ElapsedMs        int64               `json:"elapsed_ms"`
	Artifacts        map[string]string   `json:"artifacts,omitempty"`
}

type workflowPresetDefinition struct {
	Name             string
	DefaultFPS       float64
	DefaultFormat    string
	MediaPreset      string
	ChunkDurationSec int
	Description      string
	Legacy           bool
}

type resolvedWorkflowSettings struct {
	Preset           string  `json:"preset"`
	MediaPreset      string  `json:"media_preset"`
	FPS              float64 `json:"fps"`
	FPSMode          string  `json:"fps_mode,omitempty"`
	FrameFormat      string  `json:"frame_format"`
	ChunkDurationSec int     `json:"chunk_duration_sec,omitempty"`
}

type workloadGuardrail struct {
	ID        string `json:"id"`
	Severity  string `json:"severity"`
	Metric    string `json:"metric"`
	Actual    string `json:"actual"`
	Threshold string `json:"threshold"`
	Message   string `json:"message"`
}

type workloadAssessment struct {
	RequiresOverride bool                `json:"requires_override,omitempty"`
	OverrideFlag     string              `json:"override_flag,omitempty"`
	Guardrails       []workloadGuardrail `json:"guardrails,omitempty"`
}

type automationError struct {
	Code      string `json:"code"`
	Class     string `json:"class,omitempty"`
	Message   string `json:"message"`
	Recovery  string `json:"recovery,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

type automationEnvelope struct {
	SchemaVersion string           `json:"schema_version"`
	Command       string           `json:"command"`
	Status        string           `json:"status"`
	StartedAt     string           `json:"started_at"`
	EndedAt       string           `json:"ended_at"`
	DurationMs    int64            `json:"duration_ms"`
	Data          any              `json:"data,omitempty"`
	Error         *automationError `json:"error,omitempty"`
}

type transcribeRunResult struct {
	RunDir          string `json:"run_dir"`
	AudioPath       string `json:"audio_path"`
	TranscriptTxt   string `json:"transcript_txt,omitempty"`
	TranscriptJSON  string `json:"transcript_json,omitempty"`
	ManifestPath    string `json:"manifest_path,omitempty"`
	Chunked         bool   `json:"chunked,omitempty"`
	TotalChunks     int    `json:"total_chunks,omitempty"`
	CompletedChunks int    `json:"completed_chunks,omitempty"`
	AlreadyPresent  bool   `json:"already_present,omitempty"`
}

type batchItemResult struct {
	Input  string                 `json:"input"`
	Status string                 `json:"status"`
	Result *extractWorkflowResult `json:"result,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

type telemetryEvent struct {
	TimestampUTC  string            `json:"timestamp_utc"`
	Command       string            `json:"command"`
	Status        string            `json:"status"`
	DurationMs    int64             `json:"duration_ms"`
	Input         string            `json:"input,omitempty"`
	ResolvedInput string            `json:"resolved_input,omitempty"`
	OutDir        string            `json:"out_dir,omitempty"`
	FPS           float64           `json:"fps,omitempty"`
	FrameFormat   string            `json:"frame_format,omitempty"`
	HWAccel       string            `json:"hwaccel,omitempty"`
	Preset        string            `json:"preset,omitempty"`
	Voice         bool              `json:"voice,omitempty"`
	FrameCount    int               `json:"frame_count,omitempty"`
	Artifacts     map[string]string `json:"artifacts,omitempty"`
	Error         string            `json:"error,omitempty"`
	GOOS          string            `json:"goos"`
	GOARCH        string            `json:"goarch"`
}

const automationSchemaVersion = "framescli.v1"

// version, commit, and date are populated by goreleaser via -ldflags at build
// time. Default values reflect "built from source without ldflags" (typical
// `go build` or `make build`).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func versionString() string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = "dev"
	}
	if commit != "none" && commit != "" {
		short := commit
		if len(short) > 7 {
			short = short[:7]
		}
		v = v + " (" + short + ")"
	}
	return v
}

// existingTranscriptArtifacts reports whether a usable transcript pair already
// exists in voiceDir. Returns the txt path, json path, and true when both
// files are present and non-empty so the caller can short-circuit re-running
// whisper. The file naming follows the convention used by media.TranscribeAudio
// (transcript.txt + transcript.json, or voice.txt + voice.json depending on
// backend output).
func existingTranscriptArtifacts(voiceDir string) (string, string, bool) {
	candidatePairs := [][2]string{
		{filepath.Join(voiceDir, "transcript.txt"), filepath.Join(voiceDir, "transcript.json")},
		{filepath.Join(voiceDir, "voice.txt"), filepath.Join(voiceDir, "voice.json")},
	}
	for _, pair := range candidatePairs {
		txtInfo, err1 := os.Stat(pair[0])
		jsonInfo, err2 := os.Stat(pair[1])
		if err1 == nil && err2 == nil && txtInfo.Size() > 0 && jsonInfo.Size() > 0 {
			return pair[0], pair[1], true
		}
	}
	return "", "", false
}

// ffmpegSupportsHWAccel reports whether ffmpeg's reported hwaccel list
// contains the requested accel. Used by doctor to flag the (common!)
// situation where a GPU is present but the local ffmpeg build was compiled
// without GPU support, so any --hwaccel cuda call would fall back to CPU.
func ffmpegSupportsHWAccel(available []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" || want == "none" || want == "auto" {
		return true
	}
	for _, hw := range available {
		if strings.EqualFold(strings.TrimSpace(hw), want) {
			return true
		}
	}
	return false
}

func looksLikeURL(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

type errorDetails struct {
	Class     string         `json:"class,omitempty"`
	Message   string         `json:"message"`
	Recovery  string         `json:"recovery,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

func classifyError(err error) errorDetails {
	if err == nil {
		return errorDetails{}
	}

	msg := err.Error()
	lower := strings.ToLower(msg)
	details := errorDetails{
		Class:   "command_failed",
		Message: msg,
	}

	switch {
	case errors.Is(err, context.Canceled):
		details.Class = "interrupted"
		details.Recovery = "Re-run the command. Existing output remains on disk."
		details.Retryable = true
	case errors.Is(err, context.DeadlineExceeded):
		details.Class = "interrupted"
		details.Recovery = "Increase the timeout or re-run the command."
		details.Retryable = true
	case strings.Contains(lower, "timed out"):
		details.Class = "interrupted"
		details.Recovery = "Increase the timeout or re-run the command."
		details.Retryable = true
	case strings.Contains(lower, "cancelled"),
		strings.Contains(lower, "canceled"),
		strings.Contains(lower, "interrupted"):
		details.Class = "interrupted"
		details.Recovery = "Re-run the command. Existing output remains on disk."
		details.Retryable = true
	case strings.Contains(lower, "no space left on device"),
		strings.Contains(lower, "insufficient disk space"),
		strings.Contains(lower, "file too large"),
		strings.Contains(lower, "file size limit exceeded"):
		details.Class = "disk_space"
		details.Recovery = "Free disk space, choose a different output path, or reduce output size before retrying."
		details.Retryable = true
	case strings.Contains(lower, "not found on path"),
		strings.Contains(lower, "executable file not found"),
		strings.Contains(lower, "install faster-whisper or whisper"),
		strings.Contains(lower, "missing required tools"),
		strings.Contains(lower, "folder picker unavailable"),
		strings.Contains(lower, "no clipboard tool found"):
		details.Class = "missing_dependency"
		details.Recovery = "Install the missing dependency, confirm it is on PATH, then retry."
		details.Retryable = true
	case strings.Contains(lower, "file not found:"),
		strings.Contains(lower, "path is a directory"):
		details.Class = "file_not_found"
		details.Recovery = "Check the path is correct, or use 'recent' for the most recently modified video in your input dirs."
	case strings.Contains(lower, "unsupported"),
		strings.Contains(lower, "ffprobe failed for"),
		strings.Contains(lower, "video probe failed"),
		strings.Contains(lower, "invalid video input"),
		strings.Contains(lower, "video has no audio stream"),
		strings.Contains(lower, "invalid or unsupported video stream"),
		strings.Contains(lower, "no transcript json found"):
		details.Class = "unsupported_media"
		details.Recovery = "Use a supported, readable media file or output format, then retry."
	case strings.Contains(lower, "path outside allowed roots"),
		strings.Contains(lower, "only local filesystem paths are allowed"),
		strings.Contains(lower, "must be local filesystem paths"),
		strings.Contains(lower, "must be a local filesystem path"):
		details.Class = "path_not_allowed"
		details.Recovery = "Use a local path inside the configured allowed roots."
	case strings.Contains(lower, "requires "),
		strings.Contains(lower, "path is required"),
		strings.Contains(lower, "video path is required"),
		strings.Contains(lower, "audio path is required"),
		strings.Contains(lower, "output directory is required"),
		strings.Contains(lower, "invalid "),
		strings.Contains(lower, "must be"),
		strings.Contains(lower, "cannot be"),
		strings.Contains(lower, "failed to resolve video input"),
		strings.Contains(lower, "no video directories configured"):
		details.Class = "invalid_input"
		details.Recovery = "Fix the command input or configuration and retry."
	case strings.Contains(lower, "not found"),
		strings.Contains(lower, "no runs found"),
		strings.Contains(lower, "no recent videos found"):
		details.Class = "not_found"
		details.Recovery = "Check the input path or run directory and retry."
	}

	details.Data = map[string]any{
		"class":     details.Class,
		"recovery":  details.Recovery,
		"retryable": details.Retryable,
	}
	return details
}

func renderUserFacingError(err error) string {
	if err == nil {
		return ""
	}
	details := classifyError(err)
	if strings.TrimSpace(details.Recovery) == "" {
		return details.Message
	}
	return details.Message + "\nRecovery: " + details.Recovery
}

func buildMCPError(code int, err error) *mcpError {
	if err == nil {
		return nil
	}
	details := classifyError(err)
	return &mcpError{
		Code:    code,
		Message: details.Message,
		Data:    details.Data,
	}
}

func buildAutomationEnvelope(command string, started time.Time, status string, data any, err error) automationEnvelope {
	env := automationEnvelope{
		SchemaVersion: automationSchemaVersion,
		Command:       command,
		Status:        status,
		StartedAt:     started.UTC().Format(time.RFC3339),
		EndedAt:       time.Now().UTC().Format(time.RFC3339),
		DurationMs:    time.Since(started).Milliseconds(),
		Data:          data,
	}
	if err != nil {
		details := classifyError(err)
		env.Error = &automationError{
			Code:      "command_failed",
			Class:     details.Class,
			Message:   details.Message,
			Recovery:  details.Recovery,
			Retryable: details.Retryable,
		}
	}
	return env
}

func emitAutomationJSON(command string, started time.Time, status string, data any, err error) {
	env := buildAutomationEnvelope(command, started, status, data, err)
	raw, marshalErr := json.MarshalIndent(env, "", "  ")
	if marshalErr != nil {
		failf("failed to marshal json response: %v\n", marshalErr)
		return
	}
	fmt.Println(string(raw))
}

func runExtractWorkflow(ctx context.Context, videoInput string, fps float64, opts extractWorkflowOptions) error {
	_, err := runExtractWorkflowResult(ctx, videoInput, fps, opts, true)
	return err
}

func isCancellationError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func normalizeCancellationError(ctx context.Context, err error) error {
	if isCancellationError(err) {
		return err
	}
	if ctx == nil {
		return nil
	}
	if isCancellationError(ctx.Err()) {
		return ctx.Err()
	}
	return nil
}

func annotateStorageError(err error) error {
	if err == nil {
		return nil
	}
	lower := strings.ToLower(err.Error())
	if errors.Is(err, syscall.ENOSPC) ||
		errors.Is(err, syscall.EFBIG) ||
		strings.Contains(lower, "no space left on device") ||
		strings.Contains(lower, "file too large") ||
		strings.Contains(lower, "file size limit exceeded") {
		return fmt.Errorf("insufficient disk space or file-size limit: %w", err)
	}
	return err
}

func resolveRunAudioPath(runDir string, md media.RunMetadata) (string, error) {
	audioPath := strings.TrimSpace(md.AudioPath)
	defaultAudioPath := filepath.Join(runDir, "voice", "voice.wav")
	if audioPath == "" {
		audioPath = defaultAudioPath
	} else if !filepath.IsAbs(audioPath) {
		audioPath = filepath.Join(runDir, audioPath)
	}
	audioPath, err := filepath.Abs(audioPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(audioPath); err == nil {
		return audioPath, nil
	}
	defaultAudioPath, err = filepath.Abs(defaultAudioPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(defaultAudioPath); err == nil {
		return defaultAudioPath, nil
	}
	return "", fmt.Errorf("no extracted audio found in %s", filepath.Join(runDir, "voice"))
}

func runTranscribeRunResult(ctx context.Context, runDir string, opts media.TranscribeOptions) (transcribeRunResult, error) {
	runDir, err := filepath.Abs(runDir)
	if err != nil {
		return transcribeRunResult{}, err
	}
	md, err := media.ReadRunMetadata(runDir)
	if err != nil {
		return transcribeRunResult{}, err
	}
	audioPath, err := resolveRunAudioPath(runDir, md)
	if err != nil {
		return transcribeRunResult{}, err
	}

	voiceDir := filepath.Join(runDir, "voice")
	transcriptJSON := filepath.Join(voiceDir, "transcript.json")
	transcriptTxt := filepath.Join(voiceDir, "transcript.txt")
	manifestPath := filepath.Join(voiceDir, "transcription-manifest.json")
	result := transcribeRunResult{
		RunDir:    runDir,
		AudioPath: audioPath,
	}
	manifestComplete := false
	if _, err := os.Stat(manifestPath); err == nil {
		result.ManifestPath = manifestPath
		manifestComplete, err = media.IsTranscriptionManifestComplete(manifestPath)
		if err != nil {
			return result, err
		}
	}
	if _, err := os.Stat(transcriptJSON); err == nil {
		result.TranscriptJSON = transcriptJSON
		if _, txtErr := os.Stat(transcriptTxt); txtErr == nil {
			result.TranscriptTxt = transcriptTxt
		}
		if result.ManifestPath == "" || manifestComplete {
			result.AlreadyPresent = true
			return result, nil
		}
	}

	opts.Context = ctx
	opts.AudioPath = audioPath
	opts.OutDir = voiceDir
	transcribeResult, err := media.TranscribeAudioWithDetails(opts)
	if err != nil {
		return result, err
	}
	result.TranscriptTxt = transcribeResult.TranscriptTxt
	result.TranscriptJSON = transcribeResult.TranscriptJSON
	result.ManifestPath = transcribeResult.ManifestPath
	result.Chunked = transcribeResult.Chunked
	result.TotalChunks = transcribeResult.TotalChunks
	result.CompletedChunks = transcribeResult.CompletedChunks
	_ = refreshRunsIndexForRun(result.RunDir)
	return result, nil
}

func runExtractWorkflowResult(ctx context.Context, videoInput string, fps float64, opts extractWorkflowOptions, interactive bool) (extractWorkflowResult, error) {
	started := time.Now()
	result := extractWorkflowResult{
		VideoInput: videoInput,
		Preset:     opts.Preset,
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.SheetCols <= 0 {
		return result, fmt.Errorf("invalid sheet-cols value. use a positive number")
	}

	// Handle URL download if --url was provided
	downloadNote := ""
	sourceType := "file"
	sourceURL := ""
	sourceTitle := ""
	if opts.URL != "" {
		if interactive {
			fmt.Println("[1/5] Downloading video from URL")
		}
		// Use default cache dir: ~/.cache/framescli/videos
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return result, fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir := filepath.Join(homeDir, ".cache", "framescli", "videos")
		downloadOpts := media.DownloadOptions{
			CacheDir: cacheDir,
			UseCache: !opts.NoCache,
			Context:  ctx,
		}
		if interactive {
			downloadOpts.ProgressWriter = os.Stdout
		}
		downloadResult, err := media.DownloadVideo(opts.URL, downloadOpts)
		if err != nil {
			return result, fmt.Errorf("failed to download video: %w", err)
		}
		videoInput = downloadResult.LocalPath
		sourceType = "url"
		sourceURL = opts.URL
		sourceTitle = downloadResult.Metadata.Title
		if downloadResult.WasCached {
			downloadNote = fmt.Sprintf("cached from %s", opts.URL)
			if interactive {
				fmt.Printf("  using cached video\n")
			}
		} else {
			downloadNote = fmt.Sprintf("downloaded from %s", opts.URL)
			if interactive {
				fmt.Printf("  downloaded: %s\n", downloadResult.Metadata.Title)
			}
		}
	}

	if interactive {
		if opts.URL != "" {
			fmt.Println("[2/5] Resolving video input")
		} else {
			fmt.Println("[1/4] Resolving video input")
		}
	}
	resolvedPath, note, err := resolveVideoInput(videoInput)
	if err != nil {
		return result, fmt.Errorf("failed to resolve video input: %w", err)
	}
	videoPath := resolvedPath
	result.ResolvedVideo = videoPath
	// Combine download note with resolution note
	if downloadNote != "" {
		if note != "" {
			result.SourceNote = downloadNote + " (" + note + ")"
		} else {
			result.SourceNote = downloadNote
		}
	} else {
		result.SourceNote = note
	}
	if interactive && result.SourceNote != "" {
		fmt.Printf("  source: %s\n", result.SourceNote)
	}
	videoInfo, err := media.ProbeVideoInfo(videoPath)
	if err != nil {
		// File-not-found errors already say "file not found: <path>" — don't
		// wrap them in "invalid video input" since that obscures the real
		// problem.
		if strings.HasPrefix(err.Error(), "file not found:") || strings.HasPrefix(err.Error(), "path is a directory") {
			return result, err
		}
		return result, fmt.Errorf("invalid video input %q: %w", videoPath, err)
	}
	resolved := resolveWorkflowSettings(videoInfo, fps, opts.FrameFormat, opts.Preset, opts.FPSExplicit, opts.FormatExplicit, opts.PresetExplicit, opts.Voice, opts.ChunkDurationSec, opts.ChunkExplicit)
	transcribeAccelInfo := probeTranscribeAccel(appCfg, opts.TranscribeBackend, doctorHasGPU())
	transcriptPlan := buildTranscriptPlanWithSettings(videoInfo.DurationSec, opts.TranscribeBackend, opts.TranscribeModel, appCfg, transcribeAccelInfo.UsesGPU)
	previewMode := "frames"
	if opts.Voice {
		previewMode = "both"
	}
	estimate := estimateOutputs(videoInfo, resolved.FPS, resolved.FrameFormat, previewMode, transcriptPlan, resolved.ChunkDurationSec)
	result.FPS = resolved.FPS
	result.VideoInfo = videoInfo
	result.Preset = resolved.Preset
	result.MediaPreset = resolved.MediaPreset
	result.FrameFormat = resolved.FrameFormat
	result.ChunkDurationSec = resolved.ChunkDurationSec
	result.Guardrails = estimate.Guardrails.Guardrails
	for _, guardrail := range estimate.Guardrails.Guardrails {
		if guardrail.Severity == "warn" {
			result.Warnings = append(result.Warnings, guardrail.Message)
		}
	}
	if err := enforceGuardrails(estimate.Guardrails, opts.AllowExpensive); err != nil {
		return result, err
	}
	if interactive {
		fmt.Printf("  video: %s\n", videoPath)
		fmt.Printf("  duration: %.2fs\n", videoInfo.DurationSec)
		fmt.Printf("  resolution: %dx%d\n", videoInfo.Width, videoInfo.Height)
		fmt.Printf("  source fps: %.2f\n", videoInfo.FrameRate)
		if resolved.FPSMode == "auto" {
			fmt.Printf("  fps: %.2f (auto, computed from duration %.1fs), format: %s\n", resolved.FPS, videoInfo.DurationSec, resolved.FrameFormat)
		} else {
			fmt.Printf("  fps: %.2f, format: %s\n", resolved.FPS, resolved.FrameFormat)
		}
		fmt.Printf("  hwaccel: %s, preset: %s (media=%s)\n", opts.HWAccel, resolved.Preset, resolved.MediaPreset)
		if opts.Voice {
			// Only advertise chunking when it actually kicks in. For clips that
			// fit in one chunk (with the same overhang the transcriber uses),
			// the single-shot path is taken and the chunk value is irrelevant.
			willChunk := resolved.ChunkDurationSec > 0 &&
				videoInfo.DurationSec > float64(resolved.ChunkDurationSec)*media.SingleShotChunkOverhangRatio
			if willChunk {
				chunks := int(math.Ceil(videoInfo.DurationSec / float64(resolved.ChunkDurationSec)))
				fmt.Printf("  transcript chunking: %ds (%d chunks)\n", resolved.ChunkDurationSec, chunks)
			}
		}
	}
	if interactive && (strings.TrimSpace(opts.StartTime) != "" || strings.TrimSpace(opts.EndTime) != "") {
		fmt.Printf("  time range: %s -> %s\n", valueOrDash(opts.StartTime), valueOrDash(opts.EndTime))
	}
	if interactive && (opts.StartFrame > 0 || opts.EndFrame > 0) {
		fmt.Printf("  frame range: %d -> %d\n", opts.StartFrame, opts.EndFrame)
	}
	if interactive && opts.EveryN > 1 {
		fmt.Printf("  sampling: every %d source frames\n", opts.EveryN)
	}

	// Note: Pre-extraction hint removed since GPU is now auto-detected by default
	// Users only need hints if they explicitly disable GPU or if auto-detection failed

	if interactive {
		if opts.URL != "" {
			fmt.Println("[3/5] Extracting media")
		} else {
			fmt.Println("[2/4] Extracting media")
		}
	}
	var progressFn func(float64)
	progressTracker := newStageProgressTracker(started)
	if interactive && !opts.Verbose {
		progressFn = func(percent float64) {
			weighted := percent * 0.70
			progressTracker.render("extracting frames/audio", weighted)
		}
	}

	extractResult, err := media.ExtractMedia(media.ExtractMediaOptions{
		Context:      ctx,
		VideoPath:    videoPath,
		FPS:          resolved.FPS,
		FPSMode:      resolved.FPSMode,
		OutDir:       opts.OutDir,
		FramesRoot:   appCfg.FramesRoot,
		FrameFormat:  resolved.FrameFormat,
		JPGQuality:   opts.JPGQuality,
		HWAccel:      opts.HWAccel,
		Preset:       resolved.MediaPreset,
		ExtractAudio: opts.Voice,
		ProgressFn:   progressFn,
		Verbose:      opts.Verbose,
		StartTime:    opts.StartTime,
		EndTime:      opts.EndTime,
		StartFrame:   opts.StartFrame,
		EndFrame:     opts.EndFrame,
		EveryNFrames: opts.EveryN,
		FramePattern: opts.FrameName,
		ZipFrames:    opts.ZipFrames,
		LogFile:      opts.LogFile,
		MetadataCSV:  opts.MetadataCSV,
		AudioFormat:  opts.AudioFormat,
		AudioBitrate: opts.AudioBitrate,
		Normalize:    opts.NormalizeAudio,
		AudioStart:   opts.AudioStart,
		AudioEnd:     opts.AudioEnd,
		SourceType:   sourceType,
		SourceURL:    sourceURL,
		SourceTitle:  sourceTitle,
		Force:        opts.Force,
	})
	if err != nil {
		if interactive && !opts.Verbose {
			fmt.Println("")
		}
		if cerr := normalizeCancellationError(ctx, err); cerr != nil {
			return result, cerr
		}
		err = annotateStorageError(err)
		return result, fmt.Errorf("extraction failed: %w", err)
	}
	if interactive && !opts.Verbose {
		progressTracker.render("extracting frames/audio", 70)
		fmt.Println("")
	}
	if interactive {
		fmt.Printf("  output: %s\n", extractResult.OutDir)
	}
	result.OutDir = extractResult.OutDir
	result.FrameFormat = extractResult.FrameFormat
	result.HWAccel = extractResult.UsedHWAccel
	result.AudioPath = extractResult.AudioPath

	sheet := ""
	if opts.NoSheet {
		if interactive {
			if opts.URL != "" {
				fmt.Println("[4/5] Skipping contact sheet (--no-sheet)")
			} else {
				fmt.Println("[3/4] Skipping contact sheet (--no-sheet)")
			}
		}
		if interactive && !opts.Verbose {
			progressTracker.render("contact sheet skipped", 85)
			fmt.Println("")
		}
	} else {
		if interactive {
			if opts.URL != "" {
				fmt.Println("[4/5] Building contact sheet")
			} else {
				fmt.Println("[3/4] Building contact sheet")
			}
		}
		sheetsDir := filepath.Join(extractResult.ImagesDir, "sheets")
		if interactive && opts.Verbose {
			sheet, err = media.CreateContactSheetWithContext(ctx, extractResult.ImagesDir, opts.SheetCols, filepath.Join(sheetsDir, "contact-sheet.png"), true)
		} else if interactive {
			progressTracker.render("building contact sheet", 72)
			err = runSpinner("  contact-sheet", func() error {
				var spinnerErr error
				sheet, spinnerErr = media.CreateContactSheetWithContext(ctx, extractResult.ImagesDir, opts.SheetCols, filepath.Join(sheetsDir, "contact-sheet.png"), false)
				return spinnerErr
			})
		} else {
			sheet, err = media.CreateContactSheetWithContext(ctx, extractResult.ImagesDir, opts.SheetCols, filepath.Join(sheetsDir, "contact-sheet.png"), false)
		}
		if err != nil {
			if cerr := normalizeCancellationError(ctx, err); cerr != nil {
				return result, cerr
			}
			err = annotateStorageError(err)
			return result, fmt.Errorf("contact sheet failed: %w", err)
		}
		if interactive && !opts.Verbose {
			progressTracker.render("contact sheet complete", 85)
			fmt.Println("")
		}
		if interactive {
			fmt.Printf("  sheet: %s\n", sheet)
		}
	}
	result.ContactSheet = sheet

	transcriptTxt := ""
	transcriptJSON := ""
	transcriptionTimedOut := false
	if opts.Voice {
		if interactive {
			if opts.URL != "" {
				fmt.Println("[5/5] Transcribing voice")
			} else {
				fmt.Println("[4/4] Transcribing voice")
			}
		}
		voiceDir := filepath.Join(extractResult.OutDir, "voice")
		if extractResult.AudioPath == "" {
			return result, fmt.Errorf("voice extraction did not produce audio output")
		}
		if !opts.Force {
			if existingTxt, existingJSON, ok := existingTranscriptArtifacts(voiceDir); ok {
				transcriptTxt = existingTxt
				transcriptJSON = existingJSON
				if interactive {
					fmt.Printf("  transcribe: skipped (transcript already exists, use --force to re-run)\n")
				}
				result.Warnings = append(result.Warnings, "resumed: transcript already on disk; skipped whisper")
				goto transcriptDone
			}
		}
		if interactive {
			printTranscribeHeader(opts.TranscribeBackend, opts.TranscribeModel, resolved.ChunkDurationSec, videoInfo.DurationSec, transcribeAccelInfo)
		}
		if interactive && opts.Verbose {
			transcriptTxt, transcriptJSON, err = media.TranscribeAudioWithOptions(media.TranscribeOptions{
				Context:          ctx,
				AudioPath:        extractResult.AudioPath,
				OutDir:           voiceDir,
				Model:            opts.TranscribeModel,
				Backend:          opts.TranscribeBackend,
				Bin:              opts.TranscribeBin,
				Language:         opts.TranscribeLanguage,
				ChunkDurationSec: resolved.ChunkDurationSec,
				TimeoutSec:       opts.TranscribeTimeout,
				Verbose:          true,
				ProgressFn: func(stage string, pct float64) {
					fmt.Printf("  transcribe: %s (%.0f%%)\n", stage, pct*100)
				},
			})
		} else if interactive {
			progressTracker.render("transcribing voice", 88)
			renderer := newTranscribeProgressRenderer("  transcribe")
			err = renderer.run(func(progressFn func(stage string, pct float64)) error {
				var innerErr error
				transcriptTxt, transcriptJSON, innerErr = media.TranscribeAudioWithOptions(media.TranscribeOptions{
					Context:          ctx,
					AudioPath:        extractResult.AudioPath,
					OutDir:           voiceDir,
					Model:            opts.TranscribeModel,
					Backend:          opts.TranscribeBackend,
					Bin:              opts.TranscribeBin,
					Language:         opts.TranscribeLanguage,
					ChunkDurationSec: resolved.ChunkDurationSec,
					TimeoutSec:       opts.TranscribeTimeout,
					Verbose:          false,
					ProgressFn:       progressFn,
				})
				return innerErr
			})
		} else {
			transcriptTxt, transcriptJSON, err = media.TranscribeAudioWithOptions(media.TranscribeOptions{
				Context:          ctx,
				AudioPath:        extractResult.AudioPath,
				OutDir:           voiceDir,
				Model:            opts.TranscribeModel,
				Backend:          opts.TranscribeBackend,
				Bin:              opts.TranscribeBin,
				Language:         opts.TranscribeLanguage,
				ChunkDurationSec: resolved.ChunkDurationSec,
				TimeoutSec:       opts.TranscribeTimeout,
				Verbose:          false,
			})
		}
		if err != nil {
			if cerr := normalizeCancellationError(ctx, err); cerr != nil {
				return result, cerr
			}
			var timeoutErr *media.ErrTranscribeTimeout
			if errors.As(err, &timeoutErr) {
				warning := timeoutErr.Error()
				transcriptionTimedOut = true
				result.Warnings = append(result.Warnings, warning)
				if interactive && !opts.Verbose {
					progressTracker.render("transcription timed out", 98)
					fmt.Println("")
				}
				if interactive {
					fmt.Printf("Warning:       %s\n", warning)
				}
			} else {
				err = annotateStorageError(err)
				return result, fmt.Errorf("voice transcription failed: %w", err)
			}
		}
		if interactive && !opts.Verbose && !transcriptionTimedOut {
			progressTracker.render("transcription complete", 98)
			fmt.Println("")
		}
	transcriptDone:
	}
	result.TranscriptTxt = transcriptTxt
	result.TranscriptJSON = transcriptJSON

	frameCount, err := media.CountFrames(extractResult.ImagesDir)
	if err != nil {
		return result, fmt.Errorf("failed to count frames: %w", err)
	}
	result.FrameCount = frameCount

	if interactive && !opts.Verbose {
		progressTracker.render("finalizing", 100)
		fmt.Println("")
	}
	if interactive {
		fmt.Println("")
		fmt.Println("Summary")
		fmt.Println("-------")
		fmt.Printf("Frames:        %d\n", frameCount)
		fmt.Printf("FPS:           %.2f\n", resolved.FPS)
		fmt.Printf("Format:        %s\n", extractResult.FrameFormat)
		fmt.Printf("HWAccel:       %s\n", extractResult.UsedHWAccel)
		fmt.Printf("Preset:        %s\n", resolved.Preset)
		fmt.Printf("Media Preset:  %s\n", resolved.MediaPreset)
		fmt.Printf("Output:        %s\n", extractResult.OutDir)
	}
	if extractResult.MetadataPath != "" {
		if interactive {
			fmt.Printf("Metadata:      %s\n", extractResult.MetadataPath)
		}
	}
	if extractResult.CSVPath != "" {
		if interactive {
			fmt.Printf("Metadata CSV:  %s\n", extractResult.CSVPath)
		}
	}
	if extractResult.FramesZipPath != "" {
		if interactive {
			fmt.Printf("Frames ZIP:    %s\n", extractResult.FramesZipPath)
		}
	}
	if interactive {
		if sheet != "" {
			fmt.Printf("Contact Sheet: %s\n", sheet)
		} else {
			fmt.Printf("Contact Sheet: %s\n", "(skipped)")
		}
		if opts.Voice {
			fmt.Printf("Transcript:    %s\n", transcriptTxt)
			fmt.Printf("TranscriptJSON:%s\n", transcriptJSON)
		} else {
			fmt.Printf("Transcript:    %s\n", "(disabled)")
		}
	}
	for _, guardrail := range result.Guardrails {
		if interactive {
			fmt.Printf("Guardrail:     [%s] %s\n", guardrail.Severity, guardrail.Message)
		}
	}
	for _, w := range extractResult.Warnings {
		if interactive {
			fmt.Printf("Warning:       %s\n", w)
		}
	}
	result.MetadataPath = extractResult.MetadataPath
	result.MetadataCSVPath = extractResult.CSVPath
	result.FramesZipPath = extractResult.FramesZipPath
	result.Warnings = append(result.Warnings, extractResult.Warnings...)
	if hookCmd, hookTimeout := effectivePostHook(opts); hookCmd != "" {
		if interactive {
			fmt.Printf("Post Hook:     %s\n", hookCmd)
		}
		hookOut, hookErr := runPostExtractHook(ctx, hookCmd, hookTimeout, result)
		if hookErr != nil {
			result.Warnings = append(result.Warnings, hookErr.Error())
			if interactive {
				fmt.Printf("Hook Warning:  %s\n", hookErr)
			}
		} else if interactive && strings.TrimSpace(hookOut) != "" {
			fmt.Printf("Hook Output:   %s\n", strings.TrimSpace(hookOut))
		}
	}
	result.ElapsedMs = time.Since(started).Milliseconds()
	result.Artifacts = map[string]string{}
	if result.OutDir != "" {
		result.Artifacts["out_dir"] = result.OutDir
	}
	if result.MetadataPath != "" {
		result.Artifacts["metadata"] = result.MetadataPath
	}
	if result.MetadataCSVPath != "" {
		result.Artifacts["metadata_csv"] = result.MetadataCSVPath
	}
	if result.FramesZipPath != "" {
		result.Artifacts["frames_zip"] = result.FramesZipPath
	}
	if result.ContactSheet != "" {
		result.Artifacts["contact_sheet"] = result.ContactSheet
	}
	if result.TranscriptTxt != "" {
		result.Artifacts["transcript_txt"] = result.TranscriptTxt
	}
	if result.TranscriptJSON != "" {
		result.Artifacts["transcript_json"] = result.TranscriptJSON
	}
	if result.AudioPath != "" {
		result.Artifacts["audio"] = result.AudioPath
	}
	if interactive {
		fmt.Printf("Elapsed:       %s\n", time.Duration(result.ElapsedMs)*time.Millisecond)
		if hint := framesRootStorageHint(appCfg.FramesRoot); hint != "" {
			fmt.Println(hint)
		}
	}

	// Show performance hints only for fallback scenarios
	if interactive && !opts.Verbose {
		gpu := detectGPU()

		// Only show hint if GPU acceleration was attempted but failed
		if gpu.Available && extractResult.FallbackUsed {
			fmt.Println("")
			fmt.Printf("⚠️  Note: GPU acceleration (%s) failed, fell back to CPU\n", opts.HWAccel)
			fmt.Printf("   Run 'framescli doctor' to verify GPU setup\n")
			fmt.Printf("   Or try --hwaccel auto to let ffmpeg choose the best method\n")
		}
	}

	if err := refreshRunsIndexForRun(result.OutDir); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("artifact index update failed: %v", err))
	}
	return result, nil
}

func refreshRunsIndexForRun(runDir string) error {
	runDir = strings.TrimSpace(runDir)
	if runDir == "" {
		return nil
	}
	rootDir := filepath.Dir(runDir)
	if rootDir == "" || rootDir == "." {
		return nil
	}
	_, err := media.RefreshRunsIndex(rootDir)
	return err
}

func withDiagnostics(stage string, inputPath string, outDir string, logPath string, runErr error) error {
	if runErr == nil {
		return nil
	}
	root := appCfg.FramesRoot
	if strings.TrimSpace(root) == "" {
		root = media.DefaultFramesRoot
	}
	diagPath, err := media.WriteDiagnosticsBundle(root, stage, inputPath, outDir, logPath, runErr)
	if err != nil {
		return runErr
	}
	return fmt.Errorf("%w (diagnostics: %s)", runErr, diagPath)
}

func telemetryLogPath() string {
	if strings.TrimSpace(appCfg.TelemetryPath) != "" {
		return strings.TrimSpace(appCfg.TelemetryPath)
	}
	root := strings.TrimSpace(appCfg.FramesRoot)
	if root == "" {
		root = media.DefaultFramesRoot
	}
	return filepath.Join(root, "telemetry", "events.jsonl")
}

func appendTelemetryEvent(evt telemetryEvent) error {
	if !appCfg.TelemetryEnabled {
		return nil
	}
	evt.TimestampUTC = time.Now().UTC().Format(time.RFC3339)
	evt.GOOS = runtime.GOOS
	evt.GOARCH = runtime.GOARCH
	raw, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	path := telemetryLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func readLastLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	buf := make([]string, 0, n)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if len(buf) < n {
			buf = append(buf, line)
			continue
		}
		copy(buf, buf[1:])
		buf[len(buf)-1] = line
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return buf, nil
}

func pruneToLastLines(path string, keep int) (int, error) {
	lines, err := readLastLines(path, keep)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	if len(lines) == 0 {
		if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
			return 0, err
		}
		return 0, nil
	}
	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		return 0, err
	}
	return len(lines), nil
}

func effectivePostHook(opts extractWorkflowOptions) (string, time.Duration) {
	hook := strings.TrimSpace(opts.PostHook)
	if hook == "" {
		hook = strings.TrimSpace(appCfg.PostExtractHook)
	}
	timeout := opts.PostHookTimeout
	if timeout <= 0 && appCfg.PostHookTimeout > 0 {
		timeout = time.Duration(appCfg.PostHookTimeout) * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return hook, timeout
}

func runPostExtractHook(ctx context.Context, command string, timeout time.Duration, res extractWorkflowResult) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx := ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	runCtx, cancel := context.WithTimeout(runCtx, timeout)
	defer cancel()

	resultJSON, _ := json.Marshal(res)
	artifactsJSON, _ := json.Marshal(res.Artifacts)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(runCtx, "cmd.exe", "/C", command)
	} else {
		cmd = exec.CommandContext(runCtx, "/bin/sh", "-lc", command)
	}
	cmd.Env = append(os.Environ(),
		"FRAMESCLI_HOOK_EVENT=post_extract",
		"FRAMESCLI_HOOK_INPUT="+strings.TrimSpace(res.VideoInput),
		"FRAMESCLI_HOOK_VIDEO="+strings.TrimSpace(res.ResolvedVideo),
		"FRAMESCLI_HOOK_OUT_DIR="+strings.TrimSpace(res.OutDir),
		"FRAMESCLI_HOOK_ARTIFACTS_JSON="+string(artifactsJSON),
		"FRAMESCLI_HOOK_RESULT_JSON="+string(resultJSON),
	)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if runCtx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("post hook timed out after %s", timeout)
	}
	if err != nil {
		if output == "" {
			return "", fmt.Errorf("post hook failed: %w", err)
		}
		return output, fmt.Errorf("post hook failed: %w (output: %s)", err, output)
	}
	return output, nil
}

func normalizeDroppedPath(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		if u, err := url.Parse(s); err == nil {
			if decoded, decErr := url.PathUnescape(u.Path); decErr == nil {
				s = decoded
			} else {
				s = u.Path
			}
		}
	}
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			if unq, err := strconv.Unquote(s); err == nil {
				s = unq
			} else {
				s = s[1 : len(s)-1]
			}
		}
	}
	looksWindowsPath := strings.Contains(s, `:\`) || strings.HasPrefix(s, `\\`)
	if !looksWindowsPath && strings.Contains(s, `\`) {
		var b strings.Builder
		escaped := false
		for _, r := range s {
			if escaped {
				b.WriteRune(r)
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			b.WriteRune(r)
		}
		if escaped {
			b.WriteRune('\\')
		}
		s = b.String()
	}
	return strings.TrimSpace(s)
}

// validateFrameFormatFlag rejects format values that ffmpeg can't produce via
// our extraction pipeline before any user-visible stage header prints. The
// media layer will reject the same values later, but at that point the user
// has already seen "[1/4] Resolving video input" which reads as progress.
func validateFrameFormatFlag(format string) error {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" || f == "png" || f == "jpg" || f == "jpeg" {
		return nil
	}
	return fmt.Errorf("unsupported --format %q: use png or jpg", format)
}

func parseExtractFPSSpec(spec string) (float64, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" || spec == "auto" {
		return 0, nil
	}
	fps, err := strconv.ParseFloat(spec, 64)
	if err != nil {
		return 0, err
	}
	if fps < 0 {
		return 0, fmt.Errorf("fps must be >= 0")
	}
	return fps, nil
}

// autoFPSForDuration targets ~480 total frames — dense enough that
// action-dense content doesn't slip between samples, sparse enough that
// long recordings don't explode disk usage. The old formula
// (round(60/duration)) clamped to 1 fps for anything >45s, which made
// "auto" a constant for every real-world video. The new formula scales
// the choice to duration, clamped to [1, 8] fps.
func autoFPSForDuration(durationSec float64) float64 {
	if durationSec <= 0 {
		return 1
	}
	const autoTargetFrames = 480.0
	fps := autoTargetFrames / durationSec
	if fps > 8 {
		return 8
	}
	if fps < 1 {
		return 1
	}
	// Round to nearest 0.5 for clean display ("fps: 1.50 (auto, ...)").
	return math.Round(fps*2) / 2
}

func workflowPresetDefinitions() []workflowPresetDefinition {
	return []workflowPresetDefinition{
		{
			Name:             "laptop-safe",
			DefaultFPS:       1,
			DefaultFormat:    "jpg",
			MediaPreset:      "safe",
			ChunkDurationSec: 300,
			Description:      "lowest-cost preset for long recordings and CPU-only machines",
		},
		{
			Name:             "balanced",
			DefaultFPS:       4,
			DefaultFormat:    "png",
			MediaPreset:      "balanced",
			ChunkDurationSec: 600,
			Description:      "default preset for general-purpose extraction and resumable transcripts",
		},
		{
			Name:             "high-fidelity",
			DefaultFPS:       8,
			DefaultFormat:    "png",
			MediaPreset:      "fast",
			ChunkDurationSec: 900,
			Description:      "denser sampling for short clips when disk and CPU budget are available",
		},
		{
			Name:             "fast",
			DefaultFPS:       4,
			DefaultFormat:    "jpg",
			MediaPreset:      "fast",
			ChunkDurationSec: 0,
			Description:      "legacy speed-first preset kept for backward compatibility",
			Legacy:           true,
		},
	}
}

func canonicalWorkflowPreset(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "balanced":
		return "balanced"
	case "laptop-safe", "laptop_safe", "laptopsafe", "safe":
		return "laptop-safe"
	case "high-fidelity", "high_fidelity", "highfidelity":
		return "high-fidelity"
	case "fast":
		return "fast"
	default:
		return "balanced"
	}
}

func workflowPresetDefinitionFor(name string) workflowPresetDefinition {
	canonical := canonicalWorkflowPreset(name)
	for _, preset := range workflowPresetDefinitions() {
		if preset.Name == canonical {
			return preset
		}
	}
	return workflowPresetDefinitionFor("balanced")
}

func userFacingPresetChoices() []string {
	return []string{"laptop-safe", "balanced", "high-fidelity"}
}

func resolveWorkflowSettings(info media.VideoInfo, requestedFPS float64, requestedFormat string, requestedPreset string, fpsExplicit bool, formatExplicit bool, presetExplicit bool, voice bool, chunkDurationSec int, chunkExplicit bool) resolvedWorkflowSettings {
	preset := workflowPresetDefinitionFor(requestedPreset)
	defaultCfg := appconfig.Default()
	fps := requestedFPS
	implicitConfigFPS := !fpsExplicit && requestedFPS > 0 && math.Abs(requestedFPS-defaultCfg.DefaultFPS) < 0.0001
	fpsMode := ""
	switch {
	case fpsExplicit && fps <= 0:
		fps = autoFPSForDuration(info.DurationSec)
		fpsMode = "auto"
	case fpsExplicit:
	case presetExplicit:
		fps = preset.DefaultFPS
	case implicitConfigFPS:
		fps = preset.DefaultFPS
	case fps <= 0:
		fps = preset.DefaultFPS
	}
	format := strings.ToLower(strings.TrimSpace(requestedFormat))
	if format == "jpeg" {
		format = "jpg"
	}
	implicitConfigFormat := !formatExplicit && format != "" && format == defaultCfg.DefaultFormat
	switch {
	case formatExplicit && format != "":
	case presetExplicit:
		format = preset.DefaultFormat
	case implicitConfigFormat:
		format = preset.DefaultFormat
	case format == "":
		format = preset.DefaultFormat
	}
	resolvedChunkDuration := 0
	if voice {
		switch {
		case chunkExplicit:
			resolvedChunkDuration = chunkDurationSec
		default:
			resolvedChunkDuration = preset.ChunkDurationSec
		}
	}
	return resolvedWorkflowSettings{
		Preset:           preset.Name,
		MediaPreset:      preset.MediaPreset,
		FPS:              fps,
		FPSMode:          fpsMode,
		FrameFormat:      format,
		ChunkDurationSec: resolvedChunkDuration,
	}
}

func importDeprecatedCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "import [<url>]",
		Short:  "Deprecated: use `framescli extract --url <url>` instead",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			target := "<url>"
			if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
				target = args[0]
			}
			failf("The `import` command has been removed. Use:\n  framescli extract --url %s\n\nPositional URLs also work: `framescli extract %s`.\n", target, target)
		},
	}
}

func previewModeUsesTranscript(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "both", "audio":
		return true
	default:
		return false
	}
}

func extractCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract [<videoPath|recent>] [fps]",
		Short: "Extract video frames (png/jpg), optional voice transcription",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			// Handle URL vs path validation
			urlFlag, _ := cmd.Flags().GetString("url")
			hasURL := cmd.Flags().Changed("url")
			hasPath := len(args) > 0

			if hasURL && hasPath {
				failln("Cannot specify both --url and video path argument")
				return
			}
			if !hasURL && !hasPath {
				failln("Must specify either --url or video path argument")
				return
			}

			videoInput := ""
			if hasURL {
				videoInput = urlFlag
			} else {
				videoInput = args[0]
			}

			// Auto-route: if positional arg is a URL, treat it as --url
			if !hasURL && hasPath && looksLikeURL(videoInput) {
				fmt.Printf("Detected URL — downloading via yt-dlp...\n")
				urlFlag = videoInput
				hasURL = true
				hasPath = false
			}

			fpsSpec, _ := cmd.Flags().GetString("fps")
			fpsExplicit := cmd.Flags().Changed("fps")
			// FPS can be second arg only if first arg is video path (not when using --url)
			if !hasURL && len(args) >= 2 {
				fpsSpec = args[1]
				fpsExplicit = true
			}
			fps, err := parseExtractFPSSpec(fpsSpec)
			if err != nil {
				failln("Invalid fps value. Use a positive number, 0, or auto.")
				return
			}
			if fps == 0 && strings.TrimSpace(strings.ToLower(fpsSpec)) != "auto" {
				// allow explicit 0 as auto mode
			}
			outDir, _ := cmd.Flags().GetString("out")
			voice, _ := cmd.Flags().GetBool("voice")
			frameFormat, _ := cmd.Flags().GetString("format")
			if !cmd.Flags().Changed("format") && appCfg.DefaultFormat != "" {
				frameFormat = appCfg.DefaultFormat
			}
			if err := validateFrameFormatFlag(frameFormat); err != nil {
				failln(err.Error())
				return
			}
			jpgQuality, _ := cmd.Flags().GetInt("quality")
			noSheet, _ := cmd.Flags().GetBool("no-sheet")
			sheetCols, _ := cmd.Flags().GetInt("sheet-cols")
			verbose, _ := cmd.Flags().GetBool("verbose")
			hwaccel, _ := cmd.Flags().GetString("hwaccel")
			preset, _ := cmd.Flags().GetString("preset")
			chunkDuration, _ := cmd.Flags().GetInt("chunk-duration")
			startTime, _ := cmd.Flags().GetString("from")
			endTime, _ := cmd.Flags().GetString("to")
			startFrame, _ := cmd.Flags().GetInt("frame-start")
			endFrame, _ := cmd.Flags().GetInt("frame-end")
			everyN, _ := cmd.Flags().GetInt("every-n")
			frameName, _ := cmd.Flags().GetString("name-template")
			zipFrames, _ := cmd.Flags().GetBool("zip")
			logFile, _ := cmd.Flags().GetString("log-file")
			metadataCSV, _ := cmd.Flags().GetBool("metadata-csv")
			audioFormat, _ := cmd.Flags().GetString("audio-format")
			audioBitrate, _ := cmd.Flags().GetString("audio-bitrate")
			normalizeAudio, _ := cmd.Flags().GetBool("normalize-audio")
			audioStart, _ := cmd.Flags().GetString("audio-from")
			audioEnd, _ := cmd.Flags().GetString("audio-to")
			transcribeModel, _ := cmd.Flags().GetString("transcribe-model")
			transcribeBackend, _ := cmd.Flags().GetString("transcribe-backend")
			transcribeBin, _ := cmd.Flags().GetString("transcribe-bin")
			transcribeLanguage, _ := cmd.Flags().GetString("transcribe-language")
			transcribeTimeout, _ := cmd.Flags().GetInt("transcribe-timeout")
			allowExpensive, _ := cmd.Flags().GetBool("allow-expensive")
			postHook, _ := cmd.Flags().GetString("post-hook")
			postHookTimeout, _ := cmd.Flags().GetDuration("post-hook-timeout")
			jsonOut, _ := cmd.Flags().GetBool("json")
			noCache, _ := cmd.Flags().GetBool("no-cache")
			force, _ := cmd.Flags().GetBool("force")
			if !cmd.Flags().Changed("hwaccel") {
				hwaccel = appCfg.HWAccel
			}
			if !cmd.Flags().Changed("preset") {
				preset = appCfg.PerformanceMode
			}
			opts := extractWorkflowOptions{
				URL:                urlFlag,
				NoCache:            noCache,
				OutDir:             outDir,
				Voice:              voice,
				FrameFormat:        frameFormat,
				FormatExplicit:     cmd.Flags().Changed("format"),
				JPGQuality:         jpgQuality,
				NoSheet:            noSheet,
				SheetCols:          sheetCols,
				Verbose:            verbose,
				HWAccel:            hwaccel,
				Preset:             preset,
				PresetExplicit:     cmd.Flags().Changed("preset"),
				FPSExplicit:        fpsExplicit,
				StartTime:          startTime,
				EndTime:            endTime,
				StartFrame:         startFrame,
				EndFrame:           endFrame,
				EveryN:             everyN,
				FrameName:          frameName,
				ZipFrames:          zipFrames,
				LogFile:            logFile,
				MetadataCSV:        metadataCSV,
				AudioFormat:        audioFormat,
				AudioBitrate:       audioBitrate,
				NormalizeAudio:     normalizeAudio,
				AudioStart:         audioStart,
				AudioEnd:           audioEnd,
				TranscribeModel:    transcribeModel,
				TranscribeBackend:  transcribeBackend,
				TranscribeBin:      transcribeBin,
				TranscribeLanguage: transcribeLanguage,
				ChunkDurationSec:   chunkDuration,
				ChunkExplicit:      cmd.Flags().Changed("chunk-duration"),
				TranscribeTimeout:  transcribeTimeout,
				AllowExpensive:     allowExpensive,
				PostHook:           postHook,
				PostHookTimeout:    postHookTimeout,
				Force:              force,
			}
			if jsonOut {
				started := time.Now()
				res, err := runExtractWorkflowResult(cmd.Context(), videoInput, fps, opts, false)
				if err != nil {
					if !isCancellationError(err) {
						err = withDiagnostics("extract", videoInput, opts.OutDir, opts.LogFile, err)
					}
					_ = appendTelemetryEvent(telemetryEvent{
						Command:     "extract",
						Status:      "error",
						DurationMs:  time.Since(started).Milliseconds(),
						Input:       videoInput,
						FPS:         fps,
						FrameFormat: opts.FrameFormat,
						HWAccel:     opts.HWAccel,
						Preset:      opts.Preset,
						Voice:       opts.Voice,
						Error:       err.Error(),
					})
					emitAutomationJSON("extract", started, "error", map[string]string{"input": videoInput}, err)
					if isCancellationError(err) {
						markCommandInterrupted()
					} else {
						markCommandFailure()
					}
					return
				}
				_ = appendTelemetryEvent(telemetryEvent{
					Command:       "extract",
					Status:        "success",
					DurationMs:    res.ElapsedMs,
					Input:         videoInput,
					ResolvedInput: res.ResolvedVideo,
					OutDir:        res.OutDir,
					FPS:           res.FPS,
					FrameFormat:   res.FrameFormat,
					HWAccel:       res.HWAccel,
					Preset:        res.Preset,
					Voice:         opts.Voice,
					FrameCount:    res.FrameCount,
					Artifacts:     res.Artifacts,
				})
				emitAutomationJSON("extract", started, "success", res, nil)
				return
			}
			started := time.Now()
			res, err := runExtractWorkflowResult(cmd.Context(), videoInput, fps, opts, true)
			if err != nil {
				if !isCancellationError(err) {
					err = withDiagnostics("extract", videoInput, opts.OutDir, opts.LogFile, err)
				}
				_ = appendTelemetryEvent(telemetryEvent{
					Command:     "extract",
					Status:      "error",
					DurationMs:  time.Since(started).Milliseconds(),
					Input:       videoInput,
					FPS:         fps,
					FrameFormat: opts.FrameFormat,
					HWAccel:     opts.HWAccel,
					Preset:      opts.Preset,
					Voice:       opts.Voice,
					Error:       err.Error(),
				})
				if isCancellationError(err) {
					markCommandInterrupted()
					fmt.Fprintln(os.Stderr, "Cancelled.")
				} else {
					failf("%s\n", renderUserFacingError(err))
				}
				return
			}
			_ = appendTelemetryEvent(telemetryEvent{
				Command:       "extract",
				Status:        "success",
				DurationMs:    res.ElapsedMs,
				Input:         videoInput,
				ResolvedInput: res.ResolvedVideo,
				OutDir:        res.OutDir,
				FPS:           res.FPS,
				FrameFormat:   res.FrameFormat,
				HWAccel:       res.HWAccel,
				Preset:        res.Preset,
				Voice:         opts.Voice,
				FrameCount:    res.FrameCount,
				Artifacts:     res.Artifacts,
			})
		},
	}

	cmd.Flags().String("url", "", "Video URL to download and extract (mutually exclusive with video path)")
	cmd.Flags().Bool("no-cache", false, "Skip cache and re-download video even if already cached")
	cmd.Flags().String("fps", fmt.Sprintf("%g", appCfg.DefaultFPS), "Frames per second, or 0/auto for automatic ~60-frame mode")
	cmd.Flags().String("out", "", "Output directory")
	cmd.Flags().Bool("voice", false, "Extract audio and generate transcript in the output folder")
	cmd.Flags().String("format", "png", "Frame output format: png or jpg")
	cmd.Flags().Int("quality", 3, "JPG quality (1-31, lower is better quality)")
	cmd.Flags().Bool("no-sheet", false, "Skip contact sheet generation")
	cmd.Flags().Int("sheet-cols", 6, "Columns in generated contact sheet")
	cmd.Flags().Bool("verbose", false, "Show raw ffmpeg/whisper logs")
	cmd.Flags().String("hwaccel", appCfg.HWAccel, "Hardware acceleration mode: none|auto|cuda|vaapi|qsv")
	cmd.Flags().String("preset", appCfg.PerformanceMode, "Workflow preset: laptop-safe|balanced|high-fidelity (legacy: safe|fast)")
	cmd.Flags().String("from", "", "Start timestamp (e.g. 00:30 or seconds)")
	cmd.Flags().String("to", "", "End timestamp (e.g. 01:45 or seconds)")
	cmd.Flags().Int("frame-start", 0, "Source frame start index (inclusive)")
	cmd.Flags().Int("frame-end", 0, "Source frame end index (inclusive)")
	cmd.Flags().Int("every-n", 0, "Sample every N source frames")
	cmd.Flags().String("name-template", "frame-%04d", "Frame filename template without extension")
	cmd.Flags().Bool("zip", false, "Zip extracted frames as frames.zip")
	cmd.Flags().Bool("metadata-csv", false, "Export frame metadata CSV")
	cmd.Flags().String("log-file", "", "Write extraction logs to a file")
	cmd.Flags().String("audio-format", "wav", "Audio format: wav|mp3|aac")
	cmd.Flags().String("audio-bitrate", "128k", "Audio bitrate for mp3/aac")
	cmd.Flags().Bool("normalize-audio", false, "Apply loudnorm audio normalization")
	cmd.Flags().String("audio-from", "", "Audio trim start time (seconds or HH:MM:SS)")
	cmd.Flags().String("audio-to", "", "Audio trim end time (seconds or HH:MM:SS)")
	cmd.Flags().String("transcribe-model", "", "Whisper model override (e.g. tiny/base)")
	cmd.Flags().String("transcribe-backend", appCfg.TranscribeBackend, "Transcription backend: auto|whisper|faster-whisper")
	cmd.Flags().String("transcribe-bin", "", "Override transcription CLI binary path/name for selected backend")
	cmd.Flags().String("transcribe-language", appCfg.WhisperLanguage, "Transcription language override (blank=auto)")
	cmd.Flags().Int("chunk-duration", 0, "Split voice transcription into N-second chunks (0 uses preset default)")
	cmd.Flags().Int("transcribe-timeout", 0, "Skip transcription if it runs longer than N seconds (0 disables timeout)")
	cmd.Flags().Bool("allow-expensive", false, "Allow workloads that exceed long-input guardrails")
	cmd.Flags().String("post-hook", "", "Run command after successful extraction (overrides config post_extract_hook)")
	cmd.Flags().Duration("post-hook-timeout", 0, "Post-hook timeout (e.g. 30s, 2m)")
	cmd.Flags().Bool("force", false, "Re-extract frames/audio even if matching artifacts already exist in --out")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

func extractBatchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract-batch <videoPathOrGlob...>",
		Short: "Extract frames/audio from multiple videos (supports glob patterns)",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fpsSpec, _ := cmd.Flags().GetString("fps")
			fpsExplicit := cmd.Flags().Changed("fps")
			fps, err := parseExtractFPSSpec(fpsSpec)
			if err != nil {
				failln("Invalid fps value. Use a positive number, 0, or auto.")
				return
			}
			voice, _ := cmd.Flags().GetBool("voice")
			frameFormat, _ := cmd.Flags().GetString("format")
			if !cmd.Flags().Changed("format") && appCfg.DefaultFormat != "" {
				frameFormat = appCfg.DefaultFormat
			}
			if err := validateFrameFormatFlag(frameFormat); err != nil {
				failln(err.Error())
				return
			}
			jpgQuality, _ := cmd.Flags().GetInt("quality")
			noSheet, _ := cmd.Flags().GetBool("no-sheet")
			sheetCols, _ := cmd.Flags().GetInt("sheet-cols")
			verbose, _ := cmd.Flags().GetBool("verbose")
			hwaccel, _ := cmd.Flags().GetString("hwaccel")
			preset, _ := cmd.Flags().GetString("preset")
			chunkDuration, _ := cmd.Flags().GetInt("chunk-duration")
			startTime, _ := cmd.Flags().GetString("from")
			endTime, _ := cmd.Flags().GetString("to")
			startFrame, _ := cmd.Flags().GetInt("frame-start")
			endFrame, _ := cmd.Flags().GetInt("frame-end")
			everyN, _ := cmd.Flags().GetInt("every-n")
			frameName, _ := cmd.Flags().GetString("name-template")
			zipFrames, _ := cmd.Flags().GetBool("zip")
			logFile, _ := cmd.Flags().GetString("log-file")
			metadataCSV, _ := cmd.Flags().GetBool("metadata-csv")
			audioFormat, _ := cmd.Flags().GetString("audio-format")
			audioBitrate, _ := cmd.Flags().GetString("audio-bitrate")
			normalizeAudio, _ := cmd.Flags().GetBool("normalize-audio")
			audioStart, _ := cmd.Flags().GetString("audio-from")
			audioEnd, _ := cmd.Flags().GetString("audio-to")
			transcribeModel, _ := cmd.Flags().GetString("transcribe-model")
			transcribeBackend, _ := cmd.Flags().GetString("transcribe-backend")
			transcribeBin, _ := cmd.Flags().GetString("transcribe-bin")
			transcribeLanguage, _ := cmd.Flags().GetString("transcribe-language")
			allowExpensive, _ := cmd.Flags().GetBool("allow-expensive")
			postHook, _ := cmd.Flags().GetString("post-hook")
			postHookTimeout, _ := cmd.Flags().GetDuration("post-hook-timeout")
			jsonOut, _ := cmd.Flags().GetBool("json")
			if !cmd.Flags().Changed("hwaccel") {
				hwaccel = appCfg.HWAccel
			}
			if !cmd.Flags().Changed("preset") {
				preset = appCfg.PerformanceMode
			}

			videos := expandVideoInputs(args)
			if len(videos) == 0 {
				failln("No videos found from provided paths/globs.")
				return
			}
			if !jsonOut {
				fmt.Printf("Batch size: %d\n", len(videos))
			}
			failures := 0
			items := make([]batchItemResult, 0, len(videos))
			started := time.Now()
			for i, video := range videos {
				itemStarted := time.Now()
				if !jsonOut {
					fmt.Printf("\n[%d/%d] %s\n", i+1, len(videos), video)
				}
				outDir := ""
				if baseOut, _ := cmd.Flags().GetString("out"); strings.TrimSpace(baseOut) != "" {
					stem := strings.TrimSuffix(filepath.Base(video), filepath.Ext(video))
					outDir = filepath.Join(baseOut, stem)
				}
				opts := extractWorkflowOptions{
					OutDir:             outDir,
					Voice:              voice,
					FrameFormat:        frameFormat,
					FormatExplicit:     cmd.Flags().Changed("format"),
					JPGQuality:         jpgQuality,
					NoSheet:            noSheet,
					SheetCols:          sheetCols,
					Verbose:            verbose,
					HWAccel:            hwaccel,
					Preset:             preset,
					PresetExplicit:     cmd.Flags().Changed("preset"),
					FPSExplicit:        fpsExplicit,
					StartTime:          startTime,
					EndTime:            endTime,
					StartFrame:         startFrame,
					EndFrame:           endFrame,
					EveryN:             everyN,
					FrameName:          frameName,
					ZipFrames:          zipFrames,
					LogFile:            logFile,
					MetadataCSV:        metadataCSV,
					AudioFormat:        audioFormat,
					AudioBitrate:       audioBitrate,
					NormalizeAudio:     normalizeAudio,
					AudioStart:         audioStart,
					AudioEnd:           audioEnd,
					TranscribeModel:    transcribeModel,
					TranscribeBackend:  transcribeBackend,
					TranscribeBin:      transcribeBin,
					TranscribeLanguage: transcribeLanguage,
					ChunkDurationSec:   chunkDuration,
					ChunkExplicit:      cmd.Flags().Changed("chunk-duration"),
					AllowExpensive:     allowExpensive,
					PostHook:           postHook,
					PostHookTimeout:    postHookTimeout,
				}
				if jsonOut {
					res, err := runExtractWorkflowResult(cmd.Context(), video, fps, opts, false)
					if err != nil {
						if !isCancellationError(err) {
							err = withDiagnostics("extract_batch", video, opts.OutDir, opts.LogFile, err)
						}
						_ = appendTelemetryEvent(telemetryEvent{
							Command:     "extract_batch",
							Status:      "error",
							DurationMs:  time.Since(itemStarted).Milliseconds(),
							Input:       video,
							FPS:         fps,
							FrameFormat: opts.FrameFormat,
							HWAccel:     opts.HWAccel,
							Preset:      opts.Preset,
							Voice:       opts.Voice,
							Error:       err.Error(),
						})
						failures++
						items = append(items, batchItemResult{Input: video, Status: "error", Error: err.Error()})
						continue
					}
					_ = appendTelemetryEvent(telemetryEvent{
						Command:       "extract_batch",
						Status:        "success",
						DurationMs:    res.ElapsedMs,
						Input:         video,
						ResolvedInput: res.ResolvedVideo,
						OutDir:        res.OutDir,
						FPS:           res.FPS,
						FrameFormat:   res.FrameFormat,
						HWAccel:       res.HWAccel,
						Preset:        res.Preset,
						Voice:         opts.Voice,
						FrameCount:    res.FrameCount,
						Artifacts:     res.Artifacts,
					})
					items = append(items, batchItemResult{Input: video, Status: "success", Result: &res})
					continue
				}
				res, err := runExtractWorkflowResult(cmd.Context(), video, fps, opts, true)
				if err != nil {
					if !isCancellationError(err) {
						err = withDiagnostics("extract_batch", video, opts.OutDir, opts.LogFile, err)
					}
					_ = appendTelemetryEvent(telemetryEvent{
						Command:     "extract_batch",
						Status:      "error",
						DurationMs:  time.Since(itemStarted).Milliseconds(),
						Input:       video,
						FPS:         fps,
						FrameFormat: opts.FrameFormat,
						HWAccel:     opts.HWAccel,
						Preset:      opts.Preset,
						Voice:       opts.Voice,
						Error:       err.Error(),
					})
					failures++
					if isCancellationError(err) {
						markCommandInterrupted()
						fmt.Fprintf(os.Stderr, "Cancelled during batch item: %s\n", video)
					} else {
						failf("Failed: %v\n", err)
					}
					continue
				}
				_ = appendTelemetryEvent(telemetryEvent{
					Command:       "extract_batch",
					Status:        "success",
					DurationMs:    res.ElapsedMs,
					Input:         video,
					ResolvedInput: res.ResolvedVideo,
					OutDir:        res.OutDir,
					FPS:           res.FPS,
					FrameFormat:   res.FrameFormat,
					HWAccel:       res.HWAccel,
					Preset:        res.Preset,
					Voice:         opts.Voice,
					FrameCount:    res.FrameCount,
					Artifacts:     res.Artifacts,
				})
			}
			if jsonOut {
				status := "success"
				if failures > 0 && failures < len(videos) {
					status = "partial"
				} else if failures == len(videos) {
					status = "error"
				}
				payload := map[string]any{
					"total":    len(videos),
					"failures": failures,
					"items":    items,
				}
				emitAutomationJSON("extract-batch", started, status, payload, nil)
				if failures > 0 {
					interrupted := false
					for _, it := range items {
						if it.Status != "error" {
							continue
						}
						if strings.Contains(strings.ToLower(it.Error), "canceled") || strings.Contains(strings.ToLower(it.Error), "deadline exceeded") {
							interrupted = true
							break
						}
					}
					if interrupted {
						markCommandInterrupted()
					} else {
						markCommandFailure()
					}
				}
				return
			}
			if failures > 0 {
				failf("\nBatch completed with %d failure(s)\n", failures)
			} else {
				fmt.Println("\nBatch completed successfully")
			}
		},
	}
	cmd.Flags().String("fps", fmt.Sprintf("%g", appCfg.DefaultFPS), "Frames per second, or 0/auto for automatic ~60-frame mode")
	cmd.Flags().String("out", "", "Output root directory (one subdir per input video)")
	cmd.Flags().Bool("voice", false, "Extract audio and generate transcript in the output folder")
	cmd.Flags().String("format", appCfg.DefaultFormat, "Frame output format: png or jpg")
	cmd.Flags().Int("quality", 3, "JPG quality (1-31, lower is better quality)")
	cmd.Flags().Bool("no-sheet", false, "Skip contact sheet generation")
	cmd.Flags().Int("sheet-cols", 6, "Columns in generated contact sheet")
	cmd.Flags().Bool("verbose", false, "Show raw ffmpeg/whisper logs")
	cmd.Flags().String("hwaccel", appCfg.HWAccel, "Hardware acceleration mode: none|auto|cuda|vaapi|qsv")
	cmd.Flags().String("preset", appCfg.PerformanceMode, "Workflow preset: laptop-safe|balanced|high-fidelity (legacy: safe|fast)")
	cmd.Flags().String("from", "", "Start timestamp (e.g. 00:30 or seconds)")
	cmd.Flags().String("to", "", "End timestamp (e.g. 01:45 or seconds)")
	cmd.Flags().Int("frame-start", 0, "Source frame start index (inclusive)")
	cmd.Flags().Int("frame-end", 0, "Source frame end index (inclusive)")
	cmd.Flags().Int("every-n", 0, "Sample every N source frames")
	cmd.Flags().String("name-template", "frame-%04d", "Frame filename template without extension")
	cmd.Flags().Bool("zip", false, "Zip extracted frames as frames.zip")
	cmd.Flags().Bool("metadata-csv", false, "Export frame metadata CSV")
	cmd.Flags().String("log-file", "", "Write extraction logs to a file")
	cmd.Flags().String("audio-format", "wav", "Audio format: wav|mp3|aac")
	cmd.Flags().String("audio-bitrate", "128k", "Audio bitrate for mp3/aac")
	cmd.Flags().Bool("normalize-audio", false, "Apply loudnorm audio normalization")
	cmd.Flags().String("audio-from", "", "Audio trim start time (seconds or HH:MM:SS)")
	cmd.Flags().String("audio-to", "", "Audio trim end time (seconds or HH:MM:SS)")
	cmd.Flags().String("transcribe-model", "", "Whisper model override (e.g. tiny/base)")
	cmd.Flags().String("transcribe-backend", appCfg.TranscribeBackend, "Transcription backend: auto|whisper|faster-whisper")
	cmd.Flags().String("transcribe-bin", "", "Override transcription CLI binary path/name for selected backend")
	cmd.Flags().String("transcribe-language", appCfg.WhisperLanguage, "Transcription language override (blank=auto)")
	cmd.Flags().Int("chunk-duration", 0, "Split voice transcription into N-second chunks (0 uses preset default)")
	cmd.Flags().Bool("allow-expensive", false, "Allow workloads that exceed long-input guardrails")
	cmd.Flags().String("post-hook", "", "Run command after each successful extraction (overrides config post_extract_hook)")
	cmd.Flags().Duration("post-hook-timeout", 0, "Post-hook timeout per item (e.g. 30s, 2m)")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

func previewCommand() *cobra.Command {
	var fpsSpec string
	var format string
	var mode string
	var preset string
	var chunkDuration int
	cmd := &cobra.Command{
		Use:   "preview <videoPath|recent>",
		Short: "Dry-run preview of estimated outputs without running extraction",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			started := time.Now()
			jsonOut, _ := cmd.Flags().GetBool("json")
			videoPath, note, err := resolveVideoInput(args[0])
			if err != nil {
				if jsonOut {
					emitAutomationJSON("preview", started, "error", map[string]string{"input": args[0]}, err)
					markCommandFailure()
					return
				}
				failf("%s\n", renderUserFacingError(fmt.Errorf("failed to resolve video input: %w", err)))
				return
			}
			info, err := media.ProbeVideoInfo(videoPath)
			if err != nil {
				if jsonOut {
					emitAutomationJSON("preview", started, "error", map[string]string{"input": videoPath}, err)
					markCommandFailure()
					return
				}
				failf("%s\n", renderUserFacingError(fmt.Errorf("invalid video input %q: %w", videoPath, err)))
				return
			}
			fps, err := parseExtractFPSSpec(fpsSpec)
			if err != nil {
				err = fmt.Errorf("invalid fps value. use a positive number, 0, or auto")
				if jsonOut {
					emitAutomationJSON("preview", started, "error", map[string]string{"input": args[0]}, err)
					markCommandFailure()
					return
				}
				failf("%s\n", renderUserFacingError(err))
				return
			}
			resolved := resolveWorkflowSettings(info, fps, format, preset, cmd.Flags().Changed("fps"), cmd.Flags().Changed("format"), cmd.Flags().Changed("preset"), previewModeUsesTranscript(mode), chunkDuration, cmd.Flags().Changed("chunk-duration"))
			transcriptPlan := buildTranscriptPlan(info.DurationSec, appCfg, probeTranscribeAccel(appCfg, "", doctorHasGPU()).UsesGPU)
			estimate := estimateOutputs(info, resolved.FPS, resolved.FrameFormat, mode, transcriptPlan, resolved.ChunkDurationSec)
			if jsonOut {
				payload := map[string]any{
					"input":              args[0],
					"resolved":           videoPath,
					"source_note":        note,
					"video_info":         info,
					"target_fps":         resolved.FPS,
					"format":             resolved.FrameFormat,
					"mode":               mode,
					"preset":             resolved.Preset,
					"media_preset":       resolved.MediaPreset,
					"chunk_duration_sec": resolved.ChunkDurationSec,
					"estimate":           estimate,
				}
				if resolved.FPSMode != "" {
					payload["fps_mode"] = resolved.FPSMode
				}
				emitAutomationJSON("preview", started, "success", payload, nil)
				return
			}
			fmt.Println("Preview")
			fmt.Println("-------")
			if note != "" {
				fmt.Printf("Source:      %s\n", note)
			}
			fmt.Printf("Video:       %s\n", videoPath)
			fmt.Printf("Duration:    %.2fs\n", info.DurationSec)
			fmt.Printf("Resolution:  %dx%d\n", info.Width, info.Height)
			fmt.Printf("Source FPS:  %.2f\n", info.FrameRate)
			fmt.Printf("Mode:        %s\n", mode)
			fmt.Printf("Preset:      %s (media=%s)\n", resolved.Preset, resolved.MediaPreset)
			if resolved.FPSMode == "auto" {
				fmt.Printf("Target FPS:  %.2f (auto, computed from duration %.1fs)\n", resolved.FPS, info.DurationSec)
			} else {
				fmt.Printf("Target FPS:  %.2f\n", resolved.FPS)
			}
			fmt.Printf("Format:      %s\n", resolved.FrameFormat)
			if previewModeUsesTranscript(mode) {
				fmt.Printf("Chunking:    %ds\n", resolved.ChunkDurationSec)
			}
			fmt.Printf("Frames est:  %d\n", estimate.FrameCount)
			fmt.Printf("Disk est:    %s\n", estimate.DiskSummary)
			fmt.Println("Artifacts:")
			for _, a := range estimate.Artifacts {
				fmt.Printf("- %s\n", a)
			}
			if len(estimate.DiskProfiles) > 0 {
				fmt.Println("Common disk profiles:")
				for _, profile := range estimate.DiskProfiles {
					prefix := "-"
					if profile.Selected {
						prefix = "*"
					}
					fmt.Printf("%s %-12s %.2ffps %s %s (%d frames)\n", prefix, profile.Format, profile.FPS, profile.DiskSummary, profile.Label, profile.FrameCount)
				}
			}
			if estimate.Transcript.Enabled {
				fmt.Println("Transcript:")
				fmt.Printf("- backend=%s model=%s hardware=%s\n", estimate.Transcript.Backend, estimate.Transcript.Model, estimate.Transcript.Hardware)
				fmt.Printf("- class=%s\n", estimate.Transcript.RuntimeClass)
				fmt.Printf("- hint=%s\n", estimate.Transcript.CostHint)
				if estimate.Transcript.RuntimeSummary != "" {
					fmt.Printf("- runtime=%s\n", estimate.Transcript.RuntimeSummary)
				}
			}
			if estimate.MachineBenchmark != nil {
				fmt.Println("Machine hint:")
				fmt.Printf("- %s\n", estimate.MachineBenchmark.EstimatedSummary)
				fmt.Printf("- %s\n", estimate.MachineBenchmark.Caveat)
			}
			if len(estimate.Warnings) > 0 {
				fmt.Println("Warnings:")
				for _, warning := range estimate.Warnings {
					fmt.Printf("- %s\n", warning)
				}
			}
			if len(estimate.Guardrails.Guardrails) > 0 {
				fmt.Println("Guardrails:")
				for _, guardrail := range estimate.Guardrails.Guardrails {
					fmt.Printf("- [%s] %s (%s=%s, threshold %s)\n", guardrail.Severity, guardrail.Message, guardrail.Metric, guardrail.Actual, guardrail.Threshold)
				}
			}
		},
	}
	cmd.Flags().StringVar(&fpsSpec, "fps", fmt.Sprintf("%g", appCfg.DefaultFPS), "Target extraction FPS for estimate, or 0/auto for automatic ~60-frame mode")
	cmd.Flags().StringVar(&format, "format", appCfg.DefaultFormat, "Frame format estimate: png|jpg")
	cmd.Flags().StringVar(&mode, "mode", "both", "Output mode estimate: frames|audio|both")
	cmd.Flags().StringVar(&preset, "preset", appCfg.PerformanceMode, "Workflow preset: laptop-safe|balanced|high-fidelity (legacy: safe|fast)")
	cmd.Flags().IntVar(&chunkDuration, "chunk-duration", 0, "Transcript chunk duration in seconds for estimate (0 uses preset default)")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

type outputEstimate struct {
	FrameCount       int                   `json:"frame_count"`
	EstimatedMB      float64               `json:"estimated_mb"`
	EstimatedMBLow   float64               `json:"estimated_mb_low,omitempty"`
	EstimatedMBHigh  float64               `json:"estimated_mb_high,omitempty"`
	DiskSummary      string                `json:"disk_summary,omitempty"`
	Artifacts        []string              `json:"artifacts"`
	Warning          string                `json:"warning,omitempty"`
	Warnings         []string              `json:"warnings,omitempty"`
	DiskProfiles     []previewDiskProfile  `json:"disk_profiles,omitempty"`
	Transcript       previewTranscriptPlan `json:"transcript"`
	MachineBenchmark *previewBenchmarkHint `json:"machine_benchmark,omitempty"`
	Guardrails       workloadAssessment    `json:"guardrails"`
}

type previewDiskProfile struct {
	Label           string  `json:"label"`
	Format          string  `json:"format"`
	FPS             float64 `json:"fps"`
	FrameCount      int     `json:"frame_count"`
	EstimatedMB     float64 `json:"estimated_mb"`
	EstimatedMBLow  float64 `json:"estimated_mb_low"`
	EstimatedMBHigh float64 `json:"estimated_mb_high"`
	DiskSummary     string  `json:"disk_summary"`
	Selected        bool    `json:"selected,omitempty"`
}

type previewTranscriptPlan struct {
	Enabled              bool     `json:"enabled"`
	Backend              string   `json:"backend"`
	Model                string   `json:"model"`
	Hardware             string   `json:"hardware"`
	RuntimeClass         string   `json:"runtime_class"`
	CostHint             string   `json:"cost_hint"`
	RuntimeSummary       string   `json:"runtime_summary,omitempty"`
	EstimatedMinutesLow  float64  `json:"estimated_minutes_low,omitempty"`
	EstimatedMinutesHigh float64  `json:"estimated_minutes_high,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	ChunkDurationSec     int      `json:"chunk_duration_sec,omitempty"`
}

type previewBenchmarkHint struct {
	HistoryPath        string  `json:"history_path"`
	BestRealtimeFactor float64 `json:"best_realtime_factor"`
	EstimatedMinutes   float64 `json:"estimated_minutes"`
	EstimatedSummary   string  `json:"estimated_summary"`
	Caveat             string  `json:"caveat"`
}

func estimateOutputs(info media.VideoInfo, fps float64, format string, mode string, transcript previewTranscriptPlan, chunkDurationSec int) outputEstimate {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "both"
	}
	if fps <= 0 {
		fps = autoFPSForDuration(info.DurationSec)
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "png"
	}
	est := outputEstimate{
		Transcript: transcript,
	}
	if mode == "frames" {
		est.Transcript.Enabled = false
		est.Transcript.RuntimeSummary = ""
		est.Transcript.Warnings = nil
		est.Transcript.ChunkDurationSec = 0
	}
	switch mode {
	case "audio":
		est.Artifacts = []string{"voice/voice.wav (or selected audio format)", "voice/transcript.{txt,json,srt,vtt}"}
		est.EstimatedMB = (info.DurationSec / 60.0) * 9.5
		est.EstimatedMBLow = est.EstimatedMB * 0.7
		est.EstimatedMBHigh = est.EstimatedMB * 1.35
		est.DiskSummary = fmt.Sprintf("%s audio + transcript sidecars", formatMBRange(est.EstimatedMBLow, est.EstimatedMBHigh))
	default:
		frames := int(math.Ceil(info.DurationSec * fps))
		est.FrameCount = frames
		est.EstimatedMB, est.EstimatedMBLow, est.EstimatedMBHigh = estimateFrameDiskMB(info, frames, format)
		est.DiskSummary = fmt.Sprintf("%s for extracted frames + sheet", formatMBRange(est.EstimatedMBLow, est.EstimatedMBHigh))
		est.Artifacts = []string{
			"images/frame-XXXX." + format,
			"images/sheets/contact-sheet.png",
			"run.json + frames.json",
		}
		if mode == "both" {
			est.Artifacts = append(est.Artifacts, "voice/voice.wav", "voice/transcript.{txt,json,srt,vtt}")
		}
		est.DiskProfiles = buildDiskProfiles(info, fps, format)
		est.MachineBenchmark = latestBenchmarkPreviewHint(info.DurationSec)
	}

	est.Transcript.ChunkDurationSec = chunkDurationSec
	est.Guardrails = evaluateWorkloadGuardrails(info, mode, est, chunkDurationSec)
	est.Warnings = buildPreviewWarnings(info, mode, fps, est)
	if len(est.Warnings) > 0 {
		est.Warning = est.Warnings[0]
	}
	return est
}

func estimateFrameDiskMB(info media.VideoInfo, frameCount int, format string) (float64, float64, float64) {
	if frameCount <= 0 {
		return 0, 0, 0
	}
	scale := float64(max(info.Width*info.Height, 640*360)) / float64(1920*1080)
	typicalKB := 180.0 * scale
	if format == "jpg" || format == "jpeg" {
		typicalKB = 90.0 * scale
	}
	lowMB := (float64(frameCount) * typicalKB * 0.65) / 1024.0
	highMB := (float64(frameCount) * typicalKB * 1.45) / 1024.0
	approxMB := (lowMB + highMB) / 2.0
	return approxMB, lowMB, highMB
}

func buildDiskProfiles(info media.VideoInfo, selectedFPS float64, selectedFormat string) []previewDiskProfile {
	profiles := make([]previewDiskProfile, 0, 9)
	added := map[string]struct{}{}
	add := func(label string, fps float64, format string, selected bool) {
		key := fmt.Sprintf("%.3f|%s", fps, format)
		if _, ok := added[key]; ok {
			return
		}
		added[key] = struct{}{}
		frameCount := int(math.Ceil(info.DurationSec * fps))
		approxMB, lowMB, highMB := estimateFrameDiskMB(info, frameCount, format)
		profiles = append(profiles, previewDiskProfile{
			Label:           label,
			Format:          format,
			FPS:             fps,
			FrameCount:      frameCount,
			EstimatedMB:     approxMB,
			EstimatedMBLow:  lowMB,
			EstimatedMBHigh: highMB,
			DiskSummary:     formatMBRange(lowMB, highMB),
			Selected:        selected,
		})
	}
	add("selected", selectedFPS, selectedFormat, true)
	for _, fps := range []float64{1, 2, 4, 12} {
		for _, format := range []string{"jpg", "png"} {
			label := fmt.Sprintf("%s @ %.0ffps", format, fps)
			add(label, fps, format, fps == selectedFPS && format == selectedFormat)
		}
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		if profiles[i].Selected != profiles[j].Selected {
			return profiles[i].Selected
		}
		if profiles[i].FPS != profiles[j].FPS {
			return profiles[i].FPS < profiles[j].FPS
		}
		return profiles[i].Format < profiles[j].Format
	})
	return profiles
}

func buildTranscriptPlan(durationSec float64, cfg appconfig.Config, transcribeGPU bool) previewTranscriptPlan {
	return buildTranscriptPlanWithSettings(durationSec, cfg.TranscribeBackend, cfg.WhisperModel, cfg, transcribeGPU)
}

// buildTranscriptPlanWithSettings: transcribeGPU narrowly reflects whether the
// *transcription backend* is expected to use GPU acceleration — NOT whether
// the machine has a GPU for ffmpeg. See probeTranscribeAccel for the
// separation between hardware GPU and transcription GPU.
func buildTranscriptPlanWithSettings(durationSec float64, backendInput string, modelInput string, cfg appconfig.Config, transcribeGPU bool) previewTranscriptPlan {
	model := strings.ToLower(strings.TrimSpace(modelInput))
	if model == "" {
		model = "base"
	}
	cfg.TranscribeBackend = backendInput
	cfg.WhisperModel = model
	backend := resolvePreviewTranscribeBackend(cfg)
	hardware := "cpu"
	if transcribeGPU {
		hardware = "gpu-capable"
	}
	plan := previewTranscriptPlan{
		Enabled:      true,
		Backend:      backend,
		Model:        model,
		Hardware:     hardware,
		RuntimeClass: "moderate",
		CostHint:     "transcript runtime depends on backend, model, and local hardware",
	}
	if backend == "unavailable" {
		plan.RuntimeClass = "unavailable"
		plan.CostHint = "install whisper or faster-whisper before using transcript workflows"
		plan.Warnings = append(plan.Warnings, "Transcript backend not detected; --voice runs will fail until whisper or faster-whisper is installed.")
		return plan
	}

	lowFactor, highFactor := transcriptRealtimeRange(backend, model, transcribeGPU)
	midFactor := (lowFactor + highFactor) / 2
	switch {
	case midFactor >= 6:
		plan.RuntimeClass = "fast"
	case midFactor >= 2:
		plan.RuntimeClass = "moderate"
	case midFactor >= 1:
		plan.RuntimeClass = "near-realtime"
	case midFactor >= 0.4:
		plan.RuntimeClass = "slow"
	default:
		plan.RuntimeClass = "heavy"
	}
	switch {
	case transcribeGPU && midFactor >= 3:
		plan.CostHint = "GPU-backed transcription should stay well below video runtime in common cases"
	case transcribeGPU:
		plan.CostHint = "GPU helps, but larger Whisper models can still add noticeable runtime"
	case midFactor >= 1:
		plan.CostHint = "CPU-only transcription is plausible here, but expect runtime close to the clip length"
	default:
		plan.CostHint = "CPU-only transcription can dominate total runtime on longer recordings"
	}
	if durationSec > 0 {
		plan.EstimatedMinutesLow = (durationSec / highFactor) / 60.0
		plan.EstimatedMinutesHigh = (durationSec / lowFactor) / 60.0
		plan.RuntimeSummary = fmt.Sprintf("%s of transcript time for this clip", formatMinuteRange(plan.EstimatedMinutesLow, plan.EstimatedMinutesHigh))
	}
	if !transcribeGPU && (model == "large" || model == "medium") {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s on CPU-only hardware is the slowest transcript path in common use.", model))
	}
	return plan
}

func resolvePreviewTranscribeBackend(cfg appconfig.Config) string {
	backend := strings.ToLower(strings.TrimSpace(cfg.TranscribeBackend))
	if backend == "" || backend == "auto" {
		if _, err := exec.LookPath(strings.TrimSpace(cfg.FasterWhisperBin)); err == nil {
			return "faster-whisper"
		}
		if _, err := exec.LookPath(strings.TrimSpace(cfg.WhisperBin)); err == nil {
			return "whisper"
		}
		if _, err := exec.LookPath("faster-whisper"); err == nil {
			return "faster-whisper"
		}
		if _, err := exec.LookPath("whisper"); err == nil {
			return "whisper"
		}
		return "unavailable"
	}
	if backend == "faster_whisper" || backend == "fasterwhisper" {
		backend = "faster-whisper"
	}
	return backend
}

func transcriptRealtimeRange(backend string, model string, transcribeGPU bool) (float64, float64) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		model = "base"
	}
	if transcribeGPU {
		switch backend {
		case "faster-whisper":
			switch model {
			case "tiny", "turbo":
				return 10, 18
			case "base":
				return 8, 14
			case "small":
				return 5, 9
			case "medium":
				return 3, 6
			case "large":
				return 1.5, 3
			}
		default:
			switch model {
			case "tiny", "turbo":
				return 6, 10
			case "base":
				return 4, 8
			case "small":
				return 2.5, 5
			case "medium":
				return 1.5, 3
			case "large":
				return 0.8, 1.5
			}
		}
	}
	switch backend {
	case "faster-whisper":
		switch model {
		case "tiny", "turbo":
			return 2, 4
		case "base":
			return 1.5, 3
		case "small":
			return 0.8, 1.8
		case "medium":
			return 0.4, 1.0
		case "large":
			return 0.15, 0.4
		}
	default:
		switch model {
		case "tiny", "turbo":
			return 1, 2
		case "base":
			return 0.7, 1.5
		case "small":
			return 0.35, 0.9
		case "medium":
			return 0.15, 0.45
		case "large":
			return 0.05, 0.18
		}
	}
	return 0.5, 1.2
}

func buildPreviewWarnings(info media.VideoInfo, mode string, fps float64, est outputEstimate) []string {
	warnings := make([]string, 0, 4)
	switch {
	case mode != "audio" && est.FrameCount >= 20000:
		warnings = append(warnings, "Very large frame set; consider lower FPS or a narrower --from/--to window.")
	case mode != "audio" && info.DurationSec >= 2*3600 && fps >= 4:
		warnings = append(warnings, "Long-form recording at this sampling density can create a large extraction workload.")
	}
	if est.Transcript.Enabled {
		warnings = append(warnings, est.Transcript.Warnings...)
		if est.Transcript.Hardware == "cpu" && info.DurationSec >= 30*60 {
			warnings = append(warnings, "CPU-only transcript path on a long recording may take substantially longer than the frame extraction.")
		}
	}
	for _, guardrail := range est.Guardrails.Guardrails {
		warnings = append(warnings, guardrail.Message)
	}
	if mode != "audio" && est.MachineBenchmark == nil && info.DurationSec >= 45*60 {
		warnings = append(warnings, "No local benchmark history found; extraction timing is disk-size only until you run `framescli benchmark`.")
	}
	return uniqueStrings(warnings)
}

func evaluateWorkloadGuardrails(info media.VideoInfo, mode string, est outputEstimate, chunkDurationSec int) workloadAssessment {
	report := workloadAssessment{
		OverrideFlag: "--allow-expensive",
	}
	add := func(id string, blocking bool, metric string, actual string, threshold string, message string) {
		severity := "warn"
		if blocking {
			severity = "block"
			report.RequiresOverride = true
		}
		report.Guardrails = append(report.Guardrails, workloadGuardrail{
			ID:        id,
			Severity:  severity,
			Metric:    metric,
			Actual:    actual,
			Threshold: threshold,
			Message:   message,
		})
	}
	if mode != "audio" {
		switch {
		case est.FrameCount >= 40000:
			add("frame_count", true, "estimated_frame_count", fmt.Sprintf("%d", est.FrameCount), ">= 40000", "Estimated frame count exceeds 40000; narrow the time range, lower --fps, or rerun with --allow-expensive.")
		case est.FrameCount >= 20000:
			add("frame_count", false, "estimated_frame_count", fmt.Sprintf("%d", est.FrameCount), ">= 20000", "Estimated frame count exceeds 20000; expect a large extraction workload.")
		}
		switch {
		case est.EstimatedMBHigh >= 4096:
			add("disk_usage", true, "estimated_disk_mb_high", fmt.Sprintf("%.0f", est.EstimatedMBHigh), ">= 4096", "Estimated extracted frame footprint exceeds 4 GB; use a smaller window, JPG, or rerun with --allow-expensive.")
		case est.EstimatedMBHigh >= 2048:
			add("disk_usage", false, "estimated_disk_mb_high", fmt.Sprintf("%.0f", est.EstimatedMBHigh), ">= 2048", "Estimated extracted frame footprint exceeds 2 GB.")
		}
	}
	switch {
	case info.DurationSec >= 3*3600:
		add("duration", true, "duration_minutes", fmt.Sprintf("%.1f", info.DurationSec/60.0), ">= 180", "Recording duration exceeds 3 hours; long-form runs at this size require --allow-expensive.")
	case info.DurationSec >= 2*3600:
		add("duration", false, "duration_minutes", fmt.Sprintf("%.1f", info.DurationSec/60.0), ">= 120", "Recording duration exceeds 2 hours; preview estimates should be reviewed before extraction.")
	}
	if est.Transcript.Enabled && est.Transcript.Hardware == "cpu" {
		runtimeClass := strings.ToLower(strings.TrimSpace(est.Transcript.RuntimeClass))
		if info.DurationSec >= 45*60 && (runtimeClass == "slow" || runtimeClass == "heavy") && chunkDurationSec <= 0 {
			add("cpu_transcript", true, "transcript_runtime_class", est.Transcript.RuntimeClass, "slow/heavy on CPU without chunking", "CPU-only transcription is in a slow/heavy runtime class for a recording over 45 minutes; enable chunking or rerun with --allow-expensive.")
		} else if info.DurationSec >= 30*60 && (runtimeClass == "slow" || runtimeClass == "heavy") {
			if chunkDurationSec > 0 {
				add("cpu_transcript", false, "transcript_chunk_duration_sec", fmt.Sprintf("%d", chunkDurationSec), "chunked", fmt.Sprintf("CPU-only transcription is chunked at %d-second intervals to keep long runs resumable.", chunkDurationSec))
			} else {
				add("cpu_transcript", false, "transcript_runtime_class", est.Transcript.RuntimeClass, "slow/heavy on CPU", "CPU-only transcription is expected to be expensive for this recording.")
			}
		}
	}
	return report
}

func enforceGuardrails(report workloadAssessment, allowExpensive bool) error {
	if !report.RequiresOverride || allowExpensive {
		return nil
	}
	blockers := make([]string, 0, len(report.Guardrails))
	for _, guardrail := range report.Guardrails {
		if guardrail.Severity == "block" {
			blockers = append(blockers, guardrail.Message)
		}
	}
	if len(blockers) == 0 {
		blockers = append(blockers, "workload exceeded configured expensive-run thresholds")
	}
	return fmt.Errorf("expensive workload requires %s: %s", report.OverrideFlag, strings.Join(blockers, " "))
}

func latestBenchmarkPreviewHint(durationSec float64) *previewBenchmarkHint {
	path := media.BenchmarkHistoryPath(appCfg.FramesRoot)
	entries, err := media.ReadBenchmarkHistory(path, 1)
	if err != nil || len(entries) == 0 {
		return nil
	}
	best := bestSuccessfulSpeed(entries[0].Results)
	if best <= 0 {
		return nil
	}
	minutes := (durationSec / best) / 60.0
	return &previewBenchmarkHint{
		HistoryPath:        path,
		BestRealtimeFactor: best,
		EstimatedMinutes:   minutes,
		EstimatedSummary:   fmt.Sprintf("recent benchmark suggests about %.1fx realtime, or ~%.1f minutes for this clip", best, minutes),
		Caveat:             "derived from the most recent benchmark run; actual extraction varies by FPS, format, and storage speed",
	}
}

func uniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func formatMBRange(low float64, high float64) string {
	if low <= 0 && high <= 0 {
		return "~0 MB"
	}
	if math.Abs(high-low) < 1 {
		return fmt.Sprintf("~%.0f MB", (low+high)/2)
	}
	return fmt.Sprintf("~%.0f-%.0f MB", low, high)
}

func formatMinuteRange(low float64, high float64) string {
	if low <= 0 && high <= 0 {
		return "<1 minute"
	}
	if high < 1 {
		return "<1 minute"
	}
	if math.Abs(high-low) < 0.5 {
		return fmt.Sprintf("~%.1f minutes", (low+high)/2)
	}
	return fmt.Sprintf("~%.1f-%.1f minutes", low, high)
}

func runPreviewResult(input string, fps float64, format string, mode string, preset string, chunkDurationSec int, fpsExplicit bool, formatExplicit bool, presetExplicit bool, chunkExplicit bool) (map[string]any, error) {
	videoPath, note, err := resolveVideoInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve video input: %w", err)
	}
	info, err := media.ProbeVideoInfo(videoPath)
	if err != nil {
		return nil, fmt.Errorf("video probe failed: %w", err)
	}
	resolved := resolveWorkflowSettings(info, fps, format, preset, fpsExplicit, formatExplicit, presetExplicit, previewModeUsesTranscript(mode), chunkDurationSec, chunkExplicit)
	estimate := estimateOutputs(info, resolved.FPS, resolved.FrameFormat, mode, buildTranscriptPlan(info.DurationSec, appCfg, probeTranscribeAccel(appCfg, "", doctorHasGPU()).UsesGPU), resolved.ChunkDurationSec)
	result := map[string]any{
		"input":              input,
		"resolved":           videoPath,
		"source_note":        note,
		"video_info":         info,
		"target_fps":         resolved.FPS,
		"format":             resolved.FrameFormat,
		"mode":               mode,
		"preset":             resolved.Preset,
		"media_preset":       resolved.MediaPreset,
		"chunk_duration_sec": resolved.ChunkDurationSec,
		"estimate":           estimate,
	}
	if resolved.FPSMode != "" {
		result["fps_mode"] = resolved.FPSMode
	}
	return result, nil
}

func openLastCommand() *cobra.Command {
	var rootDir string
	var artifact string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "open-last",
		Short: "Open artifacts from the most recent run",
		Run: func(cmd *cobra.Command, args []string) {
			started := time.Now()
			if strings.TrimSpace(rootDir) == "" {
				rootDir = appCfg.FramesRoot
			}
			if strings.TrimSpace(rootDir) == "" {
				rootDir = media.DefaultFramesRoot
			}
			target, runPath, err := findLastRunArtifact(rootDir, artifact)
			if err != nil {
				if jsonOut {
					emitAutomationJSON("open-last", started, "error", map[string]string{"root": rootDir, "artifact": artifact}, err)
					markCommandFailure()
					return
				}
				failf("%v\n", err)
				return
			}
			if jsonOut {
				payload := map[string]string{
					"root":     rootDir,
					"artifact": artifact,
					"run_path": runPath,
					"path":     target,
				}
				emitAutomationJSON("open-last", started, "success", payload, nil)
				return
			}
			if err := openPathCLI(target); err != nil {
				failf("open failed: %v\n", err)
				return
			}
			fmt.Printf("Opened: %s\n", target)
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Runs root directory (default: config frames root)")
	cmd.Flags().StringVar(&artifact, "artifact", "run", "Artifact: run|transcript|transcript-json|transcript-srt|transcript-vtt|sheet|log|metadata|frames|manifest|metadata-csv|frames-zip|audio")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output machine-readable JSON without opening")
	return cmd
}

func copyLastCommand() *cobra.Command {
	var rootDir string
	var artifact string
	cmd := &cobra.Command{
		Use:   "copy-last",
		Short: "Copy path for an artifact from the most recent run",
		Run: func(cmd *cobra.Command, args []string) {
			if strings.TrimSpace(rootDir) == "" {
				rootDir = appCfg.FramesRoot
			}
			if strings.TrimSpace(rootDir) == "" {
				rootDir = media.DefaultFramesRoot
			}
			target, _, err := findLastRunArtifact(rootDir, artifact)
			if err != nil {
				failf("%v\n", err)
				return
			}
			if err := copyTextToClipboardCLI(target); err != nil {
				if errors.Is(err, errNoClipboardTool) {
					// Headless / server / WSL-without-clipboard: print the
					// path so the user (or a wrapping agent) can still act on
					// it without the command appearing to have failed.
					fmt.Println(target)
					fmt.Fprintln(os.Stderr, "note: no clipboard tool available; path printed to stdout")
					return
				}
				failf("copy failed: %v\n", err)
				return
			}
			fmt.Printf("Copied: %s\n", target)
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Runs root directory (default: config frames root)")
	cmd.Flags().StringVar(&artifact, "artifact", "run", "Artifact: run|transcript|transcript-json|transcript-srt|transcript-vtt|sheet|log|metadata|frames|manifest|metadata-csv|frames-zip|audio")
	return cmd
}

func artifactsCommand() *cobra.Command {
	var rootDir string
	var recent int
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "artifacts [run|latest]",
		Short: "List recent runs or show indexed artifacts for a run",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			started := time.Now()
			if strings.TrimSpace(rootDir) == "" {
				rootDir = appCfg.FramesRoot
			}
			if strings.TrimSpace(rootDir) == "" {
				rootDir = media.DefaultFramesRoot
			}

			if recent > 0 {
				runs, err := media.RecentRuns(rootDir, recent)
				if err != nil {
					if jsonOut {
						emitAutomationJSON("artifacts", started, "error", map[string]any{"root": rootDir, "recent": recent}, err)
						markCommandFailure()
						return
					}
					failf("%s\n", renderUserFacingError(err))
					return
				}
				payload := map[string]any{
					"root":      rootDir,
					"index":     media.IndexFilePath(rootDir),
					"recent":    recent,
					"run_count": len(runs),
					"runs":      runs,
				}
				if jsonOut {
					emitAutomationJSON("artifacts", started, "success", payload, nil)
					return
				}
				printRecentArtifacts(rootDir, runs)
				return
			}

			selector := "latest"
			if len(args) == 1 {
				selector = args[0]
			}
			run, err := media.SelectRun(rootDir, selector)
			if err != nil {
				if jsonOut {
					emitAutomationJSON("artifacts", started, "error", map[string]any{"root": rootDir, "run": selector}, err)
					markCommandFailure()
					return
				}
				failf("%s\n", renderUserFacingError(err))
				return
			}
			payload := map[string]any{
				"root":  rootDir,
				"index": media.IndexFilePath(rootDir),
				"run":   run,
			}
			if jsonOut {
				emitAutomationJSON("artifacts", started, "success", payload, nil)
				return
			}
			printRunArtifacts(run, media.IndexFilePath(rootDir))
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Runs root directory (default: config frames root)")
	cmd.Flags().IntVar(&recent, "recent", 0, "List the N most recent indexed runs instead of showing one run")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output machine-readable JSON")
	return cmd
}

func findLastRunArtifact(rootDir string, artifact string) (string, string, error) {
	run, err := media.SelectRun(rootDir, "latest")
	if err != nil {
		return "", "", err
	}
	target, err := artifactPathForRun(run, artifact)
	if err != nil {
		return "", run.Path, err
	}
	return target, run.Path, nil
}

func artifactPathForRun(run media.IndexedRun, artifact string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(artifact))
	if kind == "" {
		kind = "run"
	}
	switch kind {
	case "run":
		return run.Path, nil
	case "transcript":
		if run.Artifacts.TranscriptTxt == "" {
			return "", fmt.Errorf("transcript not found for run: %s", run.Path)
		}
		return run.Artifacts.TranscriptTxt, nil
	case "transcript-json", "transcript_json":
		if run.Artifacts.TranscriptJSON == "" {
			return "", fmt.Errorf("transcript json not found for run: %s", run.Path)
		}
		return run.Artifacts.TranscriptJSON, nil
	case "transcript-srt", "transcript_srt":
		if run.Artifacts.TranscriptSRT == "" {
			return "", fmt.Errorf("transcript srt not found for run: %s", run.Path)
		}
		return run.Artifacts.TranscriptSRT, nil
	case "transcript-vtt", "transcript_vtt":
		if run.Artifacts.TranscriptVTT == "" {
			return "", fmt.Errorf("transcript vtt not found for run: %s", run.Path)
		}
		return run.Artifacts.TranscriptVTT, nil
	case "sheet":
		if run.Artifacts.ContactSheet == "" {
			return "", fmt.Errorf("contact sheet not found for run: %s", run.Path)
		}
		return run.Artifacts.ContactSheet, nil
	case "log":
		if run.Artifacts.Log == "" {
			return "", fmt.Errorf("log not found for run: %s", run.Path)
		}
		return run.Artifacts.Log, nil
	case "metadata":
		if run.Artifacts.Metadata == "" {
			return "", fmt.Errorf("metadata not found for run: %s", run.Path)
		}
		return run.Artifacts.Metadata, nil
	case "frames":
		if run.Artifacts.Frames == "" {
			return "", fmt.Errorf("frames manifest not found for run: %s", run.Path)
		}
		return run.Artifacts.Frames, nil
	case "manifest", "transcription-manifest", "transcription_manifest":
		if run.Artifacts.TranscriptionManifest == "" {
			return "", fmt.Errorf("transcription manifest not found for run: %s", run.Path)
		}
		return run.Artifacts.TranscriptionManifest, nil
	case "metadata-csv", "metadata_csv":
		if run.Artifacts.MetadataCSV == "" {
			return "", fmt.Errorf("metadata csv not found for run: %s", run.Path)
		}
		return run.Artifacts.MetadataCSV, nil
	case "frames-zip", "frames_zip":
		if run.Artifacts.FramesZip == "" {
			return "", fmt.Errorf("frames zip not found for run: %s", run.Path)
		}
		return run.Artifacts.FramesZip, nil
	case "audio":
		if run.Artifacts.Audio == "" {
			return "", fmt.Errorf("audio not found for run: %s", run.Path)
		}
		return run.Artifacts.Audio, nil
	default:
		return "", fmt.Errorf("unsupported artifact %q (use run|transcript|transcript-json|transcript-srt|transcript-vtt|sheet|log|metadata|frames|manifest|metadata-csv|frames-zip|audio)", artifact)
	}
}

func latestRunArtifacts(rootDir string) (map[string]string, error) {
	run, err := media.SelectRun(rootDir, "latest")
	if err != nil {
		return nil, err
	}
	return runArtifactsPayload(run), nil
}

func runArtifactsPayload(run media.IndexedRun) map[string]string {
	out := map[string]string{
		"run":      run.Path,
		"metadata": run.Artifacts.Metadata,
	}
	if run.Artifacts.Frames != "" {
		out["frames"] = run.Artifacts.Frames
	}
	if run.Artifacts.ContactSheet != "" {
		out["sheet"] = run.Artifacts.ContactSheet
	}
	if run.Artifacts.Audio != "" {
		out["audio"] = run.Artifacts.Audio
	}
	if run.Artifacts.TranscriptTxt != "" {
		out["transcript"] = run.Artifacts.TranscriptTxt
	}
	if run.Artifacts.TranscriptJSON != "" {
		out["transcript_json"] = run.Artifacts.TranscriptJSON
	}
	if run.Artifacts.TranscriptSRT != "" {
		out["transcript_srt"] = run.Artifacts.TranscriptSRT
	}
	if run.Artifacts.TranscriptVTT != "" {
		out["transcript_vtt"] = run.Artifacts.TranscriptVTT
	}
	if run.Artifacts.TranscriptionManifest != "" {
		out["manifest"] = run.Artifacts.TranscriptionManifest
	}
	if run.Artifacts.Log != "" {
		out["log"] = run.Artifacts.Log
	}
	if run.Artifacts.MetadataCSV != "" {
		out["metadata_csv"] = run.Artifacts.MetadataCSV
	}
	if run.Artifacts.FramesZip != "" {
		out["frames_zip"] = run.Artifacts.FramesZip
	}
	return out
}

func printRecentArtifacts(rootDir string, runs []media.IndexedRun) {
	fmt.Println("Recent Runs")
	fmt.Println("-----------")
	fmt.Printf("Root: %s\n", rootDir)
	fmt.Printf("Index: %s\n", media.IndexFilePath(rootDir))
	if len(runs) == 0 {
		fmt.Println("Runs: 0")
		return
	}
	for _, run := range runs {
		created := valueOrDash(run.CreatedAt)
		source := valueOrDash(run.SourceVideoPath)
		fmt.Printf("\n- %s [%s]\n", run.Name, run.Status)
		fmt.Printf("  created: %s\n", created)
		fmt.Printf("  source:  %s\n", source)
		fmt.Printf("  run:     %s\n", run.Path)
		if run.Artifacts.TranscriptTxt != "" {
			fmt.Printf("  text:    %s\n", run.Artifacts.TranscriptTxt)
		}
		if run.Artifacts.ContactSheet != "" {
			fmt.Printf("  sheet:   %s\n", run.Artifacts.ContactSheet)
		}
	}
}

func printRunArtifacts(run media.IndexedRun, indexPath string) {
	fmt.Println("Run Artifacts")
	fmt.Println("-------------")
	fmt.Printf("Run:      %s\n", run.Name)
	fmt.Printf("Status:   %s\n", run.Status)
	fmt.Printf("Index:    %s\n", indexPath)
	fmt.Printf("Dir:      %s\n", run.Path)
	fmt.Printf("Created:  %s\n", valueOrDash(run.CreatedAt))
	fmt.Printf("Updated:  %s\n", valueOrDash(run.UpdatedAt))
	fmt.Printf("Source:   %s\n", valueOrDash(run.SourceVideoPath))
	fmt.Printf("FPS:      %.2f\n", run.FPS)
	fmt.Printf("Format:   %s\n", valueOrDash(run.Format))
	fmt.Printf("Preset:   %s\n", valueOrDash(run.Preset))
	if run.DurationSec > 0 {
		fmt.Printf("Duration: %.2fs\n", run.DurationSec)
	}
	if len(run.Warnings) > 0 {
		fmt.Printf("Warnings: %s\n", strings.Join(run.Warnings, ", "))
	}
	fmt.Println("Artifacts:")
	for _, line := range []struct {
		label string
		path  string
	}{
		{"run", run.Path},
		{"metadata", run.Artifacts.Metadata},
		{"frames", run.Artifacts.Frames},
		{"contact_sheet", run.Artifacts.ContactSheet},
		{"audio", run.Artifacts.Audio},
		{"transcript_txt", run.Artifacts.TranscriptTxt},
		{"transcript_json", run.Artifacts.TranscriptJSON},
		{"transcript_srt", run.Artifacts.TranscriptSRT},
		{"transcript_vtt", run.Artifacts.TranscriptVTT},
		{"transcription_manifest", run.Artifacts.TranscriptionManifest},
		{"log", run.Artifacts.Log},
		{"metadata_csv", run.Artifacts.MetadataCSV},
		{"frames_zip", run.Artifacts.FramesZip},
	} {
		if strings.TrimSpace(line.path) == "" {
			continue
		}
		fmt.Printf("- %s: %s\n", line.label, line.path)
	}
}

func openPathCLI(target string) error {
	if target == "" {
		return fmt.Errorf("path is required")
	}
	if _, err := os.Stat(target); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", target).Start()
	}
	if _, err := exec.LookPath("wslpath"); err == nil {
		winPathBytes, err := exec.Command("wslpath", "-w", target).Output()
		if err != nil {
			return err
		}
		winPath := strings.TrimSpace(string(winPathBytes))
		if winPath == "" {
			return fmt.Errorf("failed to convert path for explorer")
		}
		return exec.Command("cmd.exe", "/C", "start", "", winPath).Start()
	}
	return exec.Command("xdg-open", target).Start()
}

// errNoClipboardTool is returned when no platform clipboard binary is
// available. Callers that can usefully fall back (e.g. copy-last printing the
// path to stdout) should test for this sentinel rather than treating it as a
// hard failure.
var errNoClipboardTool = errors.New("no clipboard tool found (pbcopy/clip/wl-copy/xclip/xsel)")

func copyTextToClipboardCLI(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("nothing to copy")
	}
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd.exe", "/C", "clip")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if _, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return errNoClipboardTool
}

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run FramesCLI MCP server over stdio",
		Run: func(cmd *cobra.Command, args []string) {
			if err := runMCPServer(os.Stdin, os.Stdout); err != nil {
				failf("MCP server error: %v\n", err)
			}
		},
	}
	return cmd
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

const (
	defaultMCPToolTimeout = 20 * time.Minute
	maxMCPToolTimeout     = 2 * time.Hour
)

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	TimeoutMS int             `json:"timeout_ms,omitempty"`
}

type mcpCancelledParams struct {
	RequestID json.RawMessage `json:"requestId"`
}

type mcpServerState struct {
	mu         sync.Mutex
	active     map[string]context.CancelFunc
	writeMu    sync.Mutex
	out        io.Writer
	timeout    time.Duration
	maxTimeout time.Duration
	heartbeat  time.Duration
}

func (s *mcpServerState) writeNotification(method string, params any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n%s", len(raw), raw)
	return err
}

func (s *mcpServerState) startHeartbeat(ctx context.Context, toolName string) func() {
	switch toolName {
	case "extract", "extract_batch", "transcribe_run":
	default:
		return func() {}
	}
	interval := s.heartbeat
	if interval <= 0 {
		interval = 10 * time.Second
	}
	started := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				elapsed := int(time.Since(started).Seconds())
				verb := "working"
				switch toolName {
				case "extract", "extract_batch":
					verb = "extracting"
				case "transcribe_run":
					verb = "transcribing"
				}
				_ = s.writeNotification("notifications/message", map[string]any{
					"level":   "info",
					"message": fmt.Sprintf("%s... %ds elapsed", verb, elapsed),
				})
			}
		}
	}()
	return func() { close(done) }
}

func mcpAllowedRoots() []string {
	roots := make([]string, 0, 8)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if abs, err := filepath.Abs(p); err == nil {
			roots = append(roots, filepath.Clean(abs))
		}
	}
	add(appCfg.FramesRoot)
	add(appCfg.AgentOutputRoot)
	for _, d := range appCfg.AgentInputDirs {
		add(d)
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

func ensureMCPPathAllowed(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || path == "recent" {
		return nil
	}
	if strings.Contains(path, "://") {
		return fmt.Errorf("only local filesystem paths are allowed")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	for _, root := range mcpAllowedRoots() {
		rel, relErr := filepath.Rel(root, abs)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path outside allowed roots: %s", path)
}

func ensureMCPGlobAllowed(input string) error {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if strings.ContainsAny(input, "*?[") {
		base := input
		for i, r := range input {
			if r == '*' || r == '?' || r == '[' {
				base = strings.TrimSpace(input[:i])
				break
			}
		}
		if strings.TrimSpace(base) == "" {
			base = "."
		}
		return ensureMCPPathAllowed(base)
	}
	return ensureMCPPathAllowed(input)
}

func newMCPServerState(out io.Writer) *mcpServerState {
	return &mcpServerState{
		active:     map[string]context.CancelFunc{},
		out:        out,
		timeout:    defaultMCPToolTimeout,
		maxTimeout: maxMCPToolTimeout,
		heartbeat:  10 * time.Second,
	}
}

func (s *mcpServerState) write(resp *mcpResponse) error {
	if resp == nil {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeMCPMessage(s.out, resp)
}

func (s *mcpServerState) registerCall(id json.RawMessage, cancel context.CancelFunc) {
	key := string(id)
	if key == "" || cancel == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[key] = cancel
}

func (s *mcpServerState) completeCall(id json.RawMessage) {
	key := string(id)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, key)
}

func (s *mcpServerState) cancelCall(id json.RawMessage) bool {
	key := string(id)
	if key == "" {
		return false
	}
	s.mu.Lock()
	cancel, ok := s.active[key]
	if ok {
		delete(s.active, key)
	}
	s.mu.Unlock()
	if ok && cancel != nil {
		cancel()
		return true
	}
	return false
}

func (s *mcpServerState) toolTimeout(overrideMS int) time.Duration {
	timeout := s.timeout
	if overrideMS > 0 {
		timeout = time.Duration(overrideMS) * time.Millisecond
	}
	if timeout <= 0 {
		timeout = defaultMCPToolTimeout
	}
	if timeout > s.maxTimeout {
		timeout = s.maxTimeout
	}
	return timeout
}

func runMCPServer(in io.Reader, out io.Writer) error {
	state := newMCPServerState(out)
	return runMCPServerWithState(in, state)
}

func runMCPServerWithState(in io.Reader, state *mcpServerState) error {
	br := bufio.NewReader(in)
	for {
		req, err := readMCPMessage(br)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if req.Method == "notifications/cancelled" {
			var params mcpCancelledParams
			_ = json.Unmarshal(req.Params, &params)
			_ = state.cancelCall(params.RequestID)
			continue
		}
		if req.Method == "tools/call" {
			if len(req.ID) == 0 {
				continue
			}
			var params mcpToolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				if writeErr := state.write(&mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: buildMCPError(-32602, errors.New("invalid params"))}); writeErr != nil {
					return writeErr
				}
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), state.toolTimeout(params.TimeoutMS))
			state.registerCall(req.ID, cancel)
			go func(reqID json.RawMessage, p mcpToolCallParams, ctx context.Context, cancel context.CancelFunc) {
				defer cancel()
				defer state.completeCall(reqID)
				stopHeartbeat := state.startHeartbeat(ctx, p.Name)
				defer stopHeartbeat()
				result, err := callMCPTool(ctx, p.Name, p.Arguments)
				if err != nil {
					code := -32000
					var invalidParams *mcpInvalidParamsError
					if errors.Is(ctx.Err(), context.Canceled) {
						code = -32800
						err = errors.New("tool call cancelled")
					} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						code = -32001
						err = errors.New("tool call timed out")
					} else if errors.As(err, &invalidParams) {
						code = -32602
					}
					_ = state.write(&mcpResponse{
						JSONRPC: "2.0",
						ID:      reqID,
						Error:   buildMCPError(code, err),
					})
					return
				}
				raw, _ := json.MarshalIndent(result, "", "  ")
				_ = state.write(&mcpResponse{
					JSONRPC: "2.0",
					ID:      reqID,
					Result: map[string]any{
						"content": []map[string]any{
							{"type": "text", "text": string(raw)},
						},
						"structuredContent": result,
					},
				})
			}(req.ID, params, ctx, cancel)
			continue
		}
		resp := handleMCPRequest(req)
		if resp == nil {
			continue
		}
		if err := state.write(resp); err != nil {
			return err
		}
	}
}

func handleMCPRequest(req mcpRequest) *mcpResponse {
	makeErr := func(code int, msg string) *mcpResponse {
		return &mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: buildMCPError(code, errors.New(msg))}
	}
	if req.Method == "notifications/initialized" || len(req.ID) == 0 {
		return nil
	}
	switch req.Method {
	case "initialize":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]any{
					"name":    "framescli",
					"version": "0.1.0",
				},
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
			},
		}
	case "tools/list":
		tools := []map[string]any{
			{
				"name":        "doctor",
				"description": "Check local toolchain readiness for FramesCLI",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			},
			{
				"name":        "preview",
				"description": "Dry-run output estimate for a video input (defaults to recent when omitted)",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input":          map[string]any{"type": "string"},
						"fps":            map[string]any{"type": "number"},
						"format":         map[string]any{"type": "string"},
						"mode":           map[string]any{"type": "string"},
						"preset":         map[string]any{"type": "string"},
						"chunk_duration": map[string]any{"type": "integer"},
					},
				},
			},
			{
				"name":        "extract",
				"description": "Run single-video extraction workflow (input defaults to recent, or use url for remote videos)",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input":               map[string]any{"type": "string"},
						"url":                 map[string]any{"type": "string"},
						"no_cache":            map[string]any{"type": "boolean"},
						"fps":                 map[string]any{"type": "number"},
						"voice":               map[string]any{"type": "boolean"},
						"format":              map[string]any{"type": "string"},
						"out":                 map[string]any{"type": "string"},
						"hwaccel":             map[string]any{"type": "string"},
						"preset":              map[string]any{"type": "string"},
						"chunk_duration":      map[string]any{"type": "integer"},
						"allow_expensive":     map[string]any{"type": "boolean"},
						"from":                map[string]any{"type": "string"},
						"to":                  map[string]any{"type": "string"},
						"frame_start":         map[string]any{"type": "integer"},
						"frame_end":           map[string]any{"type": "integer"},
						"every_n":             map[string]any{"type": "integer"},
						"name_template":       map[string]any{"type": "string"},
						"transcribe_backend":  map[string]any{"type": "string"},
						"transcribe_bin":      map[string]any{"type": "string"},
						"transcribe_language": map[string]any{"type": "string"},
					},
				},
			},
			{
				"name":        "extract_batch",
				"description": "Run extraction for multiple inputs/globs (falls back to configured agent_input_dirs)",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"inputs":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"fps":                 map[string]any{"type": "number"},
						"voice":               map[string]any{"type": "boolean"},
						"format":              map[string]any{"type": "string"},
						"preset":              map[string]any{"type": "string"},
						"chunk_duration":      map[string]any{"type": "integer"},
						"allow_expensive":     map[string]any{"type": "boolean"},
						"transcribe_backend":  map[string]any{"type": "string"},
						"transcribe_bin":      map[string]any{"type": "string"},
						"transcribe_language": map[string]any{"type": "string"},
					},
				},
			},
			{
				"name":        "open_last",
				"description": "Resolve path for artifact from most recent run",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root":     map[string]any{"type": "string"},
						"artifact": map[string]any{"type": "string"},
					},
				},
			},
			{
				"name":        "get_latest_artifacts",
				"description": "Return paths for available artifacts from the most recent run",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root": map[string]any{"type": "string"},
					},
				},
			},
			{
				"name":        "get_run_artifacts",
				"description": "Return indexed artifacts for the latest, a specific run, or a recent run list",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root":   map[string]any{"type": "string"},
						"run":    map[string]any{"type": "string"},
						"recent": map[string]any{"type": "integer"},
					},
				},
			},
			{
				"name":        "transcribe_run",
				"description": "Resume transcription for an existing run directory",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"run_dir":        map[string]any{"type": "string"},
						"chunk_duration": map[string]any{"type": "integer"},
					},
				},
			},
			{
				"name":        "prefs_get",
				"description": "Get agent path preferences for input and output roots",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			},
			{
				"name":        "prefs_set",
				"description": "Persist agent path preferences for input and output roots",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input_dirs":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"output_root": map[string]any{"type": "string"},
					},
				},
			},
		}
		return &mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools}}
	default:
		return makeErr(-32601, "method not found")
	}
}

// mcpInvalidParamsError wraps an argument-decoding failure so the outer
// tools/call handler can return JSON-RPC -32602 "Invalid params" instead of
// the generic -32000 used for tool execution errors.
type mcpInvalidParamsError struct{ inner error }

func (e *mcpInvalidParamsError) Error() string { return e.inner.Error() }
func (e *mcpInvalidParamsError) Unwrap() error { return e.inner }

// decodeMCPArgs parses a tool's arguments payload with DisallowUnknownFields,
// so a caller that sends e.g. {"video":"..."} to a tool expecting {"input":...}
// gets a clear -32602 error instead of a silent fallback that processes the
// wrong file. An absent or empty argsRaw is treated as {} so tools that take
// no parameters remain convenient to invoke.
func decodeMCPArgs(argsRaw json.RawMessage, target any) error {
	trimmed := bytes.TrimSpace(argsRaw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] != '{' {
		return &mcpInvalidParamsError{inner: fmt.Errorf("arguments must be a JSON object")}
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return &mcpInvalidParamsError{inner: err}
	}
	return nil
}

func callMCPTool(ctx context.Context, name string, argsRaw json.RawMessage) (any, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	switch name {
	case "doctor":
		var args struct{}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		started := time.Now()
		return buildAutomationEnvelope("doctor", started, "success", collectDoctorReport(appCfg), nil), nil
	case "preview":
		var args struct {
			Input         string  `json:"input"`
			FPS           float64 `json:"fps"`
			Format        string  `json:"format"`
			Mode          string  `json:"mode"`
			Preset        string  `json:"preset"`
			ChunkDuration int     `json:"chunk_duration"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.Input) == "" {
			args.Input = "recent"
		}
		if err := ensureMCPPathAllowed(args.Input); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.Mode) == "" {
			args.Mode = "both"
		}
		presetExplicit := strings.TrimSpace(args.Preset) != ""
		if !presetExplicit {
			args.Preset = appCfg.PerformanceMode
		}
		started := time.Now()
		data, err := runPreviewResult(args.Input, args.FPS, args.Format, args.Mode, args.Preset, args.ChunkDuration, args.FPS > 0, strings.TrimSpace(args.Format) != "", presetExplicit, args.ChunkDuration > 0)
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return buildAutomationEnvelope("preview", started, "error", map[string]string{"input": args.Input}, err), nil
		}
		return buildAutomationEnvelope("preview", started, "success", data, nil), nil
	case "extract":
		var args struct {
			Input              string  `json:"input"`
			URL                string  `json:"url"`
			NoCache            bool    `json:"no_cache"`
			FPS                float64 `json:"fps"`
			Voice              bool    `json:"voice"`
			Format             string  `json:"format"`
			Out                string  `json:"out"`
			HWAccel            string  `json:"hwaccel"`
			Preset             string  `json:"preset"`
			From               string  `json:"from"`
			To                 string  `json:"to"`
			FrameStart         int     `json:"frame_start"`
			FrameEnd           int     `json:"frame_end"`
			EveryN             int     `json:"every_n"`
			NameTemplate       string  `json:"name_template"`
			ChunkDuration      int     `json:"chunk_duration"`
			AllowExpensive     bool    `json:"allow_expensive"`
			TranscribeBackend  string  `json:"transcribe_backend"`
			TranscribeBin      string  `json:"transcribe_bin"`
			TranscribeLanguage string  `json:"transcribe_language"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}

		// Validate URL vs Input mutually exclusive
		hasURL := strings.TrimSpace(args.URL) != ""
		hasInput := strings.TrimSpace(args.Input) != ""
		if hasURL && hasInput {
			return nil, fmt.Errorf("cannot specify both url and input parameters")
		}
		if !hasURL && !hasInput {
			args.Input = "recent"
			hasInput = true
		}

		// Validate paths only for local input, not URLs
		if hasInput {
			if err := ensureMCPPathAllowed(args.Input); err != nil {
				return nil, err
			}
		}
		fpsExplicit := args.FPS > 0
		formatExplicit := strings.TrimSpace(args.Format) != ""
		presetExplicit := strings.TrimSpace(args.Preset) != ""
		if args.FPS <= 0 {
			args.FPS = appCfg.DefaultFPS
		}
		if !formatExplicit {
			args.Format = appCfg.DefaultFormat
		}
		if strings.TrimSpace(args.HWAccel) == "" {
			args.HWAccel = appCfg.HWAccel
		}
		if !presetExplicit {
			args.Preset = appCfg.PerformanceMode
		}
		if strings.TrimSpace(args.TranscribeBackend) == "" {
			args.TranscribeBackend = appCfg.TranscribeBackend
		}
		if strings.TrimSpace(args.TranscribeLanguage) == "" {
			args.TranscribeLanguage = appCfg.WhisperLanguage
		}
		if strings.TrimSpace(args.Out) == "" && strings.TrimSpace(appCfg.AgentOutputRoot) != "" {
			args.Out = filepath.Join(appCfg.AgentOutputRoot, "Run_"+time.Now().UTC().Format("20060102-150405"))
		}
		if err := ensureMCPPathAllowed(args.Out); err != nil {
			return nil, err
		}
		started := time.Now()

		// Determine input to use (URL or path)
		videoInput := args.Input
		if hasURL {
			videoInput = args.URL
		}

		res, err := runExtractWorkflowResult(ctx, videoInput, args.FPS, extractWorkflowOptions{
			URL:                args.URL,
			NoCache:            args.NoCache,
			OutDir:             args.Out,
			Voice:              args.Voice,
			FrameFormat:        args.Format,
			FormatExplicit:     formatExplicit,
			HWAccel:            args.HWAccel,
			Preset:             args.Preset,
			PresetExplicit:     presetExplicit,
			FPSExplicit:        fpsExplicit,
			StartTime:          args.From,
			EndTime:            args.To,
			StartFrame:         args.FrameStart,
			EndFrame:           args.FrameEnd,
			EveryN:             args.EveryN,
			FrameName:          args.NameTemplate,
			ChunkDurationSec:   args.ChunkDuration,
			ChunkExplicit:      args.ChunkDuration > 0,
			AllowExpensive:     args.AllowExpensive,
			TranscribeBackend:  args.TranscribeBackend,
			TranscribeBin:      args.TranscribeBin,
			TranscribeLanguage: args.TranscribeLanguage,
			JPGQuality:         3,
			SheetCols:          6,
		}, false)
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			err = withDiagnostics("mcp_extract", videoInput, args.Out, "", err)
			return buildAutomationEnvelope("extract", started, "error", map[string]string{"input": videoInput}, err), nil
		}
		return buildAutomationEnvelope("extract", started, "success", res, nil), nil
	case "extract_batch":
		var args struct {
			Inputs             []string `json:"inputs"`
			FPS                float64  `json:"fps"`
			Voice              bool     `json:"voice"`
			Format             string   `json:"format"`
			Preset             string   `json:"preset"`
			ChunkDuration      int      `json:"chunk_duration"`
			AllowExpensive     bool     `json:"allow_expensive"`
			TranscribeBackend  string   `json:"transcribe_backend"`
			TranscribeBin      string   `json:"transcribe_bin"`
			TranscribeLanguage string   `json:"transcribe_language"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		for _, in := range args.Inputs {
			if err := ensureMCPGlobAllowed(in); err != nil {
				return nil, err
			}
		}
		if len(args.Inputs) == 0 && len(appCfg.AgentInputDirs) > 0 {
			exts := appCfg.RecentExts
			if len(exts) == 0 {
				exts = []string{"mp4", "mkv", "mov"}
			}
			for _, d := range appCfg.AgentInputDirs {
				trimmed := strings.TrimSpace(d)
				if trimmed == "" {
					continue
				}
				for _, ext := range exts {
					cleanExt := strings.TrimPrefix(strings.TrimSpace(ext), ".")
					if cleanExt == "" {
						continue
					}
					args.Inputs = append(args.Inputs, filepath.Join(trimmed, "*."+cleanExt))
				}
			}
		}
		for _, in := range args.Inputs {
			if err := ensureMCPGlobAllowed(in); err != nil {
				return nil, err
			}
		}
		if len(args.Inputs) == 0 {
			return nil, fmt.Errorf("extract_batch requires inputs (or configured agent_input_dirs)")
		}
		fpsExplicit := args.FPS > 0
		if args.FPS <= 0 {
			args.FPS = appCfg.DefaultFPS
		}
		formatExplicit := strings.TrimSpace(args.Format) != ""
		presetExplicit := strings.TrimSpace(args.Preset) != ""
		if !formatExplicit {
			args.Format = appCfg.DefaultFormat
		}
		if !presetExplicit {
			args.Preset = appCfg.PerformanceMode
		}
		if strings.TrimSpace(args.TranscribeBackend) == "" {
			args.TranscribeBackend = appCfg.TranscribeBackend
		}
		if strings.TrimSpace(args.TranscribeLanguage) == "" {
			args.TranscribeLanguage = appCfg.WhisperLanguage
		}
		started := time.Now()
		videos := expandVideoInputs(args.Inputs)
		items := make([]batchItemResult, 0, len(videos))
		failures := 0
		for _, v := range videos {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			outDir := ""
			if strings.TrimSpace(appCfg.AgentOutputRoot) != "" {
				stem := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
				outDir = filepath.Join(appCfg.AgentOutputRoot, stem+"-"+time.Now().UTC().Format("20060102-150405"))
			}
			res, err := runExtractWorkflowResult(ctx, v, args.FPS, extractWorkflowOptions{
				Voice:              args.Voice,
				FrameFormat:        args.Format,
				FormatExplicit:     formatExplicit,
				TranscribeBackend:  args.TranscribeBackend,
				TranscribeBin:      args.TranscribeBin,
				TranscribeLanguage: args.TranscribeLanguage,
				ChunkDurationSec:   args.ChunkDuration,
				ChunkExplicit:      args.ChunkDuration > 0,
				AllowExpensive:     args.AllowExpensive,
				JPGQuality:         3,
				SheetCols:          6,
				HWAccel:            appCfg.HWAccel,
				Preset:             args.Preset,
				PresetExplicit:     presetExplicit,
				FPSExplicit:        fpsExplicit,
				OutDir:             outDir,
			}, false)
			if err != nil {
				if ctx != nil && ctx.Err() != nil {
					return nil, ctx.Err()
				}
				err = withDiagnostics("mcp_extract_batch", v, outDir, "", err)
				failures++
				items = append(items, batchItemResult{Input: v, Status: "error", Error: err.Error()})
				continue
			}
			items = append(items, batchItemResult{Input: v, Status: "success", Result: &res})
		}
		status := "success"
		if failures > 0 && failures < len(videos) {
			status = "partial"
		} else if failures == len(videos) {
			status = "error"
		}
		payload := map[string]any{"total": len(videos), "failures": failures, "items": items}
		return buildAutomationEnvelope("extract-batch", started, status, payload, nil), nil
	case "open_last":
		var args struct {
			Root     string `json:"root"`
			Artifact string `json:"artifact"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.Root) == "" && strings.TrimSpace(appCfg.AgentOutputRoot) != "" {
			args.Root = appCfg.AgentOutputRoot
		}
		if strings.TrimSpace(args.Root) == "" {
			args.Root = appCfg.FramesRoot
		}
		if strings.TrimSpace(args.Root) == "" {
			args.Root = media.DefaultFramesRoot
		}
		if strings.TrimSpace(args.Artifact) == "" {
			args.Artifact = "run"
		}
		if err := ensureMCPPathAllowed(args.Root); err != nil {
			return nil, err
		}
		started := time.Now()
		path, runPath, err := findLastRunArtifact(args.Root, args.Artifact)
		if err != nil {
			return buildAutomationEnvelope("open-last", started, "error", map[string]string{"root": args.Root, "artifact": args.Artifact}, err), nil
		}
		return buildAutomationEnvelope("open-last", started, "success", map[string]string{
			"root":     args.Root,
			"artifact": args.Artifact,
			"run_path": runPath,
			"path":     path,
		}, nil), nil
	case "get_latest_artifacts":
		var args struct {
			Root string `json:"root"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.Root) == "" && strings.TrimSpace(appCfg.AgentOutputRoot) != "" {
			args.Root = appCfg.AgentOutputRoot
		}
		if strings.TrimSpace(args.Root) == "" {
			args.Root = appCfg.FramesRoot
		}
		if strings.TrimSpace(args.Root) == "" {
			args.Root = media.DefaultFramesRoot
		}
		if err := ensureMCPPathAllowed(args.Root); err != nil {
			return nil, err
		}
		started := time.Now()
		artifacts, err := latestRunArtifacts(args.Root)
		if err != nil {
			return buildAutomationEnvelope("get_latest_artifacts", started, "error", map[string]string{"root": args.Root}, err), nil
		}
		return buildAutomationEnvelope("get_latest_artifacts", started, "success", map[string]any{
			"root":      args.Root,
			"artifacts": artifacts,
		}, nil), nil
	case "get_run_artifacts":
		var args struct {
			Root   string `json:"root"`
			Run    string `json:"run"`
			Recent int    `json:"recent"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.Root) == "" && strings.TrimSpace(appCfg.AgentOutputRoot) != "" {
			args.Root = appCfg.AgentOutputRoot
		}
		if strings.TrimSpace(args.Root) == "" {
			args.Root = appCfg.FramesRoot
		}
		if strings.TrimSpace(args.Root) == "" {
			args.Root = media.DefaultFramesRoot
		}
		if err := ensureMCPPathAllowed(args.Root); err != nil {
			return nil, err
		}
		started := time.Now()
		if args.Recent > 0 {
			runs, err := media.RecentRuns(args.Root, args.Recent)
			if err != nil {
				return buildAutomationEnvelope("get_run_artifacts", started, "error", map[string]any{"root": args.Root, "recent": args.Recent}, err), nil
			}
			return buildAutomationEnvelope("get_run_artifacts", started, "success", map[string]any{
				"root":  args.Root,
				"index": media.IndexFilePath(args.Root),
				"runs":  runs,
			}, nil), nil
		}
		if strings.TrimSpace(args.Run) == "" {
			args.Run = "latest"
		}
		run, err := media.SelectRun(args.Root, args.Run)
		if err != nil {
			return buildAutomationEnvelope("get_run_artifacts", started, "error", map[string]any{"root": args.Root, "run": args.Run}, err), nil
		}
		return buildAutomationEnvelope("get_run_artifacts", started, "success", map[string]any{
			"root":      args.Root,
			"index":     media.IndexFilePath(args.Root),
			"run":       run,
			"artifacts": runArtifactsPayload(run),
		}, nil), nil
	case "transcribe_run":
		var args struct {
			RunDir        string `json:"run_dir"`
			ChunkDuration int    `json:"chunk_duration"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		if strings.TrimSpace(args.RunDir) == "" {
			return nil, fmt.Errorf("transcribe_run requires run_dir")
		}
		if err := ensureMCPPathAllowed(args.RunDir); err != nil {
			return nil, err
		}
		started := time.Now()
		res, err := runTranscribeRunResult(ctx, args.RunDir, media.TranscribeOptions{
			Model:            appCfg.WhisperModel,
			Backend:          appCfg.TranscribeBackend,
			Language:         appCfg.WhisperLanguage,
			ChunkDurationSec: args.ChunkDuration,
			TimeoutSec:       0,
			Verbose:          false,
		})
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return buildAutomationEnvelope("transcribe_run", started, "error", map[string]string{"run_dir": args.RunDir}, err), nil
		}
		return buildAutomationEnvelope("transcribe_run", started, "success", res, nil), nil
	case "prefs_get":
		started := time.Now()
		payload := map[string]any{
			"input_dirs":  appCfg.AgentInputDirs,
			"output_root": appCfg.AgentOutputRoot,
			"frames_root": appCfg.FramesRoot,
		}
		return buildAutomationEnvelope("prefs_get", started, "success", payload, nil), nil
	case "prefs_set":
		var args struct {
			InputDirs  *[]string `json:"input_dirs"`
			OutputRoot *string   `json:"output_root"`
		}
		if err := decodeMCPArgs(argsRaw, &args); err != nil {
			return nil, err
		}
		cfg := appCfg
		if args.InputDirs != nil {
			for _, p := range *args.InputDirs {
				if strings.Contains(strings.TrimSpace(p), "://") {
					return nil, fmt.Errorf("prefs_set input_dirs must be local filesystem paths")
				}
			}
			cfg.AgentInputDirs = *args.InputDirs
		}
		if args.OutputRoot != nil {
			if strings.Contains(strings.TrimSpace(*args.OutputRoot), "://") {
				return nil, fmt.Errorf("prefs_set output_root must be a local filesystem path")
			}
			cfg.AgentOutputRoot = strings.TrimSpace(*args.OutputRoot)
		}
		started := time.Now()
		path, err := appconfig.Save(cfg)
		if err != nil {
			return buildAutomationEnvelope("prefs_set", started, "error", nil, err), nil
		}
		appCfg = cfg
		payload := map[string]any{
			"config_path": path,
			"input_dirs":  appCfg.AgentInputDirs,
			"output_root": appCfg.AgentOutputRoot,
		}
		return buildAutomationEnvelope("prefs_set", started, "success", payload, nil), nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func readMCPMessage(br *bufio.Reader) (mcpRequest, error) {
	headers := map[string]string{}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return mcpRequest{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
	}
	cl, ok := headers["content-length"]
	if !ok {
		return mcpRequest{}, fmt.Errorf("missing content-length header")
	}
	n, err := strconv.Atoi(cl)
	if err != nil || n <= 0 {
		return mcpRequest{}, fmt.Errorf("invalid content-length")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(br, body); err != nil {
		return mcpRequest{}, err
	}
	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return mcpRequest{}, err
	}
	return req, nil
}

func writeMCPMessage(out io.Writer, resp *mcpResponse) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(raw)); err != nil {
		return err
	}
	_, err = out.Write(raw)
	return err
}

func sheetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sheet <framesDir>",
		Short: "Build a contact sheet from extracted frames",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cols, _ := cmd.Flags().GetInt("cols")
			outFile, _ := cmd.Flags().GetString("out")
			verbose, _ := cmd.Flags().GetBool("verbose")
			fmt.Println("Generating contact sheet...")
			result, err := media.CreateContactSheet(args[0], cols, outFile, verbose)
			if err != nil {
				failf("Contact sheet failed: %v\n", err)
				return
			}
			fmt.Printf("Done. Contact sheet: %s\n", result)
		},
	}
	cmd.Flags().Int("cols", 6, "Columns in contact sheet")
	cmd.Flags().String("out", "", "Output file")
	cmd.Flags().Bool("verbose", false, "Show raw ffmpeg logs")
	return cmd
}

func cleanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean [targetDir]",
		Short: "Clean extracted frame artifacts (selective pruning via --older-than or --keep-last)",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			var target string
			if len(args) > 0 {
				target = strings.Join(args, " ")
			}
			if target == "" {
				target = appCfg.FramesRoot
			}
			if target == "" {
				target = media.DefaultFramesRoot
			}

			olderThanSpec, _ := cmd.Flags().GetString("older-than")
			keepLast, _ := cmd.Flags().GetInt("keep-last")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			// If no selective flag was passed, preserve the historical
			// "wipe everything" behavior so existing workflows don't change.
			if strings.TrimSpace(olderThanSpec) == "" && keepLast <= 0 {
				fmt.Printf("Cleaning frames in: %s\n", target)
				if dryRun {
					fmt.Println("(dry run) would remove the entire frames root")
					return
				}
				removed, err := media.CleanFrames(target)
				if err != nil {
					failf("Frame cleanup failed: %v\n", err)
					return
				}
				if removed == 0 {
					fmt.Println("Done. Nothing to remove.")
					return
				}
				fmt.Println("Done. Frames folder reset.")
				return
			}

			var olderThan time.Duration
			if strings.TrimSpace(olderThanSpec) != "" {
				d, err := parseDurationWithDays(olderThanSpec)
				if err != nil {
					failf("invalid --older-than %q: %v\n", olderThanSpec, err)
					return
				}
				olderThan = d
			}

			result, err := media.PruneFrames(media.PruneFramesOptions{
				RootDir:   target,
				OlderThan: olderThan,
				KeepLast:  keepLast,
				DryRun:    dryRun,
			})
			if err != nil {
				failf("Prune failed: %v\n", err)
				return
			}
			label := "Pruned"
			if dryRun {
				label = "Would prune"
			}
			fmt.Printf("%s in: %s\n", label, result.RootDir)
			fmt.Printf("Runs considered: %d\n", result.TotalRuns)
			if len(result.Pruned) == 0 {
				fmt.Println("Nothing matched the prune criteria.")
				return
			}
			for _, c := range result.Pruned {
				fmt.Printf("- %s (%s, %s) [%s]\n", c.Path, c.ModTime.Format("2006-01-02"), humanSize(c.SizeBytes), c.Reason)
			}
			fmt.Printf("%s: %d runs, %s reclaimed\n", label, len(result.Pruned), humanSize(result.BytesFreed))
		},
	}
	cmd.Flags().String("older-than", "", "Prune runs older than this age (e.g. '30d', '12h', '2h30m')")
	cmd.Flags().Int("keep-last", 0, "Keep only the N most-recent runs; prune the rest")
	cmd.Flags().Bool("dry-run", false, "Preview what would be pruned without removing anything")
	return cmd
}

// parseDurationWithDays extends time.ParseDuration with a "Nd" unit for days,
// since the stdlib stops at hours. Examples: "30d", "7d12h", "2h".
func parseDurationWithDays(spec string) (time.Duration, error) {
	s := strings.TrimSpace(strings.ToLower(spec))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var days time.Duration
	if idx := strings.Index(s, "d"); idx > 0 {
		n, err := strconv.Atoi(s[:idx])
		if err != nil {
			return 0, fmt.Errorf("bad days component: %w", err)
		}
		days = time.Duration(n) * 24 * time.Hour
		s = strings.TrimSpace(s[idx+1:])
	}
	if s == "" {
		return days, nil
	}
	rest, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	return days + rest, nil
}

// framesRootStorageThreshold is the size at which extract's end-of-run summary
// nags the user to clean up. Deliberately loose (5 GB) — anyone below this
// doesn't need a reminder; anyone above benefits from knowing the number.
const framesRootStorageThreshold = 5 * 1024 * 1024 * 1024

// framesRootStorageHint returns a one-line soft warning when the frames root
// exceeds the storage threshold. Empty string when below threshold, unset, or
// the path doesn't exist yet. Non-fatal on probe errors (returns "").
func framesRootStorageHint(framesRoot string) string {
	root := strings.TrimSpace(framesRoot)
	if root == "" {
		return ""
	}
	size, runs, err := framesRootUsage(root)
	if err != nil || size < framesRootStorageThreshold {
		return ""
	}
	return fmt.Sprintf("Storage:       %s in %s across %d runs — 'framescli clean --older-than 30d' to prune",
		humanSize(size), root, runs)
}

// framesRootUsage returns (total bytes, run count). A "run" is a direct-child
// directory containing run.json; other files are counted in size but not runs.
func framesRootUsage(root string) (int64, int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, 0, err
	}
	var total int64
	runs := 0
	for _, e := range entries {
		path := filepath.Join(root, e.Name())
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(path, "run.json")); err == nil {
				runs++
			}
			size, _ := walkDirSize(path)
			total += size
			continue
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total, runs, nil
}

func walkDirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func humanSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func transcribeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transcribe <audioPath> [outDir]",
		Short: "Transcribe an audio file (auto|whisper|faster-whisper)",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			verbose, _ := cmd.Flags().GetBool("verbose")
			model, _ := cmd.Flags().GetString("model")
			backend, _ := cmd.Flags().GetString("backend")
			bin, _ := cmd.Flags().GetString("bin")
			language, _ := cmd.Flags().GetString("language")
			chunkDuration, _ := cmd.Flags().GetInt("chunk-duration")
			appconfig.ApplyEnvDefaults(appCfg)
			outDir := "voice"
			if len(args) == 2 {
				outDir = args[1]
			}
			result, err := media.TranscribeAudioWithDetails(media.TranscribeOptions{
				Context:          cmd.Context(),
				AudioPath:        args[0],
				OutDir:           outDir,
				Model:            model,
				Backend:          backend,
				Bin:              bin,
				Language:         language,
				ChunkDurationSec: chunkDuration,
				Verbose:          verbose,
			})
			if err != nil {
				failf("Transcription failed: %v\n", err)
				return
			}
			fmt.Printf("Transcript: %s\nJSON: %s\n", result.TranscriptTxt, result.TranscriptJSON)
			if result.ManifestPath != "" {
				fmt.Printf("Manifest: %s\n", result.ManifestPath)
			}
		},
	}
	cmd.Flags().Bool("verbose", false, "Show raw transcription backend logs")
	cmd.Flags().String("model", appCfg.WhisperModel, "Transcription model")
	cmd.Flags().String("backend", appCfg.TranscribeBackend, "Transcription backend: auto|whisper|faster-whisper")
	cmd.Flags().String("bin", "", "Override transcription CLI binary path/name for selected backend")
	cmd.Flags().String("language", appCfg.WhisperLanguage, "Transcription language override (blank=auto)")
	cmd.Flags().Int("chunk-duration", 0, "Split transcription into N-second chunks and persist resume manifest (0 disables chunking)")
	return cmd
}

func transcribeRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transcribe-run <runDir>",
		Short: "Resume transcription for an existing extracted run",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			started := time.Now()
			verbose, _ := cmd.Flags().GetBool("verbose")
			model, _ := cmd.Flags().GetString("model")
			backend, _ := cmd.Flags().GetString("backend")
			bin, _ := cmd.Flags().GetString("bin")
			language, _ := cmd.Flags().GetString("language")
			chunkDuration, _ := cmd.Flags().GetInt("chunk-duration")
			timeoutSec, _ := cmd.Flags().GetInt("timeout")
			jsonOut, _ := cmd.Flags().GetBool("json")
			appconfig.ApplyEnvDefaults(appCfg)
			result, err := runTranscribeRunResult(cmd.Context(), args[0], media.TranscribeOptions{
				Model:            model,
				Backend:          backend,
				Bin:              bin,
				Language:         language,
				ChunkDurationSec: chunkDuration,
				TimeoutSec:       timeoutSec,
				Verbose:          verbose,
			})
			if err != nil {
				if jsonOut {
					emitAutomationJSON("transcribe-run", started, "error", map[string]string{"run_dir": args[0]}, err)
					if isCancellationError(err) {
						markCommandInterrupted()
					} else {
						markCommandFailure()
					}
					return
				}
				if isCancellationError(err) {
					markCommandInterrupted()
					fmt.Fprintln(os.Stderr, "Cancelled.")
				} else {
					failf("%s\n", renderUserFacingError(err))
				}
				return
			}
			if jsonOut {
				emitAutomationJSON("transcribe-run", started, "success", result, nil)
				return
			}
			if result.AlreadyPresent {
				fmt.Printf("Transcript already present: %s\n", result.TranscriptJSON)
				if result.TranscriptTxt != "" {
					fmt.Printf("Transcript: %s\n", result.TranscriptTxt)
				}
				if result.ManifestPath != "" {
					fmt.Printf("Manifest: %s\n", result.ManifestPath)
				}
				return
			}
			fmt.Printf("Transcript: %s\nJSON: %s\n", result.TranscriptTxt, result.TranscriptJSON)
			if result.ManifestPath != "" {
				fmt.Printf("Manifest: %s\n", result.ManifestPath)
			}
		},
	}
	cmd.Flags().Bool("verbose", false, "Show raw transcription backend logs")
	cmd.Flags().String("model", appCfg.WhisperModel, "Transcription model")
	cmd.Flags().String("backend", appCfg.TranscribeBackend, "Transcription backend: auto|whisper|faster-whisper")
	cmd.Flags().String("bin", "", "Override transcription CLI binary path/name for selected backend")
	cmd.Flags().String("language", appCfg.WhisperLanguage, "Transcription language override (blank=auto)")
	cmd.Flags().Int("chunk-duration", 0, "Split transcription into N-second chunks and persist resume manifest (0 disables chunking)")
	cmd.Flags().Int("timeout", 0, "Stop transcription if it runs longer than N seconds (0 disables timeout)")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

func doctorCommand() *cobra.Command {
	var report bool
	var reportOut string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local toolchain readiness (ffmpeg/ffprobe/transcription backends)",
		Run: func(cmd *cobra.Command, args []string) {
			r := collectDoctorReport(appCfg)
			if jsonOut {
				raw, err := json.MarshalIndent(r, "", "  ")
				if err != nil {
					failf("Failed to marshal doctor report: %v\n", err)
					return
				}
				fmt.Println(string(raw))
			} else {
				printDoctorReport(r)
			}

			if report {
				out := reportOut
				if strings.TrimSpace(out) == "" {
					out = defaultDoctorReportPath(appCfg.FramesRoot, time.Now().UTC())
				}
				if err := saveDoctorReport(out, r); err != nil {
					failf("Failed to write doctor report: %v\n", err)
				} else {
					if jsonOut {
						fmt.Fprintf(os.Stderr, "Doctor report: %s\n", out)
					} else {
						fmt.Printf("Doctor report: %s\n", out)
					}
				}
			}

			if r.RequiredFailed {
				markCommandFailure()
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output doctor data as JSON")
	cmd.Flags().BoolVar(&report, "report", false, "Write a machine-readable doctor report")
	cmd.Flags().StringVar(&reportOut, "report-out", "", "Doctor report output path (default: <frames_root>/doctor-report-<timestamp>.json)")
	return cmd
}

type toolCheck struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Found    bool   `json:"found"`
	Path     string `json:"path,omitempty"`
	Error    string `json:"error,omitempty"`
}

type gpuInfo struct {
	Available          bool   `json:"available"`
	Vendor             string `json:"vendor"` // "nvidia", "amd", "intel", "apple", "none"
	Model              string `json:"model,omitempty"`
	RecommendedHWAccel string `json:"recommended_hwaccel"` // "cuda", "vaapi", "qsv", "videotoolbox", "none"
}

type doctorReport struct {
	GeneratedAt              string                        `json:"generated_at"`
	GOOS                     string                        `json:"goos"`
	GOARCH                   string                        `json:"goarch"`
	ConfigPath               string                        `json:"config_path"`
	Config                   appconfig.Config              `json:"config"`
	Environment              map[string]string             `json:"environment"`
	Tools                    []toolCheck                   `json:"tools"`
	GPU                      gpuInfo                       `json:"gpu"`
	GPUAvailable             bool                          `json:"gpu_available"` // Deprecated: use GPU.Available
	TranscribeBackend        string                        `json:"transcribe_backend"`
	WhisperModel             string                        `json:"whisper_model"`
	TranscribeUsesGPU        bool                          `json:"transcribe_uses_gpu"`
	TranscribeAccelReason    string                        `json:"transcribe_accel_reason,omitempty"`
	EstimatedTranscribeSpeed string                        `json:"estimated_transcribe_speed"`
	TranscribeRuntimeClass   string                        `json:"transcribe_runtime_class"`
	TranscribeCostHint       string                        `json:"transcribe_cost_hint"`
	FFmpegVersion            string                        `json:"ffmpeg_version,omitempty"`
	FFmpegVersionError       string                        `json:"ffmpeg_version_error,omitempty"`
	FFmpegHWAccels           []string                      `json:"ffmpeg_hwaccels,omitempty"`
	FFmpegHWAccelsError      string                        `json:"ffmpeg_hwaccels_error,omitempty"`
	ConfigWarnings           []appconfig.ValidationWarning `json:"config_warnings,omitempty"`
	RequiredFailed           bool                          `json:"required_failed"`
}

func detectGPU() gpuInfo {
	// Check NVIDIA GPU
	if gpu := detectNVIDIA(); gpu.Available {
		return gpu
	}

	// Check AMD GPU
	if gpu := detectAMD(); gpu.Available {
		return gpu
	}

	// Check Intel QuickSync
	if gpu := detectIntel(); gpu.Available {
		return gpu
	}

	// Check Apple Silicon
	if gpu := detectApple(); gpu.Available {
		return gpu
	}

	// No GPU detected
	return gpuInfo{
		Available:          false,
		Vendor:             "none",
		Model:              "",
		RecommendedHWAccel: "none",
	}
}

func detectNVIDIA() gpuInfo {
	// Check nvidia-smi
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		cmd := exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
		output, err := cmd.Output()
		if err == nil {
			model := strings.TrimSpace(string(output))
			if model != "" {
				// If multiple GPUs, take the first one
				if idx := strings.Index(model, "\n"); idx > 0 {
					model = model[:idx]
				}
				return gpuInfo{
					Available:          true,
					Vendor:             "nvidia",
					Model:              model,
					RecommendedHWAccel: "cuda",
				}
			}
		}
	}

	// Fallback: check for /dev/nvidia* devices
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		return gpuInfo{
			Available:          true,
			Vendor:             "nvidia",
			Model:              "NVIDIA GPU",
			RecommendedHWAccel: "cuda",
		}
	}

	return gpuInfo{Available: false}
}

func detectAMD() gpuInfo {
	// Check for AMD GPU via rocm-smi (ROCm)
	if _, err := exec.LookPath("rocm-smi"); err == nil {
		cmd := exec.Command("rocm-smi", "--showproductname")
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, "GPU") && strings.Contains(line, ":") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						model := strings.TrimSpace(parts[1])
						if model != "" {
							return gpuInfo{
								Available:          true,
								Vendor:             "amd",
								Model:              model,
								RecommendedHWAccel: "vaapi",
							}
						}
					}
				}
			}
		}
	}

	// Fallback: check for AMD GPU via /dev/dri/renderD*
	files, err := filepath.Glob("/dev/dri/renderD*")
	if err == nil && len(files) > 0 {
		// Check if it's AMD by looking at driver
		if _, err := os.Stat("/sys/module/amdgpu"); err == nil {
			return gpuInfo{
				Available:          true,
				Vendor:             "amd",
				Model:              "AMD GPU",
				RecommendedHWAccel: "vaapi",
			}
		}
	}

	return gpuInfo{Available: false}
}

func detectIntel() gpuInfo {
	// Check for Intel GPU via vainfo
	if _, err := exec.LookPath("vainfo"); err == nil {
		cmd := exec.Command("vainfo")
		output, err := cmd.Output()
		if err == nil {
			outStr := string(output)
			if strings.Contains(outStr, "Intel") {
				// Try to extract model from vainfo output
				model := "Intel GPU"
				for _, line := range strings.Split(outStr, "\n") {
					if strings.Contains(line, "Driver version:") {
						model = strings.TrimSpace(strings.TrimPrefix(line, "Driver version:"))
						break
					}
				}
				return gpuInfo{
					Available:          true,
					Vendor:             "intel",
					Model:              model,
					RecommendedHWAccel: "qsv",
				}
			}
		}
	}

	// Fallback: check for Intel GPU driver
	if _, err := os.Stat("/sys/module/i915"); err == nil {
		return gpuInfo{
			Available:          true,
			Vendor:             "intel",
			Model:              "Intel GPU",
			RecommendedHWAccel: "qsv",
		}
	}

	return gpuInfo{Available: false}
}

func detectApple() gpuInfo {
	// Apple Silicon always has integrated GPU
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		// Try to detect M-series chip
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		output, err := cmd.Output()
		model := "Apple Silicon"
		if err == nil {
			brand := strings.TrimSpace(string(output))
			if brand != "" {
				model = brand
			}
		}
		return gpuInfo{
			Available:          true,
			Vendor:             "apple",
			Model:              model,
			RecommendedHWAccel: "videotoolbox",
		}
	}

	return gpuInfo{Available: false}
}

// Deprecated: use detectGPU() instead
func doctorHasGPU() bool {
	return detectGPU().Available
}

// transcribeAccel reports whether the active transcription backend is
// expected to use GPU acceleration. This is intentionally separate from
// ffmpeg/extraction GPU usage: a machine can have a CUDA GPU that ffmpeg
// uses for frame extraction while the transcription backend (e.g.
// openai-whisper) still runs on CPU.
type transcribeAccel struct {
	UsesGPU bool
	// Reason is a user-facing explanation suitable for doctor output and
	// headers — e.g. "no GPU detected" or
	// "openai-whisper typically runs on CPU even with GPU hardware".
	Reason string
}

// probeTranscribeAccel classifies expected transcription acceleration.
// Rule-based for now: openai-whisper almost always ends up on CPU in
// typical pip installs even on GPU hardware, so we conservatively report
// CPU for that backend; faster-whisper on a GPU system is reported GPU
// because its CUDA wheel install is the common case. A python-level
// probe (ctranslate2.get_cuda_device_count, torch.cuda.is_available) can
// replace this in a follow-up if the rule misclassifies real setups.
//
// backendOverride != "" is treated as an explicit choice (e.g. --transcribe-backend);
// otherwise cfg.TranscribeBackend is used and "auto"/"" falls back to
// PATH-based resolution so messaging matches what will actually run.
func probeTranscribeAccel(cfg appconfig.Config, backendOverride string, hwGPUPresent bool) transcribeAccel {
	if strings.TrimSpace(backendOverride) != "" {
		cfg.TranscribeBackend = backendOverride
	}
	resolved := resolvePreviewTranscribeBackend(cfg)
	if !hwGPUPresent {
		return transcribeAccel{UsesGPU: false, Reason: "no GPU detected on this system"}
	}
	switch resolved {
	case "faster-whisper":
		return transcribeAccel{UsesGPU: true, Reason: "faster-whisper with GPU hardware available"}
	case "whisper":
		return transcribeAccel{
			UsesGPU: false,
			Reason:  "openai-whisper typically runs on CPU even with GPU hardware — install faster-whisper for GPU acceleration",
		}
	case "unavailable":
		return transcribeAccel{UsesGPU: false, Reason: "no transcription backend detected on PATH"}
	default:
		return transcribeAccel{UsesGPU: false, Reason: "transcription backend could not be resolved"}
	}
}

func collectDoctorReport(cfg appconfig.Config) doctorReport {
	whisperModel := strings.TrimSpace(os.Getenv("WHISPER_MODEL"))
	if whisperModel == "" {
		whisperModel = strings.TrimSpace(cfg.WhisperModel)
	}
	if whisperModel == "" {
		whisperModel = "base"
	}
	gpu := detectGPU()
	r := doctorReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Config:       cfg,
		GPU:          gpu,
		GPUAvailable: gpu.Available, // Deprecated field for backward compatibility
		WhisperModel: whisperModel,
		Environment: map[string]string{
			"OBS_VIDEO_DIR":      valueOrDash(os.Getenv("OBS_VIDEO_DIR")),
			"WHISPER_BIN":        valueOrDash(os.Getenv("WHISPER_BIN")),
			"FASTER_WHISPER_BIN": valueOrDash(os.Getenv("FASTER_WHISPER_BIN")),
			"WHISPER_MODEL":      valueOrDash(os.Getenv("WHISPER_MODEL")),
			"WHISPER_LANGUAGE":   valueOrDash(os.Getenv("WHISPER_LANGUAGE")),
			"TRANSCRIBE_BACKEND": valueOrDash(os.Getenv("TRANSCRIBE_BACKEND")),
		},
	}
	accel := probeTranscribeAccel(cfg, "", r.GPUAvailable)
	r.TranscribeUsesGPU = accel.UsesGPU
	r.TranscribeAccelReason = accel.Reason
	transcriptPlan := buildTranscriptPlan(0, cfg, accel.UsesGPU)
	r.TranscribeBackend = transcriptPlan.Backend
	r.EstimatedTranscribeSpeed = transcriptPlan.RuntimeClass
	r.TranscribeRuntimeClass = transcriptPlan.RuntimeClass
	r.TranscribeCostHint = transcriptPlan.CostHint
	if cfgPath, err := appconfig.Path(); err == nil {
		r.ConfigPath = cfgPath
	}

	checks := []struct {
		name     string
		required bool
	}{
		{name: "ffmpeg", required: true},
		{name: "ffprobe", required: true},
		{name: "yt-dlp", required: false},
		{name: strings.TrimSpace(cfg.WhisperBin), required: false},
		{name: strings.TrimSpace(cfg.FasterWhisperBin), required: false},
	}
	for _, c := range checks {
		if strings.TrimSpace(c.name) == "" {
			continue
		}
		row := toolCheck{Name: c.name, Required: c.required}
		path, err := exec.LookPath(c.name)
		if err == nil {
			row.Found = true
			row.Path = path
		} else {
			row.Found = false
			row.Error = err.Error()
			if c.required {
				r.RequiredFailed = true
			}
		}
		r.Tools = append(r.Tools, row)
	}

	if version, err := media.FFmpegVersion(); err == nil {
		r.FFmpegVersion = version
	} else {
		r.FFmpegVersionError = err.Error()
	}
	if hw, err := media.FFmpegHWAccels(); err == nil {
		r.FFmpegHWAccels = hw
	} else {
		r.FFmpegHWAccelsError = err.Error()
	}
	r.ConfigWarnings = appconfig.Validate(cfg)
	return r
}

func printDoctorReport(r doctorReport) {
	missingRequired := make([]string, 0, 2)
	for _, c := range r.Tools {
		if c.Found {
			fmt.Printf("[ok]   %s\n", c.Name)
			continue
		}
		if c.Required {
			fmt.Printf("[fail] %s (required)\n", c.Name)
			missingRequired = append(missingRequired, c.Name)
		} else {
			fmt.Printf("[warn] %s (optional: needed for transcribe)\n", c.Name)
		}
	}
	fmt.Println("")
	fmt.Println("Environment")
	fmt.Printf("GOOS/GOARCH:       %s/%s\n", r.GOOS, r.GOARCH)
	fmt.Printf("CONFIG_PATH:       %s\n", valueOrDash(r.ConfigPath))
	fmt.Printf("OBS_VIDEO_DIR:     %s\n", valueOrDash(r.Environment["OBS_VIDEO_DIR"]))
	fmt.Printf("WHISPER_BIN:       %s\n", valueOrDash(r.Environment["WHISPER_BIN"]))
	fmt.Printf("FASTER_WHISPER_BIN:%s\n", valueOrDash(r.Environment["FASTER_WHISPER_BIN"]))
	fmt.Printf("WHISPER_MODEL:     %s\n", valueOrDash(r.Environment["WHISPER_MODEL"]))
	fmt.Printf("WHISPER_LANGUAGE:  %s\n", valueOrDash(r.Environment["WHISPER_LANGUAGE"]))
	fmt.Printf("TRANSCRIBE_BACKEND:%s\n", valueOrDash(r.Environment["TRANSCRIBE_BACKEND"]))

	fmt.Println("")
	fmt.Println("Hardware")
	if r.GPU.Available {
		fmt.Printf("GPU:               %s (%s)\n", r.GPU.Model, r.GPU.Vendor)
		fmt.Printf("Recommended:       hwaccel=%s\n", r.GPU.RecommendedHWAccel)
		if !ffmpegSupportsHWAccel(r.FFmpegHWAccels, r.GPU.RecommendedHWAccel) && r.GPU.RecommendedHWAccel != "none" {
			fmt.Printf("⚠ Note:            your ffmpeg lacks %s support — extractions will fall back to CPU.\n", r.GPU.RecommendedHWAccel)
			fmt.Printf("                   Install a full-featured ffmpeg build (e.g. brew install ffmpeg or apt install ffmpeg) to use GPU.\n")
		}
	} else {
		fmt.Printf("GPU:               none detected\n")
		fmt.Printf("Mode:              CPU-only\n")
	}

	fmt.Println("")
	fmt.Println("Transcription")
	fmt.Printf("Backend:           %s\n", valueOrDash(r.TranscribeBackend))
	fmt.Printf("Model:             %s\n", valueOrDash(r.WhisperModel))
	accelLabel := "CPU"
	if r.TranscribeUsesGPU {
		accelLabel = "GPU"
	}
	fmt.Printf("Accel:             %s\n", accelLabel)
	if reason := strings.TrimSpace(r.TranscribeAccelReason); reason != "" {
		fmt.Printf("Accel reason:      %s\n", reason)
	}
	fmt.Printf("Estimated Speed:   %s\n", valueOrDash(r.EstimatedTranscribeSpeed))
	fmt.Printf("Cost Hint:         %s\n", valueOrDash(r.TranscribeCostHint))

	fmt.Println("")
	fmt.Println("Storage")
	framesRoot := strings.TrimSpace(r.Config.FramesRoot)
	if framesRoot == "" {
		framesRoot = media.DefaultFramesRoot
	}
	fmt.Printf("Output root:       %s\n", framesRoot)
	if size, runs, err := framesRootUsage(framesRoot); err == nil && runs > 0 {
		fmt.Printf("Usage:             %s across %d runs\n", humanSize(size), runs)
		if size >= framesRootStorageThreshold {
			fmt.Println("Hint:              'framescli clean --older-than 30d' to prune old runs")
		}
	} else {
		fmt.Println("Usage:             (empty or not yet created)")
	}

	fmt.Println("")
	fmt.Println("FFmpeg")
	if strings.TrimSpace(r.FFmpegVersion) != "" {
		fmt.Printf("Version:           %s\n", r.FFmpegVersion)
	} else {
		fmt.Printf("Version:           unavailable (%s)\n", valueOrDash(r.FFmpegVersionError))
	}
	if len(r.FFmpegHWAccels) > 0 {
		fmt.Printf("HW Accelerators:   %s\n", strings.Join(r.FFmpegHWAccels, ", "))
	} else {
		fmt.Printf("HW Accelerators:   unavailable (%s)\n", valueOrDash(r.FFmpegHWAccelsError))
	}
	if len(r.ConfigWarnings) > 0 {
		fmt.Println("")
		fmt.Println("Config warnings")
		for _, w := range r.ConfigWarnings {
			fmt.Printf("- %s: %s\n", w.Field, w.Message)
		}
	}
	if len(missingRequired) > 0 {
		fmt.Println("")
		fmt.Println("Recovery")
		fmt.Printf("- Missing required tools: %s\n", strings.Join(missingRequired, ", "))
		fmt.Println("- macOS: brew install ffmpeg")
		fmt.Println("- Ubuntu/Debian/WSL: sudo apt install ffmpeg")
		fmt.Println("- Fedora: sudo dnf install ffmpeg")
		fmt.Println("- Verify PATH then re-run: framescli doctor")
	}

	// Show recommendations based on detected hardware and config
	recommendations := []string{}

	// GPU recommendations — only suggest enabling hwaccel if the local
	// ffmpeg actually supports the recommended accel. Otherwise the
	// suggestion contradicts the warning printed under Hardware.
	ffmpegCanGPU := ffmpegSupportsHWAccel(r.FFmpegHWAccels, r.GPU.RecommendedHWAccel)
	if r.GPU.Available && ffmpegCanGPU {
		currentHWAccel := strings.ToLower(strings.TrimSpace(r.Config.HWAccel))
		if currentHWAccel == "none" || currentHWAccel == "" {
			recommendations = append(recommendations,
				fmt.Sprintf("Enable GPU acceleration: framescli setup --non-interactive --hwaccel %s", r.GPU.RecommendedHWAccel))
			recommendations = append(recommendations,
				fmt.Sprintf("Or add --hwaccel %s to extract commands for 10-30x faster extraction", r.GPU.RecommendedHWAccel))
		} else if currentHWAccel == "auto" {
			recommendations = append(recommendations,
				fmt.Sprintf("Consider setting specific hwaccel mode for best performance: framescli setup --non-interactive --hwaccel %s", r.GPU.RecommendedHWAccel))
		}
	}
	if r.GPU.Available {

		// Preset recommendations for GPU users
		currentPreset := strings.ToLower(strings.TrimSpace(r.Config.PerformanceMode))
		if currentPreset == "laptop-safe" || currentPreset == "safe" {
			recommendations = append(recommendations,
				"Your GPU can handle higher quality: consider --preset balanced or --preset high-fidelity")
		}
	} else {
		// CPU-only recommendations
		currentPreset := strings.ToLower(strings.TrimSpace(r.Config.PerformanceMode))
		if currentPreset != "laptop-safe" && currentPreset != "safe" {
			recommendations = append(recommendations,
				"No GPU detected: consider --preset laptop-safe for better CPU performance")
		}
	}

	// Transcription recommendations
	if r.GPU.Available && r.TranscribeBackend != "faster-whisper" {
		// Check if faster-whisper is available
		if _, err := exec.LookPath("faster-whisper"); err == nil {
			recommendations = append(recommendations,
				"Faster transcription available: framescli setup --non-interactive --transcribe-backend faster-whisper")
		}
	}

	// CPU + whisper without faster-whisper: high-visibility hint, since
	// faster-whisper is the single biggest CPU transcription speedup users
	// can get.
	if !r.GPU.Available {
		_, whisperErr := exec.LookPath("whisper")
		_, fasterErr := exec.LookPath("faster-whisper")
		if whisperErr == nil && fasterErr != nil {
			fmt.Println("")
			fmt.Println("⚠ Performance hint")
			fmt.Println("  You're running on CPU without faster-whisper.")
			fmt.Println("  Install it for 3-5x faster transcription:")
			fmt.Println("    pip install faster-whisper")
		}
	}

	// Whisper model recommendations — gate on actual transcription acceleration,
	// not just hardware GPU presence. A system can have a GPU for ffmpeg while
	// the transcription backend (e.g. openai-whisper) still runs on CPU.
	if r.WhisperModel == "base" {
		if r.TranscribeUsesGPU {
			recommendations = append(recommendations,
				"Upgrade to better model for GPU: WHISPER_MODEL=medium (better accuracy, still fast)")
		} else {
			recommendations = append(recommendations,
				"Upgrade model carefully on CPU: WHISPER_MODEL=small (better accuracy, 2-3x slower)")
		}
	}

	if len(recommendations) > 0 {
		fmt.Println("")
		fmt.Println("Recommendations")
		for _, rec := range recommendations {
			fmt.Printf("→ %s\n", rec)
		}
	}
}

func defaultDoctorReportPath(framesRoot string, now time.Time) string {
	root := strings.TrimSpace(framesRoot)
	if root == "" {
		root = media.DefaultFramesRoot
	}
	filename := fmt.Sprintf("doctor-report-%s.json", now.UTC().Format("20060102-150405"))
	return filepath.Join(root, filename)
}

func saveDoctorReport(path string, report doctorReport) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("report path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func valueOrDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func boolWord(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func indexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index [rootDir]",
		Short: "Build an index.json for fast run browsing",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			root := appCfg.FramesRoot
			if strings.TrimSpace(root) == "" {
				root = media.DefaultFramesRoot
			}
			if len(args) == 1 {
				root = args[0]
			}
			out, _ := cmd.Flags().GetString("out")
			index, err := media.WriteRunsIndex(root, out)
			if err != nil {
				failf("Index build failed: %v\n", err)
				return
			}
			if out == "" {
				out = filepath.Join(root, "index.json")
			}
			fmt.Println("Index built")
			fmt.Println("-----------")
			fmt.Printf("Root: %s\n", index.Root)
			fmt.Printf("Runs: %d\n", index.RunCount)
			fmt.Printf("File: %s\n", out)
		},
	}
	cmd.Flags().String("out", "", "Output file (default: <rootDir>/index.json)")
	return cmd
}

func benchmarkCommand() *cobra.Command {
	var duration int
	var jsonOut bool
	var verbose bool
	var cpuOnly bool
	var noAuto bool
	var apply bool
	var dryRun bool
	var noHistory bool
	var historyPath string
	var top int

	cmd := &cobra.Command{
		Use:   "benchmark <videoPath|recent>",
		Short: "Benchmark extraction performance and recommend hwaccel settings",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			videoPath, note, err := resolveVideoInput(args[0])
			if err != nil {
				failf("Failed to resolve video input: %v\n", err)
				return
			}
			if note != "" && !jsonOut {
				fmt.Printf("Source: %s\n", note)
			}

			cases := buildBenchmarkCases(cpuOnly, noAuto)
			report, err := media.BenchmarkExtraction(media.BenchmarkOptions{
				VideoPath:   videoPath,
				DurationSec: duration,
				Cases:       cases,
				Verbose:     verbose,
			})
			if err != nil {
				failf("Benchmark failed: %v\n", err)
				return
			}
			if !noHistory {
				target := historyPath
				if strings.TrimSpace(target) == "" {
					target = media.BenchmarkHistoryPath(appCfg.FramesRoot)
				}
				if err := media.AppendBenchmarkHistory(target, *report, 100); err != nil {
					if !jsonOut {
						fmt.Fprintf(os.Stderr, "History write warning: %v\n", err)
					}
				} else if !jsonOut {
					fmt.Printf("History: %s\n", target)
				}
			}
			if dryRun && !apply && !jsonOut {
				fmt.Fprintln(os.Stderr, "Note: --dry-run has no effect without --apply")
			}
			if apply {
				if strings.TrimSpace(report.Recommended.HWAccel) == "" {
					failln("Cannot apply recommendation: no successful benchmark result.")
				} else if dryRun {
					fmt.Println("")
					fmt.Println(benchmarkRecommendationPreview(appCfg, report.Recommended))
				} else if err := applyBenchmarkRecommendation(report.Recommended); err != nil {
					failf("Apply failed: %v\n", err)
				} else if !jsonOut {
					fmt.Printf("Applied config: hwaccel=%s performance-mode=%s\n", report.Recommended.HWAccel, report.Recommended.Preset)
				}
			}

			if jsonOut {
				raw, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(raw))
				return
			}

			fmt.Println("")
			fmt.Println("Benchmark Results")
			fmt.Println("-----------------")
			fmt.Printf("Video:    %s\n", report.VideoPath)
			fmt.Printf("Duration: %ds\n", report.DurationSec)
			for _, res := range report.Results {
				status := "ok"
				if !res.Success {
					status = "fail"
				}
				fmt.Printf("- %-4s hwaccel=%-6s preset=%-8s elapsed=%6dms speed=%5.2fx\n", status, res.Case.HWAccel, res.Case.Preset, res.ElapsedMs, res.RealtimeFactor)
				if !res.Success {
					fmt.Printf("        error: %s\n", res.Error)
				}
			}
			fmt.Println("")
			fmt.Printf("Recommended: hwaccel=%s preset=%s\n", report.Recommended.HWAccel, report.Recommended.Preset)
			fmt.Printf("Reason: %s\n", report.RecommendationMessage)
			fmt.Printf("Apply with: framescli setup --non-interactive --hwaccel %s --performance-mode %s\n", report.Recommended.HWAccel, report.Recommended.Preset)
			if top > 0 && !noHistory {
				target := historyPath
				if strings.TrimSpace(target) == "" {
					target = media.BenchmarkHistoryPath(appCfg.FramesRoot)
				}
				if entries, err := media.ReadBenchmarkHistory(target, 0); err == nil {
					current := bestSuccessfulSpeed(report.Results)
					summary := compareAgainstHistory(entries, current, top)
					if summary != "" {
						fmt.Println("")
						fmt.Println(summary)
					}
				}
			}
		},
	}

	cmd.Flags().IntVar(&duration, "duration", 20, "Benchmark sample duration in seconds")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Show raw ffmpeg logs during benchmark")
	cmd.Flags().BoolVar(&cpuOnly, "cpu-only", false, "Only benchmark CPU extraction modes")
	cmd.Flags().BoolVar(&noAuto, "no-auto", false, "Skip hwaccel=auto case")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply recommended hwaccel/performance settings to config")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview config changes without writing (use with --apply)")
	cmd.Flags().BoolVar(&noHistory, "no-history", false, "Do not append benchmark run to history")
	cmd.Flags().StringVar(&historyPath, "history-path", "", "Custom benchmark history file path")
	cmd.Flags().IntVar(&top, "top", 5, "Compare current run against top N historical runs")
	cmd.AddCommand(benchmarkHistoryCommand())
	return cmd
}

func benchmarkHistoryCommand() *cobra.Command {
	var limit int
	var jsonOut bool
	var historyPath string
	var pruneKeep int
	var exportCSV string
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show benchmark history entries",
		Run: func(cmd *cobra.Command, args []string) {
			target := historyPath
			if strings.TrimSpace(target) == "" {
				target = media.BenchmarkHistoryPath(appCfg.FramesRoot)
			}
			if pruneKeep > 0 {
				removed, err := media.PruneBenchmarkHistory(target, pruneKeep)
				if err != nil {
					failf("Failed to prune benchmark history: %v\n", err)
					return
				}
				if !jsonOut {
					fmt.Printf("Pruned history: removed %d entries, kept %d\n", removed, pruneKeep)
				}
			}
			entries, err := media.ReadBenchmarkHistory(target, limit)
			if err != nil {
				failf("Failed to read benchmark history: %v\n", err)
				return
			}
			if strings.TrimSpace(exportCSV) != "" {
				if err := media.ExportBenchmarkHistoryCSV(exportCSV, entries); err != nil {
					failf("Failed to export CSV: %v\n", err)
					return
				}
				if !jsonOut {
					fmt.Printf("Exported CSV: %s\n", exportCSV)
				}
			}
			if jsonOut {
				raw, _ := json.MarshalIndent(entries, "", "  ")
				fmt.Println(string(raw))
				return
			}
			fmt.Printf("History file: %s\n", target)
			fmt.Printf("Entries: %d\n\n", len(entries))
			for i, e := range entries {
				best := bestSuccessfulSpeed(e.Results)
				fmt.Printf("%d) %s  hwaccel=%s preset=%s best=%.2fx  video=%s\n", i+1, valueOrDash(e.RecordedAt), e.Recommended.HWAccel, e.Recommended.Preset, best, e.VideoPath)
				if strings.TrimSpace(e.RecommendationMessage) != "" {
					fmt.Printf("   %s\n", e.RecommendationMessage)
				}
			}
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum history entries to return")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")
	cmd.Flags().StringVar(&historyPath, "history-path", "", "Custom benchmark history file path")
	cmd.Flags().IntVar(&pruneKeep, "prune", 0, "Keep only latest N entries before printing/export")
	cmd.Flags().StringVar(&exportCSV, "export-csv", "", "Write history entries to CSV file")
	return cmd
}

func telemetryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Inspect and manage local telemetry events",
	}
	cmd.AddCommand(telemetryStatusCommand())
	cmd.AddCommand(telemetryTailCommand())
	cmd.AddCommand(telemetryPruneCommand())
	return cmd
}

func telemetryStatusCommand() *cobra.Command {
	var path string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show telemetry status, path, and event count",
		Run: func(cmd *cobra.Command, args []string) {
			target := strings.TrimSpace(path)
			if target == "" {
				target = telemetryLogPath()
			}
			info, err := os.Stat(target)
			exists := err == nil
			size := int64(0)
			events := 0
			if exists {
				size = info.Size()
				events, _ = countLines(target)
			}
			if jsonOut {
				payload := map[string]any{
					"enabled": appCfg.TelemetryEnabled,
					"path":    target,
					"exists":  exists,
					"size":    size,
					"events":  events,
				}
				raw, _ := json.MarshalIndent(payload, "", "  ")
				fmt.Println(string(raw))
				return
			}
			fmt.Println("Telemetry Status")
			fmt.Println("----------------")
			fmt.Printf("Enabled: %t\n", appCfg.TelemetryEnabled)
			fmt.Printf("Path:    %s\n", target)
			fmt.Printf("Exists:  %t\n", exists)
			fmt.Printf("Events:  %d\n", events)
			fmt.Printf("Size:    %d bytes\n", size)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Telemetry JSONL path override")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output machine-readable JSON")
	return cmd
}

func telemetryTailCommand() *cobra.Command {
	var path string
	var lines int
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Print the last telemetry events",
		Run: func(cmd *cobra.Command, args []string) {
			target := strings.TrimSpace(path)
			if target == "" {
				target = telemetryLogPath()
			}
			if lines <= 0 {
				failln("lines must be > 0")
				return
			}
			out, err := readLastLines(target, lines)
			if err != nil {
				failf("telemetry tail failed: %v\n", err)
				return
			}
			for _, line := range out {
				fmt.Println(line)
			}
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Telemetry JSONL path override")
	cmd.Flags().IntVarP(&lines, "lines", "n", 20, "Number of lines to show")
	return cmd
}

func telemetryPruneCommand() *cobra.Command {
	var path string
	var keep int
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Keep only the most recent telemetry events",
		Run: func(cmd *cobra.Command, args []string) {
			target := strings.TrimSpace(path)
			if target == "" {
				target = telemetryLogPath()
			}
			if keep <= 0 {
				failln("keep must be > 0")
				return
			}
			kept, err := pruneToLastLines(target, keep)
			if err != nil {
				failf("telemetry prune failed: %v\n", err)
				return
			}
			fmt.Printf("Telemetry pruned: kept %d events at %s\n", kept, target)
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "Telemetry JSONL path override")
	cmd.Flags().IntVar(&keep, "keep", 2000, "Number of most recent events to keep")
	return cmd
}

func configCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show resolved FramesCLI config",
		Run: func(cmd *cobra.Command, args []string) {
			path, _ := appconfig.Path()
			fmt.Println("Config")
			fmt.Println("------")
			fmt.Printf("Path:            %s\n", path)
			fmt.Printf("Frames Root:     %s\n", appCfg.FramesRoot)
			fmt.Printf("OBS Video Dir:   %s\n", valueOrDash(appCfg.OBSVideoDir))
			fmt.Printf("Recent Dirs:     %s\n", strings.Join(appCfg.RecentVideoDirs, ", "))
			fmt.Printf("Recent Exts:     %s\n", strings.Join(appCfg.RecentExts, ", "))
			fmt.Printf("HWAccel:         %s\n", appCfg.HWAccel)
			fmt.Printf("Performance:     %s\n", appCfg.PerformanceMode)
			fmt.Printf("Default FPS:     %.2f\n", appCfg.DefaultFPS)
			fmt.Printf("Default Format:  %s\n", appCfg.DefaultFormat)
			fmt.Printf("Whisper Bin:     %s\n", appCfg.WhisperBin)
			fmt.Printf("FasterWhisper:   %s\n", appCfg.FasterWhisperBin)
			fmt.Printf("Whisper Model:   %s\n", appCfg.WhisperModel)
			fmt.Printf("Whisper Language:%s\n", valueOrDash(appCfg.WhisperLanguage))
			fmt.Printf("Transcribe Back: %s\n", appCfg.TranscribeBackend)
			warnings := appconfig.Validate(appCfg)
			if len(warnings) > 0 {
				fmt.Println("")
				fmt.Println("Warnings")
				fmt.Println("--------")
				for _, w := range warnings {
					fmt.Printf("- %s: %s\n", w.Field, w.Message)
				}
			}
		},
	}
}

func setupCommand() *cobra.Command {
	var yes bool
	var nonInteractive bool
	var framesRoot string
	var obsVideoDir string
	var recentVideoDirs string
	var recentExts string
	var defaultFPS float64
	var defaultFormat string
	var hwaccel string
	var performanceMode string
	var whisperBin string
	var fasterWhisperBin string
	var whisperModel string
	var whisperLanguage string
	var transcribeBackend string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Guided setup for defaults and toolchain config",
		Run: func(cmd *cobra.Command, args []string) {
			cfg := appCfg
			if nonInteractive {
				if cmd.Flags().Changed("frames-root") {
					cfg.FramesRoot = framesRoot
				}
				if cmd.Flags().Changed("obs-video-dir") {
					cfg.OBSVideoDir = obsVideoDir
				}
				if cmd.Flags().Changed("recent-video-dirs") {
					cfg.RecentVideoDirs = splitCSVPaths(recentVideoDirs)
				}
				if cmd.Flags().Changed("recent-extensions") {
					cfg.RecentExts = splitCSVExts(recentExts)
				}
				if cmd.Flags().Changed("default-fps") {
					cfg.DefaultFPS = defaultFPS
				}
				if cmd.Flags().Changed("default-format") {
					cfg.DefaultFormat = defaultFormat
				}
				if cmd.Flags().Changed("hwaccel") {
					cfg.HWAccel = hwaccel
				}
				if cmd.Flags().Changed("performance-mode") {
					cfg.PerformanceMode = performanceMode
				}
				if cmd.Flags().Changed("whisper-bin") {
					cfg.WhisperBin = whisperBin
				}
				if cmd.Flags().Changed("faster-whisper-bin") {
					cfg.FasterWhisperBin = fasterWhisperBin
				}
				if cmd.Flags().Changed("whisper-model") {
					cfg.WhisperModel = whisperModel
				}
				if cmd.Flags().Changed("whisper-language") {
					cfg.WhisperLanguage = whisperLanguage
				}
				if cmd.Flags().Changed("transcribe-backend") {
					cfg.TranscribeBackend = transcribeBackend
				}
				path, err := appconfig.Save(cfg)
				if err != nil {
					failf("Failed to save config: %v\n", err)
					return
				}
				appCfg = cfg
				appconfig.ApplyEnvDefaults(appCfg)
				warnings := appconfig.Validate(appCfg)
				if len(warnings) > 0 {
					fmt.Println("Config warnings:")
					for _, w := range warnings {
						fmt.Printf("- %s: %s\n", w.Field, w.Message)
					}
				}
				fmt.Printf("Saved config: %s\n", path)
				return
			}
			if yes {
				path, err := appconfig.Save(cfg)
				if err != nil {
					failf("Failed to save config: %v\n", err)
					return
				}
				appCfg = cfg
				appconfig.ApplyEnvDefaults(appCfg)
				warnings := appconfig.Validate(appCfg)
				if len(warnings) > 0 {
					fmt.Println("Config warnings:")
					for _, w := range warnings {
						fmt.Printf("- %s: %s\n", w.Field, w.Message)
					}
				}
				fmt.Printf("Saved config: %s\n", path)
				return
			}

			result, cancelled, err := runSetupWizard(cfg)
			if err != nil {
				failf("Setup wizard failed: %v\n", err)
				return
			}
			if cancelled {
				fmt.Println("Setup canceled. No config changes were written.")
				return
			}
			cfg = result.Config

			path, err := appconfig.Save(cfg)
			if err != nil {
				failf("Failed to save config: %v\n", err)
				return
			}
			appCfg = cfg
			appconfig.ApplyEnvDefaults(appCfg)
			warnings := appconfig.Validate(appCfg)
			if len(warnings) > 0 {
				fmt.Println("Config warnings:")
				for _, w := range warnings {
					fmt.Printf("- %s: %s\n", w.Field, w.Message)
				}
			}
			fmt.Printf("Saved config: %s\n", path)
			fmt.Println("Next steps:")
			fmt.Println("- Run `framescli doctor` to validate local dependencies.")
			fmt.Println("- Run `framescli extract <video>` to process video, or `framescli mcp` for agent integration.")
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Write current defaults without prompts")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Apply provided flags without prompts")
	cmd.Flags().StringVar(&framesRoot, "frames-root", appCfg.FramesRoot, "Default frames root directory")
	cmd.Flags().StringVar(&obsVideoDir, "obs-video-dir", appCfg.OBSVideoDir, "Default OBS recordings directory")
	cmd.Flags().StringVar(&recentVideoDirs, "recent-video-dirs", strings.Join(appCfg.RecentVideoDirs, ","), "CSV of directories used by extract recent")
	cmd.Flags().StringVar(&recentExts, "recent-extensions", strings.Join(appCfg.RecentExts, ","), "CSV of file extensions for extract recent")
	cmd.Flags().Float64Var(&defaultFPS, "default-fps", appCfg.DefaultFPS, "Default extraction FPS")
	cmd.Flags().StringVar(&defaultFormat, "default-format", appCfg.DefaultFormat, "Default frame format (png/jpg)")
	cmd.Flags().StringVar(&hwaccel, "hwaccel", appCfg.HWAccel, "Default hardware acceleration mode: none|auto|cuda|vaapi|qsv")
	cmd.Flags().StringVar(&performanceMode, "performance-mode", appCfg.PerformanceMode, "Default workflow preset: laptop-safe|balanced|high-fidelity (legacy: safe|fast)")
	cmd.Flags().StringVar(&whisperBin, "whisper-bin", appCfg.WhisperBin, "Whisper executable name/path")
	cmd.Flags().StringVar(&fasterWhisperBin, "faster-whisper-bin", appCfg.FasterWhisperBin, "Faster-Whisper executable name/path")
	cmd.Flags().StringVar(&whisperModel, "whisper-model", appCfg.WhisperModel, "Default Whisper model")
	cmd.Flags().StringVar(&whisperLanguage, "whisper-language", appCfg.WhisperLanguage, "Default Whisper language (blank=auto)")
	cmd.Flags().StringVar(&transcribeBackend, "transcribe-backend", appCfg.TranscribeBackend, "Default transcription backend: auto|whisper|faster-whisper")
	return cmd
}

func resolveVideoInput(input string) (string, string, error) {
	videoPath := input
	if strings.EqualFold(input, "recent") {
		recentDirs := appCfg.RecentVideoDirs
		if len(recentDirs) == 0 {
			if strings.TrimSpace(appCfg.OBSVideoDir) != "" {
				recentDirs = []string{appCfg.OBSVideoDir}
			} else {
				recentDirs = []string{media.DefaultOBSVideoDir()}
			}
		}
		recentPath, sourceDir, err := media.FindMostRecentVideoInDirs(recentDirs, appCfg.RecentExts)
		if err != nil {
			return "", "", err
		}
		return recentPath, "recent from " + sourceDir, nil
	}
	normalized := media.NormalizeVideoPath(videoPath)
	if normalized != videoPath {
		return normalized, "normalized from Windows path", nil
	}
	return videoPath, "", nil
}

func expandVideoInputs(inputs []string) []string {
	out := make([]string, 0, len(inputs))
	seen := map[string]struct{}{}
	for _, in := range inputs {
		trimmed := strings.TrimSpace(in)
		if trimmed == "" {
			continue
		}
		add := func(path string) {
			if strings.TrimSpace(path) == "" {
				return
			}
			if _, ok := seen[path]; ok {
				return
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
		if strings.ContainsAny(trimmed, "*?[]") {
			matches, err := filepath.Glob(trimmed)
			if err != nil || len(matches) == 0 {
				// Keep unmatched/invalid globs as explicit inputs so batch runs
				// report a concrete per-item error instead of silently skipping.
				add(trimmed)
				continue
			}
			sort.Strings(matches)
			for _, m := range matches {
				add(m)
			}
			continue
		}
		add(trimmed)
	}
	return out
}

func buildBenchmarkCases(cpuOnly bool, noAuto bool) []media.BenchmarkCase {
	cases := []media.BenchmarkCase{
		{HWAccel: "none", Preset: "balanced"},
		{HWAccel: "none", Preset: "fast"},
	}
	if cpuOnly {
		return dedupeBenchmarkCases(cases)
	}
	if !noAuto {
		cases = append(cases, media.BenchmarkCase{HWAccel: "auto", Preset: "balanced"})
	}
	if hw, err := media.FFmpegHWAccels(); err == nil {
		for _, accel := range hw {
			switch accel {
			case "cuda", "vaapi", "qsv":
				cases = append(cases, media.BenchmarkCase{HWAccel: accel, Preset: "balanced"})
			}
		}
	}
	return dedupeBenchmarkCases(cases)
}

func dedupeBenchmarkCases(in []media.BenchmarkCase) []media.BenchmarkCase {
	out := make([]media.BenchmarkCase, 0, len(in))
	seen := map[string]struct{}{}
	for _, c := range in {
		key := strings.ToLower(strings.TrimSpace(c.HWAccel)) + "|" + strings.ToLower(strings.TrimSpace(c.Preset))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}

func bestSuccessfulSpeed(results []media.BenchmarkResult) float64 {
	best := 0.0
	for _, r := range results {
		if !r.Success {
			continue
		}
		if r.RealtimeFactor > best {
			best = r.RealtimeFactor
		}
	}
	return best
}

func compareAgainstHistory(entries []media.BenchmarkHistoryEntry, current float64, topN int) string {
	if current <= 0 || topN <= 0 {
		return ""
	}
	speeds := make([]float64, 0, len(entries))
	for _, e := range entries {
		best := bestSuccessfulSpeed(e.Results)
		if best > 0 {
			speeds = append(speeds, best)
		}
	}
	if len(speeds) < 2 {
		return ""
	}
	if len(speeds) > 1 {
		speeds = speeds[1:]
	}
	if len(speeds) == 0 {
		return ""
	}
	sort.Slice(speeds, func(i, j int) bool { return speeds[i] > speeds[j] })
	if len(speeds) > topN {
		speeds = speeds[:topN]
	}
	sum := 0.0
	for _, v := range speeds {
		sum += v
	}
	baseline := sum / float64(len(speeds))
	if baseline <= 0 {
		return ""
	}
	deltaPct := ((current - baseline) / baseline) * 100
	status := "within baseline"
	if deltaPct < -15 {
		status = "regression detected"
	} else if deltaPct > 15 {
		status = "improvement detected"
	}
	return fmt.Sprintf("Baseline(top %d): %.2fx | Current: %.2fx | Delta: %.1f%% (%s)", len(speeds), baseline, current, deltaPct, status)
}

func applyBenchmarkRecommendation(rec media.BenchmarkCase) error {
	if strings.TrimSpace(rec.HWAccel) == "" {
		return fmt.Errorf("missing hwaccel recommendation")
	}
	cfg := appCfg
	cfg.HWAccel = rec.HWAccel
	cfg.PerformanceMode = rec.Preset
	path, err := appconfig.Save(cfg)
	if err != nil {
		return err
	}
	appCfg = cfg
	appconfig.ApplyEnvDefaults(appCfg)
	fmt.Printf("Saved config: %s\n", path)
	return nil
}

func benchmarkRecommendationPreview(current appconfig.Config, rec media.BenchmarkCase) string {
	lines := []string{"Config dry-run", "--------------"}
	lines = append(lines, fmt.Sprintf("HWAccel:      %s -> %s", valueOrDash(current.HWAccel), valueOrDash(rec.HWAccel)))
	lines = append(lines, fmt.Sprintf("Performance:  %s -> %s", valueOrDash(current.PerformanceMode), valueOrDash(rec.Preset)))
	if strings.TrimSpace(current.HWAccel) == strings.TrimSpace(rec.HWAccel) &&
		strings.TrimSpace(current.PerformanceMode) == strings.TrimSpace(rec.Preset) {
		lines = append(lines, "No changes would be written.")
	} else {
		lines = append(lines, "No changes written. Re-run without --dry-run to persist.")
	}
	return strings.Join(lines, "\n")
}

func promptWithDefault(reader *bufio.Reader, label, def string) string {
	fmt.Printf("%s [%s]: ", label, def)
	line, err := reader.ReadString('\n')
	if err != nil {
		return def
	}
	v := strings.TrimSpace(line)
	if v == "" {
		return def
	}
	return v
}

func printSetupSection(title string, body string) {
	fmt.Println("")
	fmt.Println(title)
	fmt.Println(strings.Repeat("-", len(title)))
	if strings.TrimSpace(body) != "" {
		fmt.Println(body)
	}
}

func promptChoice(reader *bufio.Reader, label, current string, choices []string) string {
	if len(choices) == 0 {
		return promptWithDefault(reader, label, current)
	}
	if strings.TrimSpace(current) == "" {
		current = choices[0]
	}
	fmt.Printf("%s\n", label)
	for i, choice := range choices {
		marker := " "
		if choice == current {
			marker = "*"
		}
		fmt.Printf("  %d) %s %s\n", i+1, choice, marker)
	}
	fmt.Printf("Choice [%s]: ", current)
	line, err := reader.ReadString('\n')
	if err != nil {
		return current
	}
	choice := strings.TrimSpace(strings.ToLower(line))
	if choice == "" {
		return current
	}
	if n, parseErr := strconv.Atoi(choice); parseErr == nil {
		if n >= 1 && n <= len(choices) {
			return choices[n-1]
		}
		return current
	}
	for _, item := range choices {
		if choice == item {
			return item
		}
	}
	return current
}

func promptYesNo(reader *bufio.Reader, label string, defaultYes bool) bool {
	suffix := "[y/N]"
	def := "n"
	if defaultYes {
		suffix = "[Y/n]"
		def = "y"
	}
	fmt.Printf("%s %s: ", label, suffix)
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultYes
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	if answer == "" {
		answer = def
	}
	return answer == "y" || answer == "yes"
}

func promptWhisperModel(reader *bufio.Reader, current string) string {
	models := appconfig.SupportedWhisperModels()
	if strings.TrimSpace(current) == "" {
		current = "base"
	}
	fmt.Println("Choose Whisper model:")
	for i, model := range models {
		marker := " "
		if model == current {
			marker = "*"
		}
		fmt.Printf("  %d) %s %s\n", i+1, model, marker)
	}
	customIndex := len(models) + 1
	fmt.Printf("  %d) custom\n", customIndex)
	fmt.Printf("Model [%s]: ", current)
	line, err := reader.ReadString('\n')
	if err != nil {
		return current
	}
	choice := strings.TrimSpace(line)
	if choice == "" {
		return current
	}
	if n, parseErr := strconv.Atoi(choice); parseErr == nil {
		if n >= 1 && n <= len(models) {
			return models[n-1]
		}
		if n == customIndex {
			return promptWithDefault(reader, "Custom whisper model", current)
		}
		return current
	}
	for _, model := range models {
		if choice == model {
			return model
		}
	}
	return strings.ToLower(choice)
}

func splitCSVPaths(input string) []string {
	if strings.TrimSpace(input) == "" {
		return []string{}
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func splitCSVExts(input string) []string {
	if strings.TrimSpace(input) == "" {
		return []string{}
	}
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		v := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(p), "."))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
