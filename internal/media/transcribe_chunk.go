package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	transcriptionManifestName = "transcription-manifest.json"

	transcriptionChunkPending    = "pending"
	transcriptionChunkInProgress = "in_progress"
	transcriptionChunkCompleted  = "completed"
	transcriptionChunkFailed     = "failed"

	transcriptionMergePending    = "pending"
	transcriptionMergeInProgress = "in_progress"
	transcriptionMergeCompleted  = "completed"
	transcriptionMergeFailed     = "failed"
)

type TranscribeResult struct {
	TranscriptTxt   string `json:"transcript_txt,omitempty"`
	TranscriptJSON  string `json:"transcript_json,omitempty"`
	ManifestPath    string `json:"manifest_path,omitempty"`
	Chunked         bool   `json:"chunked,omitempty"`
	TotalChunks     int    `json:"total_chunks,omitempty"`
	CompletedChunks int    `json:"completed_chunks,omitempty"`
}

type TranscriptionManifest struct {
	SchemaVersion    string                       `json:"schema_version"`
	SourceAudioPath  string                       `json:"source_audio_path"`
	ChunkDurationSec int                          `json:"chunk_duration_sec"`
	TotalDurationSec float64                      `json:"total_duration_sec"`
	ChunkRootDir     string                       `json:"chunk_root_dir"`
	CreatedAt        string                       `json:"created_at"`
	UpdatedAt        string                       `json:"updated_at"`
	Chunks           []TranscriptionManifestChunk `json:"chunks"`
	Merge            TranscriptionManifestMerge   `json:"merge"`
}

type TranscriptionManifestChunk struct {
	Index          int     `json:"index"`
	StartSec       float64 `json:"start_sec"`
	EndSec         float64 `json:"end_sec"`
	Status         string  `json:"status"`
	AudioPath      string  `json:"audio_path"`
	OutputDir      string  `json:"output_dir"`
	TranscriptTxt  string  `json:"transcript_txt,omitempty"`
	TranscriptJSON string  `json:"transcript_json,omitempty"`
	LastError      string  `json:"last_error,omitempty"`
}

type TranscriptionManifestMerge struct {
	Status         string `json:"status"`
	TranscriptTxt  string `json:"transcript_txt,omitempty"`
	TranscriptJSON string `json:"transcript_json,omitempty"`
	TranscriptSRT  string `json:"transcript_srt,omitempty"`
	TranscriptVTT  string `json:"transcript_vtt,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

var (
	probeTranscriptionDuration = probeMediaDurationSeconds
	splitTranscriptionChunk    = extractTranscriptionChunk
	transcribeSingleAudio      = transcribeAudioOnce
)

func TranscribeAudioWithDetails(opts TranscribeOptions) (TranscribeResult, error) {
	audioDurationSec := probeAudioDurationOrZero(opts.AudioPath)
	if shouldUseChunkedTranscription(opts, audioDurationSec) {
		return transcribeAudioChunked(opts)
	}
	stop := startTranscribeHeartbeat(opts.ProgressFn, "running", 0, 1, estimateTranscribeSeconds(audioDurationSec))
	txt, jsonPath, err := transcribeSingleAudio(opts)
	stop()
	if err != nil {
		return TranscribeResult{}, err
	}
	if opts.ProgressFn != nil {
		opts.ProgressFn("done", 1.0)
	}
	return TranscribeResult{
		TranscriptTxt:  txt,
		TranscriptJSON: jsonPath,
	}, nil
}

func probeAudioDurationOrZero(audioPath string) float64 {
	p := strings.TrimSpace(audioPath)
	if p == "" {
		return 0
	}
	d, err := probeTranscriptionDuration(p)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// 10% overhang so a clip slightly above one chunk (e.g. 630s @ 600s chunks)
// still goes single-shot rather than spawning a 30s second chunk.
const SingleShotChunkOverhangRatio = 1.10

// shouldUseChunkedTranscription: durationSec == 0 means "unknown" — falls back
// to the safe default (chunk when ChunkDurationSec > 0).
func shouldUseChunkedTranscription(opts TranscribeOptions, durationSec float64) bool {
	// Existing manifest always resumes chunked — protects interrupted long runs.
	if outDir := strings.TrimSpace(opts.OutDir); outDir != "" {
		if _, err := os.Stat(transcriptionManifestPath(outDir)); err == nil {
			return true
		}
	}
	if opts.ChunkDurationSec <= 0 {
		return false
	}
	if durationSec > 0 && durationSec <= float64(opts.ChunkDurationSec)*SingleShotChunkOverhangRatio {
		return false
	}
	return true
}

// Rough CPU-whisper-tiny realtime estimate (audio seconds × factor ≈ wall
// seconds). Kept conservative so pct interpolation leans "a bit slow" rather
// than racing past 100% mid-chunk. The heartbeat caps interpolation at 0.9 so
// under-estimate is safer than over-estimate.
const transcribeExpectedRealtimeFactor = 1.5

func estimateTranscribeSeconds(audioSec float64) float64 {
	if audioSec <= 0 {
		return 60
	}
	expected := audioSec * transcribeExpectedRealtimeFactor
	if expected < 30 {
		return 30
	}
	return expected
}

func formatTranscribeElapsed(d time.Duration) string {
	total := int(d.Seconds())
	if total < 0 {
		total = 0
	}
	if total < 3600 {
		return fmt.Sprintf("%02d:%02d", total/60, total%60)
	}
	return fmt.Sprintf("%02d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}

// startTranscribeHeartbeat spawns a goroutine that calls progressFn every
// second with an interpolated pct inside [base, base+span] and a stage string
// augmented with elapsed time. The interpolation caps at 0.9 of span so the
// bar never claims completion before the work returns. Returns a stop func
// that must be called to release the goroutine.
func startTranscribeHeartbeat(progressFn func(string, float64), stage string, base, span, expectedSec float64) func() {
	if progressFn == nil {
		return func() {}
	}
	start := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				frac := 0.0
				if expectedSec > 0 {
					frac = elapsed.Seconds() / expectedSec
					if frac > 0.9 {
						frac = 0.9
					}
				}
				progressFn(
					fmt.Sprintf("%s · %s elapsed", stage, formatTranscribeElapsed(elapsed)),
					base+span*frac,
				)
			}
		}
	}()
	return func() { close(done) }
}

func transcriptionManifestPath(outDir string) string {
	return filepath.Join(outDir, transcriptionManifestName)
}

func loadTranscriptionManifest(path string) (*TranscriptionManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest TranscriptionManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func IsTranscriptionManifestComplete(path string) (bool, error) {
	manifest, err := loadTranscriptionManifest(path)
	if err != nil {
		return false, err
	}
	if manifest.Merge.Status != transcriptionMergeCompleted {
		return false, nil
	}
	if strings.TrimSpace(manifest.Merge.TranscriptJSON) == "" || strings.TrimSpace(manifest.Merge.TranscriptTxt) == "" {
		return false, nil
	}
	if _, err := os.Stat(manifest.Merge.TranscriptJSON); err != nil {
		return false, nil
	}
	if _, err := os.Stat(manifest.Merge.TranscriptTxt); err != nil {
		return false, nil
	}
	for _, chunk := range manifest.Chunks {
		if chunk.Status != transcriptionChunkCompleted || !chunkOutputsExist(chunk) {
			return false, nil
		}
	}
	return true, nil
}

func saveTranscriptionManifest(path string, manifest *TranscriptionManifest) error {
	manifest.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func transcribeAudioChunked(opts TranscribeOptions) (TranscribeResult, error) {
	audioPath := strings.TrimSpace(opts.AudioPath)
	outDir := strings.TrimSpace(opts.OutDir)
	if audioPath == "" {
		return TranscribeResult{}, errors.New("audio path is required")
	}
	if outDir == "" {
		return TranscribeResult{}, errors.New("output directory is required")
	}
	audioPath, err := filepath.Abs(audioPath)
	if err != nil {
		return TranscribeResult{}, err
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return TranscribeResult{}, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return TranscribeResult{}, err
	}

	manifestPath := transcriptionManifestPath(outDir)
	manifest, err := ensureTranscriptionManifest(manifestPath, audioPath, outDir, opts.ChunkDurationSec)
	if err != nil {
		return TranscribeResult{}, err
	}

	runCtx := ctxOrBackground(opts.Context)
	var cancel context.CancelFunc
	if opts.TimeoutSec > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(opts.TimeoutSec)*time.Second)
		defer cancel()
	}

	totalChunks := len(manifest.Chunks)
	for i := range manifest.Chunks {
		chunk := &manifest.Chunks[i]
		if chunk.Status == transcriptionChunkCompleted && chunkOutputsExist(*chunk) {
			if opts.ProgressFn != nil {
				opts.ProgressFn(fmt.Sprintf("chunk %d/%d (cached)", i+1, totalChunks), float64(i+1)/float64(totalChunks))
			}
			continue
		}
		if chunk.Status == transcriptionChunkCompleted && !chunkOutputsExist(*chunk) {
			chunk.Status = transcriptionChunkPending
			chunk.LastError = "completed chunk outputs missing on disk; retrying"
		}

		chunk.Status = transcriptionChunkInProgress
		chunk.LastError = ""
		if opts.ProgressFn != nil {
			opts.ProgressFn(fmt.Sprintf("chunk %d/%d", i+1, totalChunks), float64(i)/float64(totalChunks))
		}
		if err := os.MkdirAll(chunk.OutputDir, 0o755); err != nil {
			return transcriptionResultFromManifest(manifestPath, manifest), wrapTranscriptionManifestError(err, manifestPath)
		}
		if err := saveTranscriptionManifest(manifestPath, manifest); err != nil {
			return transcriptionResultFromManifest(manifestPath, manifest), err
		}

		chunkDuration := chunk.EndSec - chunk.StartSec
		if err := splitTranscriptionChunk(runCtx, manifest.SourceAudioPath, chunk.AudioPath, chunk.StartSec, chunkDuration); err != nil {
			chunk.LastError = err.Error()
			if !isCancellationLike(err) {
				chunk.Status = transcriptionChunkFailed
			}
			_ = saveTranscriptionManifest(manifestPath, manifest)
			return transcriptionResultFromManifest(manifestPath, manifest), wrapTranscriptionManifestError(err, manifestPath)
		}

		stopHeartbeat := startTranscribeHeartbeat(
			opts.ProgressFn,
			fmt.Sprintf("chunk %d/%d", i+1, totalChunks),
			float64(i)/float64(totalChunks),
			1.0/float64(totalChunks),
			estimateTranscribeSeconds(chunkDuration),
		)
		chunkTxt, chunkJSON, err := transcribeSingleAudio(TranscribeOptions{
			Context:    runCtx,
			AudioPath:  chunk.AudioPath,
			OutDir:     chunk.OutputDir,
			Model:      opts.Model,
			Language:   opts.Language,
			Bin:        opts.Bin,
			Backend:    opts.Backend,
			Verbose:    opts.Verbose,
			TimeoutSec: opts.TimeoutSec,
		})
		stopHeartbeat()
		if err != nil {
			chunk.LastError = err.Error()
			if !isCancellationLike(err) {
				chunk.Status = transcriptionChunkFailed
			}
			_ = saveTranscriptionManifest(manifestPath, manifest)
			return transcriptionResultFromManifest(manifestPath, manifest), wrapTranscriptionManifestError(err, manifestPath)
		}

		chunk.Status = transcriptionChunkCompleted
		chunk.TranscriptTxt = chunkTxt
		chunk.TranscriptJSON = chunkJSON
		chunk.LastError = ""
		if err := saveTranscriptionManifest(manifestPath, manifest); err != nil {
			return transcriptionResultFromManifest(manifestPath, manifest), err
		}
	}

	manifest.Merge.Status = transcriptionMergeInProgress
	manifest.Merge.LastError = ""
	if err := saveTranscriptionManifest(manifestPath, manifest); err != nil {
		return transcriptionResultFromManifest(manifestPath, manifest), err
	}

	merged, err := mergeChunkTranscripts(outDir, manifest)
	if err != nil {
		manifest.Merge.Status = transcriptionMergeFailed
		manifest.Merge.LastError = err.Error()
		_ = saveTranscriptionManifest(manifestPath, manifest)
		return transcriptionResultFromManifest(manifestPath, manifest), wrapTranscriptionManifestError(err, manifestPath)
	}

	manifest.Merge.Status = transcriptionMergeCompleted
	manifest.Merge.TranscriptTxt = merged.TranscriptTxt
	manifest.Merge.TranscriptJSON = merged.TranscriptJSON
	manifest.Merge.TranscriptSRT = filepath.Join(outDir, "transcript.srt")
	if _, err := os.Stat(manifest.Merge.TranscriptSRT); err != nil {
		manifest.Merge.TranscriptSRT = ""
	}
	manifest.Merge.TranscriptVTT = filepath.Join(outDir, "transcript.vtt")
	if _, err := os.Stat(manifest.Merge.TranscriptVTT); err != nil {
		manifest.Merge.TranscriptVTT = ""
	}
	manifest.Merge.LastError = ""
	if err := saveTranscriptionManifest(manifestPath, manifest); err != nil {
		return transcriptionResultFromManifest(manifestPath, manifest), err
	}

	result := transcriptionResultFromManifest(manifestPath, manifest)
	result.TranscriptTxt = merged.TranscriptTxt
	result.TranscriptJSON = merged.TranscriptJSON
	return result, nil
}

func ensureTranscriptionManifest(path, audioPath, outDir string, chunkDurationSec int) (*TranscriptionManifest, error) {
	if _, err := os.Stat(path); err == nil {
		manifest, err := loadTranscriptionManifest(path)
		if err != nil {
			return nil, err
		}
		if manifest.SourceAudioPath != audioPath {
			return nil, fmt.Errorf("existing transcription manifest points to %q, not %q", manifest.SourceAudioPath, audioPath)
		}
		if chunkDurationSec > 0 && manifest.ChunkDurationSec != chunkDurationSec {
			return nil, fmt.Errorf("existing transcription manifest uses --chunk-duration %d; requested %d", manifest.ChunkDurationSec, chunkDurationSec)
		}
		return manifest, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if chunkDurationSec <= 0 {
		return nil, errors.New("chunk duration must be > 0 when creating a transcription manifest")
	}
	durationSec, err := probeTranscriptionDuration(audioPath)
	if err != nil {
		return nil, err
	}
	if durationSec <= 0 {
		return nil, errors.New("audio duration must be > 0 for chunked transcription")
	}

	chunkRoot := filepath.Join(outDir, "chunks")
	if err := os.MkdirAll(chunkRoot, 0o755); err != nil {
		return nil, err
	}
	manifest := &TranscriptionManifest{
		SchemaVersion:    "framescli.transcription_manifest.v1",
		SourceAudioPath:  audioPath,
		ChunkDurationSec: chunkDurationSec,
		TotalDurationSec: durationSec,
		ChunkRootDir:     chunkRoot,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		Chunks:           buildTranscriptionManifestChunks(chunkRoot, durationSec, chunkDurationSec),
		Merge: TranscriptionManifestMerge{
			Status: transcriptionMergePending,
		},
	}
	if err := saveTranscriptionManifest(path, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func buildTranscriptionManifestChunks(chunkRoot string, totalDurationSec float64, chunkDurationSec int) []TranscriptionManifestChunk {
	totalChunks := int(math.Ceil(totalDurationSec / float64(chunkDurationSec)))
	if totalChunks < 1 {
		totalChunks = 1
	}
	chunks := make([]TranscriptionManifestChunk, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		startSec := float64(i * chunkDurationSec)
		endSec := math.Min(totalDurationSec, startSec+float64(chunkDurationSec))
		outputDir := filepath.Join(chunkRoot, fmt.Sprintf("chunk-%04d", i))
		chunks = append(chunks, TranscriptionManifestChunk{
			Index:     i,
			StartSec:  startSec,
			EndSec:    endSec,
			Status:    transcriptionChunkPending,
			AudioPath: filepath.Join(outputDir, "audio.wav"),
			OutputDir: outputDir,
		})
	}
	return chunks
}

func transcriptionResultFromManifest(manifestPath string, manifest *TranscriptionManifest) TranscribeResult {
	result := TranscribeResult{
		ManifestPath: manifestPath,
		Chunked:      true,
	}
	if manifest == nil {
		return result
	}
	result.TotalChunks = len(manifest.Chunks)
	for _, chunk := range manifest.Chunks {
		if chunk.Status == transcriptionChunkCompleted && chunkOutputsExist(chunk) {
			result.CompletedChunks++
		}
	}
	result.TranscriptTxt = manifest.Merge.TranscriptTxt
	result.TranscriptJSON = manifest.Merge.TranscriptJSON
	return result
}

func chunkOutputsExist(chunk TranscriptionManifestChunk) bool {
	if strings.TrimSpace(chunk.TranscriptJSON) == "" || strings.TrimSpace(chunk.TranscriptTxt) == "" {
		return false
	}
	if _, err := os.Stat(chunk.TranscriptJSON); err != nil {
		return false
	}
	if _, err := os.Stat(chunk.TranscriptTxt); err != nil {
		return false
	}
	return true
}

func mergeChunkTranscripts(outDir string, manifest *TranscriptionManifest) (TranscribeResult, error) {
	textParts := make([]string, 0, len(manifest.Chunks))
	segments := make([]WhisperSegment, 0, len(manifest.Chunks)*4)

	for _, chunk := range manifest.Chunks {
		if chunk.Status != transcriptionChunkCompleted || !chunkOutputsExist(chunk) {
			return TranscribeResult{}, fmt.Errorf("chunk %d is not completed", chunk.Index)
		}

		chunkText, chunkSegments, err := readChunkTranscript(chunk)
		if err != nil {
			return TranscribeResult{}, err
		}
		if chunkText != "" {
			textParts = append(textParts, chunkText)
		}
		for _, seg := range chunkSegments {
			segments = append(segments, WhisperSegment{
				Start: seg.Start + chunk.StartSec,
				End:   seg.End + chunk.StartSec,
				Text:  seg.Text,
			})
		}
	}

	finalText := strings.Join(textParts, "\n")
	finalJSONPath := filepath.Join(outDir, "transcript.json")
	finalTxtPath := filepath.Join(outDir, "transcript.txt")
	payload := WhisperJSON{
		Text:     finalText,
		Segments: segments,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return TranscribeResult{}, err
	}
	if err := os.WriteFile(finalJSONPath, raw, 0o644); err != nil {
		return TranscribeResult{}, err
	}
	if err := os.WriteFile(finalTxtPath, []byte(finalText+"\n"), 0o644); err != nil {
		return TranscribeResult{}, err
	}
	srtPath := filepath.Join(outDir, "transcript.srt")
	vttPath := filepath.Join(outDir, "transcript.vtt")
	if len(segments) > 0 {
		if err := writeSRT(srtPath, segments); err != nil {
			return TranscribeResult{}, err
		}
		if err := writeVTT(vttPath, segments); err != nil {
			return TranscribeResult{}, err
		}
	} else {
		_ = os.Remove(srtPath)
		_ = os.Remove(vttPath)
	}
	return TranscribeResult{
		TranscriptTxt:  finalTxtPath,
		TranscriptJSON: finalJSONPath,
	}, nil
}

func readChunkTranscript(chunk TranscriptionManifestChunk) (string, []WhisperSegment, error) {
	raw, err := os.ReadFile(chunk.TranscriptJSON)
	if err != nil {
		return "", nil, err
	}
	var parsed WhisperJSON
	if err := json.Unmarshal(raw, &parsed); err == nil {
		return strings.TrimSpace(parsed.Text), parsed.Segments, nil
	}
	txtRaw, err := os.ReadFile(chunk.TranscriptTxt)
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(string(txtRaw)), nil, nil
}

func extractTranscriptionChunk(ctx context.Context, sourceAudioPath, outPath string, startSec, durationSec float64) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-ss", strconv.FormatFloat(startSec, 'f', 3, 64),
		"-i", sourceAudioPath,
		"-t", strconv.FormatFloat(durationSec, 'f', 3, 64),
		"-vn",
		"-acodec", "pcm_s16le",
		outPath,
	}
	cmd := exec.CommandContext(ctxOrBackground(ctx), "ffmpeg", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("ffmpeg chunk extraction failed: %w", err)
		}
		return fmt.Errorf("ffmpeg chunk extraction failed: %w: %s", err, msg)
	}
	return nil
}

func probeMediaDurationSeconds(mediaPath string) (float64, error) {
	absMediaPath, err := filepath.Abs(mediaPath)
	if err != nil {
		return 0, err
	}
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		absMediaPath,
	).Output()
	if err != nil {
		return 0, err
	}
	durationSec, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, err
	}
	return durationSec, nil
}

func wrapTranscriptionManifestError(err error, manifestPath string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (manifest: %s)", err, manifestPath)
}

func isCancellationLike(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(strings.ToLower(err.Error()), "timed out")
}
