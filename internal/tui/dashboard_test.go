package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/wraelen/framescli/internal/media"
)

func TestScanRuns_LoadsMetadataAndTranscript(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "Run_1")
	imagesDir := filepath.Join(runDir, "images")
	voiceDir := filepath.Join(runDir, "voice")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(imagesDir, "sheets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(voiceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		name := filepath.Join(imagesDir, fmt.Sprintf("frame-%04d.png", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(imagesDir, "sheets", "contact-sheet.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voiceDir, "transcript.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	transcriptJSON := media.WhisperJSON{
		Text: "hello world",
		Segments: []media.WhisperSegment{
			{Start: 0, End: 1.2, Text: "hello"},
			{Start: 1.2, End: 2.5, Text: "world"},
		},
	}
	segRaw, err := json.Marshal(transcriptJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(voiceDir, "transcript.json"), segRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	md := media.RunMetadata{FPS: 4, FrameFormat: "png", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(md)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media.MetadataPathForRun(runDir), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	runs, err := scanRuns(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if !runs[0].metadataPresent {
		t.Fatalf("expected metadata to be loaded")
	}
	if runs[0].transcriptPath == "" || runs[0].contactSheet == "" {
		t.Fatalf("expected transcript and contact sheet paths")
	}
	if len(runs[0].segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(runs[0].segments))
	}
}

func TestTrimText(t *testing.T) {
	if got := trimText("", 10); got != "(no transcript)" {
		t.Fatalf("unexpected empty behavior: %s", got)
	}
	long := strings.Repeat("a", 20)
	if got := trimText(long, 10); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncation with ellipsis, got %s", got)
	}
}

func TestSegmentFrameRange(t *testing.T) {
	seg := transcriptSegment{Start: 1.0, End: 2.0}
	start, end := segmentFrameRange(seg, 4, 20)
	if start != 5 || end != 8 {
		t.Fatalf("unexpected frame range: %d-%d", start, end)
	}
}

func TestSelectedSegmentFramePath(t *testing.T) {
	run := t.TempDir()
	images := filepath.Join(run, "images")
	if err := os.MkdirAll(images, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(images, "frame-0005.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	it := runItem{
		path:            run,
		frameCount:      20,
		frameExt:        "png",
		segments:        []transcriptSegment{{Start: 1.0, End: 2.0, Text: "hello"}},
		metadata:        &media.RunMetadata{FPS: 4},
		metadataPresent: true,
	}
	got, err := selectedSegmentFramePath(it, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(images, "frame-0005.png") {
		t.Fatalf("unexpected frame path: %s", got)
	}
}

func TestLoadRuns_UsesIndexWhenPresent(t *testing.T) {
	root := t.TempDir()
	index := media.RunsIndex{
		Root:        root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RunCount:    1,
		Runs: []media.IndexedRun{
			{
				Name:     "Indexed_Run",
				Path:     filepath.Join(root, "Indexed_Run"),
				Frames:   42,
				FrameExt: "png",
				FPS:      4,
				Format:   "png",
			},
		},
	}
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media.IndexFilePath(root), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	items, source, _, err := loadRuns(root)
	if err != nil {
		t.Fatal(err)
	}
	if source != "index" {
		t.Fatalf("expected source=index got %s", source)
	}
	if len(items) != 1 || items[0].title != "Indexed_Run" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestFindSegmentMatches(t *testing.T) {
	segments := []transcriptSegment{
		{Text: "Initialize project"},
		{Text: "Run tests and fix failure"},
		{Text: "Final pass"},
	}
	got := findSegmentMatches(segments, "test")
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("unexpected matches: %#v", got)
	}
}

func TestSearchInput_JumpsToMatch(t *testing.T) {
	m := dashboard{
		tab: tabTranscript,
		list: listWithItems([]runItem{
			{
				title: "RunA",
				segments: []transcriptSegment{
					{Text: "alpha"},
					{Text: "beta gamma"},
					{Text: "gamma delta"},
				},
			},
		}),
		searchMode: true,
	}
	m.handleSearchInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if len(m.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.searchMatches))
	}
	if m.selectedSeg != 1 {
		t.Fatalf("expected jump to first match index 1, got %d", m.selectedSeg)
	}
}

func TestAdvanceSearchMatch_Wraps(t *testing.T) {
	m := dashboard{
		tab:         tabTranscript,
		searchQuery: "g",
		list: listWithItems([]runItem{
			{
				title: "RunA",
				segments: []transcriptSegment{
					{Text: "alpha"},
					{Text: "beta gamma"},
					{Text: "gamma delta"},
				},
			},
		}),
	}
	m.updateSearchMatches(true)
	if m.selectedSeg != 1 {
		t.Fatalf("expected first selected match=1, got %d", m.selectedSeg)
	}
	m.advanceSearchMatch(1)
	if m.selectedSeg != 2 {
		t.Fatalf("expected next selected match=2, got %d", m.selectedSeg)
	}
	m.advanceSearchMatch(1)
	if m.selectedSeg != 1 {
		t.Fatalf("expected wrapped selected match=1, got %d", m.selectedSeg)
	}
}

func listWithItems(items []runItem) list.Model {
	listItems := make([]list.Item, 0, len(items))
	for _, it := range items {
		listItems = append(listItems, it)
	}
	l := list.New(listItems, list.NewDefaultDelegate(), 0, 0)
	if len(items) > 0 {
		l.Select(0)
	}
	return l
}

func TestFilteredPaletteActions(t *testing.T) {
	m := dashboard{}
	m.paletteQuery = "transcript"
	actions := m.filteredPaletteActions()
	if len(actions) == 0 {
		t.Fatalf("expected filtered actions")
	}
	found := false
	for _, a := range actions {
		if a.ID == "tab_transcript" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tab_transcript in filtered results: %#v", actions)
	}
}

func TestExecutePaletteAction_TabSwitch(t *testing.T) {
	m := dashboard{tab: tabOverview}
	if err := m.executePaletteAction("tab_timeline"); err != nil {
		t.Fatal(err)
	}
	if m.tab != tabTimeline {
		t.Fatalf("expected tab_timeline to set timeline tab")
	}
}

func TestExecutePaletteAction_Quit(t *testing.T) {
	m := dashboard{}
	if err := m.executePaletteAction("quit"); err != nil {
		t.Fatal(err)
	}
	if !m.shouldQuit {
		t.Fatalf("expected shouldQuit to be true")
	}
}

func TestHelpModeToggle(t *testing.T) {
	m := dashboard{
		list: listWithItems([]runItem{{title: "RunA"}}),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	next := model.(dashboard)
	if !next.helpMode {
		t.Fatalf("expected help mode enabled")
	}
	model, _ = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next = model.(dashboard)
	if next.helpMode {
		t.Fatalf("expected help mode disabled by esc")
	}
}

func TestHelpModeConsumesKeys(t *testing.T) {
	m := dashboard{
		tab:      tabOverview,
		helpMode: true,
		list:     listWithItems([]runItem{{title: "RunA"}}),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	next := model.(dashboard)
	if next.tab != tabOverview {
		t.Fatalf("expected tab unchanged while help mode is active")
	}
}

func TestSimpleModeSkipsTimelineTab(t *testing.T) {
	m := dashboard{
		simpleMode: true,
		tab:        tabTranscript,
	}
	if got := m.nextTab(1); got != tabFiles {
		t.Fatalf("expected simple mode to skip timeline, got %v", got)
	}
}

func TestSetErrorStoresShortAndDetail(t *testing.T) {
	m := dashboard{}
	m.setError("This is a long error message that should be shortened in the status strip while keeping full detail.")
	if m.errorShort == "" || m.errorDetail == "" {
		t.Fatalf("expected both short and detailed errors to be set")
	}
	if m.statusLevel != statusError {
		t.Fatalf("expected error status level, got %v", m.statusLevel)
	}
}

func TestWelcomeDismissPersistsConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	if err := os.Setenv("FRAMES_CONFIG", cfgPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	m := dashboard{
		welcomeMode: true,
		simpleMode:  true,
		themePreset: themeDefault,
		list:        listWithItems([]runItem{{title: "RunA"}}),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := model.(dashboard)
	if next.welcomeMode {
		t.Fatalf("expected welcome mode to close on enter")
	}
}

func TestViewSmallTerminalFallback(t *testing.T) {
	m := dashboard{
		winW:   30,
		winH:   10,
		styles: makeStyles(themeDefault),
	}
	out := m.View()
	if !strings.Contains(out, "too small") {
		t.Fatalf("expected small terminal fallback message")
	}
}

func TestThemeSwitchViaKey(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	if err := os.Setenv("FRAMES_CONFIG", cfgPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	m := dashboard{
		themePreset: themeDefault,
		simpleMode:  true,
		list:        listWithItems([]runItem{{title: "RunA"}}),
		styles:      makeStyles(themeDefault),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	next := model.(dashboard)
	if next.themePreset != themeLight {
		t.Fatalf("expected theme to cycle to light, got %s", next.themePreset)
	}
}

func TestPaletteActionsSimpleModeHidesTimeline(t *testing.T) {
	m := dashboard{simpleMode: true}
	actions := m.paletteActions()
	for _, a := range actions {
		if a.ID == "tab_timeline" {
			t.Fatalf("timeline action should be hidden in simple mode")
		}
	}
}

func TestRightArrowSwitchesTab(t *testing.T) {
	m := dashboard{
		tab:    tabOverview,
		list:   listWithItems([]runItem{{title: "RunA"}}),
		styles: makeStyles(themeDefault),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	next := model.(dashboard)
	if next.tab != tabTranscript {
		t.Fatalf("expected right arrow to switch to transcript tab, got %v", next.tab)
	}
}

func TestLeftArrowSwitchesTabBack(t *testing.T) {
	m := dashboard{
		tab:    tabTranscript,
		list:   listWithItems([]runItem{{title: "RunA"}}),
		styles: makeStyles(themeDefault),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	next := model.(dashboard)
	if next.tab != tabOverview {
		t.Fatalf("expected left arrow to switch to overview tab, got %v", next.tab)
	}
}

func TestWizardASCIIPreview(t *testing.T) {
	m := dashboard{
		dropVideoInfo: &media.VideoInfo{DurationSec: 120},
	}
	out := m.wizardASCIIPreview()
	if !strings.Contains(out, "[#]") || !strings.Contains(out, "00:01:") {
		t.Fatalf("unexpected preview output: %s", out)
	}
}

func TestRenderTinyProgress(t *testing.T) {
	out := renderTinyProgress("Extract", 42.5, "elapsed 00:10, eta 00:14")
	if !strings.Contains(out, "42.5%") || !strings.Contains(out, "Extract [") {
		t.Fatalf("unexpected progress render: %s", out)
	}
	if !strings.Contains(out, "elapsed 00:10") {
		t.Fatalf("expected timing suffix in progress render: %s", out)
	}
}

func TestEstimateDropOutput(t *testing.T) {
	info := &media.VideoInfo{DurationSec: 120}
	out := estimateDropOutput(info, "frames", "jpg", "4")
	if !strings.Contains(out, "frames") || !strings.Contains(out, "MB") {
		t.Fatalf("unexpected estimate: %s", out)
	}
}

func TestApplyDropProfile(t *testing.T) {
	m := dashboard{dropProfile: "high-fidelity"}
	m.applyDropProfile()
	if m.dropPreset != "safe" || m.dropFormat != "png" || !m.dropVoice {
		t.Fatalf("profile not applied: %+v", m)
	}
}

func TestSaveLoadJobHistory(t *testing.T) {
	root := t.TempDir()
	rows := []jobHistoryEntry{{Status: "success", VideoPath: "/tmp/a.mp4"}}
	if err := saveJobHistory(root, rows); err != nil {
		t.Fatal(err)
	}
	got, err := loadJobHistory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != "success" {
		t.Fatalf("unexpected history: %+v", got)
	}
}

func TestWelcomeStartsTourOnFirstRun(t *testing.T) {
	m := dashboard{
		welcomeMode:  true,
		firstRunTour: true,
		styles:       makeStyles(themeDefault),
		list:         listWithItems([]runItem{{title: "RunA"}}),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := model.(dashboard)
	if next.welcomeMode {
		t.Fatalf("expected welcome to close")
	}
	if !next.tourMode {
		t.Fatalf("expected guided tour to open")
	}
}

func TestTourAdvanceAndClose(t *testing.T) {
	m := dashboard{
		tourMode: true,
		tourStep: 0,
		styles:   makeStyles(themeDefault),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := model.(dashboard)
	if next.tourStep != 1 || !next.tourMode {
		t.Fatalf("expected tour step advance, got step=%d mode=%v", next.tourStep, next.tourMode)
	}
	model2, _ := next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	end := model2.(dashboard)
	if end.tourMode {
		t.Fatalf("expected tour close")
	}
}

func TestViewWelcomeOverlayShowsWelcomeText(t *testing.T) {
	m := dashboard{
		winW:        120,
		winH:        36,
		welcomeMode: true,
		simpleMode:  true,
		themePreset: themeDefault,
		styles:      makeStyles(themeDefault),
		list:        listWithItems([]runItem{{title: "RunA"}}),
	}
	out := m.View()
	if !strings.Contains(out, "Welcome to FramesCLI") {
		t.Fatalf("expected welcome overlay text")
	}
}

func TestCommandBarShowsArrowHints(t *testing.T) {
	m := dashboard{simpleMode: true, winW: 120, styles: makeStyles(themeDefault)}
	out := m.renderCommandBar()
	if !strings.Contains(out, "Left/Right") {
		t.Fatalf("expected command bar to include arrow guidance")
	}
}

func TestCommandBarShowsVimHintsWhenEnabled(t *testing.T) {
	m := dashboard{simpleMode: true, vimMode: true, winW: 120, styles: makeStyles(themeDefault)}
	out := m.renderCommandBar()
	if !strings.Contains(out, "h/l") || !strings.Contains(out, "j/k") {
		t.Fatalf("expected command bar to include vim guidance")
	}
}

func TestVimModeTogglePersists(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	if err := os.Setenv("FRAMES_CONFIG", cfgPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("FRAMES_CONFIG") })

	m := dashboard{
		simpleMode: true,
		vimMode:    false,
		list:       listWithItems([]runItem{{title: "RunA"}}),
		styles:     makeStyles(themeDefault),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	next := model.(dashboard)
	if !next.vimMode {
		t.Fatalf("expected vim mode enabled")
	}
}

func TestHelpPanelShowsVimVariant(t *testing.T) {
	m := dashboard{vimMode: true, styles: makeStyles(themeDefault)}
	out := m.renderHelpPanel()
	if !strings.Contains(out, "Keymap: Vim mode") || !strings.Contains(out, "[h/l] Switch detail section") {
		t.Fatalf("expected vim help variant, got: %s", out)
	}
}

func TestCopyClipboardFallbackToXclip(t *testing.T) {
	origOS := runtimeGOOS
	origLook := clipboardLookPath
	origRun := clipboardRunCmd
	t.Cleanup(func() {
		runtimeGOOS = origOS
		clipboardLookPath = origLook
		clipboardRunCmd = origRun
	})

	runtimeGOOS = "linux"
	clipboardLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	calls := []string{}
	clipboardRunCmd = func(name string, args []string, stdin string) error {
		calls = append(calls, name)
		if name == "wl-copy" {
			return errors.New("wl-copy unavailable")
		}
		if name == "xclip" {
			return nil
		}
		return errors.New("unexpected")
	}
	if err := copyTextToClipboard("hello"); err != nil {
		t.Fatalf("expected fallback to xclip, got %v", err)
	}
	if len(calls) < 2 || calls[0] != "wl-copy" || calls[1] != "xclip" {
		t.Fatalf("unexpected clipboard call order: %+v", calls)
	}
}

func TestCopyClipboardUsesPlatformSpecificTool(t *testing.T) {
	origOS := runtimeGOOS
	origLook := clipboardLookPath
	origRun := clipboardRunCmd
	t.Cleanup(func() {
		runtimeGOOS = origOS
		clipboardLookPath = origLook
		clipboardRunCmd = origRun
	})

	clipboardLookPath = func(name string) (string, error) { return "/usr/bin/" + name, nil }
	nameUsed := ""
	clipboardRunCmd = func(name string, args []string, stdin string) error {
		nameUsed = name
		return nil
	}
	runtimeGOOS = "darwin"
	if err := copyTextToClipboard("hello"); err != nil {
		t.Fatal(err)
	}
	if nameUsed != "pbcopy" {
		t.Fatalf("expected pbcopy on darwin, got %s", nameUsed)
	}

	runtimeGOOS = "windows"
	if err := copyTextToClipboard("hello"); err != nil {
		t.Fatal(err)
	}
	if nameUsed != "clip" {
		t.Fatalf("expected clip on windows, got %s", nameUsed)
	}
}

func TestViewSnapshotWideLayout(t *testing.T) {
	m := dashboard{
		rootDir:       "frames",
		dataSource:    "index",
		simpleMode:    true,
		themePreset:   themeDefault,
		styles:        makeStyles(themeDefault),
		list:          listWithItems([]runItem{{title: "RunA", description: "1 PNG frame"}}),
		statusText:    "Ready",
		statusLevel:   statusInfo,
		welcomeMode:   false,
		compactMode:   false,
		focus:         focusList,
		itemCount:     1,
		dataHint:      "loaded from frames/index.json",
		lastSelection: "RunA",
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	out := model.(dashboard).View()
	assertContainsAll(t, out, []string{
		"FramesCLI",
		"RunA",
		"Overview",
		"[Left/Right]",
	})
}

func TestViewSnapshotCompactLayout(t *testing.T) {
	m := dashboard{
		rootDir:       "frames",
		dataSource:    "live",
		simpleMode:    true,
		themePreset:   themeDefault,
		styles:        makeStyles(themeDefault),
		list:          listWithItems([]runItem{{title: "RunA", description: "1 PNG frame"}}),
		statusText:    "Ready",
		statusLevel:   statusInfo,
		welcomeMode:   false,
		focus:         focusList,
		itemCount:     1,
		lastSelection: "RunA",
	}
	model, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 26})
	next := model.(dashboard)
	if !next.compactMode {
		t.Fatalf("expected compact mode for 90x26")
	}
	out := next.View()
	assertContainsAll(t, out, []string{
		"FramesCLI",
		"RunA",
		"Overview",
		"Ready",
	})
}

func TestViewSnapshotWelcomeScreen(t *testing.T) {
	m := dashboard{
		winW:        120,
		winH:        36,
		welcomeMode: true,
		simpleMode:  true,
		themePreset: themeDefault,
		styles:      makeStyles(themeDefault),
		list:        listWithItems([]runItem{{title: "RunA"}}),
	}
	out := m.View()
	assertContainsAll(t, out, []string{
		"FramesCLI Setup",
		"Welcome to FramesCLI",
		"Quick keys:",
	})
}

func assertContainsAll(t *testing.T, got string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(got, n) {
			t.Fatalf("expected output to contain %q", n)
		}
	}
}

func TestConsumeDropPasteOpensDropModal(t *testing.T) {
	m := dashboard{
		list:   listWithItems([]runItem{{title: "RunA"}}),
		styles: makeStyles(themeDefault),
	}
	if !m.consumeDropPaste(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/tmp/My\\ Clip.mp4")}) {
		t.Fatalf("expected paste to be consumed")
	}
	if m.dropModal {
		t.Fatalf("modal should open on enter, not immediately")
	}
	if !m.consumeDropPaste(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatalf("expected enter to be consumed for import modal")
	}
	if !m.dropModal {
		t.Fatalf("expected import modal to open")
	}
	if got := m.dropPath; got != "/tmp/My Clip.mp4" {
		t.Fatalf("unexpected normalized drop path: %q", got)
	}
}

func TestNormalizeDroppedPath(t *testing.T) {
	if got := normalizeDroppedPath(`"/tmp/My File.mov"`); got != "/tmp/My File.mov" {
		t.Fatalf("unexpected normalized quoted path: %s", got)
	}
	if got := normalizeDroppedPath("file:///tmp/My%20File.mp4"); got != "/tmp/My File.mp4" {
		t.Fatalf("unexpected normalized file URI path: %s", got)
	}
}

func TestRenderDropCueStripValidPath(t *testing.T) {
	m := dashboard{
		winW:        120,
		pendingDrop: "/tmp/clip.mp4",
		styles:      makeStyles(themeDefault),
	}
	out := m.renderDropCueStrip()
	if !strings.Contains(out, "Video detected:") {
		t.Fatalf("expected valid import cue")
	}
}

func TestRenderDropCueStripInvalidPath(t *testing.T) {
	m := dashboard{
		winW:        120,
		pendingDrop: "/tmp/not-a-video.txt",
		styles:      makeStyles(themeDefault),
	}
	out := m.renderDropCueStrip()
	if !strings.Contains(out, "not a recognized video path yet") {
		t.Fatalf("expected invalid drop cue")
	}
}

func TestEscClearsPendingDrop(t *testing.T) {
	m := dashboard{
		pendingDrop: "/tmp/clip.mp4",
		list:        listWithItems([]runItem{{title: "RunA"}}),
		styles:      makeStyles(themeDefault),
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := model.(dashboard)
	if next.pendingDrop != "" {
		t.Fatalf("expected pending drop to be cleared")
	}
}
