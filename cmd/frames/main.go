package main

import (
	"bufio"
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
	"github.com/wraelen/framescli/internal/tui"
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
	fmt.Printf("\r%s [%s] %6.2f%%", label, bar, percent)
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
}

func newStageProgressTracker(started time.Time) stageProgressTracker {
	if started.IsZero() {
		started = time.Now()
	}
	return stageProgressTracker{started: started}
}

func (t stageProgressTracker) render(stage string, percent float64) {
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

	root := &cobra.Command{
		Use:          "framescli",
		Short:        "FramesCLI - extract frame timelines from coding recordings",
		SilenceUsage: true,
	}

	root.AddCommand(extractCommand())
	root.AddCommand(extractBatchCommand())
	root.AddCommand(previewCommand())
	root.AddCommand(openLastCommand())
	root.AddCommand(copyLastCommand())
	root.AddCommand(mcpCommand())
	root.AddCommand(importCommand())
	root.AddCommand(sheetCommand())
	root.AddCommand(cleanCommand())
	root.AddCommand(transcribeCommand())
	root.AddCommand(tuiCommand())
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
	OutDir             string
	Voice              bool
	FrameFormat        string
	JPGQuality         int
	NoSheet            bool
	SheetCols          int
	Verbose            bool
	HWAccel            string
	Preset             string
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
	PostHook           string
	PostHookTimeout    time.Duration
}

type extractWorkflowResult struct {
	VideoInput      string            `json:"video_input"`
	ResolvedVideo   string            `json:"resolved_video"`
	SourceNote      string            `json:"source_note,omitempty"`
	VideoInfo       media.VideoInfo   `json:"video_info"`
	FPS             float64           `json:"fps"`
	FrameFormat     string            `json:"frame_format"`
	OutDir          string            `json:"out_dir"`
	FrameCount      int               `json:"frame_count"`
	HWAccel         string            `json:"hwaccel"`
	Preset          string            `json:"preset"`
	MetadataPath    string            `json:"metadata_path,omitempty"`
	MetadataCSVPath string            `json:"metadata_csv_path,omitempty"`
	FramesZipPath   string            `json:"frames_zip_path,omitempty"`
	ContactSheet    string            `json:"contact_sheet,omitempty"`
	TranscriptTxt   string            `json:"transcript_txt,omitempty"`
	TranscriptJSON  string            `json:"transcript_json,omitempty"`
	AudioPath       string            `json:"audio_path,omitempty"`
	Warnings        []string          `json:"warnings,omitempty"`
	ElapsedMs       int64             `json:"elapsed_ms"`
	Artifacts       map[string]string `json:"artifacts,omitempty"`
}

type automationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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
		env.Error = &automationError{
			Code:    "command_failed",
			Message: err.Error(),
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

func runExtractWorkflowResult(ctx context.Context, videoInput string, fps float64, opts extractWorkflowOptions, interactive bool) (extractWorkflowResult, error) {
	started := time.Now()
	result := extractWorkflowResult{
		VideoInput: videoInput,
		FPS:        fps,
		Preset:     opts.Preset,
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if fps <= 0 {
		return result, fmt.Errorf("invalid fps value. use a positive number")
	}
	if opts.SheetCols <= 0 {
		return result, fmt.Errorf("invalid sheet-cols value. use a positive number")
	}

	if interactive {
		fmt.Println("[1/4] Resolving video input")
	}
	resolvedPath, note, err := resolveVideoInput(videoInput)
	if err != nil {
		return result, fmt.Errorf("failed to resolve video input: %w", err)
	}
	videoPath := resolvedPath
	result.ResolvedVideo = videoPath
	result.SourceNote = note
	if interactive && note != "" {
		fmt.Printf("  source: %s\n", note)
	}
	videoInfo, err := media.ProbeVideoInfo(videoPath)
	if err != nil {
		return result, fmt.Errorf("invalid video input %q: %w", videoPath, err)
	}
	result.VideoInfo = videoInfo
	if interactive {
		fmt.Printf("  video: %s\n", videoPath)
		fmt.Printf("  duration: %.2fs\n", videoInfo.DurationSec)
		fmt.Printf("  resolution: %dx%d\n", videoInfo.Width, videoInfo.Height)
		fmt.Printf("  source fps: %.2f\n", videoInfo.FrameRate)
		fmt.Printf("  fps: %.2f, format: %s\n", fps, opts.FrameFormat)
		fmt.Printf("  hwaccel: %s, preset: %s\n", opts.HWAccel, opts.Preset)
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

	if interactive {
		fmt.Println("[2/4] Extracting media")
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
		FPS:          fps,
		OutDir:       opts.OutDir,
		FrameFormat:  opts.FrameFormat,
		JPGQuality:   opts.JPGQuality,
		HWAccel:      opts.HWAccel,
		Preset:       opts.Preset,
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
			fmt.Println("[3/4] Skipping contact sheet (--no-sheet)")
		}
		if interactive && !opts.Verbose {
			progressTracker.render("contact sheet skipped", 85)
			fmt.Println("")
		}
	} else {
		if interactive {
			fmt.Println("[3/4] Building contact sheet")
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
	if opts.Voice {
		if interactive {
			fmt.Println("[4/4] Transcribing voice")
		}
		voiceDir := filepath.Join(extractResult.OutDir, "voice")
		if extractResult.AudioPath == "" {
			return result, fmt.Errorf("voice extraction did not produce audio output")
		}
		if interactive && opts.Verbose {
			transcriptTxt, transcriptJSON, err = media.TranscribeAudioWithOptions(media.TranscribeOptions{
				Context:   ctx,
				AudioPath: extractResult.AudioPath,
				OutDir:    voiceDir,
				Model:     opts.TranscribeModel,
				Backend:   opts.TranscribeBackend,
				Bin:       opts.TranscribeBin,
				Language:  opts.TranscribeLanguage,
				Verbose:   true,
			})
		} else if interactive {
			progressTracker.render("transcribing voice", 88)
			err = runSpinner("  transcribe", func() error {
				var spinnerErr error
				transcriptTxt, transcriptJSON, spinnerErr = media.TranscribeAudioWithOptions(media.TranscribeOptions{
					Context:   ctx,
					AudioPath: extractResult.AudioPath,
					OutDir:    voiceDir,
					Model:     opts.TranscribeModel,
					Backend:   opts.TranscribeBackend,
					Bin:       opts.TranscribeBin,
					Language:  opts.TranscribeLanguage,
					Verbose:   false,
				})
				return spinnerErr
			})
		} else {
			transcriptTxt, transcriptJSON, err = media.TranscribeAudioWithOptions(media.TranscribeOptions{
				Context:   ctx,
				AudioPath: extractResult.AudioPath,
				OutDir:    voiceDir,
				Model:     opts.TranscribeModel,
				Backend:   opts.TranscribeBackend,
				Bin:       opts.TranscribeBin,
				Language:  opts.TranscribeLanguage,
				Verbose:   false,
			})
		}
		if err != nil {
			if cerr := normalizeCancellationError(ctx, err); cerr != nil {
				return result, cerr
			}
			err = annotateStorageError(err)
			return result, fmt.Errorf("voice transcription failed: %w", err)
		}
		if interactive && !opts.Verbose {
			progressTracker.render("transcription complete", 98)
			fmt.Println("")
		}
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
		fmt.Printf("FPS:           %.2f\n", fps)
		fmt.Printf("Format:        %s\n", extractResult.FrameFormat)
		fmt.Printf("HWAccel:       %s\n", extractResult.UsedHWAccel)
		fmt.Printf("Preset:        %s\n", opts.Preset)
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
	}
	return result, nil
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

func extractCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract <videoPath|recent> [fps]",
		Short: "Extract video frames (png/jpg), optional voice transcription",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			fps, _ := cmd.Flags().GetFloat64("fps")
			if !cmd.Flags().Changed("fps") && appCfg.DefaultFPS > 0 {
				fps = appCfg.DefaultFPS
			}
			if len(args) >= 2 {
				parsedFPS, err := strconv.ParseFloat(args[1], 64)
				if err != nil || parsedFPS <= 0 {
					failln("Invalid fps value. Use a positive number.")
					return
				}
				fps = parsedFPS
			}
			outDir, _ := cmd.Flags().GetString("out")
			voice, _ := cmd.Flags().GetBool("voice")
			frameFormat, _ := cmd.Flags().GetString("format")
			if !cmd.Flags().Changed("format") && appCfg.DefaultFormat != "" {
				frameFormat = appCfg.DefaultFormat
			}
			jpgQuality, _ := cmd.Flags().GetInt("quality")
			noSheet, _ := cmd.Flags().GetBool("no-sheet")
			sheetCols, _ := cmd.Flags().GetInt("sheet-cols")
			verbose, _ := cmd.Flags().GetBool("verbose")
			hwaccel, _ := cmd.Flags().GetString("hwaccel")
			preset, _ := cmd.Flags().GetString("preset")
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
			postHook, _ := cmd.Flags().GetString("post-hook")
			postHookTimeout, _ := cmd.Flags().GetDuration("post-hook-timeout")
			jsonOut, _ := cmd.Flags().GetBool("json")
			if !cmd.Flags().Changed("hwaccel") {
				hwaccel = appCfg.HWAccel
			}
			if !cmd.Flags().Changed("preset") {
				preset = appCfg.PerformanceMode
			}
			opts := extractWorkflowOptions{
				OutDir:             outDir,
				Voice:              voice,
				FrameFormat:        frameFormat,
				JPGQuality:         jpgQuality,
				NoSheet:            noSheet,
				SheetCols:          sheetCols,
				Verbose:            verbose,
				HWAccel:            hwaccel,
				Preset:             preset,
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
				PostHook:           postHook,
				PostHookTimeout:    postHookTimeout,
			}
			if jsonOut {
				started := time.Now()
				res, err := runExtractWorkflowResult(cmd.Context(), args[0], fps, opts, false)
				if err != nil {
					if !isCancellationError(err) {
						err = withDiagnostics("extract", args[0], opts.OutDir, opts.LogFile, err)
					}
					_ = appendTelemetryEvent(telemetryEvent{
						Command:     "extract",
						Status:      "error",
						DurationMs:  time.Since(started).Milliseconds(),
						Input:       args[0],
						FPS:         fps,
						FrameFormat: opts.FrameFormat,
						HWAccel:     opts.HWAccel,
						Preset:      opts.Preset,
						Voice:       opts.Voice,
						Error:       err.Error(),
					})
					emitAutomationJSON("extract", started, "error", map[string]string{"input": args[0]}, err)
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
					Input:         args[0],
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
			res, err := runExtractWorkflowResult(cmd.Context(), args[0], fps, opts, true)
			if err != nil {
				if !isCancellationError(err) {
					err = withDiagnostics("extract", args[0], opts.OutDir, opts.LogFile, err)
				}
				_ = appendTelemetryEvent(telemetryEvent{
					Command:     "extract",
					Status:      "error",
					DurationMs:  time.Since(started).Milliseconds(),
					Input:       args[0],
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
					failf("%v\n", err)
				}
				return
			}
			_ = appendTelemetryEvent(telemetryEvent{
				Command:       "extract",
				Status:        "success",
				DurationMs:    res.ElapsedMs,
				Input:         args[0],
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

	cmd.Flags().Float64("fps", 4, "Frames per second")
	cmd.Flags().String("out", "", "Output directory")
	cmd.Flags().Bool("voice", false, "Extract audio and generate transcript in the output folder")
	cmd.Flags().String("format", "png", "Frame output format: png or jpg")
	cmd.Flags().Int("quality", 3, "JPG quality (1-31, lower is better quality)")
	cmd.Flags().Bool("no-sheet", false, "Skip contact sheet generation")
	cmd.Flags().Int("sheet-cols", 6, "Columns in generated contact sheet")
	cmd.Flags().Bool("verbose", false, "Show raw ffmpeg/whisper logs")
	cmd.Flags().String("hwaccel", appCfg.HWAccel, "Hardware acceleration mode: none|auto|cuda|vaapi|qsv")
	cmd.Flags().String("preset", appCfg.PerformanceMode, "Performance mode: safe|balanced|fast")
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
	cmd.Flags().String("post-hook", "", "Run command after successful extraction (overrides config post_extract_hook)")
	cmd.Flags().Duration("post-hook-timeout", 0, "Post-hook timeout (e.g. 30s, 2m)")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

func extractBatchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract-batch <videoPathOrGlob...>",
		Short: "Extract frames/audio from multiple videos (supports glob patterns)",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			fps, _ := cmd.Flags().GetFloat64("fps")
			if !cmd.Flags().Changed("fps") && appCfg.DefaultFPS > 0 {
				fps = appCfg.DefaultFPS
			}
			voice, _ := cmd.Flags().GetBool("voice")
			frameFormat, _ := cmd.Flags().GetString("format")
			if !cmd.Flags().Changed("format") && appCfg.DefaultFormat != "" {
				frameFormat = appCfg.DefaultFormat
			}
			jpgQuality, _ := cmd.Flags().GetInt("quality")
			noSheet, _ := cmd.Flags().GetBool("no-sheet")
			sheetCols, _ := cmd.Flags().GetInt("sheet-cols")
			verbose, _ := cmd.Flags().GetBool("verbose")
			hwaccel, _ := cmd.Flags().GetString("hwaccel")
			preset, _ := cmd.Flags().GetString("preset")
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
					JPGQuality:         jpgQuality,
					NoSheet:            noSheet,
					SheetCols:          sheetCols,
					Verbose:            verbose,
					HWAccel:            hwaccel,
					Preset:             preset,
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
	cmd.Flags().Float64("fps", appCfg.DefaultFPS, "Frames per second")
	cmd.Flags().String("out", "", "Output root directory (one subdir per input video)")
	cmd.Flags().Bool("voice", false, "Extract audio and generate transcript in the output folder")
	cmd.Flags().String("format", appCfg.DefaultFormat, "Frame output format: png or jpg")
	cmd.Flags().Int("quality", 3, "JPG quality (1-31, lower is better quality)")
	cmd.Flags().Bool("no-sheet", false, "Skip contact sheet generation")
	cmd.Flags().Int("sheet-cols", 6, "Columns in generated contact sheet")
	cmd.Flags().Bool("verbose", false, "Show raw ffmpeg/whisper logs")
	cmd.Flags().String("hwaccel", appCfg.HWAccel, "Hardware acceleration mode: none|auto|cuda|vaapi|qsv")
	cmd.Flags().String("preset", appCfg.PerformanceMode, "Performance mode: safe|balanced|fast")
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
	cmd.Flags().String("post-hook", "", "Run command after each successful extraction (overrides config post_extract_hook)")
	cmd.Flags().Duration("post-hook-timeout", 0, "Post-hook timeout per item (e.g. 30s, 2m)")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

func previewCommand() *cobra.Command {
	var fps float64
	var format string
	var mode string
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
				failf("failed to resolve video input: %v\n", err)
				return
			}
			info, err := media.ProbeVideoInfo(videoPath)
			if err != nil {
				if jsonOut {
					emitAutomationJSON("preview", started, "error", map[string]string{"input": videoPath}, err)
					markCommandFailure()
					return
				}
				failf("video probe failed: %v\n", err)
				return
			}
			if !cmd.Flags().Changed("fps") && appCfg.DefaultFPS > 0 {
				fps = appCfg.DefaultFPS
			}
			if !cmd.Flags().Changed("format") && appCfg.DefaultFormat != "" {
				format = appCfg.DefaultFormat
			}
			estimate := estimateOutputs(info, fps, format, mode)
			if jsonOut {
				payload := map[string]any{
					"input":       args[0],
					"resolved":    videoPath,
					"source_note": note,
					"video_info":  info,
					"target_fps":  fps,
					"format":      format,
					"mode":        mode,
					"estimate":    estimate,
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
			fmt.Printf("Target FPS:  %.2f\n", fps)
			fmt.Printf("Format:      %s\n", format)
			fmt.Printf("Frames est:  %d\n", estimate.FrameCount)
			fmt.Printf("Disk est:    ~%.1f MB\n", estimate.EstimatedMB)
			fmt.Println("Artifacts:")
			for _, a := range estimate.Artifacts {
				fmt.Printf("- %s\n", a)
			}
			if estimate.Warning != "" {
				fmt.Printf("Warning:     %s\n", estimate.Warning)
			}
		},
	}
	cmd.Flags().Float64Var(&fps, "fps", appCfg.DefaultFPS, "Target extraction FPS for estimate")
	cmd.Flags().StringVar(&format, "format", appCfg.DefaultFormat, "Frame format estimate: png|jpg")
	cmd.Flags().StringVar(&mode, "mode", "both", "Output mode estimate: frames|audio|both")
	cmd.Flags().Bool("json", false, "Output machine-readable JSON")
	return cmd
}

type outputEstimate struct {
	FrameCount  int      `json:"frame_count"`
	EstimatedMB float64  `json:"estimated_mb"`
	Artifacts   []string `json:"artifacts"`
	Warning     string   `json:"warning,omitempty"`
}

func estimateOutputs(info media.VideoInfo, fps float64, format string, mode string) outputEstimate {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "both"
	}
	if fps <= 0 {
		fps = 4
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "png"
	}
	est := outputEstimate{}
	switch mode {
	case "audio":
		est.Artifacts = []string{"voice/voice.wav (or selected audio format)", "voice/transcript.{txt,json,srt,vtt}"}
		est.EstimatedMB = (info.DurationSec / 60.0) * 9.5
	default:
		frames := int(math.Ceil(info.DurationSec * fps))
		perFrameKB := 180.0
		if format == "jpg" || format == "jpeg" {
			perFrameKB = 90.0
		}
		est.FrameCount = frames
		est.EstimatedMB = (float64(frames) * perFrameKB) / 1024.0
		est.Artifacts = []string{
			"images/frame-XXXX." + format,
			"images/sheets/contact-sheet.png",
			"run.json + frames.json",
		}
		if mode == "both" {
			est.Artifacts = append(est.Artifacts, "voice/voice.wav", "voice/transcript.{txt,json,srt,vtt}")
		}
		if frames > 3000 || est.EstimatedMB > 800 {
			est.Warning = "Large run estimate. Consider lower FPS or smaller time range."
		}
	}
	return est
}

func runPreviewResult(input string, fps float64, format string, mode string) (map[string]any, error) {
	videoPath, note, err := resolveVideoInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve video input: %w", err)
	}
	info, err := media.ProbeVideoInfo(videoPath)
	if err != nil {
		return nil, fmt.Errorf("video probe failed: %w", err)
	}
	estimate := estimateOutputs(info, fps, format, mode)
	return map[string]any{
		"input":       input,
		"resolved":    videoPath,
		"source_note": note,
		"video_info":  info,
		"target_fps":  fps,
		"format":      format,
		"mode":        mode,
		"estimate":    estimate,
	}, nil
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
	cmd.Flags().StringVar(&artifact, "artifact", "run", "Artifact: run|transcript|sheet|log|metadata|audio")
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
				failf("copy failed: %v\n", err)
				return
			}
			fmt.Printf("Copied: %s\n", target)
		},
	}
	cmd.Flags().StringVar(&rootDir, "root", "", "Runs root directory (default: config frames root)")
	cmd.Flags().StringVar(&artifact, "artifact", "run", "Artifact: run|transcript|sheet|log|metadata|audio")
	return cmd
}

func findLastRunArtifact(rootDir string, artifact string) (string, string, error) {
	items, err := scanRunDirs(rootDir)
	if err != nil {
		return "", "", err
	}
	if len(items) == 0 {
		return "", "", fmt.Errorf("no runs found in %s", rootDir)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	runPath := items[0].path
	kind := strings.ToLower(strings.TrimSpace(artifact))
	if kind == "" {
		kind = "run"
	}
	switch kind {
	case "run":
		return runPath, runPath, nil
	case "transcript":
		p := filepath.Join(runPath, "voice", "transcript.txt")
		if _, err := os.Stat(p); err != nil {
			return "", runPath, fmt.Errorf("transcript not found for last run: %s", runPath)
		}
		return p, runPath, nil
	case "sheet":
		p := filepath.Join(runPath, "images", "sheets", "contact-sheet.png")
		if _, err := os.Stat(p); err != nil {
			return "", runPath, fmt.Errorf("contact sheet not found for last run: %s", runPath)
		}
		return p, runPath, nil
	case "log":
		p := filepath.Join(runPath, "extract.log")
		if _, err := os.Stat(p); err != nil {
			return "", runPath, fmt.Errorf("log not found for last run: %s", runPath)
		}
		return p, runPath, nil
	case "metadata":
		p := media.MetadataPathForRun(runPath)
		if _, err := os.Stat(p); err != nil {
			return "", runPath, fmt.Errorf("metadata not found for last run: %s", runPath)
		}
		return p, runPath, nil
	case "audio":
		candidates, _ := filepath.Glob(filepath.Join(runPath, "voice", "voice.*"))
		if len(candidates) == 0 {
			return "", runPath, fmt.Errorf("audio not found for last run: %s", runPath)
		}
		return candidates[0], runPath, nil
	default:
		return "", "", fmt.Errorf("unsupported artifact %q (use run|transcript|sheet|log|metadata|audio)", artifact)
	}
}

func latestRunArtifacts(rootDir string) (map[string]string, error) {
	_, runPath, err := findLastRunArtifact(rootDir, "run")
	if err != nil {
		return nil, err
	}
	out := map[string]string{"run": runPath}
	if p, _, err := findLastRunArtifact(rootDir, "transcript"); err == nil {
		out["transcript"] = p
	}
	if p, _, err := findLastRunArtifact(rootDir, "sheet"); err == nil {
		out["sheet"] = p
	}
	if p, _, err := findLastRunArtifact(rootDir, "log"); err == nil {
		out["log"] = p
	}
	if p, _, err := findLastRunArtifact(rootDir, "metadata"); err == nil {
		out["metadata"] = p
	}
	if p, _, err := findLastRunArtifact(rootDir, "audio"); err == nil {
		out["audio"] = p
	}
	return out, nil
}

type runDirItem struct {
	path string
	mod  time.Time
}

func scanRunDirs(rootDir string) ([]runDirItem, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []runDirItem{}, nil
		}
		return nil, err
	}
	out := make([]runDirItem, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(rootDir, e.Name())
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		out = append(out, runDirItem{path: p, mod: info.ModTime()})
	}
	return out, nil
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
	return fmt.Errorf("no clipboard tool found (pbcopy/clip/wl-copy/xclip/xsel)")
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
	Code    int    `json:"code"`
	Message string `json:"message"`
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
	br := bufio.NewReader(in)
	state := newMCPServerState(out)
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
				if writeErr := state.write(&mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32602, Message: "invalid params"}}); writeErr != nil {
					return writeErr
				}
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), state.toolTimeout(params.TimeoutMS))
			state.registerCall(req.ID, cancel)
			go func(reqID json.RawMessage, p mcpToolCallParams, ctx context.Context, cancel context.CancelFunc) {
				defer cancel()
				defer state.completeCall(reqID)
				result, err := callMCPTool(ctx, p.Name, p.Arguments)
				if err != nil {
					msg := err.Error()
					code := -32000
					if errors.Is(ctx.Err(), context.Canceled) {
						code = -32800
						msg = "tool call cancelled"
					} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						code = -32001
						msg = "tool call timed out"
					}
					_ = state.write(&mcpResponse{
						JSONRPC: "2.0",
						ID:      reqID,
						Error:   &mcpError{Code: code, Message: msg},
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
		return &mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: code, Message: msg}}
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
						"input":  map[string]any{"type": "string"},
						"fps":    map[string]any{"type": "number"},
						"format": map[string]any{"type": "string"},
						"mode":   map[string]any{"type": "string"},
					},
				},
			},
			{
				"name":        "extract",
				"description": "Run single-video extraction workflow (input defaults to recent)",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input":               map[string]any{"type": "string"},
						"fps":                 map[string]any{"type": "number"},
						"voice":               map[string]any{"type": "boolean"},
						"format":              map[string]any{"type": "string"},
						"out":                 map[string]any{"type": "string"},
						"hwaccel":             map[string]any{"type": "string"},
						"preset":              map[string]any{"type": "string"},
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

func callMCPTool(ctx context.Context, name string, argsRaw json.RawMessage) (any, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	switch name {
	case "doctor":
		started := time.Now()
		return buildAutomationEnvelope("doctor", started, "success", collectDoctorReport(appCfg), nil), nil
	case "preview":
		var args struct {
			Input  string  `json:"input"`
			FPS    float64 `json:"fps"`
			Format string  `json:"format"`
			Mode   string  `json:"mode"`
		}
		_ = json.Unmarshal(argsRaw, &args)
		if strings.TrimSpace(args.Input) == "" {
			args.Input = "recent"
		}
		if err := ensureMCPPathAllowed(args.Input); err != nil {
			return nil, err
		}
		if args.FPS <= 0 {
			args.FPS = appCfg.DefaultFPS
		}
		if strings.TrimSpace(args.Format) == "" {
			args.Format = appCfg.DefaultFormat
		}
		if strings.TrimSpace(args.Mode) == "" {
			args.Mode = "both"
		}
		started := time.Now()
		data, err := runPreviewResult(args.Input, args.FPS, args.Format, args.Mode)
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
			TranscribeBackend  string  `json:"transcribe_backend"`
			TranscribeBin      string  `json:"transcribe_bin"`
			TranscribeLanguage string  `json:"transcribe_language"`
		}
		_ = json.Unmarshal(argsRaw, &args)
		if strings.TrimSpace(args.Input) == "" {
			args.Input = "recent"
		}
		if err := ensureMCPPathAllowed(args.Input); err != nil {
			return nil, err
		}
		if args.FPS <= 0 {
			args.FPS = appCfg.DefaultFPS
		}
		if strings.TrimSpace(args.Format) == "" {
			args.Format = appCfg.DefaultFormat
		}
		if strings.TrimSpace(args.HWAccel) == "" {
			args.HWAccel = appCfg.HWAccel
		}
		if strings.TrimSpace(args.Preset) == "" {
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
		res, err := runExtractWorkflowResult(ctx, args.Input, args.FPS, extractWorkflowOptions{
			OutDir:             args.Out,
			Voice:              args.Voice,
			FrameFormat:        args.Format,
			HWAccel:            args.HWAccel,
			Preset:             args.Preset,
			StartTime:          args.From,
			EndTime:            args.To,
			StartFrame:         args.FrameStart,
			EndFrame:           args.FrameEnd,
			EveryN:             args.EveryN,
			FrameName:          args.NameTemplate,
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
			err = withDiagnostics("mcp_extract", args.Input, args.Out, "", err)
			return buildAutomationEnvelope("extract", started, "error", map[string]string{"input": args.Input}, err), nil
		}
		return buildAutomationEnvelope("extract", started, "success", res, nil), nil
	case "extract_batch":
		var args struct {
			Inputs             []string `json:"inputs"`
			FPS                float64  `json:"fps"`
			Voice              bool     `json:"voice"`
			Format             string   `json:"format"`
			TranscribeBackend  string   `json:"transcribe_backend"`
			TranscribeBin      string   `json:"transcribe_bin"`
			TranscribeLanguage string   `json:"transcribe_language"`
		}
		_ = json.Unmarshal(argsRaw, &args)
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
		if args.FPS <= 0 {
			args.FPS = appCfg.DefaultFPS
		}
		if strings.TrimSpace(args.Format) == "" {
			args.Format = appCfg.DefaultFormat
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
				TranscribeBackend:  args.TranscribeBackend,
				TranscribeBin:      args.TranscribeBin,
				TranscribeLanguage: args.TranscribeLanguage,
				JPGQuality:         3,
				SheetCols:          6,
				HWAccel:            appCfg.HWAccel,
				Preset:             appCfg.PerformanceMode,
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
		_ = json.Unmarshal(argsRaw, &args)
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
		_ = json.Unmarshal(argsRaw, &args)
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
		_ = json.Unmarshal(argsRaw, &args)
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

func importCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "import [videoPath]",
		Aliases: []string{"drop"},
		Short:   "Import a video path and run extraction",
		Args:    cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			videoPath := ""
			if len(args) == 1 {
				videoPath = args[0]
			} else {
				reader := bufio.NewReader(os.Stdin)
				fmt.Print("Paste a video path, then press Enter: ")
				line, err := reader.ReadString('\n')
				if err != nil && strings.TrimSpace(line) == "" {
					failf("failed to read video path: %v\n", err)
					return
				}
				videoPath = line
			}
			videoPath = normalizeDroppedPath(videoPath)
			if strings.TrimSpace(videoPath) == "" {
				failln("no video path provided")
				return
			}
			noModal, _ := cmd.Flags().GetBool("no-modal")
			fps, _ := cmd.Flags().GetFloat64("fps")
			if !cmd.Flags().Changed("fps") && appCfg.DefaultFPS > 0 {
				fps = appCfg.DefaultFPS
			}
			outDir, _ := cmd.Flags().GetString("out")
			voice, _ := cmd.Flags().GetBool("voice")
			frameFormat, _ := cmd.Flags().GetString("format")
			if !cmd.Flags().Changed("format") && appCfg.DefaultFormat != "" {
				frameFormat = appCfg.DefaultFormat
			}
			jpgQuality, _ := cmd.Flags().GetInt("quality")
			noSheet, _ := cmd.Flags().GetBool("no-sheet")
			sheetCols, _ := cmd.Flags().GetInt("sheet-cols")
			verbose, _ := cmd.Flags().GetBool("verbose")
			hwaccel, _ := cmd.Flags().GetString("hwaccel")
			preset, _ := cmd.Flags().GetString("preset")
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
			if !cmd.Flags().Changed("hwaccel") {
				hwaccel = appCfg.HWAccel
			}
			if !cmd.Flags().Changed("preset") {
				preset = appCfg.PerformanceMode
			}
			opts := extractWorkflowOptions{
				OutDir:             outDir,
				Voice:              voice,
				FrameFormat:        frameFormat,
				JPGQuality:         jpgQuality,
				NoSheet:            noSheet,
				SheetCols:          sheetCols,
				Verbose:            verbose,
				HWAccel:            hwaccel,
				Preset:             preset,
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
			}
			if !noModal {
				result, cancelled, err := runDropModal(videoPath, fps, opts)
				if err != nil {
					failf("import modal failed: %v\n", err)
					return
				}
				if cancelled {
					fmt.Println("Cancelled.")
					return
				}
				fps = result.FPS
				opts = result.Opts
			}
			if err := runExtractWorkflow(cmd.Context(), videoPath, fps, opts); err != nil {
				if isCancellationError(err) {
					markCommandInterrupted()
					fmt.Fprintln(os.Stderr, "Cancelled.")
				} else {
					failf("%v\n", err)
				}
			}
		},
	}
	cmd.Flags().Float64("fps", appCfg.DefaultFPS, "Frames per second")
	cmd.Flags().String("out", "", "Output directory")
	cmd.Flags().Bool("voice", false, "Extract audio and generate transcript in the output folder")
	cmd.Flags().String("format", appCfg.DefaultFormat, "Frame output format: png or jpg")
	cmd.Flags().Int("quality", 3, "JPG quality (1-31, lower is better quality)")
	cmd.Flags().Bool("no-sheet", false, "Skip contact sheet generation")
	cmd.Flags().Int("sheet-cols", 6, "Columns in generated contact sheet")
	cmd.Flags().Bool("verbose", false, "Show raw ffmpeg/whisper logs")
	cmd.Flags().Bool("no-modal", false, "Skip import config modal and run immediately")
	cmd.Flags().String("hwaccel", appCfg.HWAccel, "Hardware acceleration mode: none|auto|cuda|vaapi|qsv")
	cmd.Flags().String("preset", appCfg.PerformanceMode, "Performance mode: safe|balanced|fast")
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
	return cmd
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
	return &cobra.Command{
		Use:   "clean [targetDir]",
		Short: "Clean extracted frame artifacts",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			var target string
			if len(args) > 0 {
				target = strings.Join(args, " ")
			}
			if target == "" {
				target = media.DefaultFramesRoot
			}
			fmt.Printf("Cleaning frames in: %s\n", target)
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
		},
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
			appconfig.ApplyEnvDefaults(appCfg)
			outDir := "voice"
			if len(args) == 2 {
				outDir = args[1]
			}
			txt, jsonPath, err := media.TranscribeAudioWithOptions(media.TranscribeOptions{
				Context:   cmd.Context(),
				AudioPath: args[0],
				OutDir:    outDir,
				Model:     model,
				Backend:   backend,
				Bin:       bin,
				Language:  language,
				Verbose:   verbose,
			})
			if err != nil {
				failf("Transcription failed: %v\n", err)
				return
			}
			fmt.Printf("Transcript: %s\nJSON: %s\n", txt, jsonPath)
		},
	}
	cmd.Flags().Bool("verbose", false, "Show raw transcription backend logs")
	cmd.Flags().String("model", appCfg.WhisperModel, "Transcription model")
	cmd.Flags().String("backend", appCfg.TranscribeBackend, "Transcription backend: auto|whisper|faster-whisper")
	cmd.Flags().String("bin", "", "Override transcription CLI binary path/name for selected backend")
	cmd.Flags().String("language", appCfg.WhisperLanguage, "Transcription language override (blank=auto)")
	return cmd
}

func tuiCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch FramesCLI dashboard TUI",
		Run: func(cmd *cobra.Command, args []string) {
			rootDir, _ := cmd.Flags().GetString("root")
			if !cmd.Flags().Changed("root") && strings.TrimSpace(appCfg.FramesRoot) != "" {
				rootDir = appCfg.FramesRoot
			}
			if err := tui.Run(rootDir); err != nil {
				failf("TUI failed: %v\n", err)
			}
		},
	}
	cmd.Flags().String("root", media.DefaultFramesRoot, "Directory containing extracted runs")
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

type doctorReport struct {
	GeneratedAt         string                        `json:"generated_at"`
	GOOS                string                        `json:"goos"`
	GOARCH              string                        `json:"goarch"`
	ConfigPath          string                        `json:"config_path"`
	Config              appconfig.Config              `json:"config"`
	Environment         map[string]string             `json:"environment"`
	Tools               []toolCheck                   `json:"tools"`
	FFmpegVersion       string                        `json:"ffmpeg_version,omitempty"`
	FFmpegVersionError  string                        `json:"ffmpeg_version_error,omitempty"`
	FFmpegHWAccels      []string                      `json:"ffmpeg_hwaccels,omitempty"`
	FFmpegHWAccelsError string                        `json:"ffmpeg_hwaccels_error,omitempty"`
	ConfigWarnings      []appconfig.ValidationWarning `json:"config_warnings,omitempty"`
	RequiredFailed      bool                          `json:"required_failed"`
}

func collectDoctorReport(cfg appconfig.Config) doctorReport {
	r := doctorReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		Config:      cfg,
		Environment: map[string]string{
			"OBS_VIDEO_DIR":      valueOrDash(os.Getenv("OBS_VIDEO_DIR")),
			"WHISPER_BIN":        valueOrDash(os.Getenv("WHISPER_BIN")),
			"FASTER_WHISPER_BIN": valueOrDash(os.Getenv("FASTER_WHISPER_BIN")),
			"WHISPER_MODEL":      valueOrDash(os.Getenv("WHISPER_MODEL")),
			"WHISPER_LANGUAGE":   valueOrDash(os.Getenv("WHISPER_LANGUAGE")),
			"TRANSCRIBE_BACKEND": valueOrDash(os.Getenv("TRANSCRIBE_BACKEND")),
		},
	}
	if cfgPath, err := appconfig.Path(); err == nil {
		r.ConfigPath = cfgPath
	}

	checks := []struct {
		name     string
		required bool
	}{
		{name: "ffmpeg", required: true},
		{name: "ffprobe", required: true},
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
	if strings.TrimSpace(r.FFmpegVersion) != "" {
		fmt.Printf("FFMPEG_VERSION:    %s\n", r.FFmpegVersion)
	} else {
		fmt.Printf("FFMPEG_VERSION:    unavailable (%s)\n", valueOrDash(r.FFmpegVersionError))
	}
	if len(r.FFmpegHWAccels) > 0 {
		fmt.Printf("FFMPEG_HWACCELS:   %s\n", strings.Join(r.FFmpegHWAccels, ", "))
	} else {
		fmt.Printf("FFMPEG_HWACCELS:   unavailable (%s)\n", valueOrDash(r.FFmpegHWAccelsError))
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
			fmt.Printf("TUI Theme:       %s\n", appCfg.TUIThemePreset)
			fmt.Printf("TUI Simple Mode: %s\n", boolWord(appCfg.TUISimpleMode))
			fmt.Printf("TUI Welcome Seen:%s\n", boolWord(appCfg.TUIWelcomeSeen))
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
			fmt.Println("- Run `framescli import` or `framescli tui` to start processing video.")
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
	cmd.Flags().StringVar(&performanceMode, "performance-mode", appCfg.PerformanceMode, "Default performance mode: safe|balanced|fast")
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
