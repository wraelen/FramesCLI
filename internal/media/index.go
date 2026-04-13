package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const runsIndexSchemaVersion = "framescli.run_index.v2"

type RunArtifacts struct {
	Run                   string `json:"run"`
	Metadata              string `json:"metadata,omitempty"`
	Frames                string `json:"frames,omitempty"`
	FramesZip             string `json:"frames_zip,omitempty"`
	MetadataCSV           string `json:"metadata_csv,omitempty"`
	ContactSheet          string `json:"contact_sheet,omitempty"`
	Audio                 string `json:"audio,omitempty"`
	TranscriptTxt         string `json:"transcript_txt,omitempty"`
	TranscriptJSON        string `json:"transcript_json,omitempty"`
	TranscriptSRT         string `json:"transcript_srt,omitempty"`
	TranscriptVTT         string `json:"transcript_vtt,omitempty"`
	TranscriptionManifest string `json:"transcription_manifest,omitempty"`
	Log                   string `json:"log,omitempty"`
}

type IndexedRun struct {
	Name              string       `json:"name"`
	Path              string       `json:"path"`
	CreatedAt         string       `json:"created_at,omitempty"`
	UpdatedAt         string       `json:"updated_at,omitempty"`
	SourceVideoPath   string       `json:"source_video_path,omitempty"`
	Frames            int          `json:"frames"`
	FrameExt          string       `json:"frame_ext,omitempty"`
	FPS               float64      `json:"fps,omitempty"`
	Format            string       `json:"format,omitempty"`
	Preset            string       `json:"preset,omitempty"`
	DurationSec       float64      `json:"duration_sec,omitempty"`
	HasVoice          bool         `json:"has_voice"`
	HasTranscript     bool         `json:"has_transcript"`
	HasContactSheet   bool         `json:"has_contact_sheet"`
	SegmentCount      int          `json:"segment_count"`
	ChunkedTranscript bool         `json:"chunked_transcript,omitempty"`
	ManifestComplete  bool         `json:"manifest_complete,omitempty"`
	TotalChunks       int          `json:"total_chunks,omitempty"`
	CompletedChunks   int          `json:"completed_chunks,omitempty"`
	Status            string       `json:"status"`
	Warnings          []string     `json:"warnings,omitempty"`
	Tags              []string     `json:"tags,omitempty"`
	Artifacts         RunArtifacts `json:"artifacts"`
}

type RunsIndex struct {
	SchemaVersion string       `json:"schema_version"`
	Root          string       `json:"root"`
	GeneratedAt   string       `json:"generated_at"`
	RunCount      int          `json:"run_count"`
	Runs          []IndexedRun `json:"runs"`
}

func IndexFilePath(rootDir string) string {
	return filepath.Join(rootDir, "index.json")
}

func BuildRunsIndex(rootDir string) (RunsIndex, error) {
	rootDir = strings.TrimSpace(rootDir)
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyRunsIndex(rootDir), nil
		}
		return RunsIndex{}, err
	}

	runs := make([]IndexedRun, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runPath := filepath.Join(rootDir, e.Name())
		run, ok := buildIndexedRun(runPath, e.Name())
		if !ok {
			continue
		}
		runs = append(runs, run)
	}

	sort.Slice(runs, func(i, j int) bool {
		left := runs[i].CreatedAt
		if left == "" {
			left = runs[i].UpdatedAt
		}
		right := runs[j].CreatedAt
		if right == "" {
			right = runs[j].UpdatedAt
		}
		if left == right {
			return runs[i].Name > runs[j].Name
		}
		return left > right
	})

	return RunsIndex{
		SchemaVersion: runsIndexSchemaVersion,
		Root:          rootDir,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		RunCount:      len(runs),
		Runs:          runs,
	}, nil
}

func WriteRunsIndex(rootDir, outPath string) (RunsIndex, error) {
	index, err := BuildRunsIndex(rootDir)
	if err != nil {
		return RunsIndex{}, err
	}
	if outPath == "" {
		outPath = IndexFilePath(rootDir)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return RunsIndex{}, err
	}
	raw, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return RunsIndex{}, fmt.Errorf("marshal index: %w", err)
	}
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return RunsIndex{}, err
	}
	return index, nil
}

func ReadRunsIndex(path string) (RunsIndex, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunsIndex{}, err
	}
	var idx RunsIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return RunsIndex{}, err
	}
	if idx.Runs == nil {
		idx.Runs = []IndexedRun{}
	}
	if idx.SchemaVersion == "" {
		idx.SchemaVersion = runsIndexSchemaVersion
	}
	return idx, nil
}

func LoadOrCreateRunsIndex(rootDir string) (RunsIndex, error) {
	path := IndexFilePath(rootDir)
	idx, err := ReadRunsIndex(path)
	if err == nil {
		if idx.Root == "" {
			idx.Root = rootDir
		}
		return idx, nil
	}
	if !os.IsNotExist(err) {
		return WriteRunsIndex(rootDir, path)
	}
	return WriteRunsIndex(rootDir, path)
}

func RefreshRunsIndex(rootDir string) (RunsIndex, error) {
	return WriteRunsIndex(rootDir, IndexFilePath(rootDir))
}

func RecentRuns(rootDir string, limit int) ([]IndexedRun, error) {
	idx, err := LoadOrCreateRunsIndex(rootDir)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit >= len(idx.Runs) {
		return idx.Runs, nil
	}
	return idx.Runs[:limit], nil
}

func SelectRun(rootDir string, selector string) (IndexedRun, error) {
	idx, err := LoadOrCreateRunsIndex(rootDir)
	if err != nil {
		return IndexedRun{}, err
	}
	return selectRunFromIndex(idx, selector)
}

func buildIndexedRun(runPath string, name string) (IndexedRun, bool) {
	metadataPath := MetadataPathForRun(runPath)
	framesPath := filepath.Join(runPath, "frames.json")
	logPath := filepath.Join(runPath, "extract.log")
	audioCandidates, _ := filepath.Glob(filepath.Join(runPath, "voice", "voice.*"))
	audioPath := firstExistingPath(audioCandidates...)
	manifestPath := filepath.Join(runPath, "voice", "transcription-manifest.json")

	artifacts := RunArtifacts{
		Run:                   runPath,
		Metadata:              existingPath(metadataPath),
		Frames:                existingPath(framesPath),
		FramesZip:             existingPath(filepath.Join(runPath, "frames.zip")),
		ContactSheet:          existingPath(filepath.Join(runPath, "images", "sheets", "contact-sheet.png")),
		Audio:                 audioPath,
		TranscriptTxt:         existingPath(filepath.Join(runPath, "voice", "transcript.txt")),
		TranscriptJSON:        existingPath(filepath.Join(runPath, "voice", "transcript.json")),
		TranscriptSRT:         existingPath(filepath.Join(runPath, "voice", "transcript.srt")),
		TranscriptVTT:         existingPath(filepath.Join(runPath, "voice", "transcript.vtt")),
		TranscriptionManifest: existingPath(manifestPath),
		Log:                   existingPath(logPath),
		MetadataCSV:           existingPath(filepath.Join(runPath, "frames.csv")),
	}

	imagesDir := filepath.Join(runPath, "images")
	frames, ext, frameUpdated := scanFrameStats(imagesDir)
	segments := readSegmentCount(artifacts.TranscriptJSON)
	hasVoice := artifacts.Audio != ""
	hasTranscript := artifacts.TranscriptTxt != "" || artifacts.TranscriptJSON != ""
	hasSheet := artifacts.ContactSheet != ""

	run := IndexedRun{
		Name:            name,
		Path:            runPath,
		Frames:          frames,
		FrameExt:        ext,
		UpdatedAt:       frameUpdated,
		HasVoice:        hasVoice,
		HasTranscript:   hasTranscript,
		HasContactSheet: hasSheet,
		SegmentCount:    segments,
		Artifacts:       artifacts,
		Status:          "partial",
	}

	md, mdErr := ReadRunMetadata(runPath)
	if mdErr == nil {
		run.CreatedAt = md.CreatedAt
		run.SourceVideoPath = md.VideoPath
		run.FPS = md.FPS
		run.Format = md.FrameFormat
		run.Preset = md.Preset
		run.DurationSec = md.DurationSec
		if artifacts.FramesZip == "" && strings.TrimSpace(md.FramesZipPath) != "" {
			artifacts.FramesZip = existingPath(md.FramesZipPath, filepath.Join(runPath, filepath.Base(md.FramesZipPath)))
			run.Artifacts = artifacts
		}
	}

	if run.CreatedAt == "" {
		run.CreatedAt = newestTimestamp(runPath, metadataPath, framesPath, artifacts.TranscriptTxt, artifacts.TranscriptJSON, artifacts.Audio, artifacts.ContactSheet, artifacts.Log, artifacts.TranscriptionManifest)
	}
	run.UpdatedAt = newestTimestamp(runPath, metadataPath, framesPath, artifacts.TranscriptTxt, artifacts.TranscriptJSON, artifacts.TranscriptSRT, artifacts.TranscriptVTT, artifacts.Audio, artifacts.ContactSheet, artifacts.Log, artifacts.TranscriptionManifest)

	if run.UpdatedAt == "" && run.CreatedAt == "" {
		return IndexedRun{}, false
	}

	if artifacts.TranscriptionManifest != "" {
		run.ChunkedTranscript = true
		if manifest, err := loadTranscriptionManifest(artifacts.TranscriptionManifest); err == nil {
			run.TotalChunks = len(manifest.Chunks)
			for _, chunk := range manifest.Chunks {
				if chunk.Status == transcriptionChunkCompleted && chunkOutputsExist(chunk) {
					run.CompletedChunks++
				}
			}
			run.ManifestComplete = run.TotalChunks > 0 && run.CompletedChunks == run.TotalChunks
		} else if complete, err := IsTranscriptionManifestComplete(artifacts.TranscriptionManifest); err == nil {
			run.ManifestComplete = complete
		}
	}

	run.Warnings = buildIndexedRunWarnings(run, md)
	run.Tags = buildIndexedRunTags(run)
	run.Status = buildIndexedRunStatus(run, md)
	return run, true
}

func buildIndexedRunWarnings(run IndexedRun, md RunMetadata) []string {
	warnings := make([]string, 0, 4)
	if md.FallbackUsed {
		warnings = append(warnings, "hwaccel_fallback")
	}
	if md.ExtractedAudio && run.Artifacts.Audio == "" {
		warnings = append(warnings, "audio_missing")
	}
	if md.ExtractedAudio && !run.HasTranscript {
		warnings = append(warnings, "transcript_missing")
	}
	if run.ChunkedTranscript && !run.ManifestComplete {
		warnings = append(warnings, "transcription_manifest_incomplete")
	}
	if run.Frames > 0 && !run.HasContactSheet {
		warnings = append(warnings, "contact_sheet_missing")
	}
	return warnings
}

func buildIndexedRunTags(run IndexedRun) []string {
	tags := make([]string, 0, 6)
	if run.HasVoice {
		tags = append(tags, "voice")
	}
	if run.HasTranscript {
		tags = append(tags, "transcript")
	}
	if run.HasContactSheet {
		tags = append(tags, "sheet")
	}
	if run.ChunkedTranscript {
		tags = append(tags, "chunked")
	}
	if len(run.Warnings) > 0 {
		tags = append(tags, "warning")
	}
	return tags
}

func buildIndexedRunStatus(run IndexedRun, md RunMetadata) string {
	if run.Artifacts.Metadata == "" {
		return "partial"
	}
	if len(run.Warnings) > 0 {
		return "partial"
	}
	if md.ExtractedAudio && (!run.HasVoice || !run.HasTranscript) {
		return "partial"
	}
	if run.Frames > 0 || run.HasVoice || run.HasContactSheet {
		return "complete"
	}
	return "partial"
}

func selectRunFromIndex(idx RunsIndex, selector string) (IndexedRun, error) {
	if len(idx.Runs) == 0 {
		return IndexedRun{}, fmt.Errorf("no runs found in %s", idx.Root)
	}
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.EqualFold(selector, "latest") {
		return idx.Runs[0], nil
	}

	if absSelector, err := filepath.Abs(selector); err == nil {
		for _, run := range idx.Runs {
			if samePath(run.Path, absSelector) {
				return run, nil
			}
		}
	}

	for _, run := range idx.Runs {
		if run.Name == selector || filepath.Base(run.Path) == selector {
			return run, nil
		}
	}
	return IndexedRun{}, fmt.Errorf("run %q not found in %s", selector, idx.Root)
}

func scanFrameStats(imagesDir string) (int, string, string) {
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return 0, "", ""
	}
	count := 0
	ext := ""
	newest := time.Time{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "frame-") {
			continue
		}
		count++
		if ext == "" {
			ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(e.Name())), ".")
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	updated := ""
	if !newest.IsZero() {
		updated = newest.UTC().Format(time.RFC3339)
	}
	return count, ext, updated
}

func readSegmentCount(transcriptJSON string) int {
	raw, err := os.ReadFile(transcriptJSON)
	if err != nil {
		return 0
	}
	var payload WhisperJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0
	}
	count := 0
	for _, seg := range payload.Segments {
		if strings.TrimSpace(seg.Text) != "" {
			count++
		}
	}
	return count
}

func existingPath(candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func firstExistingPath(candidates ...string) string {
	return existingPath(candidates...)
}

func newestTimestamp(paths ...string) string {
	var newest time.Time
	for _, p := range paths {
		info, ok := statOK(p)
		if !ok {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339)
}

func emptyRunsIndex(rootDir string) RunsIndex {
	return RunsIndex{
		SchemaVersion: runsIndexSchemaVersion,
		Root:          rootDir,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		RunCount:      0,
		Runs:          []IndexedRun{},
	}
}

func samePath(left string, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	return left != "" && right != "" && left == right
}

func statOK(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}
