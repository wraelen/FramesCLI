package media

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DefaultFramesRoot = "frames"
const contactSheetMaxTiles = 48
const contactTileWidth = 420

var obsDefaultDir = `C:\Users\wraelen\Videos\OBS`

type ExtractMediaOptions struct {
	Context      context.Context
	VideoPath    string
	FPS          float64
	OutDir       string
	FrameFormat  string
	JPGQuality   int
	HWAccel      string
	Preset       string
	ExtractAudio bool
	ProgressFn   func(percent float64)
	Verbose      bool
	StartTime    string
	EndTime      string
	StartFrame   int
	EndFrame     int
	EveryNFrames int
	FramePattern string
	ZipFrames    bool
	LogFile      string
	AudioFormat  string
	AudioBitrate string
	Normalize    bool
	AudioStart   string
	AudioEnd     string
	MetadataCSV  bool
}

type ExtractMediaResult struct {
	OutDir        string
	ImagesDir     string
	FrameFormat   string
	AudioPath     string
	UsedHWAccel   string
	FallbackUsed  bool
	Warnings      []string
	MetadataPath  string
	FramesZipPath string
	CSVPath       string
	VideoInfo     VideoInfo
}

type RunMetadata struct {
	VideoPath      string  `json:"video_path"`
	FPS            float64 `json:"fps"`
	FrameFormat    string  `json:"frame_format"`
	ExtractedAudio bool    `json:"extracted_audio"`
	AudioPath      string  `json:"audio_path,omitempty"`
	HWAccel        string  `json:"hwaccel,omitempty"`
	FallbackUsed   bool    `json:"fallback_used,omitempty"`
	Preset         string  `json:"preset,omitempty"`
	FFmpegVersion  string  `json:"ffmpeg_version,omitempty"`
	DurationSec    float64 `json:"duration_sec,omitempty"`
	CreatedAt      string  `json:"created_at"`
	Width          int     `json:"width,omitempty"`
	Height         int     `json:"height,omitempty"`
	SourceFPS      float64 `json:"source_fps,omitempty"`
	StartTime      string  `json:"start_time,omitempty"`
	EndTime        string  `json:"end_time,omitempty"`
	StartFrame     int     `json:"start_frame,omitempty"`
	EndFrame       int     `json:"end_frame,omitempty"`
	EveryNFrames   int     `json:"every_n_frames,omitempty"`
	FramePattern   string  `json:"frame_pattern,omitempty"`
	MetadataCSV    bool    `json:"metadata_csv,omitempty"`
	FramesZipPath  string  `json:"frames_zip_path,omitempty"`
}

type VideoInfo struct {
	DurationSec float64 `json:"duration_sec"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FrameRate   float64 `json:"frame_rate"`
	HasAudio    bool    `json:"has_audio"`
}

type FrameRecord struct {
	File            string  `json:"file"`
	Index           int     `json:"index"`
	TimestampSec    float64 `json:"timestamp_sec"`
	TimestampPretty string  `json:"timestamp_pretty"`
}

const runMetadataName = "run.json"

func MetadataPathForRun(runDir string) string {
	return filepath.Join(runDir, runMetadataName)
}

func ReadRunMetadata(runDir string) (RunMetadata, error) {
	path := MetadataPathForRun(runDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunMetadata{}, err
	}
	var out RunMetadata
	if err := json.Unmarshal(raw, &out); err != nil {
		return RunMetadata{}, err
	}
	return out, nil
}

type WhisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type WhisperJSON struct {
	Text     string           `json:"text"`
	Segments []WhisperSegment `json:"segments"`
}

func NormalizeVideoPath(input string) string {
	if runtime.GOOS == "windows" {
		return input
	}
	if len(input) >= 2 && input[1] == ':' {
		drive := strings.ToLower(string(input[0]))
		rest := strings.ReplaceAll(input[2:], "\\", "/")
		return "/mnt/" + drive + "/" + strings.TrimLeft(rest, "/")
	}
	return input
}

func ExtractMedia(opts ExtractMediaOptions) (*ExtractMediaResult, error) {
	if opts.VideoPath == "" {
		return nil, errors.New("video path is required")
	}
	if opts.FPS <= 0 || math.IsNaN(opts.FPS) {
		return nil, errors.New("fps must be a positive number")
	}
	if opts.EveryNFrames < 0 {
		return nil, errors.New("every-n-frames must be >= 0")
	}
	if opts.StartFrame < 0 || opts.EndFrame < 0 {
		return nil, errors.New("frame range values must be >= 0")
	}
	if opts.EndFrame > 0 && opts.StartFrame > opts.EndFrame {
		return nil, errors.New("start-frame cannot be greater than end-frame")
	}

	frameFormat, err := normalizeFrameFormat(opts.FrameFormat)
	if err != nil {
		return nil, err
	}
	if opts.JPGQuality <= 0 {
		opts.JPGQuality = 3
	}
	if opts.JPGQuality > 31 {
		return nil, errors.New("jpg quality must be between 1 and 31")
	}
	hwaccel := normalizeHWAccel(opts.HWAccel)
	preset := normalizePreset(opts.Preset)
	audioFormat, err := normalizeAudioFormat(opts.AudioFormat)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.AudioBitrate) == "" {
		opts.AudioBitrate = "128k"
	}
	if strings.TrimSpace(opts.FramePattern) == "" {
		opts.FramePattern = "frame-%04d"
	}
	if !strings.Contains(opts.FramePattern, "%") || !strings.Contains(opts.FramePattern, "d") {
		return nil, errors.New("frame pattern must include a printf integer placeholder, e.g. frame_%04d")
	}

	videoPath := NormalizeVideoPath(opts.VideoPath)
	videoPath, err = filepath.Abs(videoPath)
	if err != nil {
		return nil, err
	}
	videoInfo, err := ProbeVideoInfo(videoPath)
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed for %q: %w", opts.VideoPath, err)
	}
	outDir := opts.OutDir
	if outDir == "" {
		timestamp := makeRunFolderName(time.Now())
		outDir = filepath.Join(DefaultFramesRoot, timestamp)
		outDir = ensureUniquePath(outDir)
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	imagesDir := filepath.Join(outDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return nil, err
	}
	audioPath := ""
	if opts.ExtractAudio {
		audioPath = filepath.Join(outDir, "voice", "voice."+audioFormat)
		if err := os.MkdirAll(filepath.Dir(audioPath), 0o755); err != nil {
			return nil, err
		}
	}

	framePattern := filepath.Join(imagesDir, opts.FramePattern+"."+frameFormat)
	videoFilter := buildVideoFilter(opts.FPS, opts.EveryNFrames, opts.StartFrame, opts.EndFrame)
	args := []string{"-y"}
	if hw := hwaccelArgs(hwaccel); len(hw) > 0 {
		args = append(args, hw...)
	}
	args = append(args, "-i", videoPath, "-map", "0:v:0")
	if strings.TrimSpace(opts.StartTime) != "" {
		args = append(args, "-ss", strings.TrimSpace(opts.StartTime))
	}
	if strings.TrimSpace(opts.EndTime) != "" {
		args = append(args, "-to", strings.TrimSpace(opts.EndTime))
	}
	args = append(args, "-vf", videoFilter)
	if frameFormat == "jpg" {
		args = append(args, "-q:v", strconv.Itoa(opts.JPGQuality))
	}
	args = append(args, "-vsync", "vfr", framePattern)

	if opts.ExtractAudio {
		args = append(args, "-map", "0:a:0?", "-vn")
		if strings.TrimSpace(opts.AudioStart) != "" {
			args = append(args, "-ss", strings.TrimSpace(opts.AudioStart))
		}
		if strings.TrimSpace(opts.AudioEnd) != "" {
			args = append(args, "-to", strings.TrimSpace(opts.AudioEnd))
		}
		if opts.Normalize {
			args = append(args, "-af", "loudnorm")
		}
		args = append(args, buildAudioOutputArgs(audioFormat, opts.AudioBitrate)...)
		args = append(args, audioPath)
	}
	if perfArgs := performanceArgs(preset); len(perfArgs) > 0 {
		args = append(args, perfArgs...)
	}

	durationSec := videoInfo.DurationSec
	warnings := make([]string, 0, 1)
	usedHWAccel := hwaccel
	fallbackUsed := false
	logWriter, closer, _ := createLogWriter(opts.LogFile)
	if closer != nil {
		defer closer()
	}
	if err := runFFmpeg(opts.Context, args, durationSec, opts.ProgressFn, opts.Verbose, logWriter); err != nil {
		shouldFallback := hwaccel != "none"
		if !shouldFallback {
			return nil, err
		}
		fallbackUsed = true
		usedHWAccel = "none"
		warnings = append(warnings, fmt.Sprintf("hwaccel %q failed; falling back to CPU", hwaccel))
		cpuArgs := []string{"-y", "-i", videoPath, "-map", "0:v:0"}
		if strings.TrimSpace(opts.StartTime) != "" {
			cpuArgs = append(cpuArgs, "-ss", strings.TrimSpace(opts.StartTime))
		}
		if strings.TrimSpace(opts.EndTime) != "" {
			cpuArgs = append(cpuArgs, "-to", strings.TrimSpace(opts.EndTime))
		}
		cpuArgs = append(cpuArgs, "-vf", videoFilter)
		if frameFormat == "jpg" {
			cpuArgs = append(cpuArgs, "-q:v", strconv.Itoa(opts.JPGQuality))
		}
		cpuArgs = append(cpuArgs, "-vsync", "vfr", framePattern)
		if opts.ExtractAudio {
			cpuArgs = append(cpuArgs, "-map", "0:a:0?", "-vn")
			if opts.Normalize {
				cpuArgs = append(cpuArgs, "-af", "loudnorm")
			}
			cpuArgs = append(cpuArgs, buildAudioOutputArgs(audioFormat, opts.AudioBitrate)...)
			cpuArgs = append(cpuArgs, audioPath)
		}
		if perfArgs := performanceArgs(preset); len(perfArgs) > 0 {
			cpuArgs = append(cpuArgs, perfArgs...)
		}
		if cpuErr := runFFmpeg(opts.Context, cpuArgs, durationSec, opts.ProgressFn, opts.Verbose, logWriter); cpuErr != nil {
			return nil, fmt.Errorf("hwaccel attempt failed (%v) and cpu fallback failed (%w)", err, cpuErr)
		}
	}

	frameRecords := []FrameRecord{}
	csvPath := ""
	if records, recordsErr := buildFrameRecords(imagesDir, opts, videoInfo); recordsErr == nil {
		frameRecords = records
		if opts.MetadataCSV {
			csvPath = filepath.Join(outDir, "frames.csv")
			_ = writeFrameRecordsCSV(csvPath, frameRecords)
		}
	}
	zipPath := ""
	if opts.ZipFrames {
		zipPath = filepath.Join(outDir, "frames.zip")
		if zipErr := zipDir(imagesDir, zipPath); zipErr != nil {
			warnings = append(warnings, fmt.Sprintf("zip failed: %v", zipErr))
		}
	}

	ffmpegVersion, _ := FFmpegVersion()
	metadata := RunMetadata{
		VideoPath:      opts.VideoPath,
		FPS:            opts.FPS,
		FrameFormat:    frameFormat,
		ExtractedAudio: opts.ExtractAudio,
		AudioPath:      audioPath,
		HWAccel:        usedHWAccel,
		FallbackUsed:   fallbackUsed,
		Preset:         preset,
		FFmpegVersion:  ffmpegVersion,
		DurationSec:    durationSec,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		Width:          videoInfo.Width,
		Height:         videoInfo.Height,
		SourceFPS:      videoInfo.FrameRate,
		StartTime:      strings.TrimSpace(opts.StartTime),
		EndTime:        strings.TrimSpace(opts.EndTime),
		StartFrame:     opts.StartFrame,
		EndFrame:       opts.EndFrame,
		EveryNFrames:   opts.EveryNFrames,
		FramePattern:   opts.FramePattern,
		MetadataCSV:    opts.MetadataCSV,
		FramesZipPath:  zipPath,
	}
	metadataPath := MetadataPathForRun(outDir)
	if raw, err := json.MarshalIndent(metadata, "", "  "); err == nil {
		_ = os.WriteFile(metadataPath, raw, 0o644)
	}
	if len(frameRecords) > 0 {
		framesPath := filepath.Join(outDir, "frames.json")
		if raw, err := json.MarshalIndent(frameRecords, "", "  "); err == nil {
			_ = os.WriteFile(framesPath, raw, 0o644)
		}
	}

	return &ExtractMediaResult{
		OutDir:        outDir,
		ImagesDir:     imagesDir,
		FrameFormat:   frameFormat,
		AudioPath:     audioPath,
		UsedHWAccel:   usedHWAccel,
		FallbackUsed:  fallbackUsed,
		Warnings:      warnings,
		MetadataPath:  metadataPath,
		FramesZipPath: zipPath,
		CSVPath:       csvPath,
		VideoInfo:     videoInfo,
	}, nil
}

func ExtractAudioFromVideo(videoPath string, outDir string, filename string, verbose bool) (string, error) {
	return ExtractAudioFromVideoWithOptions(ExtractAudioOptions{
		VideoPath: videoPath,
		OutDir:    outDir,
		Filename:  filename,
		Format:    "wav",
		Bitrate:   "128k",
		Verbose:   verbose,
	})
}

type ExtractAudioOptions struct {
	Context   context.Context
	VideoPath string
	OutDir    string
	Filename  string
	Format    string
	Bitrate   string
	Normalize bool
	StartTime string
	EndTime   string
	LogFile   string
	Verbose   bool
}

func ExtractAudioFromVideoWithOptions(opts ExtractAudioOptions) (string, error) {
	videoPath := opts.VideoPath
	outDir := opts.OutDir
	filename := opts.Filename
	if videoPath == "" {
		return "", errors.New("video path is required")
	}
	if outDir == "" {
		return "", errors.New("output directory is required")
	}
	format, err := normalizeAudioFormat(opts.Format)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(opts.Bitrate) == "" {
		opts.Bitrate = "128k"
	}
	if filename == "" {
		filename = "voice." + format
	}

	videoPath = NormalizeVideoPath(videoPath)
	videoPath, err = filepath.Abs(videoPath)
	if err != nil {
		return "", err
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}

	audioPath := filepath.Join(outDir, filename)
	audioPath, err = filepath.Abs(audioPath)
	if err != nil {
		return "", err
	}
	args := []string{
		"-y",
		"-i",
		videoPath,
		"-vn",
	}
	if strings.TrimSpace(opts.StartTime) != "" {
		args = append(args, "-ss", strings.TrimSpace(opts.StartTime))
	}
	if strings.TrimSpace(opts.EndTime) != "" {
		args = append(args, "-to", strings.TrimSpace(opts.EndTime))
	}
	if opts.Normalize {
		args = append(args, "-af", "loudnorm")
	}
	args = append(args, buildAudioOutputArgs(format, opts.Bitrate)...)
	args = append(args, audioPath)
	logWriter, closer, _ := createLogWriter(opts.LogFile)
	if closer != nil {
		defer closer()
	}
	if err := runFFmpeg(opts.Context, args, 0, nil, opts.Verbose, logWriter); err != nil {
		return "", err
	}

	return audioPath, nil
}

func TranscribeAudio(audioPath string, outDir string, verbose bool) (string, string, error) {
	return TranscribeAudioWithOptions(TranscribeOptions{
		AudioPath: audioPath,
		OutDir:    outDir,
		Verbose:   verbose,
	})
}

type TranscribeOptions struct {
	Context    context.Context
	AudioPath  string
	OutDir     string
	Model      string
	Language   string
	Bin        string
	Backend    string
	TimeoutSec int
	Verbose    bool
}

type ErrTranscribeTimeout struct {
	Seconds int
}

func (e *ErrTranscribeTimeout) Error() string {
	if e == nil || e.Seconds <= 0 {
		return "transcription timed out"
	}
	return fmt.Sprintf("transcription timed out after %ds", e.Seconds)
}

const (
	TranscribeBackendAuto          = "auto"
	TranscribeBackendWhisper       = "whisper"
	TranscribeBackendFasterWhisper = "faster-whisper"
)

func normalizeTranscribeBackend(backend string) string {
	b := strings.ToLower(strings.TrimSpace(backend))
	switch b {
	case "", TranscribeBackendAuto:
		return TranscribeBackendAuto
	case TranscribeBackendWhisper:
		return TranscribeBackendWhisper
	case "faster_whisper", "fasterwhisper", TranscribeBackendFasterWhisper:
		return TranscribeBackendFasterWhisper
	default:
		return ""
	}
}

func resolveTranscribeBackendAndBin(opts TranscribeOptions) (string, string, error) {
	backend := normalizeTranscribeBackend(opts.Backend)
	if backend == "" {
		return "", "", fmt.Errorf("unsupported transcribe backend %q (use auto|whisper|faster-whisper)", opts.Backend)
	}
	explicitBin := strings.TrimSpace(opts.Bin)
	whisperBin := strings.TrimSpace(os.Getenv("WHISPER_BIN"))
	if whisperBin == "" {
		whisperBin = "whisper"
	}
	fasterBin := strings.TrimSpace(os.Getenv("FASTER_WHISPER_BIN"))
	if fasterBin == "" {
		fasterBin = "faster-whisper"
	}

	switch backend {
	case TranscribeBackendWhisper:
		bin := explicitBin
		if bin == "" {
			bin = whisperBin
		}
		if _, err := exec.LookPath(bin); err != nil {
			return "", "", fmt.Errorf("whisper backend selected but binary %q not found on PATH", bin)
		}
		return TranscribeBackendWhisper, bin, nil
	case TranscribeBackendFasterWhisper:
		bin := explicitBin
		if bin == "" {
			bin = fasterBin
		}
		if _, err := exec.LookPath(bin); err != nil {
			return "", "", fmt.Errorf("faster-whisper backend selected but binary %q not found on PATH", bin)
		}
		return TranscribeBackendFasterWhisper, bin, nil
	default:
		// auto: prefer faster-whisper if available, then whisper.
		if explicitBin != "" {
			if _, err := exec.LookPath(explicitBin); err != nil {
				return "", "", fmt.Errorf("transcribe binary override %q not found on PATH", explicitBin)
			}
			return TranscribeBackendAuto, explicitBin, nil
		}
		if _, err := exec.LookPath(fasterBin); err == nil {
			return TranscribeBackendFasterWhisper, fasterBin, nil
		}
		if _, err := exec.LookPath(whisperBin); err == nil {
			return TranscribeBackendWhisper, whisperBin, nil
		}
		return "", "", fmt.Errorf("no transcription backend found: install faster-whisper or whisper")
	}
}

func resolveTranscriptJSONPath(outDir, audioPath string) (string, error) {
	base := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	candidates := []string{
		filepath.Join(outDir, base+".json"),
		filepath.Join(outDir, "transcript.json"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "*.json"))
	if err != nil {
		return "", err
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		sort.Strings(matches)
		return matches[len(matches)-1], nil
	}
	return "", fmt.Errorf("no transcript json found in %s", outDir)
}

func hasGPU() bool {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		return true
	}
	if _, err := os.Stat("/dev/nvidia0"); err == nil {
		return true
	}
	return false
}

func whisperModelCachePath(model string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	model = strings.TrimSpace(strings.ToLower(model))
	if model == "" {
		return "", errors.New("whisper model is required")
	}
	return filepath.Join(homeDir, ".cache", "whisper", model+".pt"), nil
}

func whisperModelDownloadSize(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "tiny":
		return "75MB"
	case "base":
		return "145MB"
	case "small":
		return "461MB"
	case "medium":
		return "1.5GB"
	default:
		return "unknown size"
	}
}

func TranscribeAudioWithOptions(opts TranscribeOptions) (string, string, error) {
	audioPath := opts.AudioPath
	outDir := opts.OutDir
	verbose := opts.Verbose
	if audioPath == "" {
		return "", "", errors.New("audio path is required")
	}
	if outDir == "" {
		return "", "", errors.New("output directory is required")
	}
	audioPath, err := filepath.Abs(audioPath)
	if err != nil {
		return "", "", err
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return "", "", err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", "", err
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("WHISPER_MODEL"))
	}
	modelWasDefaulted := model == ""
	if modelWasDefaulted {
		model = "base"
	}
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = strings.TrimSpace(os.Getenv("WHISPER_LANGUAGE"))
	}
	backend := strings.TrimSpace(opts.Backend)
	if backend == "" {
		backend = strings.TrimSpace(os.Getenv("TRANSCRIBE_BACKEND"))
	}
	if backend == "" {
		backend = TranscribeBackendAuto
	}
	resolvedBackend, transcribeBin, err := resolveTranscribeBackendAndBin(TranscribeOptions{
		Backend: backend,
		Bin:     opts.Bin,
	})
	if err != nil {
		return "", "", err
	}

	if !hasGPU() {
		lowerModel := strings.ToLower(model)
		baseOrLarger := strings.HasPrefix(lowerModel, "base") ||
			strings.HasPrefix(lowerModel, "small") ||
			strings.HasPrefix(lowerModel, "medium") ||
			strings.HasPrefix(lowerModel, "large")
		if baseOrLarger {
			if modelWasDefaulted {
				model = "tiny"
				fmt.Fprintf(os.Stderr, "[framescli] info: no GPU detected — auto-selected model %q for CPU performance. Override with --transcribe-model.\n", model)
			} else {
				fmt.Fprintf(os.Stderr, "[framescli] warning: no GPU detected — transcription may be slow with model %q. Consider using --transcribe-model tiny for faster results on CPU.\n", model)
			}
		}
	}

	args := []string{
		audioPath,
		"--model", model,
		"--output_dir", outDir,
		"--output_format", "json",
	}
	if language != "" {
		args = append(args, "--language", language)
	}

	runCtx := ctxOrBackground(opts.Context)
	var cancel context.CancelFunc
	if opts.TimeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(opts.TimeoutSec)*time.Second)
		defer cancel()
	}

	if resolvedBackend == TranscribeBackendWhisper {
		cachePath, cacheErr := whisperModelCachePath(model)
		if cacheErr == nil {
			if _, err := os.Stat(cachePath); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "[framescli] downloading whisper model %q (~%s) — this only happens once.\n", model, whisperModelDownloadSize(model))
			}
		}
	}

	cmd := exec.CommandContext(runCtx, transcribeBin, args...)
	var stderr bytes.Buffer
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			fmt.Fprintf(os.Stderr, "[framescli] transcription timed out after %ds — skipping transcript.\n", opts.TimeoutSec)
			return "", "", &ErrTranscribeTimeout{Seconds: opts.TimeoutSec}
		}
		if !verbose {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("%s backend failed: %w: %s", resolvedBackend, err, msg)
			}
		}
		return "", "", fmt.Errorf("%s backend failed: %w", resolvedBackend, err)
	}

	transcriptRawPath, err := resolveTranscriptJSONPath(outDir, audioPath)
	if err != nil {
		return "", "", err
	}
	raw, err := os.ReadFile(transcriptRawPath)
	if err != nil {
		return "", "", err
	}

	transcriptJSON := filepath.Join(outDir, "transcript.json")
	if err := os.WriteFile(transcriptJSON, raw, 0o644); err != nil {
		return "", "", err
	}

	text := ""
	var parsed WhisperJSON
	if err := json.Unmarshal(raw, &parsed); err == nil {
		text = strings.TrimSpace(parsed.Text)
	}
	if text == "" {
		text = strings.TrimSpace(string(raw))
	}

	transcriptTxt := filepath.Join(outDir, "transcript.txt")
	if err := os.WriteFile(transcriptTxt, []byte(text+"\n"), 0o644); err != nil {
		return "", "", err
	}

	if len(parsed.Segments) > 0 {
		srtPath := filepath.Join(outDir, "transcript.srt")
		if err := writeSRT(srtPath, parsed.Segments); err != nil {
			return "", "", err
		}
		vttPath := filepath.Join(outDir, "transcript.vtt")
		if err := writeVTT(vttPath, parsed.Segments); err != nil {
			return "", "", err
		}
	}

	return transcriptTxt, transcriptJSON, nil
}

func CreateContactSheet(framesDir string, cols int, outFile string, verbose bool) (string, error) {
	return CreateContactSheetWithContext(context.Background(), framesDir, cols, outFile, verbose)
}

func CreateContactSheetWithContext(ctx context.Context, framesDir string, cols int, outFile string, verbose bool) (string, error) {
	if framesDir == "" {
		return "", errors.New("frames dir is required")
	}
	if cols <= 0 {
		cols = 6
	}
	if outFile == "" {
		outFile = filepath.Join(framesDir, "contact-sheet.png")
	}
	if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
		return "", err
	}

	frames, frameExt, err := listFrameFiles(framesDir)
	if err != nil {
		return "", err
	}
	if len(frames) == 0 {
		return "", errors.New("no frames found")
	}

	pattern := filepath.Join(framesDir, "frame-*."+frameExt)
	samplingStep := int(math.Ceil(float64(len(frames)) / float64(contactSheetMaxTiles)))
	if samplingStep < 1 {
		samplingStep = 1
	}
	sampledCount := int(math.Ceil(float64(len(frames)) / float64(samplingStep)))
	if sampledCount < 1 {
		sampledCount = 1
	}
	if cols > sampledCount {
		cols = sampledCount
	}
	rows := int(math.Ceil(float64(sampledCount) / float64(cols)))

	filter := fmt.Sprintf(
		"select='not(mod(n\\,%d))',scale=%d:-1,tile=%dx%d",
		samplingStep,
		contactTileWidth,
		cols,
		rows,
	)

	args := []string{
		"-pattern_type",
		"glob",
		"-i",
		pattern,
		"-vf",
		filter,
		"-frames:v",
		"1",
		"-update",
		"1",
		outFile,
	}
	if err := runFFmpeg(ctx, args, 0, nil, verbose, nil); err != nil {
		return "", err
	}

	return outFile, nil
}

func CleanFrames(targetDir string) (int, error) {
	if targetDir == "" {
		targetDir = DefaultFramesRoot
	}
	removed := 0
	if _, err := os.Stat(targetDir); err == nil {
		removed = 1
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return 0, err
	}
	if targetDir == DefaultFramesRoot {
		if err := os.MkdirAll(DefaultFramesRoot, 0o755); err != nil {
			return 0, err
		}
	}
	return removed, nil
}

func CountFrames(imagesDir string) (int, error) {
	frames, _, err := listFrameFiles(imagesDir)
	if err != nil {
		return 0, err
	}
	return len(frames), nil
}

func DefaultOBSVideoDir() string {
	if env := os.Getenv("OBS_VIDEO_DIR"); env != "" {
		return env
	}
	return obsDefaultDir
}

func FindMostRecentVideo(dir string) (string, error) {
	return FindMostRecentVideoWithExts(dir, []string{"mp4", "mkv", "mov"})
}

func FindMostRecentVideoWithExts(dir string, exts []string) (string, error) {
	if dir == "" {
		return "", errors.New("video directory is required")
	}
	dir = NormalizeVideoPath(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var newestPath string
	var newestTime time.Time
	extSet := map[string]struct{}{}
	for _, e := range exts {
		e = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(e), "."))
		if e == "" {
			continue
		}
		extSet["."+e] = struct{}{}
	}
	if len(extSet) == 0 {
		extSet[".mp4"] = struct{}{}
		extSet[".mkv"] = struct{}{}
		extSet[".mov"] = struct{}{}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if _, ok := extSet[ext]; !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newestPath == "" || info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestPath = filepath.Join(dir, entry.Name())
		}
	}

	if newestPath == "" {
		return "", errors.New("no video files found")
	}
	return newestPath, nil
}

func FindMostRecentVideoInDirs(dirs []string, exts []string) (string, string, error) {
	if len(dirs) == 0 {
		return "", "", errors.New("no video directories configured")
	}
	var newestPath string
	var newestDir string
	var newestTime time.Time
	for _, dir := range dirs {
		trimmed := strings.TrimSpace(dir)
		if trimmed == "" {
			continue
		}
		videoPath, err := FindMostRecentVideoWithExts(trimmed, exts)
		if err != nil {
			continue
		}
		info, statErr := os.Stat(videoPath)
		if statErr != nil {
			continue
		}
		if newestPath == "" || info.ModTime().After(newestTime) {
			newestPath = videoPath
			newestDir = trimmed
			newestTime = info.ModTime()
		}
	}
	if newestPath == "" {
		return "", "", errors.New("no recent videos found in configured directories")
	}
	return newestPath, newestDir, nil
}

func OpenInExplorer(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	path = absPath
	if runtime.GOOS == "windows" {
		return exec.Command("explorer.exe", path).Start()
	}
	if _, err := exec.LookPath("wslpath"); err == nil {
		winPathBytes, err := exec.Command("wslpath", "-w", path).Output()
		if err != nil {
			return err
		}
		winPath := strings.TrimSpace(string(winPathBytes))
		if winPath == "" {
			return errors.New("failed to convert path for explorer")
		}
		return exec.Command("cmd.exe", "/C", "start", "", winPath).Start()
	}
	return exec.Command("xdg-open", path).Start()
}

func WriteDiagnosticsBundle(rootDir string, stage string, inputPath string, runPath string, logPath string, runErr error) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = DefaultFramesRoot
	}
	diagRoot := filepath.Join(rootDir, "diagnostics")
	if err := os.MkdirAll(diagRoot, 0o755); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	diagPath := filepath.Join(diagRoot, "diag-"+now.Format("20060102-150405")+".json")
	ffmpegVersion, _ := FFmpegVersion()
	whisperBin := strings.TrimSpace(os.Getenv("WHISPER_BIN"))
	if whisperBin == "" {
		whisperBin = "whisper"
	}
	payload := map[string]any{
		"generated_at": now.Format(time.RFC3339),
		"stage":        strings.TrimSpace(stage),
		"input_path":   strings.TrimSpace(inputPath),
		"run_path":     strings.TrimSpace(runPath),
		"log_path":     strings.TrimSpace(logPath),
		"error":        errorString(runErr),
		"runtime": map[string]any{
			"goos":   runtime.GOOS,
			"goarch": runtime.GOARCH,
		},
		"tools": map[string]any{
			"ffmpeg_version": ffmpegVersion,
			"whisper_bin":    whisperBin,
			"whisper_model":  strings.TrimSpace(os.Getenv("WHISPER_MODEL")),
			"whisper_lang":   strings.TrimSpace(os.Getenv("WHISPER_LANGUAGE")),
		},
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(diagPath, raw, 0o644); err != nil {
		return "", err
	}
	return diagPath, nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func makeRunFolderName(now time.Time) string {
	weekday := now.Weekday().String()
	hour := now.Hour()
	minute := now.Minute()
	ampm := "am"
	if hour >= 12 {
		ampm = "pm"
	}
	hour12 := hour % 12
	if hour12 == 0 {
		hour12 = 12
	}
	suffix := "000000"
	var buf [3]byte
	if _, err := rand.Read(buf[:]); err == nil {
		suffix = fmt.Sprintf("%x", buf)
	}
	return fmt.Sprintf("%s_%d-%02d%s-%s", weekday, hour12, minute, ampm, suffix)
}

func ensureUniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	for i := 2; i < 1000; i++ {
		alt := fmt.Sprintf("%s-%d", path, i)
		if _, err := os.Stat(alt); os.IsNotExist(err) {
			return alt
		}
	}
	return path
}

func normalizeFrameFormat(format string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" {
		f = "png"
	}
	switch f {
	case "png":
		return "png", nil
	case "jpg", "jpeg":
		return "jpg", nil
	default:
		return "", fmt.Errorf("unsupported frame format %q (use png or jpg)", format)
	}
}

func listFrameFiles(imagesDir string) ([]string, string, error) {
	type candidate struct {
		ext   string
		files []string
	}
	candidates := []candidate{{ext: "png"}, {ext: "jpg"}, {ext: "jpeg"}}

	best := candidate{}
	for i := range candidates {
		files, err := filepath.Glob(filepath.Join(imagesDir, "*."+candidates[i].ext))
		if err != nil {
			return nil, "", err
		}
		filtered := files[:0]
		for _, f := range files {
			base := strings.ToLower(filepath.Base(f))
			if strings.Contains(base, "contact-sheet") {
				continue
			}
			filtered = append(filtered, f)
		}
		files = filtered
		candidates[i].files = files
		if len(files) > len(best.files) {
			best = candidates[i]
		}
	}
	if len(best.files) == 0 {
		return nil, "", nil
	}
	sort.Strings(best.files)
	return best.files, best.ext, nil
}

func probeVideoDurationSeconds(videoPath string) (float64, error) {
	info, err := ProbeVideoInfo(videoPath)
	if err != nil {
		return 0, err
	}
	return info.DurationSec, nil
}

func ProbeVideoInfo(videoPath string) (VideoInfo, error) {
	absVideoPath, err := filepath.Abs(videoPath)
	if err != nil {
		return VideoInfo{}, err
	}
	videoPath = absVideoPath
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type,width,height,r_frame_rate",
		"-of", "json",
		videoPath,
	}
	out, err := exec.Command("ffprobe", args...).Output()
	if err != nil {
		return VideoInfo{}, err
	}
	var payload struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return VideoInfo{}, err
	}
	durationSec, _ := strconv.ParseFloat(strings.TrimSpace(payload.Format.Duration), 64)
	info := VideoInfo{DurationSec: durationSec}
	for _, s := range payload.Streams {
		switch s.CodecType {
		case "video":
			info.Width = s.Width
			info.Height = s.Height
			if fps, ok := parseRate(s.RFrameRate); ok {
				info.FrameRate = fps
			}
		case "audio":
			info.HasAudio = true
		}
	}
	if info.DurationSec <= 0 || info.Width <= 0 || info.Height <= 0 {
		return VideoInfo{}, errors.New("invalid or unsupported video stream")
	}
	return info, nil
}

func FFmpegVersion() (string, error) {
	out, err := exec.Command("ffmpeg", "-version").Output()
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	return line, nil
}

func FFmpegHWAccels() ([]string, error) {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-hwaccels").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	accels := make([]string, 0, 8)
	started := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "Hardware acceleration methods") {
			started = true
			continue
		}
		if !started {
			continue
		}
		accels = append(accels, strings.ToLower(trimmed))
	}
	return accels, nil
}

func normalizeHWAccel(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "none", "auto", "cuda", "vaapi", "qsv":
		if v == "" {
			return "none"
		}
		return v
	default:
		return "none"
	}
}

func normalizePreset(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "safe", "balanced", "fast":
		if v == "" {
			return "balanced"
		}
		return v
	default:
		return "balanced"
	}
}

func hwaccelArgs(mode string) []string {
	switch mode {
	case "none":
		return nil
	case "auto":
		return []string{"-hwaccel", "auto"}
	case "cuda":
		return []string{"-hwaccel", "cuda"}
	case "vaapi":
		return []string{"-hwaccel", "vaapi"}
	case "qsv":
		return []string{"-hwaccel", "qsv"}
	default:
		return nil
	}
}

func performanceArgs(preset string) []string {
	switch preset {
	case "safe":
		return []string{"-threads", "1"}
	case "balanced":
		return nil
	case "fast":
		return []string{"-threads", "0"}
	default:
		return nil
	}
}

func runFFmpeg(ctx context.Context, args []string, durationSec float64, progressFn func(percent float64), verbose bool, logWriter io.Writer) error {
	if verbose {
		cmd := exec.CommandContext(ctxOrBackground(ctx), "ffmpeg", args...)
		cmd.Stdout = os.Stdout
		if logWriter != nil {
			cmd.Stderr = io.MultiWriter(os.Stderr, logWriter)
		} else {
			cmd.Stderr = os.Stderr
		}
		return cmd.Run()
	}

	progressArgs := append([]string{"-hide_banner", "-loglevel", "error", "-nostats"}, args...)
	progressArgs = append(progressArgs, "-progress", "pipe:1")
	cmd := exec.CommandContext(ctxOrBackground(ctx), "ffmpeg", progressArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	if progressFn != nil {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "out_time_ms=") && durationSec > 0 {
				v := strings.TrimPrefix(line, "out_time_ms=")
				ms, parseErr := strconv.ParseFloat(v, 64)
				if parseErr == nil && ms >= 0 {
					percent := (ms / (durationSec * 1000000.0)) * 100.0
					if percent < 0 {
						percent = 0
					}
					if percent > 100 {
						percent = 100
					}
					progressFn(percent)
				}
			}
			if line == "progress=end" {
				progressFn(100)
			}
		}
	} else {
		_, _ = io.Copy(io.Discard, stdout)
	}

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if logWriter != nil && msg != "" {
			_, _ = io.WriteString(logWriter, msg+"\n")
		}
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func buildVideoFilter(fps float64, everyNFrames int, startFrame int, endFrame int) string {
	filters := make([]string, 0, 3)
	conds := make([]string, 0, 2)
	if everyNFrames > 1 {
		conds = append(conds, fmt.Sprintf("not(mod(n\\,%d))", everyNFrames))
	}
	if startFrame > 0 || endFrame > 0 {
		low := 0
		high := 2147483647
		if startFrame > 0 {
			low = startFrame
		}
		if endFrame > 0 {
			high = endFrame
		}
		conds = append(conds, fmt.Sprintf("between(n\\,%d\\,%d)", low, high))
	}
	if len(conds) > 0 {
		filters = append(filters, "select='"+strings.Join(conds, "*")+"'")
	}
	if fps > 0 {
		filters = append(filters, fmt.Sprintf("fps=%g", fps))
	}
	if len(filters) == 0 {
		return "fps=4"
	}
	return strings.Join(filters, ",")
}

func buildAudioOutputArgs(format string, bitrate string) []string {
	switch format {
	case "wav":
		return []string{"-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1"}
	case "mp3":
		return []string{"-acodec", "libmp3lame", "-b:a", bitrate}
	case "aac":
		return []string{"-acodec", "aac", "-b:a", bitrate}
	default:
		return []string{"-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1"}
	}
}

func normalizeAudioFormat(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "wav":
		return "wav", nil
	case "mp3":
		return "mp3", nil
	case "aac":
		return "aac", nil
	default:
		return "", fmt.Errorf("unsupported audio format %q (use wav|mp3|aac)", v)
	}
}

func parseRate(v string) (float64, bool) {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		if len(parts) != 2 {
			return 0, false
		}
		num, errNum := strconv.ParseFloat(parts[0], 64)
		den, errDen := strconv.ParseFloat(parts[1], 64)
		if errNum != nil || errDen != nil || den == 0 {
			return 0, false
		}
		return num / den, true
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil || val <= 0 {
		return 0, false
	}
	return val, true
}

func createLogWriter(path string) (io.Writer, func(), error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	_, _ = io.WriteString(f, fmt.Sprintf("[%s] extraction start\n", time.Now().UTC().Format(time.RFC3339)))
	return f, func() { _ = f.Close() }, nil
}

func buildFrameRecords(imagesDir string, opts ExtractMediaOptions, info VideoInfo) ([]FrameRecord, error) {
	frames, _, err := listFrameFiles(imagesDir)
	if err != nil {
		return nil, err
	}
	out := make([]FrameRecord, 0, len(frames))
	baseFPS := opts.FPS
	if opts.EveryNFrames > 1 && info.FrameRate > 0 {
		baseFPS = info.FrameRate / float64(opts.EveryNFrames)
	}
	if baseFPS <= 0 {
		baseFPS = opts.FPS
	}
	offset := parseDurationToSeconds(opts.StartTime)
	for i, framePath := range frames {
		idx := i + 1
		ts := offset
		if baseFPS > 0 {
			ts += float64(i) / baseFPS
		}
		out = append(out, FrameRecord{
			File:            filepath.Base(framePath),
			Index:           idx,
			TimestampSec:    ts,
			TimestampPretty: formatTimestamp(ts),
		})
	}
	return out, nil
}

func writeFrameRecordsCSV(path string, records []FrameRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"file", "index", "timestamp_sec", "timestamp"}); err != nil {
		return err
	}
	for _, r := range records {
		row := []string{
			r.File,
			strconv.Itoa(r.Index),
			strconv.FormatFloat(r.TimestampSec, 'f', 3, 64),
			r.TimestampPretty,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func zipDir(dir, outZip string) error {
	zf, err := os.Create(outZip)
	if err != nil {
		return err
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)
	defer zw.Close()
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	})
}

func parseDurationToSeconds(in string) float64 {
	in = strings.TrimSpace(in)
	if in == "" {
		return 0
	}
	if v, err := strconv.ParseFloat(in, 64); err == nil && v >= 0 {
		return v
	}
	parts := strings.Split(in, ":")
	if len(parts) == 2 || len(parts) == 3 {
		total := 0.0
		pow := 1.0
		for i := len(parts) - 1; i >= 0; i-- {
			v, err := strconv.ParseFloat(parts[i], 64)
			if err != nil || v < 0 {
				return 0
			}
			total += v * pow
			pow *= 60
		}
		return total
	}
	return 0
}

func formatTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalMs := int(math.Round(seconds * 1000))
	ms := totalMs % 1000
	totalSec := totalMs / 1000
	sec := totalSec % 60
	totalMin := totalSec / 60
	min := totalMin % 60
	hour := totalMin / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hour, min, sec, ms)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func formatSRTTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalMs := int(math.Round(seconds * 1000))
	ms := totalMs % 1000
	totalSec := totalMs / 1000
	sec := totalSec % 60
	totalMin := totalSec / 60
	min := totalMin % 60
	hour := totalMin / 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hour, min, sec, ms)
}

func formatVTTTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	totalMs := int(math.Round(seconds * 1000))
	ms := totalMs % 1000
	totalSec := totalMs / 1000
	sec := totalSec % 60
	totalMin := totalSec / 60
	min := totalMin % 60
	hour := totalMin / 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hour, min, sec, ms)
}

func writeSRT(path string, segments []WhisperSegment) error {
	var b strings.Builder
	idx := 1
	for _, seg := range segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		start := formatSRTTime(seg.Start)
		end := formatSRTTime(seg.End)
		b.WriteString(fmt.Sprintf("%d\n%s --> %s\n%s\n\n", idx, start, end, text))
		idx++
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeVTT(path string, segments []WhisperSegment) error {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, seg := range segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		start := formatVTTTime(seg.Start)
		end := formatVTTTime(seg.End)
		b.WriteString(fmt.Sprintf("%s --> %s\n%s\n\n", start, end, text))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
