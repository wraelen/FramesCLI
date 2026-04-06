package media

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type BenchmarkCase struct {
	HWAccel string `json:"hwaccel"`
	Preset  string `json:"preset"`
}

type BenchmarkResult struct {
	Case           BenchmarkCase `json:"case"`
	Success        bool          `json:"success"`
	ElapsedMs      int64         `json:"elapsed_ms"`
	RealtimeFactor float64       `json:"realtime_factor"`
	Error          string        `json:"error,omitempty"`
}

type BenchmarkOptions struct {
	VideoPath   string
	DurationSec int
	Cases       []BenchmarkCase
	Verbose     bool
}

type BenchmarkReport struct {
	GeneratedAt           string            `json:"generated_at"`
	VideoPath             string            `json:"video_path"`
	DurationSec           int               `json:"duration_sec"`
	Results               []BenchmarkResult `json:"results"`
	Recommended           BenchmarkCase     `json:"recommended"`
	RecommendationMessage string            `json:"recommendation_message"`
}

type BenchmarkHistoryEntry struct {
	RecordedAt            string            `json:"recorded_at"`
	VideoPath             string            `json:"video_path"`
	DurationSec           int               `json:"duration_sec"`
	Results               []BenchmarkResult `json:"results"`
	Recommended           BenchmarkCase     `json:"recommended"`
	RecommendationMessage string            `json:"recommendation_message"`
}

func BenchmarkExtraction(opts BenchmarkOptions) (*BenchmarkReport, error) {
	if strings.TrimSpace(opts.VideoPath) == "" {
		return nil, fmt.Errorf("video path is required")
	}
	if opts.DurationSec <= 0 {
		opts.DurationSec = 20
	}
	if len(opts.Cases) == 0 {
		return nil, fmt.Errorf("at least one benchmark case is required")
	}

	videoPath := NormalizeVideoPath(opts.VideoPath)
	results := make([]BenchmarkResult, 0, len(opts.Cases))
	for _, c := range opts.Cases {
		normCase := BenchmarkCase{HWAccel: normalizeHWAccel(c.HWAccel), Preset: normalizePreset(c.Preset)}
		start := time.Now()
		err := runBenchmarkCase(videoPath, opts.DurationSec, normCase, opts.Verbose)
		elapsed := time.Since(start)
		res := BenchmarkResult{
			Case:           normCase,
			Success:        err == nil,
			ElapsedMs:      elapsed.Milliseconds(),
			RealtimeFactor: float64(opts.DurationSec) / elapsed.Seconds(),
		}
		if err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}

	recommended, ok := chooseRecommended(results)
	message := "No successful benchmark cases; stick with hwaccel=none and verify ffmpeg tooling."
	if ok {
		message = fmt.Sprintf("Use hwaccel=%s preset=%s (%.2fx realtime).", recommended.Case.HWAccel, recommended.Case.Preset, recommended.RealtimeFactor)
	}

	return &BenchmarkReport{
		GeneratedAt:           time.Now().UTC().Format(time.RFC3339),
		VideoPath:             opts.VideoPath,
		DurationSec:           opts.DurationSec,
		Results:               results,
		Recommended:           recommended.Case,
		RecommendationMessage: message,
	}, nil
}

func BenchmarkHistoryPath(rootDir string) string {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = DefaultFramesRoot
	}
	return filepath.Join(rootDir, "benchmark-history.json")
}

func AppendBenchmarkHistory(path string, report BenchmarkReport, maxEntries int) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("history path is required")
	}
	if maxEntries <= 0 {
		maxEntries = 100
	}
	entries, _ := ReadBenchmarkHistory(path, 0)
	entry := BenchmarkHistoryEntry{
		RecordedAt:            time.Now().UTC().Format(time.RFC3339),
		VideoPath:             report.VideoPath,
		DurationSec:           report.DurationSec,
		Results:               report.Results,
		Recommended:           report.Recommended,
		RecommendationMessage: report.RecommendationMessage,
	}
	entries = append([]BenchmarkHistoryEntry{entry}, entries...)
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func ReadBenchmarkHistory(path string, limit int) ([]BenchmarkHistoryEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []BenchmarkHistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	if limit > 0 && len(entries) > limit {
		return entries[:limit], nil
	}
	return entries, nil
}

func WriteBenchmarkHistory(path string, entries []BenchmarkHistoryEntry) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("history path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func PruneBenchmarkHistory(path string, keep int) (int, error) {
	if keep <= 0 {
		return 0, fmt.Errorf("keep must be > 0")
	}
	entries, err := ReadBenchmarkHistory(path, 0)
	if err != nil {
		return 0, err
	}
	if len(entries) <= keep {
		return 0, nil
	}
	removed := len(entries) - keep
	entries = entries[:keep]
	if err := WriteBenchmarkHistory(path, entries); err != nil {
		return 0, err
	}
	return removed, nil
}

func ExportBenchmarkHistoryCSV(path string, entries []BenchmarkHistoryEntry) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("csv output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"recorded_at", "video_path", "duration_sec", "recommended_hwaccel", "recommended_preset", "best_realtime_factor", "recommendation_message"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, e := range entries {
		row := []string{
			e.RecordedAt,
			e.VideoPath,
			strconv.Itoa(e.DurationSec),
			e.Recommended.HWAccel,
			e.Recommended.Preset,
			fmt.Sprintf("%.4f", bestHistorySpeed(e.Results)),
			e.RecommendationMessage,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

func bestHistorySpeed(results []BenchmarkResult) float64 {
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

func runBenchmarkCase(videoPath string, durationSec int, c BenchmarkCase, verbose bool) error {
	args := []string{"-y"}
	if hw := hwaccelArgs(c.HWAccel); len(hw) > 0 {
		args = append(args, hw...)
	}
	args = append(args,
		"-t", strconv.Itoa(durationSec),
		"-i", videoPath,
		"-map", "0:v:0",
		"-an",
		"-vf", "fps=4",
	)
	if perfArgs := performanceArgs(c.Preset); len(perfArgs) > 0 {
		args = append(args, perfArgs...)
	}
	args = append(args, "-f", "null", "-")

	cmd := exec.Command("ffmpeg", args...)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

func chooseRecommended(results []BenchmarkResult) (BenchmarkResult, bool) {
	ok := make([]BenchmarkResult, 0, len(results))
	for _, r := range results {
		if r.Success {
			ok = append(ok, r)
		}
	}
	if len(ok) == 0 {
		return BenchmarkResult{}, false
	}
	sort.Slice(ok, func(i, j int) bool {
		if ok[i].RealtimeFactor == ok[j].RealtimeFactor {
			if ok[i].Case.HWAccel == ok[j].Case.HWAccel {
				return ok[i].Case.Preset < ok[j].Case.Preset
			}
			// Prefer explicit accelerators over none if equal speed.
			if ok[i].Case.HWAccel == "none" {
				return false
			}
			if ok[j].Case.HWAccel == "none" {
				return true
			}
			return ok[i].Case.HWAccel < ok[j].Case.HWAccel
		}
		return ok[i].RealtimeFactor > ok[j].RealtimeFactor
	})
	return ok[0], true
}
