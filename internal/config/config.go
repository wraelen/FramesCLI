package config

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	FramesRoot        string                   `json:"frames_root"`
	OBSVideoDir       string                   `json:"obs_video_dir"`
	RecentVideoDirs   []string                 `json:"recent_video_dirs,omitempty"`
	RecentExts        []string                 `json:"recent_extensions,omitempty"`
	HWAccel           string                   `json:"hwaccel,omitempty"`
	PerformanceMode   string                   `json:"performance_mode,omitempty"`
	WhisperBin        string                   `json:"whisper_bin"`
	FasterWhisperBin  string                   `json:"faster_whisper_bin,omitempty"`
	WhisperModel      string                   `json:"whisper_model"`
	WhisperLanguage   string                   `json:"whisper_language"`
	TranscribeBackend string                   `json:"transcribe_backend,omitempty"`
	DefaultFPS        float64                  `json:"default_fps"`
	DefaultFormat     string                   `json:"default_format"`
	TUIThemePreset    string                   `json:"tui_theme_preset,omitempty"`
	TUISimpleMode     bool                     `json:"tui_simple_mode"`
	TUIWelcomeSeen    bool                     `json:"tui_welcome_seen"`
	TUIVimMode        bool                     `json:"tui_vim_mode,omitempty"`
	PresetProfiles    map[string]PresetProfile `json:"preset_profiles,omitempty"`
	AgentInputDirs    []string                 `json:"agent_input_dirs,omitempty"`
	AgentOutputRoot   string                   `json:"agent_output_root,omitempty"`
	PostExtractHook   string                   `json:"post_extract_hook,omitempty"`
	PostHookTimeout   int                      `json:"post_extract_hook_timeout_sec,omitempty"`
	TelemetryEnabled  bool                     `json:"telemetry_enabled,omitempty"`
	TelemetryPath     string                   `json:"telemetry_path,omitempty"`
}

type PresetProfile struct {
	Preset  string `json:"preset,omitempty"`
	Format  string `json:"format,omitempty"`
	Voice   bool   `json:"voice,omitempty"`
	NoSheet bool   `json:"no_sheet,omitempty"`
}

type ValidationWarning struct {
	Field   string
	Message string
}

func SupportedWhisperModels() []string {
	return []string{
		"tiny",
		"base",
		"small",
		"medium",
		"large",
		"turbo",
	}
}

func Default() Config {
	return Config{
		FramesRoot:        "frames",
		OBSVideoDir:       "",
		RecentVideoDirs:   []string{},
		RecentExts:        []string{"mp4", "mkv", "mov"},
		HWAccel:           "none",
		PerformanceMode:   "balanced",
		WhisperBin:        "whisper",
		FasterWhisperBin:  "faster-whisper",
		WhisperModel:      "base",
		WhisperLanguage:   "",
		TranscribeBackend: "auto",
		DefaultFPS:        4,
		DefaultFormat:     "png",
		TUIThemePreset:    "default",
		TUISimpleMode:     true,
		TUIWelcomeSeen:    false,
		TUIVimMode:        false,
		PresetProfiles: map[string]PresetProfile{
			"fast":          {Preset: "fast", Format: "jpg", Voice: false, NoSheet: true},
			"balanced":      {Preset: "balanced", Format: "png", Voice: false, NoSheet: false},
			"high-fidelity": {Preset: "safe", Format: "png", Voice: true, NoSheet: false},
		},
		AgentInputDirs:   []string{},
		AgentOutputRoot:  "frames",
		PostExtractHook:  "",
		PostHookTimeout:  30,
		TelemetryEnabled: false,
		TelemetryPath:    "",
	}
}

func Path() (string, error) {
	if env := strings.TrimSpace(os.Getenv("FRAMES_CONFIG")); env != "" {
		return env, nil
	}
	return defaultPath()
}

func defaultPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "framescli", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "framescli", "config.json"), nil
}

func legacyPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "frames-cli", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "frames-cli", "config.json"), nil
}

func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		legacy, legacyErr := legacyPath()
		if legacyErr != nil {
			return Config{}, err
		}
		raw, err = os.ReadFile(legacy)
		if err != nil {
			return Config{}, err
		}
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	normalize(&cfg)
	return cfg, nil
}

func LoadOrDefault() (Config, error) {
	cfg, err := Load()
	if err == nil {
		return cfg, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	return Config{}, err
}

func Save(cfg Config) (string, error) {
	normalize(&cfg)
	p, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

func ApplyEnvDefaults(cfg Config) {
	if strings.TrimSpace(os.Getenv("WHISPER_BIN")) == "" && strings.TrimSpace(cfg.WhisperBin) != "" {
		_ = os.Setenv("WHISPER_BIN", cfg.WhisperBin)
	}
	if strings.TrimSpace(os.Getenv("FASTER_WHISPER_BIN")) == "" && strings.TrimSpace(cfg.FasterWhisperBin) != "" {
		_ = os.Setenv("FASTER_WHISPER_BIN", cfg.FasterWhisperBin)
	}
	if strings.TrimSpace(os.Getenv("WHISPER_MODEL")) == "" && strings.TrimSpace(cfg.WhisperModel) != "" {
		_ = os.Setenv("WHISPER_MODEL", cfg.WhisperModel)
	}
	if strings.TrimSpace(os.Getenv("WHISPER_LANGUAGE")) == "" && strings.TrimSpace(cfg.WhisperLanguage) != "" {
		_ = os.Setenv("WHISPER_LANGUAGE", cfg.WhisperLanguage)
	}
	if strings.TrimSpace(os.Getenv("TRANSCRIBE_BACKEND")) == "" && strings.TrimSpace(cfg.TranscribeBackend) != "" {
		_ = os.Setenv("TRANSCRIBE_BACKEND", cfg.TranscribeBackend)
	}
	if strings.TrimSpace(os.Getenv("OBS_VIDEO_DIR")) == "" && strings.TrimSpace(cfg.OBSVideoDir) != "" {
		_ = os.Setenv("OBS_VIDEO_DIR", cfg.OBSVideoDir)
	}
}

func normalize(cfg *Config) {
	cfg.FramesRoot = strings.TrimSpace(cfg.FramesRoot)
	if cfg.FramesRoot == "" {
		cfg.FramesRoot = "frames"
	}
	cfg.OBSVideoDir = strings.TrimSpace(cfg.OBSVideoDir)
	cfg.RecentVideoDirs = normalizePathList(cfg.RecentVideoDirs)
	if len(cfg.RecentVideoDirs) == 0 && cfg.OBSVideoDir != "" {
		cfg.RecentVideoDirs = []string{cfg.OBSVideoDir}
	}
	cfg.RecentExts = normalizeExtList(cfg.RecentExts)
	if len(cfg.RecentExts) == 0 {
		cfg.RecentExts = []string{"mp4", "mkv", "mov"}
	}
	cfg.WhisperBin = strings.TrimSpace(cfg.WhisperBin)
	cfg.FasterWhisperBin = strings.TrimSpace(cfg.FasterWhisperBin)
	cfg.HWAccel = strings.ToLower(strings.TrimSpace(cfg.HWAccel))
	switch cfg.HWAccel {
	case "", "none", "auto", "cuda", "vaapi", "qsv":
	default:
		cfg.HWAccel = "none"
	}
	if cfg.HWAccel == "" {
		cfg.HWAccel = "none"
	}
	cfg.PerformanceMode = strings.ToLower(strings.TrimSpace(cfg.PerformanceMode))
	switch cfg.PerformanceMode {
	case "", "safe", "balanced", "fast":
	default:
		cfg.PerformanceMode = "balanced"
	}
	if cfg.PerformanceMode == "" {
		cfg.PerformanceMode = "balanced"
	}
	if cfg.WhisperBin == "" {
		cfg.WhisperBin = "whisper"
	}
	if cfg.FasterWhisperBin == "" {
		cfg.FasterWhisperBin = "faster-whisper"
	}
	cfg.WhisperModel = strings.TrimSpace(cfg.WhisperModel)
	if cfg.WhisperModel == "" {
		cfg.WhisperModel = "base"
	}
	cfg.WhisperModel = strings.ToLower(cfg.WhisperModel)
	cfg.WhisperLanguage = strings.TrimSpace(cfg.WhisperLanguage)
	cfg.TranscribeBackend = strings.ToLower(strings.TrimSpace(cfg.TranscribeBackend))
	switch cfg.TranscribeBackend {
	case "", "auto", "whisper", "faster-whisper", "faster_whisper", "fasterwhisper":
	default:
		cfg.TranscribeBackend = "auto"
	}
	if cfg.TranscribeBackend == "" {
		cfg.TranscribeBackend = "auto"
	}
	if cfg.TranscribeBackend == "faster_whisper" || cfg.TranscribeBackend == "fasterwhisper" {
		cfg.TranscribeBackend = "faster-whisper"
	}
	if cfg.DefaultFPS <= 0 {
		cfg.DefaultFPS = 4
	}
	cfg.DefaultFormat = strings.ToLower(strings.TrimSpace(cfg.DefaultFormat))
	if cfg.DefaultFormat == "" {
		cfg.DefaultFormat = "png"
	}
	if cfg.DefaultFormat == "jpeg" {
		cfg.DefaultFormat = "jpg"
	}
	if cfg.DefaultFormat != "png" && cfg.DefaultFormat != "jpg" {
		cfg.DefaultFormat = "png"
	}
	cfg.TUIThemePreset = strings.ToLower(strings.TrimSpace(cfg.TUIThemePreset))
	if cfg.TUIThemePreset == "" {
		cfg.TUIThemePreset = "default"
	}
	if cfg.TUIThemePreset != "default" && cfg.TUIThemePreset != "high-contrast" && cfg.TUIThemePreset != "light" {
		cfg.TUIThemePreset = "default"
	}
	cfg.PresetProfiles = normalizeProfiles(cfg.PresetProfiles)
	cfg.AgentInputDirs = normalizePathList(cfg.AgentInputDirs)
	cfg.AgentOutputRoot = strings.TrimSpace(cfg.AgentOutputRoot)
	if cfg.AgentOutputRoot == "" {
		cfg.AgentOutputRoot = cfg.FramesRoot
	}
	cfg.PostExtractHook = strings.TrimSpace(cfg.PostExtractHook)
	if cfg.PostHookTimeout <= 0 {
		cfg.PostHookTimeout = 30
	}
	cfg.TelemetryPath = strings.TrimSpace(cfg.TelemetryPath)
}

func ParseFPS(input string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func Validate(cfg Config) []ValidationWarning {
	normalized := cfg
	normalize(&normalized)
	warnings := make([]ValidationWarning, 0, 5)
	if _, err := exec.LookPath(normalized.WhisperBin); err != nil {
		warnings = append(warnings, ValidationWarning{
			Field:   "whisper_bin",
			Message: "not found on PATH; transcription commands may fail",
		})
	}
	if normalized.TranscribeBackend == "faster-whisper" {
		if _, err := exec.LookPath(normalized.FasterWhisperBin); err != nil {
			warnings = append(warnings, ValidationWarning{
				Field:   "faster_whisper_bin",
				Message: "not found on PATH; faster-whisper backend commands may fail",
			})
		}
	}
	if normalized.TranscribeBackend != "auto" && normalized.TranscribeBackend != "whisper" && normalized.TranscribeBackend != "faster-whisper" {
		warnings = append(warnings, ValidationWarning{
			Field:   "transcribe_backend",
			Message: "unsupported value; use auto|whisper|faster-whisper",
		})
	}
	for _, dir := range normalized.RecentVideoDirs {
		if stat, err := os.Stat(dir); err != nil || !stat.IsDir() {
			warnings = append(warnings, ValidationWarning{
				Field:   "recent_video_dirs",
				Message: "missing or inaccessible directory: " + dir,
			})
		}
	}
	if normalized.DefaultFPS < 0.5 || normalized.DefaultFPS > 60 {
		warnings = append(warnings, ValidationWarning{
			Field:   "default_fps",
			Message: "outside common range (0.5 - 60); extraction speed/size may be poor",
		})
	}
	if normalized.DefaultFormat != "png" && normalized.DefaultFormat != "jpg" {
		warnings = append(warnings, ValidationWarning{
			Field:   "default_format",
			Message: "unsupported format; use png or jpg",
		})
	}
	if normalized.HWAccel != "none" && normalized.HWAccel != "auto" && normalized.HWAccel != "cuda" && normalized.HWAccel != "vaapi" && normalized.HWAccel != "qsv" {
		warnings = append(warnings, ValidationWarning{
			Field:   "hwaccel",
			Message: "unsupported value; use none|auto|cuda|vaapi|qsv",
		})
	}
	return warnings
}

func normalizePathList(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func normalizeExtList(exts []string) []string {
	out := make([]string, 0, len(exts))
	seen := map[string]struct{}{}
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(e, ".")))
		if e == "" {
			continue
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out
}

func normalizeProfiles(in map[string]PresetProfile) map[string]PresetProfile {
	if len(in) == 0 {
		return Default().PresetProfiles
	}
	out := map[string]PresetProfile{}
	for name, p := range in {
		k := strings.ToLower(strings.TrimSpace(name))
		if k == "" {
			continue
		}
		p.Preset = strings.ToLower(strings.TrimSpace(p.Preset))
		switch p.Preset {
		case "", "safe", "balanced", "fast":
			if p.Preset == "" {
				p.Preset = "balanced"
			}
		default:
			p.Preset = "balanced"
		}
		p.Format = strings.ToLower(strings.TrimSpace(p.Format))
		switch p.Format {
		case "", "png", "jpg", "jpeg":
			if p.Format == "" {
				p.Format = "png"
			}
			if p.Format == "jpeg" {
				p.Format = "jpg"
			}
		default:
			p.Format = "png"
		}
		out[k] = p
	}
	if len(out) == 0 {
		return Default().PresetProfiles
	}
	return out
}
