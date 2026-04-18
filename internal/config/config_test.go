package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.FramesRoot = "my-frames"
	cfg.DefaultFPS = 6
	cfg.DefaultFormat = "jpg"
	cfg.HWAccel = "cuda"
	cfg.PerformanceMode = "fast"

	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.FramesRoot != "my-frames" || got.DefaultFPS != 6 || got.DefaultFormat != "jpg" || got.HWAccel != "cuda" || got.PerformanceMode != "fast" {
		t.Fatalf("unexpected config: %+v", got)
	}
	if got.TUIThemePreset != "default" || !got.TUISimpleMode || got.TUIWelcomeSeen {
		t.Fatalf("unexpected tui defaults after round-trip: %+v", got)
	}
}

func TestLoadOrDefaultMissingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "missing.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg, err := LoadOrDefault()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultFPS <= 0 || cfg.DefaultFormat == "" {
		t.Fatalf("invalid defaults: %+v", cfg)
	}
}

func TestApplyEnvDefaults(t *testing.T) {
	_ = os.Unsetenv("WHISPER_BIN")
	_ = os.Unsetenv("FASTER_WHISPER_BIN")
	_ = os.Unsetenv("WHISPER_MODEL")
	_ = os.Unsetenv("WHISPER_LANGUAGE")
	_ = os.Unsetenv("TRANSCRIBE_BACKEND")
	_ = os.Unsetenv("OBS_VIDEO_DIR")
	t.Cleanup(func() {
		_ = os.Unsetenv("WHISPER_BIN")
		_ = os.Unsetenv("FASTER_WHISPER_BIN")
		_ = os.Unsetenv("WHISPER_MODEL")
		_ = os.Unsetenv("WHISPER_LANGUAGE")
		_ = os.Unsetenv("TRANSCRIBE_BACKEND")
		_ = os.Unsetenv("OBS_VIDEO_DIR")
	})

	cfg := Default()
	cfg.WhisperBin = "custom-whisper"
	cfg.FasterWhisperBin = "custom-faster-whisper"
	cfg.WhisperModel = "small"
	cfg.WhisperLanguage = "en"
	cfg.TranscribeBackend = "faster-whisper"
	cfg.OBSVideoDir = "/videos"
	ApplyEnvDefaults(cfg)

	if os.Getenv("WHISPER_BIN") != "custom-whisper" {
		t.Fatalf("expected WHISPER_BIN to be set")
	}
	if os.Getenv("OBS_VIDEO_DIR") != "/videos" {
		t.Fatalf("expected OBS_VIDEO_DIR to be set")
	}
	if os.Getenv("FASTER_WHISPER_BIN") != "custom-faster-whisper" {
		t.Fatalf("expected FASTER_WHISPER_BIN to be set")
	}
	if os.Getenv("TRANSCRIBE_BACKEND") != "faster-whisper" {
		t.Fatalf("expected TRANSCRIBE_BACKEND to be set")
	}
}

func TestSupportedWhisperModels(t *testing.T) {
	models := SupportedWhisperModels()
	if len(models) < 3 {
		t.Fatalf("expected several whisper models, got %d", len(models))
	}
	foundBase := false
	for _, m := range models {
		if m == "base" {
			foundBase = true
			break
		}
	}
	if !foundBase {
		t.Fatalf("expected base model in supported list")
	}
}

func TestValidate(t *testing.T) {
	cfg := Default()
	cfg.OBSVideoDir = "/path/that/does/not/exist"
	cfg.WhisperBin = "definitely-not-installed-bin"
	cfg.DefaultFPS = 120
	warnings := Validate(cfg)
	if len(warnings) == 0 {
		t.Fatalf("expected validation warnings")
	}
}

func TestNormalizeThemePreset(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-theme.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.TUIThemePreset = "solarized"
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.TUIThemePreset != "default" {
		t.Fatalf("expected fallback theme default, got %s", got.TUIThemePreset)
	}
}

func TestLightThemePresetPersists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-theme-light.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.TUIThemePreset = "light"
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.TUIThemePreset != "light" {
		t.Fatalf("expected light theme to persist, got %s", got.TUIThemePreset)
	}
}

func TestPresetProfilesNormalize(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-profiles.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.PresetProfiles = map[string]PresetProfile{
		"MyProfile": {Preset: "FAST", Format: "JPEG", Voice: true, NoSheet: true},
	}
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	p, ok := got.PresetProfiles["myprofile"]
	if !ok {
		t.Fatalf("expected normalized profile key")
	}
	if p.Preset != "fast" || p.Format != "jpg" {
		t.Fatalf("unexpected normalized profile: %+v", p)
	}
}

func TestPerformanceModeSafeAliasNormalizes(t *testing.T) {
	cfg := Default()
	cfg.PerformanceMode = "safe"
	cfg.PresetProfiles = map[string]PresetProfile{
		"legacy": {Preset: "safe", Format: "jpeg"},
	}
	normalize(&cfg)
	if cfg.PerformanceMode != "laptop-safe" {
		t.Fatalf("expected safe alias to normalize to laptop-safe, got %q", cfg.PerformanceMode)
	}
	if got := cfg.PresetProfiles["legacy"]; got.Preset != "laptop-safe" || got.Format != "jpg" {
		t.Fatalf("expected preset profile alias normalization, got %+v", got)
	}
}

func TestBuiltInPresetProfilesMigrateForward(t *testing.T) {
	cfg := Default()
	cfg.PresetProfiles = map[string]PresetProfile{
		"balanced":      {Preset: "balanced", Format: "png"},
		"fast":          {Preset: "fast", Format: "jpg", NoSheet: true},
		"high-fidelity": {Preset: "safe", Format: "png", Voice: true},
	}
	normalize(&cfg)
	got := cfg.PresetProfiles["high-fidelity"]
	if got.Preset != "high-fidelity" || got.Format != "png" || !got.Voice || got.NoSheet {
		t.Fatalf("expected built-in high-fidelity profile to migrate forward, got %+v", got)
	}
	if _, ok := cfg.PresetProfiles["laptop-safe"]; !ok {
		t.Fatalf("expected missing built-in laptop-safe profile to be restored")
	}
}

func TestAgentPathPreferencesPersist(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-agent-paths.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.AgentInputDirs = []string{"/tmp/a", "/tmp/a", " /tmp/b "}
	cfg.AgentOutputRoot = " /tmp/out "
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AgentInputDirs) != 2 {
		t.Fatalf("expected deduped agent_input_dirs, got %+v", got.AgentInputDirs)
	}
	if got.AgentOutputRoot != "/tmp/out" {
		t.Fatalf("unexpected agent_output_root: %q", got.AgentOutputRoot)
	}
}

func TestTUIVimModePersists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-vim-mode.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.TUIVimMode = true
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.TUIVimMode {
		t.Fatalf("expected tui_vim_mode to persist true")
	}
}

func TestPostExtractHookPersists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-hook.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.PostExtractHook = "echo done"
	cfg.PostHookTimeout = 45
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.PostExtractHook != "echo done" {
		t.Fatalf("expected post_extract_hook to persist, got %q", got.PostExtractHook)
	}
	if got.PostHookTimeout != 45 {
		t.Fatalf("expected post_extract_hook_timeout_sec=45, got %d", got.PostHookTimeout)
	}
}

func TestPostExtractHookTimeoutDefaultsTo30(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-hook-timeout-default.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.PostHookTimeout = 0
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.PostHookTimeout != 30 {
		t.Fatalf("expected default post_extract_hook_timeout_sec=30, got %d", got.PostHookTimeout)
	}
}

func TestTelemetryConfigPersists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-telemetry.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.TelemetryEnabled = true
	cfg.TelemetryPath = " /tmp/frames-telemetry/events.jsonl "
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.TelemetryEnabled {
		t.Fatalf("expected telemetry_enabled=true")
	}
	if got.TelemetryPath != "/tmp/frames-telemetry/events.jsonl" {
		t.Fatalf("unexpected telemetry_path: %q", got.TelemetryPath)
	}
}

func TestTranscribeBackendConfigNormalizes(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg-transcribe-backend.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	cfg := Default()
	cfg.TranscribeBackend = "faster_whisper"
	cfg.FasterWhisperBin = " faster-whisper-custom "
	if _, err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.TranscribeBackend != "faster-whisper" {
		t.Fatalf("expected normalized transcribe_backend faster-whisper, got %q", got.TranscribeBackend)
	}
	if got.FasterWhisperBin != "faster-whisper-custom" {
		t.Fatalf("unexpected faster_whisper_bin: %q", got.FasterWhisperBin)
	}
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Setenv("XDG_CONFIG_HOME", tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("FRAMES_CONFIG"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("XDG_CONFIG_HOME")
		_ = os.Unsetenv("FRAMES_CONFIG")
	})
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(tmp, "framescli", "config.json")
	if p != want {
		t.Fatalf("expected %s got %s", want, p)
	}
}

func TestLoadFallsBackToLegacyPath(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Setenv("XDG_CONFIG_HOME", tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("FRAMES_CONFIG"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("XDG_CONFIG_HOME")
		_ = os.Unsetenv("FRAMES_CONFIG")
	})

	legacy := filepath.Join(tmp, "frames-cli", "config.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"frames_root":"legacy-frames","default_fps":5,"default_format":"png"}`)
	if err := os.WriteFile(legacy, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FramesRoot != "legacy-frames" {
		t.Fatalf("expected legacy config to load, got %+v", cfg)
	}
}

func TestLoadMigratesStockLegacyFramesRoot(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	raw := []byte(`{"frames_root":"frames","default_fps":4,"default_format":"png"}`)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := defaultFramesRoot()
	if cfg.FramesRoot != want {
		t.Fatalf("expected stock legacy \"frames\" to migrate to %q, got %q", want, cfg.FramesRoot)
	}
}

func TestLoadPreservesExplicitFramesRoot(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "cfg.json")
	if err := os.Setenv("FRAMES_CONFIG", path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	for _, explicit := range []string{"./frames", "/var/lib/frames", "custom-dir"} {
		raw := []byte(`{"frames_root":"` + explicit + `","default_fps":4,"default_format":"png"}`)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.FramesRoot != explicit {
			t.Fatalf("expected explicit %q to be preserved, got %q", explicit, cfg.FramesRoot)
		}
	}
}
