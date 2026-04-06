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

type IndexedRun struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Frames          int      `json:"frames"`
	FrameExt        string   `json:"frame_ext"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	FPS             float64  `json:"fps,omitempty"`
	Format          string   `json:"format,omitempty"`
	DurationSec     float64  `json:"duration_sec,omitempty"`
	HasVoice        bool     `json:"has_voice"`
	HasTranscript   bool     `json:"has_transcript"`
	HasContactSheet bool     `json:"has_contact_sheet"`
	SegmentCount    int      `json:"segment_count"`
	Tags            []string `json:"tags,omitempty"`
}

type RunsIndex struct {
	Root        string       `json:"root"`
	GeneratedAt string       `json:"generated_at"`
	RunCount    int          `json:"run_count"`
	Runs        []IndexedRun `json:"runs"`
}

func IndexFilePath(rootDir string) string {
	return filepath.Join(rootDir, "index.json")
}

func BuildRunsIndex(rootDir string) (RunsIndex, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return RunsIndex{Root: rootDir, GeneratedAt: time.Now().UTC().Format(time.RFC3339), RunCount: 0, Runs: []IndexedRun{}}, nil
		}
		return RunsIndex{}, err
	}

	runs := make([]IndexedRun, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runPath := filepath.Join(rootDir, e.Name())
		imagesDir := filepath.Join(runPath, "images")
		frames, ext, updated := scanFrameStats(imagesDir)

		transcriptTxt := filepath.Join(runPath, "voice", "transcript.txt")
		_, hasTranscript := statOK(transcriptTxt)
		transcriptJSON := filepath.Join(runPath, "voice", "transcript.json")
		segments := readSegmentCount(transcriptJSON)
		voiceWav := filepath.Join(runPath, "voice", "voice.wav")
		_, hasVoice := statOK(voiceWav)

		contactSheet := filepath.Join(runPath, "images", "sheets", "contact-sheet.png")
		_, hasSheet := statOK(contactSheet)

		md, mdErr := ReadRunMetadata(runPath)
		var fps float64
		var format string
		var dur float64
		if mdErr == nil {
			fps = md.FPS
			format = md.FrameFormat
			dur = md.DurationSec
		}

		tags := make([]string, 0, 3)
		if hasVoice {
			tags = append(tags, "voice")
		}
		if hasTranscript {
			tags = append(tags, "transcript")
		}
		if hasSheet {
			tags = append(tags, "sheet")
		}

		runs = append(runs, IndexedRun{
			Name:            e.Name(),
			Path:            runPath,
			Frames:          frames,
			FrameExt:        ext,
			UpdatedAt:       updated,
			FPS:             fps,
			Format:          format,
			DurationSec:     dur,
			HasVoice:        hasVoice,
			HasTranscript:   hasTranscript,
			HasContactSheet: hasSheet,
			SegmentCount:    segments,
			Tags:            tags,
		})
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].UpdatedAt > runs[j].UpdatedAt
	})

	return RunsIndex{
		Root:        rootDir,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RunCount:    len(runs),
		Runs:        runs,
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
	return idx, nil
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

func statOK(path string) (os.FileInfo, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return info, true
}
