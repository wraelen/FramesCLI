package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/wraelen/framescli/internal/config"
	"github.com/wraelen/framescli/internal/media"
)

type detailTab int

type focusArea int

type statusLevel int

type statusTickMsg time.Time

type dropExtractDoneMsg struct {
	err               error
	out               string
	count             int
	transcriptPath    string
	contactSheet      string
	audioPath         string
	logPath           string
	queueCount        int
	queueTotal        int
	queueSuccess      int
	queueFailed       int
	queueFailedJobs   []queuedDropTask
	queueFailedErrors []string
	diagnosticsPath   string
	videoPath         string
	mode              string
	profile           string
	startedAt         time.Time
	endedAt           time.Time
}

type dropProgressMsg struct {
	percent float64
	stage   string
	elapsed time.Duration
	eta     time.Duration
}

func (m dropExtractDoneMsg) errString() string {
	if m.err == nil {
		return ""
	}
	return m.err.Error()
}

type jobHistoryEntry struct {
	StartedAt      string  `json:"started_at"`
	EndedAt        string  `json:"ended_at"`
	DurationSec    float64 `json:"duration_sec"`
	Status         string  `json:"status"`
	Mode           string  `json:"mode"`
	Profile        string  `json:"profile"`
	VideoPath      string  `json:"video_path"`
	OutputPath     string  `json:"output_path"`
	LogPath        string  `json:"log_path,omitempty"`
	Error          string  `json:"error,omitempty"`
	FramesProduced int     `json:"frames_produced,omitempty"`
}

type queuedDropTask struct {
	Path       string
	FPS        float64
	Opts       extractDropOptions
	EnqueuedAt time.Time
}

const (
	tabOverview detailTab = iota
	tabTranscript
	tabTimeline
	tabFiles
)

const (
	focusList focusArea = iota
	focusDetail
)

const (
	themeDefault      = "default"
	themeHighContrast = "high-contrast"
	themeLight        = "light"
)

const (
	statusInfo statusLevel = iota
	statusSuccess
	statusWarn
	statusError
)

var (
	allTabNames       = []string{"Overview", "Transcript", "Timeline", "Files"}
	simpleTabNames    = []string{"Overview", "Transcript", "Files"}
	smallScreenTitle  = "FramesCLI"
	runtimeGOOS       = runtime.GOOS
	clipboardLookPath = exec.LookPath
	clipboardRunCmd   = func(name string, args []string, stdin string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdin = strings.NewReader(stdin)
		return cmd.Run()
	}
)

type transcriptSegment struct {
	Start float64
	End   float64
	Text  string
}

type runItem struct {
	title           string
	description     string
	path            string
	frameCount      int
	updatedAt       time.Time
	transcriptPath  string
	contactSheet    string
	frameExt        string
	firstFrame      string
	lastFrame       string
	transcript      string
	segments        []transcriptSegment
	metadata        *media.RunMetadata
	metadataPresent bool
}

func (i runItem) Title() string       { return i.title }
func (i runItem) Description() string { return i.description }
func (i runItem) FilterValue() string {
	return strings.Join([]string{i.title, i.path, i.description, i.transcript}, " ")
}

type dashboard struct {
	list                  list.Model
	rootDir               string
	dataSource            string
	dataHint              string
	tab                   detailTab
	focus                 focusArea
	error                 string
	styles                styles
	winW                  int
	winH                  int
	scanning              bool
	itemCount             int
	detailOffset          int
	selectedSeg           int
	lastSelection         string
	searchMode            bool
	searchQuery           string
	searchMatches         []int
	searchPos             int
	paletteMode           bool
	paletteQuery          string
	paletteIndex          int
	shouldQuit            bool
	helpMode              bool
	tourMode              bool
	tourStep              int
	firstRunTour          bool
	welcomeMode           bool
	simpleMode            bool
	vimMode               bool
	themePreset           string
	compactMode           bool
	errorShort            string
	errorDetail           string
	errorExpanded         bool
	statusText            string
	statusLevel           statusLevel
	statusPulse           int
	pendingDrop           string
	dropModal             bool
	dropRunning           bool
	dropCancel            context.CancelFunc
	dropProgress          float64
	dropStage             string
	dropElapsed           time.Duration
	dropETA               time.Duration
	dropProgressC         <-chan dropProgressMsg
	dropDoneC             <-chan dropExtractDoneMsg
	dropPath              string
	dropCursor            int
	dropStep              int
	dropMode              string
	dropProfile           string
	dropVideoInfo         *media.VideoInfo
	dropPreview           []string
	dropEstimate          string
	dropResultOut         string
	dropResultCount       int
	dropResultTranscript  string
	dropResultSheet       string
	dropResultAudio       string
	dropResultErr         string
	dropResultLog         string
	dropResultDiagnostics string
	dropQueue             []queuedDropTask
	dropFailedQueue       []queuedDropTask
	dropQueueTotal        int
	dropQueueSuccess      int
	dropQueueFailed       int
	dropQueueFailedErrors []string
	historyMode           bool
	historyCursor         int
	jobHistory            []jobHistoryEntry
	presetProfiles        map[string]appconfig.PresetProfile
	dropFPSInput          string
	dropVoice             bool
	dropFormat            string
	dropPreset            string
	dropNoSheet           bool
	dropSheetCols         int
	dropVerbose           bool
	dropPreviewSamples    int
	dropPreviewWidth      int
}

type commandHint struct {
	key      string
	text     string
	advanced bool
}

type paletteAction struct {
	ID    string
	Label string
}

type styles struct {
	app         lipgloss.Style
	title       lipgloss.Style
	subtle      lipgloss.Style
	panel       lipgloss.Style
	panelFocus  lipgloss.Style
	detail      lipgloss.Style
	error       lipgloss.Style
	keyline     lipgloss.Style
	tab         lipgloss.Style
	tabActive   lipgloss.Style
	metricKey   lipgloss.Style
	metricValue lipgloss.Style
	badge       lipgloss.Style
	badgeActive lipgloss.Style
	statusInfo  lipgloss.Style
	statusOk    lipgloss.Style
	statusWarn  lipgloss.Style
	statusErr   lipgloss.Style
	commandBar  lipgloss.Style
	commandKey  lipgloss.Style
	commandText lipgloss.Style
	modal       lipgloss.Style
	modalTitle  lipgloss.Style
	errorPanel  lipgloss.Style
}

func Run(rootDir string) error {
	if strings.TrimSpace(rootDir) == "" {
		rootDir = media.DefaultFramesRoot
	}
	m, err := newDashboard(rootDir)
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func newDashboard(rootDir string) (dashboard, error) {
	items, source, hint, err := loadRuns(rootDir)
	if err != nil {
		return dashboard{}, err
	}
	listItems := toListItems(items)
	l := list.New(listItems, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Runs"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	cfg, cfgErr := appconfig.LoadOrDefault()
	if cfgErr != nil {
		cfg = appconfig.Default()
	}
	theme := cfg.TUIThemePreset
	if theme != themeDefault && theme != themeHighContrast && theme != themeLight {
		theme = themeDefault
	}
	st := makeStyles(theme)
	l.Styles.Title = st.title

	last := ""
	if len(items) > 0 {
		last = items[0].title
	}
	m := dashboard{
		list:           l,
		rootDir:        rootDir,
		dataSource:     source,
		dataHint:       hint,
		tab:            tabOverview,
		focus:          focusList,
		styles:         st,
		itemCount:      len(items),
		lastSelection:  last,
		welcomeMode:    !cfg.TUIWelcomeSeen,
		firstRunTour:   !cfg.TUIWelcomeSeen,
		simpleMode:     cfg.TUISimpleMode,
		vimMode:        cfg.TUIVimMode,
		themePreset:    theme,
		statusLevel:    statusInfo,
		statusText:     "Ready. Browse runs with arrows, or press [i] to import a video.",
		dropFPSInput:   fmt.Sprintf("%.2f", cfg.DefaultFPS),
		dropMode:       "both",
		dropProfile:    "balanced",
		dropFormat:     cfg.DefaultFormat,
		dropPreset:     cfg.PerformanceMode,
		dropSheetCols:  6,
		presetProfiles: cfg.PresetProfiles,
	}
	if history, err := loadJobHistory(rootDir); err == nil {
		m.jobHistory = history
	}
	if !m.simpleMode {
		m.statusText = "Ready. Advanced mode is on; press [i] to import or browse existing runs."
	}
	if cfgErr != nil {
		m.setStatus(statusWarn, "Using default UI settings. Could not read your config file.")
	}
	return m, nil
}

func (m dashboard) Init() tea.Cmd { return tickStatus() }

func (m dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dropProgressMsg:
		m.dropProgress = msg.percent
		if strings.TrimSpace(msg.stage) != "" {
			m.dropStage = msg.stage
		}
		m.dropElapsed = msg.elapsed
		m.dropETA = msg.eta
		if m.dropProgressC != nil && m.dropDoneC != nil {
			return m, waitForDropTaskMsg(m.dropProgressC, m.dropDoneC)
		}
		return m, nil
	case dropExtractDoneMsg:
		m.dropRunning = false
		m.dropCancel = nil
		m.dropProgressC = nil
		m.dropDoneC = nil
		m.dropProgress = 0
		m.dropStage = ""
		m.dropElapsed = 0
		m.dropETA = 0
		m.dropStep = 3
		m.dropCursor = 0
		m.dropResultOut = msg.out
		m.dropResultCount = msg.count
		m.dropResultTranscript = msg.transcriptPath
		m.dropResultSheet = msg.contactSheet
		m.dropResultAudio = msg.audioPath
		m.dropResultLog = msg.logPath
		m.dropResultDiagnostics = msg.diagnosticsPath
		m.dropQueueTotal = msg.queueTotal
		m.dropQueueSuccess = msg.queueSuccess
		m.dropQueueFailed = msg.queueFailed
		m.dropQueueFailedErrors = append([]string(nil), msg.queueFailedErrors...)
		m.dropFailedQueue = append([]queuedDropTask(nil), msg.queueFailedJobs...)
		hstatus := "success"
		if msg.err != nil {
			if errors.Is(msg.err, context.Canceled) {
				m.dropResultErr = "Canceled by user"
				hstatus = "canceled"
				m.appendJobHistory(msg, hstatus)
				m.setStatus(statusWarn, "Extraction canceled.")
				return m, nil
			}
			m.dropResultErr = msg.err.Error()
			hstatus = "failed"
			m.appendJobHistory(msg, hstatus)
			m.setError(msg.err.Error())
			return m, nil
		}
		m.dropResultErr = ""
		m.appendJobHistory(msg, hstatus)
		if msg.queueTotal > 0 {
			if msg.queueFailed > 0 {
				m.setStatus(statusWarn, fmt.Sprintf("Queue complete with failures (%d ok, %d failed).", msg.queueSuccess, msg.queueFailed))
			} else {
				m.setStatus(statusSuccess, fmt.Sprintf("Queue complete (%d jobs). Last output: %s", msg.queueTotal, msg.out))
			}
		} else if msg.queueCount > 0 {
			m.setStatus(statusSuccess, fmt.Sprintf("Queue complete (%d jobs). Last output: %s", msg.queueCount, msg.out))
		} else {
			m.setStatus(statusSuccess, fmt.Sprintf("Import complete. %d frames in %s", msg.count, msg.out))
		}
		m.refreshRuns()
		return m, nil
	case statusTickMsg:
		m.statusPulse = (m.statusPulse + 1) % 4
		return m, tickStatus()
	case tea.KeyMsg:
		s := msg.String()
		if m.dropModal {
			if handled, cmd := m.handleDropModalInput(msg); handled {
				return m, cmd
			}
		}
		if m.welcomeMode {
			switch s {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "enter", "esc", " ":
				m.welcomeMode = false
				if m.firstRunTour {
					m.tourMode = true
					m.tourStep = 0
					m.firstRunTour = false
				}
				m.setStatus(statusSuccess, "Welcome complete. Start with [i] to import a video, or browse runs with arrows.")
				if err := m.persistUIPreferences(true); err != nil {
					m.setError(err.Error())
				}
				return m, nil
			default:
				return m, nil
			}
		}
		if m.tourMode {
			switch s {
			case "ctrl+c":
				return m, tea.Quit
			case "q", "esc":
				m.tourMode = false
				m.setStatus(statusInfo, "Tour closed. Press [i] to open wizard.")
				return m, nil
			case "enter", "n", "right", "l":
				m.tourStep++
				if m.tourStep >= 4 {
					m.tourMode = false
					m.tourStep = 0
					m.setStatus(statusSuccess, "Tour complete. Press [i] to import your first video.")
				}
				return m, nil
			case "b", "left", "h":
				m.tourStep = clamp(m.tourStep-1, 0, 3)
				return m, nil
			default:
				return m, nil
			}
		}
		if m.helpMode {
			switch s {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "?":
				m.helpMode = false
				return m, nil
			default:
				return m, nil
			}
		}
		if m.paletteMode {
			switch s {
			case "ctrl+c":
				return m, tea.Quit
			}
			if m.handlePaletteInput(msg) {
				if m.shouldQuit {
					return m, tea.Quit
				}
				return m, nil
			}
		}
		if m.searchMode {
			switch s {
			case "ctrl+c":
				return m, tea.Quit
			}
			if m.handleSearchInput(msg) {
				return m, nil
			}
		}
		if m.historyMode {
			switch s {
			case "ctrl+c":
				return m, tea.Quit
			case "esc", "J":
				m.historyMode = false
				return m, nil
			case "up", "k":
				m.historyCursor = clamp(m.historyCursor-1, 0, max(0, len(m.jobHistory)-1))
				return m, nil
			case "down", "j":
				m.historyCursor = clamp(m.historyCursor+1, 0, max(0, len(m.jobHistory)-1))
				return m, nil
			case "enter":
				if len(m.jobHistory) == 0 {
					return m, nil
				}
				row := m.jobHistory[clamp(m.historyCursor, 0, len(m.jobHistory)-1)]
				target := strings.TrimSpace(row.OutputPath)
				if target == "" {
					target = strings.TrimSpace(row.LogPath)
				}
				if target == "" {
					m.setStatus(statusWarn, "No path available for selected history entry.")
					return m, nil
				}
				if err := openPath(target); err != nil {
					m.setError(err.Error())
				} else {
					m.setStatus(statusSuccess, "Opened selected history artifact.")
				}
				return m, nil
			default:
				return m, nil
			}
		}
		if m.consumeDropPaste(msg) {
			return m, nil
		}
		switch s {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			if strings.TrimSpace(m.pendingDrop) != "" {
				m.pendingDrop = ""
				m.setStatus(statusInfo, "Import buffer cleared.")
				return m, nil
			}
		case "a":
			m.simpleMode = !m.simpleMode
			if m.simpleMode && m.tab == tabTimeline {
				m.tab = tabOverview
			}
			if err := m.persistUIPreferences(false); err != nil {
				m.setError(err.Error())
				return m, nil
			}
			if m.simpleMode {
				m.setStatus(statusInfo, "Simple mode on. Beginner-friendly controls are now prioritized.")
			} else {
				m.setStatus(statusInfo, "Advanced mode on. More controls and power-user shortcuts are available.")
			}
			return m, nil
		case "m":
			m.vimMode = !m.vimMode
			if err := m.persistUIPreferences(false); err != nil {
				m.setError(err.Error())
				return m, nil
			}
			if m.vimMode {
				m.setStatus(statusInfo, "Vim mode enabled. Compact keymap hints active.")
			} else {
				m.setStatus(statusInfo, "Vim mode disabled. Standard keymap hints active.")
			}
			return m, nil
		case "v":
			m.themePreset = rotateChoice([]string{themeDefault, themeLight, themeHighContrast}, m.themePreset, 1)
			m.styles = makeStyles(m.themePreset)
			m.list.Styles.Title = m.styles.title
			if err := m.persistUIPreferences(false); err != nil {
				m.setError(err.Error())
				return m, nil
			}
			m.setStatus(statusInfo, "Theme changed.")
			return m, nil
		case "e":
			if strings.TrimSpace(m.errorDetail) != "" {
				m.errorExpanded = !m.errorExpanded
				return m, nil
			}
			m.setStatus(statusInfo, "No error details to show.")
			return m, nil
		case "?":
			m.helpMode = !m.helpMode
			m.paletteMode = false
			m.searchMode = false
			m.clearError()
			return m, nil
		case "ctrl+k":
			m.paletteMode = true
			m.helpMode = false
			m.searchMode = false
			m.shouldQuit = false
			m.paletteIndex = 0
			m.clearError()
			return m, nil
		case "g":
			m.tourMode = true
			m.tourStep = 0
			m.helpMode = false
			m.paletteMode = false
			m.searchMode = false
			return m, nil
		case "J":
			m.historyMode = !m.historyMode
			if m.historyMode {
				m.helpMode = false
				m.paletteMode = false
				m.searchMode = false
				m.historyCursor = clamp(m.historyCursor, 0, max(0, len(m.jobHistory)-1))
			}
			return m, nil
		case "ctrl+f":
			if m.tab != tabTranscript {
				m.setError("Search works in Transcript only.")
				return m, nil
			}
			selected, ok := m.selectedRun()
			if !ok || len(selected.segments) == 0 {
				m.setError("No transcript lines are available for this run.")
				return m, nil
			}
			m.searchMode = true
			m.clearError()
			m.searchPos = 0
			m.updateSearchMatches(true)
			return m, nil
		case "enter":
			if m.focus == focusList {
				m.focus = focusDetail
			} else {
				m.focus = focusList
			}
			return m, nil
		case "tab", "l", "right":
			m.tab = m.nextTab(1)
			m.detailOffset = 0
			m.searchMode = false
			m.clearError()
			return m, nil
		case "shift+tab", "h", "left":
			m.tab = m.nextTab(-1)
			m.detailOffset = 0
			m.searchMode = false
			m.clearError()
			return m, nil
		case "r":
			m.refreshRuns()
			m.setStatus(statusSuccess, "Run list refreshed.")
			return m, nil
		case "i":
			m.openDropModal(strings.TrimSpace(m.pendingDrop))
			m.pendingDrop = ""
			return m, nil
		case "o":
			if selected, ok := m.selectedRun(); ok {
				m.clearError()
				if err := openPath(selected.path); err != nil {
					m.setError(err.Error())
					return m, nil
				}
				m.setStatus(statusSuccess, "Opened run folder.")
			}
			return m, nil
		case "t":
			if selected, ok := m.selectedRun(); ok {
				m.clearError()
				if selected.transcriptPath == "" {
					m.setError("Transcript file was not found for this run.")
					return m, nil
				}
				if err := openPath(selected.transcriptPath); err != nil {
					m.setError(err.Error())
					return m, nil
				}
				m.setStatus(statusSuccess, "Opened transcript file.")
			}
			return m, nil
		case "s":
			if selected, ok := m.selectedRun(); ok {
				m.clearError()
				if selected.contactSheet == "" {
					m.setError("Contact sheet was not found for this run.")
					return m, nil
				}
				if err := openPath(selected.contactSheet); err != nil {
					m.setError(err.Error())
					return m, nil
				}
				m.setStatus(statusSuccess, "Opened contact sheet.")
			}
			return m, nil
		case "f":
			if selected, ok := m.selectedRun(); ok {
				m.clearError()
				framePath, err := selectedSegmentFramePath(selected, m.selectedSeg)
				if err != nil {
					m.setError(err.Error())
					return m, nil
				}
				if err := openPath(framePath); err != nil {
					m.setError(err.Error())
					return m, nil
				}
				m.setStatus(statusSuccess, "Opened frame image.")
			}
			return m, nil
		}

		if m.focus == focusDetail {
			h := m.detailHeight()
			maxOffset := m.maxDetailOffset()
			switch s {
			case "j", "down":
				if m.detailOffset < maxOffset {
					m.detailOffset++
				}
				return m, nil
			case "k", "up":
				if m.detailOffset > 0 {
					m.detailOffset--
				}
				return m, nil
			case "pgdown":
				m.detailOffset = min(maxOffset, m.detailOffset+h)
				return m, nil
			case "pgup":
				m.detailOffset = max(0, m.detailOffset-h)
				return m, nil
			case "g":
				m.detailOffset = 0
				return m, nil
			case "G":
				m.detailOffset = maxOffset
				return m, nil
			case "n":
				m.moveSegment(1)
				return m, nil
			case "p":
				m.moveSegment(-1)
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.winW = msg.Width
		m.winH = msg.Height
		m.compactMode = m.isCompactLayout()
		listW := max(28, int(math.Round(float64(msg.Width)*0.35)))
		if m.compactMode {
			listW = max(24, msg.Width-10)
		}
		listH := max(7, msg.Height-12)
		if m.compactMode {
			listH = max(5, msg.Height/3)
		}
		m.list.SetSize(max(20, listW-4), listH)
	}

	prevSelection := m.lastSelection
	if m.welcomeMode {
		return m, nil
	}
	if m.focus == focusList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if selected, ok := m.selectedRun(); ok {
			m.lastSelection = selected.title
		} else {
			m.lastSelection = ""
		}
		if prevSelection != m.lastSelection {
			m.detailOffset = 0
			m.selectedSeg = 0
			m.searchMatches = nil
			m.searchPos = 0
			m.clearError()
		}
		return m, cmd
	}
	return m, nil
}

func (m dashboard) View() string {
	if m.winW < 54 || m.winH < 12 {
		panel := m.styles.panel.Width(max(24, m.winW-6)).Render(
			m.styles.title.Render(smallScreenTitle) + "\n" +
				m.styles.subtle.Render("Terminal is too small for full layout.") + "\n" +
				m.styles.subtle.Render("Fallback: use `framescli extract <video>` in CLI mode.") + "\n" +
				"\n" + "[q] Quit  [r] Refresh  [a] Simple/Advanced")
		return m.styles.app.Render(panel)
	}
	if m.welcomeMode {
		return m.renderWelcomeScreen()
	}

	header := m.styles.title.Render(smallScreenTitle) + " " + m.styles.badge.Render(strings.ToUpper(m.themePreset))
	if m.focus == focusDetail {
		header += " " + m.styles.badgeActive.Render("DETAIL")
	} else {
		header += " " + m.styles.badge.Render("LIST")
	}
	header += "\n" + m.styles.subtle.Render(fmt.Sprintf("Root: %s | Source: %s", m.rootDir, m.dataSource))
	header += "\n" + m.renderStatusStrip()
	if cue := m.renderDropCueStrip(); cue != "" {
		header += "\n" + cue
	}

	leftStyle := m.styles.panel
	rightStyle := m.styles.panel
	if m.focus == focusList {
		leftStyle = m.styles.panelFocus
	} else {
		rightStyle = m.styles.panelFocus
	}

	body := ""
	if m.compactMode {
		panelW := max(26, m.winW-8)
		listBox := leftStyle.Width(panelW).Render(m.list.View())
		detailBox := rightStyle.Width(panelW).Render(m.renderDetailPanel(panelW - 4))
		body = lipgloss.JoinVertical(lipgloss.Left, listBox, "", detailBox)
	} else {
		totalWidth := max(72, m.winW-8)
		leftWidth := int(math.Round(float64(totalWidth) * 0.35))
		rightWidth := totalWidth - leftWidth - 2
		leftBox := leftStyle.Width(max(24, leftWidth)).Render(m.list.View())
		rightBox := rightStyle.Width(max(34, rightWidth)).Render(m.renderDetailPanel(max(30, rightWidth-4)))
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightBox)
	}

	parts := []string{header, "", body}
	if m.helpMode {
		parts = append(parts, "", m.renderHelpPanel())
	}
	if m.tourMode {
		parts = append(parts, "", m.renderTourPanel())
	}
	if m.historyMode {
		parts = append(parts, "", m.renderHistoryPanel())
	}
	if m.paletteMode {
		parts = append(parts, "", m.renderPalettePanel())
	}
	if m.errorExpanded && strings.TrimSpace(m.errorDetail) != "" {
		parts = append(parts, "", m.renderErrorPanel())
	}
	if m.dropModal {
		parts = append(parts, "", m.renderDropModal())
	}
	if strings.TrimSpace(m.dataHint) != "" {
		parts = append(parts, m.styles.subtle.Render(m.dataHint))
	}
	parts = append(parts, "", m.renderCommandBar())
	return m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m dashboard) renderPalettePanel() string {
	actions := m.filteredPaletteActions()
	lines := []string{
		m.styles.title.Render("Command Menu"),
		m.styles.subtle.Render("Type to filter commands | up/down or j/k move | Enter runs | Esc closes"),
		fmt.Sprintf("query: %q", m.paletteQuery),
		"",
	}
	if len(actions) == 0 {
		lines = append(lines, m.styles.subtle.Render("No matching commands."))
	} else {
		limit := min(len(actions), 10)
		for i := 0; i < limit; i++ {
			marker := " "
			if i == clamp(m.paletteIndex, 0, len(actions)-1) {
				marker = ">"
			}
			lines = append(lines, fmt.Sprintf("%s %s", marker, actions[i].Label))
		}
		if len(actions) > limit {
			lines = append(lines, m.styles.subtle.Render(fmt.Sprintf("... %d more", len(actions)-limit)))
		}
	}
	panelWidth := 72
	if m.winW > 0 {
		panelWidth = min(max(40, m.winW-14), 92)
	}
	return m.styles.panelFocus.Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func (m dashboard) renderHelpPanel() string {
	modeLine := "Keymap: Standard (arrows + vim-style aliases)"
	moveLine := "[Up/Down] Move"
	sectionLine := "[Tab/Left/Right,h,l] Switch detail section"
	if m.vimMode {
		modeLine = "Keymap: Vim mode (compact hints enabled)"
		moveLine = "[j/k] Move"
		sectionLine = "[h/l] Switch detail section"
	}
	lines := []string{
		m.styles.title.Render("FramesCLI Help"),
		m.styles.subtle.Render("Press Esc or ? to close this panel."),
		m.styles.subtle.Render(modeLine),
		"",
		"Getting Started",
		"---------------",
		"1) Press [i] to import a video.",
		"2) Use [Up/Down] to choose a run.",
		"3) Use [Left/Right] to switch sections.",
		"4) Press [Enter] to move focus into details.",
		"",
		"Global",
		"------",
		"[?] Help panel",
		"[a] Simple/Advanced",
		"[m] Toggle vim mode",
		"[v] Switch theme",
		"[/] Filter runs",
		"[Enter] Move focus list/detail",
		sectionLine,
		moveLine,
		"[r] Refresh runs",
		"[i] Import video",
		"[c] Cancel active extraction",
		"[g] Guided tour",
		"[J] Job history",
		"[q] Quit",
		"",
		"Transcript",
		"----------",
		"[Ctrl+f] Find in transcript",
		"[Enter/Ctrl+n] Next result",
		"[Ctrl+p] Previous result",
		"[n/p] Next/previous segment",
		"[f] Open selected frame",
		"",
		"Files",
		"-----",
		"[o] Open run folder",
		"[t] Open transcript",
		"[s] Open contact sheet",
	}
	panelWidth := 72
	if m.winW > 0 {
		panelWidth = min(max(40, m.winW-14), 92)
	}
	return m.styles.panelFocus.Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func (m dashboard) renderHistoryPanel() string {
	lines := []string{
		m.styles.title.Render("Job History"),
		m.styles.subtle.Render("j/k move | Enter opens output/log | Esc closes"),
		"",
	}
	if len(m.jobHistory) == 0 {
		lines = append(lines, m.styles.subtle.Render("No job history entries yet."))
	} else {
		limit := min(len(m.jobHistory), 12)
		for i := 0; i < limit; i++ {
			row := m.jobHistory[i]
			marker := " "
			if i == clamp(m.historyCursor, 0, len(m.jobHistory)-1) {
				marker = ">"
			}
			label := fmt.Sprintf("%s [%s] %s | %s | %s", marker, strings.ToUpper(row.Status), oneLine(row.VideoPath), oneLine(valueOrDash(row.OutputPath)), humanDuration(row.DurationSec))
			lines = append(lines, label)
		}
		if len(m.jobHistory) > limit {
			lines = append(lines, m.styles.subtle.Render(fmt.Sprintf("... %d more", len(m.jobHistory)-limit)))
		}
	}
	panelWidth := 96
	if m.winW > 0 {
		panelWidth = min(max(52, m.winW-12), 120)
	}
	return m.styles.panelFocus.Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func (m dashboard) renderTourPanel() string {
	steps := []struct {
		title string
		body  []string
	}{
		{
			title: "Tour 1/4: Wizard",
			body: []string{
				"Press [i] to open the extraction wizard.",
				"Choose mode, profile, then review before running.",
				"Tip: profiles tune defaults quickly.",
			},
		},
		{
			title: "Tour 2/4: Queue",
			body: []string{
				"In Review, use queue actions to enqueue tasks.",
				"Run queue executes jobs sequentially with shared progress.",
			},
		},
		{
			title: "Tour 3/4: History",
			body: []string{
				"Press [J] for Job History.",
				"Use j/k and Enter to open output/log paths.",
			},
		},
		{
			title: "Tour 4/4: Fast Keys",
			body: []string{
				"[c] cancel active extraction",
				"[Ctrl+k] command menu",
				"[?] help",
			},
		},
	}
	idx := clamp(m.tourStep, 0, len(steps)-1)
	lines := []string{
		m.styles.title.Render("First-Run Guided Tour"),
		m.styles.subtle.Render(steps[idx].title),
		"",
	}
	lines = append(lines, steps[idx].body...)
	lines = append(lines, "", m.styles.subtle.Render("[Enter]/[n] next  [b] back  [q]/[esc] close"))
	panelWidth := 74
	if m.winW > 0 {
		panelWidth = min(max(46, m.winW-12), 96)
	}
	return m.styles.panelFocus.Width(panelWidth).Render(strings.Join(lines, "\n"))
}

func (m dashboard) renderDetailPanel(width int) string {
	selected, ok := m.selectedRun()
	if !ok {
		return m.styles.detail.Render("No run selected.")
	}
	tabs := m.renderTabs()
	lines := m.detailLines(selected)
	visible := m.visibleDetailLines(lines, max(20, width))
	status := m.styles.subtle.Render(fmt.Sprintf("lines %d-%d of %d", m.detailOffset+1, min(m.detailOffset+len(visible), len(lines)), len(lines)))
	return tabs + "\n\n" + strings.Join(visible, "\n") + "\n\n" + status
}

func (m dashboard) renderTabs() string {
	names := m.currentTabs()
	parts := make([]string, 0, len(names))
	for _, name := range names {
		tab := tabFromName(name)
		if tab == m.tab {
			parts = append(parts, m.styles.tabActive.Render(name))
			continue
		}
		parts = append(parts, m.styles.tab.Render(name))
	}
	return strings.Join(parts, " ")
}

func (m dashboard) detailLines(it runItem) []string {
	switch m.tab {
	case tabOverview:
		return m.overviewLines(it)
	case tabTranscript:
		return m.transcriptLines(it)
	case tabTimeline:
		return m.timelineLines(it)
	case tabFiles:
		return m.fileLines(it)
	default:
		return m.overviewLines(it)
	}
}

func (m dashboard) overviewLines(it runItem) []string {
	rows := []string{
		m.metric("Run", it.title),
		m.metric("Frames", fmt.Sprintf("%d", it.frameCount)),
		m.metric("Updated", humanTime(it.updatedAt)),
		m.metric("Path", it.path),
		m.metric("Frame Range", frameRange(it.firstFrame, it.lastFrame)),
		m.metric("Segments", fmt.Sprintf("%d", len(it.segments))),
	}
	if it.metadataPresent && it.metadata != nil {
		rows = append(rows,
			m.metric("FPS", fmt.Sprintf("%.2f", it.metadata.FPS)),
			m.metric("Format", it.metadata.FrameFormat),
			m.metric("Duration", humanDuration(it.metadata.DurationSec)),
			m.metric("Voice", yesNo(it.metadata.ExtractedAudio)),
		)
	}
	return rows
}

func (m dashboard) transcriptLines(it runItem) []string {
	if it.transcriptPath == "" {
		return []string{m.styles.subtle.Render("No transcript available for this run.")}
	}
	lines := []string{m.metric("Transcript File", it.transcriptPath), ""}
	if len(it.segments) == 0 {
		lines = append(lines, "Transcript Text", "---------------")
		lines = append(lines, strings.Split(trimText(it.transcript, 2600), "\n")...)
		return lines
	}
	selected := clamp(m.selectedSeg, 0, len(it.segments)-1)
	seg := it.segments[selected]
	matchCount := len(m.searchMatches)
	matchLabel := "-"
	if matchCount > 0 {
		matchLabel = fmt.Sprintf("%d", matchCount)
	}
	lines = append(lines,
		m.metric("Segments", fmt.Sprintf("%d", len(it.segments))),
		m.metric("Selected", fmt.Sprintf("%d", selected+1)),
		m.metric("Window", formatSeconds(seg.Start)+" -> "+formatSeconds(seg.End)),
		m.metric("Search", dashIfEmpty(strings.TrimSpace(m.searchQuery))),
		m.metric("Matches", matchLabel),
	)
	if fps := runFPS(it); fps > 0 {
		startFrame, endFrame := segmentFrameRange(seg, fps, it.frameCount)
		lines = append(lines, m.metric("Frame Est", fmt.Sprintf("%d -> %d", startFrame, endFrame)))
	}
	lines = append(lines, "", "Segments (n/p to move)", "---------------------")
	matched := make(map[int]struct{}, len(m.searchMatches))
	for _, idx := range m.searchMatches {
		matched[idx] = struct{}{}
	}
	for i, s := range it.segments {
		marker := " "
		_, isMatch := matched[i]
		if i == selected && isMatch {
			marker = "@"
		} else if i == selected {
			marker = ">"
		} else if isMatch {
			marker = "*"
		}
		line := fmt.Sprintf("%s %3d  %s - %s  %s", marker, i+1, formatSeconds(s.Start), formatSeconds(s.End), oneLine(s.Text))
		lines = append(lines, line)
	}
	return lines
}

func (m dashboard) timelineLines(it runItem) []string {
	fps := runFPS(it)
	if fps <= 0 || it.frameCount == 0 {
		return []string{m.styles.subtle.Render("Timeline unavailable. Extract with metadata (fps) to enable timeline view.")}
	}
	points := 16
	if it.frameCount < points {
		points = it.frameCount
	}
	if points < 2 {
		points = 2
	}
	lines := []string{m.metric("FPS", fmt.Sprintf("%.2f", fps)), m.metric("Frames", fmt.Sprintf("%d", it.frameCount)), "", "Sampled frame timeline", "--------------------"}
	for i := 0; i < points; i++ {
		idx := 1 + int(math.Round(float64(i)*float64(it.frameCount-1)/float64(points-1)))
		sec := float64(idx-1) / fps
		label := fmt.Sprintf("%4d  %s", idx, formatSeconds(sec))
		if i%2 == 0 {
			label += "  *"
		}
		lines = append(lines, label)
	}
	if len(it.segments) > 0 {
		lines = append(lines, "", "Segment anchors", "---------------")
		for i, seg := range it.segments {
			startFrame, endFrame := segmentFrameRange(seg, fps, it.frameCount)
			lines = append(lines, fmt.Sprintf("%3d  %s -> %s  frames %d-%d", i+1, formatSeconds(seg.Start), formatSeconds(seg.End), startFrame, endFrame))
		}
	}
	return lines
}

func (m dashboard) fileLines(it runItem) []string {
	lines := []string{
		m.metric("Run", it.path),
		m.metric("Transcript", dashIfEmpty(it.transcriptPath)),
		m.metric("Contact Sheet", dashIfEmpty(it.contactSheet)),
		m.metric("Frames Dir", filepath.Join(it.path, "images")),
	}
	if it.metadataPresent {
		lines = append(lines, m.metric("Metadata", media.MetadataPathForRun(it.path)))
	}
	jsonPath := filepath.Join(it.path, "voice", "transcript.json")
	if _, err := os.Stat(jsonPath); err == nil {
		lines = append(lines, m.metric("Transcript JSON", jsonPath))
	}
	return lines
}

func (m dashboard) visibleDetailLines(lines []string, width int) []string {
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapLine(line, max(20, width))...)
	}
	h := m.detailHeight()
	if len(wrapped) == 0 {
		return []string{""}
	}
	maxOffset := max(0, len(wrapped)-h)
	if m.detailOffset > maxOffset {
		m.detailOffset = maxOffset
	}
	start := clamp(m.detailOffset, 0, maxOffset)
	end := min(len(wrapped), start+h)
	return wrapped[start:end]
}

func (m dashboard) detailHeight() int {
	return max(10, m.winH-14)
}

func (m dashboard) maxDetailOffset() int {
	selected, ok := m.selectedRun()
	if !ok {
		return 0
	}
	lines := m.detailLines(selected)
	wrappedCount := 0
	for _, line := range lines {
		wrappedCount += len(wrapLine(line, max(20, m.winW/2)))
	}
	return max(0, wrappedCount-m.detailHeight())
}

func (m *dashboard) moveSegment(delta int) {
	if m.tab != tabTranscript && m.tab != tabTimeline {
		return
	}
	selected, ok := m.selectedRun()
	if !ok || len(selected.segments) == 0 {
		return
	}
	m.selectedSeg = clamp(m.selectedSeg+delta, 0, len(selected.segments)-1)
	m.detailOffset = 0
}

func (m *dashboard) handleSearchInput(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchMode = false
		m.clearError()
		return true
	case tea.KeyEnter:
		m.advanceSearchMatch(1)
		return true
	case tea.KeyCtrlN:
		m.advanceSearchMatch(1)
		return true
	case tea.KeyCtrlP:
		m.advanceSearchMatch(-1)
		return true
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(m.searchQuery)
		if len(runes) > 0 {
			m.searchQuery = string(runes[:len(runes)-1])
		}
		m.searchPos = 0
		m.updateSearchMatches(true)
		return true
	default:
		if len(msg.Runes) == 0 || msg.Alt {
			return false
		}
		m.searchQuery += string(msg.Runes)
		m.searchPos = 0
		m.updateSearchMatches(true)
		return true
	}
}

func (m *dashboard) handlePaletteInput(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		m.paletteMode = false
		m.paletteQuery = ""
		m.paletteIndex = 0
		return true
	case tea.KeyEnter:
		actions := m.filteredPaletteActions()
		if len(actions) == 0 {
			m.setError("No matching command in menu.")
			return true
		}
		action := actions[clamp(m.paletteIndex, 0, len(actions)-1)]
		if err := m.executePaletteAction(action.ID); err != nil {
			m.setError(err.Error())
			return true
		}
		m.paletteMode = false
		m.paletteQuery = ""
		m.paletteIndex = 0
		m.clearError()
		m.setStatus(statusSuccess, "Command completed.")
		return true
	case tea.KeyUp:
		actions := m.filteredPaletteActions()
		if len(actions) > 0 {
			m.paletteIndex = clamp(m.paletteIndex-1, 0, len(actions)-1)
		}
		return true
	case tea.KeyDown:
		actions := m.filteredPaletteActions()
		if len(actions) > 0 {
			m.paletteIndex = clamp(m.paletteIndex+1, 0, len(actions)-1)
		}
		return true
	case tea.KeyBackspace, tea.KeyDelete:
		runes := []rune(m.paletteQuery)
		if len(runes) > 0 {
			m.paletteQuery = string(runes[:len(runes)-1])
		}
		m.paletteIndex = 0
		return true
	default:
		if msg.String() == "j" {
			actions := m.filteredPaletteActions()
			if len(actions) > 0 {
				m.paletteIndex = clamp(m.paletteIndex+1, 0, len(actions)-1)
			}
			return true
		}
		if msg.String() == "k" {
			actions := m.filteredPaletteActions()
			if len(actions) > 0 {
				m.paletteIndex = clamp(m.paletteIndex-1, 0, len(actions)-1)
			}
			return true
		}
		if len(msg.Runes) == 0 || msg.Alt {
			return false
		}
		m.paletteQuery += string(msg.Runes)
		m.paletteIndex = 0
		return true
	}
}

func (m dashboard) paletteActions() []paletteAction {
	out := []paletteAction{
		{ID: "focus_list", Label: "Focus: List Pane"},
		{ID: "focus_detail", Label: "Focus: Detail Pane"},
		{ID: "tab_overview", Label: "Tab: Overview"},
		{ID: "tab_transcript", Label: "Tab: Transcript"},
		{ID: "tab_files", Label: "Tab: Files"},
		{ID: "segment_next", Label: "Segment: Next"},
		{ID: "segment_prev", Label: "Segment: Previous"},
		{ID: "run_open", Label: "Open: Run Folder"},
		{ID: "run_open_transcript", Label: "Open: Transcript"},
		{ID: "run_open_sheet", Label: "Open: Contact Sheet"},
		{ID: "run_open_frame", Label: "Open: Selected Segment Frame"},
		{ID: "toggle_mode", Label: "Toggle: Simple/Advanced"},
		{ID: "toggle_vim", Label: "Toggle: Vim Mode"},
		{ID: "toggle_theme", Label: "Toggle: Theme Preset"},
		{ID: "refresh", Label: "Refresh Runs"},
		{ID: "quit", Label: "Quit FramesCLI TUI"},
	}
	if !m.simpleMode {
		out = append(out, paletteAction{ID: "tab_timeline", Label: "Tab: Timeline"})
	}
	return out
}

func (m dashboard) filteredPaletteActions() []paletteAction {
	actions := m.paletteActions()
	q := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	if q == "" {
		return actions
	}
	out := make([]paletteAction, 0, len(actions))
	for _, a := range actions {
		if strings.Contains(strings.ToLower(a.Label), q) || strings.Contains(strings.ToLower(a.ID), q) {
			out = append(out, a)
		}
	}
	return out
}

func (m *dashboard) executePaletteAction(id string) error {
	switch id {
	case "focus_list":
		m.focus = focusList
		return nil
	case "focus_detail":
		m.focus = focusDetail
		return nil
	case "tab_overview":
		m.tab = tabOverview
		m.detailOffset = 0
		return nil
	case "tab_transcript":
		m.tab = tabTranscript
		m.detailOffset = 0
		return nil
	case "tab_timeline":
		if m.simpleMode {
			return fmt.Errorf("timeline is hidden in simple mode")
		}
		m.tab = tabTimeline
		m.detailOffset = 0
		return nil
	case "tab_files":
		m.tab = tabFiles
		m.detailOffset = 0
		return nil
	case "segment_next":
		m.moveSegment(1)
		return nil
	case "segment_prev":
		m.moveSegment(-1)
		return nil
	case "refresh":
		m.refreshRuns()
		return nil
	case "toggle_mode":
		m.simpleMode = !m.simpleMode
		if m.simpleMode && m.tab == tabTimeline {
			m.tab = tabOverview
		}
		if err := m.persistUIPreferences(false); err != nil {
			return err
		}
		return nil
	case "toggle_vim":
		m.vimMode = !m.vimMode
		if err := m.persistUIPreferences(false); err != nil {
			return err
		}
		return nil
	case "toggle_theme":
		m.themePreset = rotateChoice([]string{themeDefault, themeLight, themeHighContrast}, m.themePreset, 1)
		m.styles = makeStyles(m.themePreset)
		m.list.Styles.Title = m.styles.title
		if err := m.persistUIPreferences(false); err != nil {
			return err
		}
		return nil
	case "run_open":
		selected, ok := m.selectedRun()
		if !ok {
			return fmt.Errorf("no run selected")
		}
		return openPath(selected.path)
	case "run_open_transcript":
		selected, ok := m.selectedRun()
		if !ok {
			return fmt.Errorf("no run selected")
		}
		if selected.transcriptPath == "" {
			return fmt.Errorf("transcript not found")
		}
		return openPath(selected.transcriptPath)
	case "run_open_sheet":
		selected, ok := m.selectedRun()
		if !ok {
			return fmt.Errorf("no run selected")
		}
		if selected.contactSheet == "" {
			return fmt.Errorf("contact sheet not found")
		}
		return openPath(selected.contactSheet)
	case "run_open_frame":
		selected, ok := m.selectedRun()
		if !ok {
			return fmt.Errorf("no run selected")
		}
		framePath, err := selectedSegmentFramePath(selected, m.selectedSeg)
		if err != nil {
			return err
		}
		return openPath(framePath)
	case "quit":
		m.shouldQuit = true
		return nil
	default:
		return fmt.Errorf("unknown palette action: %s", id)
	}
}

func (m *dashboard) updateSearchMatches(jump bool) {
	selected, ok := m.selectedRun()
	if !ok || len(selected.segments) == 0 {
		m.searchMatches = nil
		m.setError("No transcript lines are available for this run.")
		return
	}
	m.searchMatches = findSegmentMatches(selected.segments, m.searchQuery)
	if len(strings.TrimSpace(m.searchQuery)) == 0 {
		m.clearError()
		return
	}
	if len(m.searchMatches) == 0 {
		m.setError(fmt.Sprintf("No transcript matches %q.", m.searchQuery))
		return
	}
	m.clearError()
	if m.searchPos < 0 || m.searchPos >= len(m.searchMatches) {
		m.searchPos = 0
	}
	if jump {
		m.selectedSeg = m.searchMatches[m.searchPos]
		m.detailOffset = 0
	}
}

func (m *dashboard) advanceSearchMatch(delta int) {
	if len(strings.TrimSpace(m.searchQuery)) == 0 {
		m.setError("Search box is empty.")
		return
	}
	m.updateSearchMatches(false)
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchPos = (m.searchPos + delta) % len(m.searchMatches)
	if m.searchPos < 0 {
		m.searchPos += len(m.searchMatches)
	}
	m.selectedSeg = m.searchMatches[m.searchPos]
	m.detailOffset = 0
}

func (m dashboard) metric(keyLabel string, value string) string {
	return m.styles.metricKey.Render(keyLabel+":") + " " + m.styles.metricValue.Render(value)
}

func (m *dashboard) refreshRuns() {
	m.scanning = true
	currentTitle := ""
	if selected, ok := m.selectedRun(); ok {
		currentTitle = selected.title
	}
	items, source, hint, err := loadRuns(m.rootDir)
	m.scanning = false
	if err != nil {
		m.setError(err.Error())
		return
	}
	m.dataSource = source
	m.dataHint = hint
	m.itemCount = len(items)
	m.list.SetItems(toListItems(items))
	if currentTitle == "" {
		return
	}
	for i, item := range items {
		if item.title == currentTitle {
			m.list.Select(i)
			m.lastSelection = item.title
			return
		}
	}
	m.lastSelection = ""
}

func (m dashboard) currentTabs() []string {
	if m.simpleMode {
		return simpleTabNames
	}
	return allTabNames
}

func (m dashboard) nextTab(delta int) detailTab {
	tabs := m.currentTabs()
	current := 0
	for i, name := range tabs {
		if tabFromName(name) == m.tab {
			current = i
			break
		}
	}
	next := (current + delta) % len(tabs)
	if next < 0 {
		next += len(tabs)
	}
	return tabFromName(tabs[next])
}

func (m dashboard) isCompactLayout() bool {
	return m.winW > 0 && (m.winW < 110 || m.winH < 28)
}

func (m dashboard) renderStatusStrip() string {
	spin := ""
	if m.scanning {
		spin = []string{"|", "/", "-", `\`}[m.statusPulse%4] + " "
	}
	text := spin + m.statusText
	if strings.TrimSpace(m.errorShort) != "" {
		text = m.errorShort + " [e] Details"
	}
	switch m.statusLevel {
	case statusSuccess:
		return m.styles.statusOk.Width(max(20, m.winW-8)).Render(text)
	case statusWarn:
		return m.styles.statusWarn.Width(max(20, m.winW-8)).Render(text)
	case statusError:
		return m.styles.statusErr.Width(max(20, m.winW-8)).Render(text)
	default:
		return m.styles.statusInfo.Width(max(20, m.winW-8)).Render(text)
	}
}

func (m dashboard) renderDropCueStrip() string {
	if strings.TrimSpace(m.pendingDrop) == "" {
		return ""
	}
	candidate := normalizeDroppedPath(m.pendingDrop)
	label := trimForModal(candidate, 80)
	if looksLikeVideoPath(candidate) {
		return m.styles.statusOk.Width(max(20, m.winW-8)).Render("Video detected: " + label + "  [Enter] Configure  [Esc] Clear")
	}
	return m.styles.statusWarn.Width(max(20, m.winW-8)).Render("Import buffer: " + label + "  not a recognized video path yet")
}

func (m dashboard) renderCommandBar() string {
	hintViews := make([]string, 0, 12)
	for _, h := range m.visibleCommandHints() {
		hintViews = append(hintViews, m.hint(h.key, h.text))
	}
	return m.styles.commandBar.Width(max(20, m.winW-8)).Render(strings.Join(hintViews, "  "))
}

func (m dashboard) hint(key, text string) string {
	return m.styles.commandKey.Render(key) + " " + m.styles.commandText.Render(text)
}

func (m dashboard) renderWelcomeModal() string {
	modeLabel := "Simple"
	quickKeys := "Quick keys: [Up/Down] browse, [Left/Right] switch sections, [Enter] focus details, [q] quit."
	if !m.simpleMode {
		modeLabel = "Advanced"
		quickKeys = "Quick keys: [Up/Down] browse, [Left/Right] switch sections, [Enter] focus, [Ctrl+k] command menu."
	}
	lines := []string{
		m.styles.modalTitle.Render("Welcome to FramesCLI"),
		"",
		"FramesCLI helps you import recordings, extract frames, and review artifacts locally.",
		"Start simple: import a video, then browse the generated run and transcript.",
		"",
		fmt.Sprintf("Current mode: %s", modeLabel),
		"Current theme: " + m.themePreset,
		quickKeys,
		"",
		"[Enter] Continue    [i] Import video    [a] Toggle Simple/Advanced    [v] Change theme",
	}
	w := min(max(44, m.winW-18), 88)
	return m.styles.modal.Width(w).Render(strings.Join(lines, "\n"))
}

func (m dashboard) renderWelcomeScreen() string {
	title := m.styles.title.Render(smallScreenTitle + " Setup")
	sub := m.styles.subtle.Render("One-time onboarding")
	modal := m.renderWelcomeModal()
	body := lipgloss.Place(max(40, m.winW-4), max(14, m.winH-6), lipgloss.Center, lipgloss.Center, modal)
	return m.styles.app.Render(lipgloss.JoinVertical(lipgloss.Left, title, sub, "", body))
}

func (m dashboard) renderErrorPanel() string {
	lines := []string{
		m.styles.modalTitle.Render("Error details"),
		"",
		m.errorDetail,
		"",
		"Press [e] to close details.",
	}
	w := min(max(40, m.winW-12), 100)
	return m.styles.errorPanel.Width(w).Render(strings.Join(lines, "\n"))
}

func (m *dashboard) setStatus(level statusLevel, text string) {
	m.statusLevel = level
	m.statusText = strings.TrimSpace(text)
	if m.statusText == "" {
		m.statusText = "Ready."
	}
}

func (m *dashboard) setError(detail string) {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return
	}
	m.errorDetail = detail
	m.errorShort = shortenSentence(detail, 72)
	m.errorExpanded = false
	m.setStatus(statusError, m.errorShort)
}

func (m *dashboard) clearError() {
	m.errorShort = ""
	m.errorDetail = ""
	m.errorExpanded = false
}

func (m *dashboard) appendJobHistory(msg dropExtractDoneMsg, status string) {
	if status == "" {
		status = "success"
	}
	duration := msg.endedAt.Sub(msg.startedAt).Seconds()
	if duration < 0 {
		duration = 0
	}
	row := jobHistoryEntry{
		StartedAt:      msg.startedAt.Format(time.RFC3339),
		EndedAt:        msg.endedAt.Format(time.RFC3339),
		DurationSec:    duration,
		Status:         status,
		Mode:           msg.mode,
		Profile:        msg.profile,
		VideoPath:      msg.videoPath,
		OutputPath:     msg.out,
		LogPath:        msg.logPath,
		Error:          strings.TrimSpace(msg.errString()),
		FramesProduced: msg.count,
	}
	m.jobHistory = append([]jobHistoryEntry{row}, m.jobHistory...)
	if len(m.jobHistory) > 200 {
		m.jobHistory = m.jobHistory[:200]
	}
	if err := saveJobHistory(m.rootDir, m.jobHistory); err != nil {
		m.setStatus(statusWarn, "Failed to save job history: "+shortenSentence(err.Error(), 60))
	}
}

func (m dashboard) visibleCommandHints() []commandHint {
	if m.vimMode {
		all := []commandHint{
			{key: "[q]", text: "Quit"},
			{key: "[h/l]", text: "Section"},
			{key: "[j/k]", text: "Move"},
			{key: "[Enter]", text: "Focus"},
			{key: "[i]", text: "Import"},
			{key: "[c]", text: "Cancel"},
			{key: "[m]", text: "Vim mode"},
			{key: "[a]", text: "Simple/Adv"},
			{key: "[v]", text: "Theme"},
			{key: "[?]", text: "Help"},
			{key: "[Ctrl+k]", text: "Menu", advanced: true},
			{key: "[/]", text: "Filter", advanced: true},
		}
		out := make([]commandHint, 0, len(all))
		for _, h := range all {
			if h.advanced && m.simpleMode {
				continue
			}
			out = append(out, h)
		}
		return out
	}
	all := []commandHint{
		{key: "[q]", text: "Quit"},
		{key: "[i]", text: "Import video"},
		{key: "[Up/Down]", text: "Browse"},
		{key: "[Left/Right]", text: "Section"},
		{key: "[Enter]", text: "Open detail"},
		{key: "[r]", text: "Refresh"},
		{key: "[J]", text: "History"},
		{key: "[g]", text: "Tour"},
		{key: "[?]", text: "Help"},
		{key: "[a]", text: "Simple/Advanced"},
		{key: "[m]", text: "Vim mode"},
		{key: "[v]", text: "Theme"},
		{key: "[c]", text: "Cancel task"},
		{key: "[Ctrl+f]", text: "Find", advanced: true},
		{key: "[Ctrl+k]", text: "Command menu", advanced: true},
		{key: "[/]", text: "Filter", advanced: true},
		{key: "[e]", text: "Error details", advanced: true},
	}
	out := make([]commandHint, 0, len(all))
	for _, h := range all {
		if h.advanced && m.simpleMode {
			continue
		}
		out = append(out, h)
	}
	return out
}

func (m *dashboard) openDropModal(path string) {
	path = normalizeDroppedPath(path)
	if path == "" {
		path = "(paste path and press Enter after closing this modal)"
	}
	m.dropModal = true
	m.dropPath = path
	m.dropCursor = 0
	m.dropStep = 0
	m.dropRunning = false
	m.dropCancel = nil
	m.dropProgress = 0
	m.dropStage = ""
	m.dropProgressC = nil
	m.dropDoneC = nil
	m.dropVideoInfo = nil
	m.dropPreview = nil
	m.dropEstimate = ""
	m.dropResultOut = ""
	m.dropResultCount = 0
	m.dropResultTranscript = ""
	m.dropResultSheet = ""
	m.dropResultAudio = ""
	m.dropResultErr = ""
	m.dropResultDiagnostics = ""
	m.dropFailedQueue = nil
	m.dropQueueTotal = 0
	m.dropQueueSuccess = 0
	m.dropQueueFailed = 0
	m.dropQueueFailedErrors = nil
	m.dropMode = "both"
	m.dropProfile = "balanced"
	if _, ok := m.presetProfiles[m.dropProfile]; !ok {
		choices := m.profileChoices()
		if len(choices) > 0 {
			m.dropProfile = choices[0]
		}
	}
	m.applyDropProfile()
	if info, err := media.ProbeVideoInfo(path); err == nil {
		infoCopy := info
		m.dropVideoInfo = &infoCopy
	}
	if strings.TrimSpace(m.dropFPSInput) == "" {
		m.dropFPSInput = "4.00"
	}
	if m.dropFormat == "" {
		m.dropFormat = "png"
	}
	if m.dropPreset == "" {
		m.dropPreset = "balanced"
	}
	if m.dropSheetCols <= 0 {
		m.dropSheetCols = 6
	}
	if m.dropPreviewSamples <= 0 {
		m.dropPreviewSamples = 4
	}
	if m.dropPreviewWidth <= 0 {
		m.dropPreviewWidth = 24
	}
}

func (m *dashboard) handleDropModalInput(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.dropRunning {
		switch msg.String() {
		case "c", "x", "esc", "ctrl+c":
			if m.dropCancel != nil {
				m.dropCancel()
				m.setStatus(statusWarn, "Cancel requested. Waiting for ffmpeg to stop...")
			}
			return true, nil
		}
		return true, nil
	}
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		m.dropModal = false
		return true, nil
	case "up", "k":
		m.dropCursor = clamp(m.dropCursor-1, 0, m.dropMaxCursor())
		return true, nil
	case "down", "j":
		m.dropCursor = clamp(m.dropCursor+1, 0, m.dropMaxCursor())
		return true, nil
	case "left", "h":
		m.adjustDropField(-1)
		return true, nil
	case "right", "l":
		m.adjustDropField(1)
		return true, nil
	case "backspace":
		if m.dropStep == 1 && m.dropCursor == 0 {
			r := []rune(m.dropFPSInput)
			if len(r) > 0 {
				m.dropFPSInput = string(r[:len(r)-1])
			}
		}
		return true, nil
	case "enter":
		cmd, ok := m.handleDropEnter()
		if ok {
			return true, cmd
		}
		m.adjustDropField(1)
		return true, nil
	default:
		if m.dropStep == 1 && m.dropCursor == 0 && len(msg.Runes) > 0 && !msg.Alt {
			r := msg.Runes[0]
			if (r >= '0' && r <= '9') || r == '.' {
				m.dropFPSInput += string(r)
			}
			return true, nil
		}
	}
	return true, nil
}

func (m *dashboard) handleDropEnter() (tea.Cmd, bool) {
	switch m.dropStep {
	case 0:
		switch m.dropCursor {
		case 0:
			m.dropMode = rotateChoice([]string{"both", "frames", "audio"}, m.dropMode, 1)
			return nil, true
		case 1:
			m.dropProfile = rotateChoice(m.profileChoices(), m.dropProfile, 1)
			m.applyDropProfile()
			return nil, true
		case 2:
			m.dropStep = 1
			m.dropCursor = 0
			return nil, true
		case 3:
			m.dropModal = false
			return nil, true
		}
	case 1:
		if m.dropCursor == 10 {
			m.dropStep = 2
			m.dropCursor = 4
			m.dropPreview = buildWizardFramePreview(m.dropPath, m.dropVideoInfo, m.dropPreviewSamples, m.dropPreviewWidth)
			m.dropEstimate = estimateDropOutput(m.dropVideoInfo, m.dropMode, m.dropFormat, m.dropFPSInput)
			return nil, true
		}
		if m.dropCursor == 11 {
			name, err := m.saveCurrentWizardProfile()
			if err != nil {
				m.setError(err.Error())
			} else {
				m.setStatus(statusSuccess, fmt.Sprintf("Saved profile: %s", name))
			}
			return nil, true
		}
		if m.dropCursor == 12 {
			m.dropStep = 0
			m.dropCursor = 0
			return nil, true
		}
		if m.dropCursor == 13 {
			m.dropModal = false
			return nil, true
		}
	case 2:
		if m.dropCursor == 4 {
			cmd, ok := m.startDropExecution()
			return cmd, ok
		}
		if m.dropCursor == 5 {
			path := strings.TrimSpace(m.dropPath)
			fps, err := strconv.ParseFloat(strings.TrimSpace(m.dropFPSInput), 64)
			if !looksLikeVideoPath(path) || err != nil || fps <= 0 {
				m.setError("Queue requires a valid path and fps.")
				return nil, true
			}
			m.dropQueue = append(m.dropQueue, queuedDropTask{
				Path: path,
				FPS:  fps,
				Opts: extractDropOptions{
					RootDir:   m.rootDir,
					Mode:      m.dropMode,
					Profile:   m.dropProfile,
					Voice:     m.dropVoice || m.dropMode == "audio",
					Format:    m.dropFormat,
					Preset:    m.dropPreset,
					NoSheet:   m.dropNoSheet || m.dropMode == "audio",
					SheetCols: m.dropSheetCols,
					Verbose:   m.dropVerbose,
					StartedAt: time.Now().UTC(),
				},
				EnqueuedAt: time.Now().UTC(),
			})
			m.setStatus(statusSuccess, fmt.Sprintf("Queued task. Pending: %d", len(m.dropQueue)))
			return nil, true
		}
		if m.dropCursor == 6 {
			if len(m.dropQueue) == 0 {
				m.setStatus(statusWarn, "Queue is empty.")
				return nil, true
			}
			m.dropRunning = true
			m.dropProgress = 0
			m.dropStage = "queue run"
			m.dropElapsed = 0
			m.dropETA = 0
			m.dropResultErr = ""
			m.dropResultDiagnostics = ""
			m.setStatus(statusInfo, fmt.Sprintf("Running queue (%d tasks)...", len(m.dropQueue)))
			ctx, cancel := context.WithCancel(context.Background())
			m.dropCancel = cancel
			for i := range m.dropQueue {
				m.dropQueue[i].Opts.Context = ctx
			}
			progressC, doneC := startDropQueueTask(append([]queuedDropTask(nil), m.dropQueue...))
			m.dropProgressC = progressC
			m.dropDoneC = doneC
			m.dropQueue = nil
			return waitForDropTaskMsg(progressC, doneC), true
		}
		if m.dropCursor == 7 {
			if len(m.dropFailedQueue) == 0 {
				m.setStatus(statusWarn, "No failed queue jobs to retry.")
				return nil, true
			}
			m.dropQueue = append([]queuedDropTask(nil), m.dropFailedQueue...)
			m.dropFailedQueue = nil
			m.dropQueueFailedErrors = nil
			m.setStatus(statusInfo, fmt.Sprintf("Loaded %d failed jobs back into queue.", len(m.dropQueue)))
			return nil, true
		}
		if m.dropCursor == 8 {
			if len(m.dropQueue) == 0 {
				m.setStatus(statusWarn, "Queue is empty.")
				return nil, true
			}
			m.dropQueue = m.dropQueue[:len(m.dropQueue)-1]
			m.setStatus(statusInfo, fmt.Sprintf("Removed last queued job. Pending: %d", len(m.dropQueue)))
			return nil, true
		}
		if m.dropCursor == 9 {
			if len(m.dropQueue) <= 1 {
				m.setStatus(statusWarn, "Need at least two jobs to reorder queue.")
				return nil, true
			}
			last := m.dropQueue[len(m.dropQueue)-1]
			m.dropQueue = append([]queuedDropTask{last}, m.dropQueue[:len(m.dropQueue)-1]...)
			m.setStatus(statusInfo, "Moved last queued job to front.")
			return nil, true
		}
		if m.dropCursor == 10 {
			m.dropQueue = nil
			m.setStatus(statusInfo, "Cleared queued jobs.")
			return nil, true
		}
		if m.dropCursor == 11 {
			name, err := m.saveCurrentWizardProfile()
			if err != nil {
				m.setError(err.Error())
			} else {
				m.setStatus(statusSuccess, fmt.Sprintf("Saved profile: %s", name))
			}
			return nil, true
		}
		if m.dropCursor == 12 {
			m.dropStep = 1
			m.dropCursor = 0
			return nil, true
		}
		if m.dropCursor == 13 {
			m.dropModal = false
			return nil, true
		}
	case 3:
		if m.dropCursor == 0 {
			cmd, ok := m.startDropExecution()
			return cmd, ok
		}
		if m.dropCursor == 1 {
			if strings.TrimSpace(m.dropResultOut) != "" {
				if err := openPath(m.dropResultOut); err != nil {
					m.setError(err.Error())
				} else {
					m.setStatus(statusSuccess, "Opened output folder.")
				}
			} else {
				m.setStatus(statusWarn, "No output folder available.")
			}
			return nil, true
		}
		if m.dropCursor == 2 {
			target := strings.TrimSpace(m.dropResultTranscript)
			if target == "" {
				target = strings.TrimSpace(m.dropResultAudio)
			}
			if target != "" {
				if err := openPath(target); err != nil {
					m.setError(err.Error())
				} else {
					m.setStatus(statusSuccess, "Opened audio/transcript artifact.")
				}
			} else {
				m.setStatus(statusWarn, "No transcript/audio artifact available.")
			}
			return nil, true
		}
		if m.dropCursor == 3 {
			if strings.TrimSpace(m.dropResultSheet) != "" {
				if err := openPath(m.dropResultSheet); err != nil {
					m.setError(err.Error())
				} else {
					m.setStatus(statusSuccess, "Opened contact sheet.")
				}
			} else {
				m.setStatus(statusWarn, "No contact sheet available.")
			}
			return nil, true
		}
		if m.dropCursor == 4 {
			if strings.TrimSpace(m.dropResultLog) != "" {
				if err := openPath(m.dropResultLog); err != nil {
					m.setError(err.Error())
				} else {
					m.setStatus(statusSuccess, "Opened extraction log.")
				}
			} else {
				m.setStatus(statusWarn, "No log file available.")
			}
			return nil, true
		}
		if m.dropCursor == 5 {
			if strings.TrimSpace(m.dropResultOut) == "" {
				m.setStatus(statusWarn, "No output folder path to copy.")
				return nil, true
			}
			if err := copyTextToClipboard(m.dropResultOut); err != nil {
				m.setStatus(statusWarn, "Copy failed: "+shortenSentence(err.Error(), 64))
			} else {
				m.setStatus(statusSuccess, "Copied output folder path.")
			}
			return nil, true
		}
		if m.dropCursor == 6 {
			target := strings.TrimSpace(m.dropResultTranscript)
			if target == "" {
				target = strings.TrimSpace(m.dropResultAudio)
			}
			if target == "" {
				m.setStatus(statusWarn, "No transcript/audio path to copy.")
				return nil, true
			}
			if err := copyTextToClipboard(target); err != nil {
				m.setStatus(statusWarn, "Copy failed: "+shortenSentence(err.Error(), 64))
			} else {
				m.setStatus(statusSuccess, "Copied transcript/audio path.")
			}
			return nil, true
		}
		if m.dropCursor == 7 {
			target := strings.TrimSpace(m.dropResultSheet)
			if target == "" {
				m.setStatus(statusWarn, "No contact sheet path to copy.")
				return nil, true
			}
			if err := copyTextToClipboard(target); err != nil {
				m.setStatus(statusWarn, "Copy failed: "+shortenSentence(err.Error(), 64))
			} else {
				m.setStatus(statusSuccess, "Copied contact sheet path.")
			}
			return nil, true
		}
		if m.dropCursor == 8 {
			if strings.TrimSpace(m.dropResultErr) == "" {
				m.setStatus(statusInfo, "No error details to copy.")
				return nil, true
			}
			if err := copyTextToClipboard(m.dropResultErr); err != nil {
				m.setStatus(statusWarn, "Copy failed: "+shortenSentence(err.Error(), 64))
			} else {
				m.setStatus(statusSuccess, "Error details copied to clipboard.")
			}
			return nil, true
		}
		if m.dropCursor == 9 {
			if len(m.dropFailedQueue) == 0 {
				m.setStatus(statusWarn, "No failed queue jobs to retry.")
				return nil, true
			}
			m.dropQueue = append([]queuedDropTask(nil), m.dropFailedQueue...)
			m.dropFailedQueue = nil
			m.dropStep = 2
			m.dropCursor = 6
			m.setStatus(statusInfo, fmt.Sprintf("Retrying %d failed queue jobs.", len(m.dropQueue)))
			return nil, true
		}
		if m.dropCursor == 10 {
			m.dropModal = false
			return nil, true
		}
	}
	return nil, false
}

func (m *dashboard) startDropExecution() (tea.Cmd, bool) {
	path := strings.TrimSpace(m.dropPath)
	if !looksLikeVideoPath(path) {
		m.setError("Import modal needs a valid video path.")
		return nil, true
	}
	fps, err := strconv.ParseFloat(strings.TrimSpace(m.dropFPSInput), 64)
	if err != nil || fps <= 0 {
		m.setError("FPS must be a positive number.")
		return nil, true
	}
	m.dropRunning = true
	m.dropProgress = 0
	m.dropStage = ""
	m.dropElapsed = 0
	m.dropETA = 0
	m.dropResultErr = ""
	m.dropResultDiagnostics = ""
	m.setStatus(statusInfo, "Running import...")
	ctx, cancel := context.WithCancel(context.Background())
	m.dropCancel = cancel
	progressC, doneC := startDropExtractTask(path, fps, extractDropOptions{
		Context:   ctx,
		RootDir:   m.rootDir,
		Mode:      m.dropMode,
		Voice:     m.dropVoice || m.dropMode == "audio",
		Format:    m.dropFormat,
		Preset:    m.dropPreset,
		Profile:   m.dropProfile,
		StartedAt: time.Now().UTC(),
		NoSheet:   m.dropNoSheet || m.dropMode == "audio",
		SheetCols: m.dropSheetCols,
		Verbose:   m.dropVerbose,
	})
	m.dropProgressC = progressC
	m.dropDoneC = doneC
	return waitForDropTaskMsg(progressC, doneC), true
}

func (m dashboard) dropMaxCursor() int {
	switch m.dropStep {
	case 0:
		return 3
	case 1:
		return 13
	case 2:
		return 13
	default:
		return 10
	}
}

func (m *dashboard) adjustDropField(delta int) {
	if m.dropStep == 0 {
		if m.dropCursor == 0 {
			m.dropMode = rotateChoice([]string{"both", "frames", "audio"}, m.dropMode, delta)
		}
		if m.dropCursor == 1 {
			m.dropProfile = rotateChoice(m.profileChoices(), m.dropProfile, delta)
			m.applyDropProfile()
		}
		return
	}
	if m.dropStep != 1 {
		return
	}
	switch m.dropCursor {
	case 1:
		m.dropMode = rotateChoice([]string{"both", "frames", "audio"}, m.dropMode, delta)
	case 2:
		if m.dropFormat == "png" {
			m.dropFormat = "jpg"
		} else {
			m.dropFormat = "png"
		}
	case 3:
		m.dropPreset = rotateChoice([]string{"safe", "balanced", "fast"}, m.dropPreset, delta)
	case 4:
		m.dropNoSheet = !m.dropNoSheet
	case 5:
		m.dropSheetCols = clamp(m.dropSheetCols+delta, 1, 24)
	case 6:
		m.dropVerbose = !m.dropVerbose
	case 7:
		m.dropVoice = !m.dropVoice
	case 8:
		m.dropPreviewSamples = clamp(m.dropPreviewSamples+delta, 2, 12)
	case 9:
		m.dropPreviewWidth = clamp(m.dropPreviewWidth+(delta*2), 12, 64)
	}
}

func (m dashboard) renderDropModal() string {
	lines := []string{
		m.styles.title.Render("Extraction Wizard"),
		m.styles.subtle.Render("Step-by-step import flow. Use j/k + Enter, h/l to adjust."),
		"",
		"Video:",
		trimForModal(m.dropPath, 88),
		"",
	}
	if m.dropVideoInfo != nil {
		lines = append(lines, fmt.Sprintf("Stats: %s | %.2fs | %.2f fps", resolutionString(m.dropVideoInfo.Width, m.dropVideoInfo.Height), m.dropVideoInfo.DurationSec, m.dropVideoInfo.FrameRate))
		lines = append(lines, "")
	}
	stepLabel := []string{"1/4 Mode", "2/4 Parameters", "3/4 Review", "4/4 Results"}[clamp(m.dropStep, 0, 3)]
	lines = append(lines, m.styles.subtle.Render("Wizard: "+stepLabel))
	if m.dropStep == 3 {
		status := "Success"
		if strings.TrimSpace(m.dropResultErr) != "" {
			status = "Failed"
		}
		lines = append(lines, "Result: "+status)
		lines = append(lines, "Output: "+valueOrDash(m.dropResultOut))
		if strings.TrimSpace(m.dropResultErr) != "" {
			lines = append(lines, "Error: "+shortenSentence(m.dropResultErr, 92))
		} else {
			lines = append(lines, fmt.Sprintf("Frames: %d", m.dropResultCount))
		}
		if m.dropQueueTotal > 0 {
			lines = append(lines, fmt.Sprintf("Queue Summary: total=%d success=%d failed=%d", m.dropQueueTotal, m.dropQueueSuccess, m.dropQueueFailed))
			for i, entry := range m.dropQueueFailedErrors {
				if i >= 3 {
					break
				}
				lines = append(lines, "  failed: "+shortenSentence(entry, 92))
			}
		}
		if strings.TrimSpace(m.dropResultDiagnostics) != "" {
			lines = append(lines, "Diagnostics: "+m.dropResultDiagnostics)
		}
		lines = append(lines, "")
	}
	if m.dropStep == 2 {
		lines = append(lines, "", m.styles.subtle.Render("ASCII Preview"))
		if len(m.dropPreview) > 0 {
			lines = append(lines, m.dropPreview...)
		} else {
			lines = append(lines, strings.Split(m.wizardASCIIPreview(), "\n")...)
		}
		lines = append(lines, "")
	}
	fields := m.dropFields()
	for i, f := range fields {
		marker := "  "
		if i == m.dropCursor {
			marker = "> "
		}
		lines = append(lines, marker+f)
	}
	if m.dropRunning {
		lines = append(lines, "", renderTinyProgress("Extract", m.dropProgress, tinyProgressTimingSuffix(m.dropElapsed, m.dropETA, m.dropProgress)))
		if strings.TrimSpace(m.dropStage) != "" {
			lines = append(lines, m.styles.subtle.Render("Stage: "+m.dropStage))
		}
		lines = append(lines, m.styles.subtle.Render("Running extraction... press [c] to cancel."))
	} else {
		lines = append(lines, "", m.styles.subtle.Render("Paste a video path into this terminal to reopen this wizard quickly."))
	}
	w := min(max(56, m.winW-16), 104)
	return m.styles.panelFocus.Width(w).Render(strings.Join(lines, "\n"))
}

func (m dashboard) dropFields() []string {
	switch m.dropStep {
	case 0:
		return []string{
			fmt.Sprintf("Mode: %s", strings.ToUpper(m.dropMode)),
			fmt.Sprintf("Profile: %s", strings.ToUpper(m.dropProfile)),
			"Next: Parameters",
			"Cancel",
		}
	case 1:
		return []string{
			fmt.Sprintf("FPS: %s", m.dropFPSInput),
			fmt.Sprintf("Mode: %s", strings.ToUpper(m.dropMode)),
			fmt.Sprintf("Format: %s", m.dropFormat),
			fmt.Sprintf("Preset: %s", m.dropPreset),
			fmt.Sprintf("Skip contact sheet: %s", onOff(m.dropNoSheet)),
			fmt.Sprintf("Sheet columns: %d", m.dropSheetCols),
			fmt.Sprintf("Verbose logs: %s", onOff(m.dropVerbose)),
			fmt.Sprintf("Transcribe audio: %s", onOff(m.dropVoice)),
			fmt.Sprintf("Preview samples: %d", m.dropPreviewSamples),
			fmt.Sprintf("Preview width: %d", m.dropPreviewWidth),
			"Next: Review",
			"Save settings as profile",
			"Back",
			"Cancel",
		}
	default:
		if m.dropStep == 3 {
			return []string{
				"Action: Retry last settings",
				"Action: Open output folder",
				"Action: Open transcript/audio",
				"Action: Open contact sheet",
				"Action: Open log file",
				"Action: Copy output path",
				"Action: Copy transcript/audio path",
				"Action: Copy contact sheet path",
				"Action: Copy error details",
				"Action: Retry failed queue",
				"Action: Close",
			}
		}
		preview := "flow=frames+transcript"
		if m.dropMode == "audio" {
			preview = "flow=audio+transcript"
		} else if m.dropMode == "frames" {
			preview = "flow=frames+sheet"
		}
		return []string{
			fmt.Sprintf("Summary: mode=%s fps=%s format=%s preset=%s %s", strings.ToUpper(m.dropMode), m.dropFPSInput, m.dropFormat, m.dropPreset, preview),
			fmt.Sprintf("Estimate: %s", m.dropEstimate),
			fmt.Sprintf("Queue: %d pending", len(m.dropQueue)),
			fmt.Sprintf("Failed Queue: %d", len(m.dropFailedQueue)),
			m.styles.subtle.Render("Choose action below:"),
			"Action: Start extraction",
			"Action: Enqueue current",
			"Action: Run queue",
			"Action: Load failed queue",
			"Action: Remove last queued",
			"Action: Move last queued -> front",
			"Action: Clear queue",
			"Action: Save settings as profile",
			"Action: Back",
			"Action: Cancel",
		}
	}
}

func resolutionString(w, h int) string {
	if w <= 0 || h <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func (m dashboard) wizardASCIIPreview() string {
	if m.dropVideoInfo == nil || m.dropVideoInfo.DurationSec <= 0 {
		return "[ no preview available ]"
	}
	tiles := clamp(m.dropPreviewSamples, 2, 12)
	dur := m.dropVideoInfo.DurationSec
	step := dur / float64(tiles)
	if step <= 0 {
		step = 1
	}
	labels := make([]string, 0, 3)
	row := make([]string, 0, tiles)
	for i := 0; i < tiles; i++ {
		ts := float64(i) * step
		row = append(row, "[#]")
		if i == 0 || i == tiles/2 || i == tiles-1 {
			labels = append(labels, formatPreviewTS(ts))
		}
	}
	return strings.Join(row, "") + "\n" + strings.Join(labels, "   ")
}

func formatPreviewTS(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int(math.Round(seconds))
	s := total % 60
	m := (total / 60) % 60
	h := total / 3600
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
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

func renderTinyProgress(label string, percent float64, suffix string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	const width = 20
	filled := int((percent / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	if strings.TrimSpace(suffix) != "" {
		return fmt.Sprintf("%s [%s] %5.1f%% (%s)", label, bar, percent, strings.TrimSpace(suffix))
	}
	return fmt.Sprintf("%s [%s] %5.1f%%", label, bar, percent)
}

func tinyProgressTimingSuffix(elapsed time.Duration, eta time.Duration, percent float64) string {
	parts := []string{}
	if elapsed > 0 {
		parts = append(parts, "elapsed "+formatClockDuration(elapsed))
	}
	if percent > 0 && percent < 100 && eta > 0 {
		parts = append(parts, "eta "+formatClockDuration(eta))
	}
	return strings.Join(parts, ", ")
}

func estimateDropOutput(info *media.VideoInfo, mode string, format string, fpsInput string) string {
	if info == nil || info.DurationSec <= 0 {
		return "unknown"
	}
	if mode == "audio" {
		mins := info.DurationSec / 60.0
		return fmt.Sprintf("audio-only %.1f min source", mins)
	}
	fps, err := strconv.ParseFloat(strings.TrimSpace(fpsInput), 64)
	if err != nil || fps <= 0 {
		fps = 4
	}
	frames := int(math.Ceil(info.DurationSec * fps))
	perFrameKB := 180.0
	if strings.ToLower(strings.TrimSpace(format)) == "jpg" {
		perFrameKB = 90.0
	}
	mb := (float64(frames) * perFrameKB) / 1024.0
	warn := ""
	if frames > 3000 || mb > 800 {
		warn = " (large run warning)"
	}
	return fmt.Sprintf("~%d frames, ~%.1f MB%s", frames, mb, warn)
}

func buildWizardFramePreview(path string, info *media.VideoInfo, sampleCount int, width int) []string {
	if info == nil || info.DurationSec <= 0 || strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil
	}
	tmpDir, err := os.MkdirTemp("", "framescli-preview-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmpDir)

	sampleCount = clamp(sampleCount, 2, 12)
	samples := make([]float64, 0, sampleCount)
	for i := 0; i < sampleCount; i++ {
		samples = append(samples, float64(i+1)/float64(sampleCount+1))
	}
	lines := make([]string, 0, len(samples)*3)
	hasChafa := false
	if _, err := exec.LookPath("chafa"); err == nil {
		hasChafa = true
	}

	renderW := clamp(width, 12, 64)
	renderH := clamp(int(math.Round(float64(renderW)/3.0)), 6, 20)
	for i, pct := range samples {
		ts := info.DurationSec * pct
		framePath := filepath.Join(tmpDir, fmt.Sprintf("preview-%d.jpg", i+1))
		args := []string{
			"-y",
			"-ss", strconv.FormatFloat(ts, 'f', 2, 64),
			"-i", path,
			"-frames:v", "1",
			"-vf", fmt.Sprintf("scale=%d:-1", renderW*2),
			framePath,
		}
		if err := exec.Command("ffmpeg", args...).Run(); err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%d] %s", i+1, formatPreviewTS(ts)))
		if hasChafa {
			out, err := exec.Command("chafa", "--size", fmt.Sprintf("%dx%d", renderW, renderH), "--colors", "16", framePath).Output()
			if err == nil {
				art := strings.Split(strings.TrimSpace(string(out)), "\n")
				maxLines := min(len(art), renderH)
				lines = append(lines, art[:maxLines]...)
			} else {
				lines = append(lines, "[ preview unavailable ]")
			}
		} else {
			lines = append(lines, "[####][####][####][####][####]")
		}
		lines = append(lines, "")
	}
	return lines
}

func (m *dashboard) consumeDropPaste(msg tea.KeyMsg) bool {
	if m.helpMode || m.tourMode || m.paletteMode || m.searchMode || m.historyMode || m.dropModal || m.welcomeMode {
		return false
	}
	if msg.Type == tea.KeyRunes {
		txt := string(msg.Runes)
		if len(msg.Runes) > 1 || strings.Contains(txt, "/") || strings.Contains(txt, "\\") || strings.Contains(strings.ToLower(txt), "file://") {
			m.pendingDrop += txt
			return true
		}
		if strings.TrimSpace(m.pendingDrop) != "" && txt == " " {
			m.pendingDrop += txt
			return true
		}
	}
	if msg.Type == tea.KeyEnter && strings.TrimSpace(m.pendingDrop) != "" {
		candidate := normalizeDroppedPath(m.pendingDrop)
		if looksLikeVideoPath(candidate) {
			m.pendingDrop = ""
			m.openDropModal(candidate)
			return true
		}
		m.setStatus(statusWarn, "Pasted text is not a supported video path. Keep pasting or press Esc.")
		return true
	}
	return false
}

type extractDropOptions struct {
	Context   context.Context
	RootDir   string
	Mode      string
	Profile   string
	StartedAt time.Time
	LogFile   string
	Voice     bool
	Format    string
	Preset    string
	NoSheet   bool
	SheetCols int
	Verbose   bool
}

func startDropExtractTask(path string, fps float64, opts extractDropOptions) (<-chan dropProgressMsg, <-chan dropExtractDoneMsg) {
	progressC := make(chan dropProgressMsg, 24)
	doneC := make(chan dropExtractDoneMsg, 1)
	started := time.Now()
	go func() {
		defer close(progressC)
		defer close(doneC)
		done := runDropExtract(path, fps, opts, func(msg dropProgressMsg) {
			msg = annotateDropProgress(started, msg)
			select {
			case progressC <- msg:
			default:
			}
		})
		doneC <- done
	}()
	return progressC, doneC
}

func startDropQueueTask(queue []queuedDropTask) (<-chan dropProgressMsg, <-chan dropExtractDoneMsg) {
	progressC := make(chan dropProgressMsg, 32)
	doneC := make(chan dropExtractDoneMsg, 1)
	started := time.Now()
	go func() {
		defer close(progressC)
		defer close(doneC)
		total := len(queue)
		if total == 0 {
			doneC <- dropExtractDoneMsg{err: fmt.Errorf("queue is empty")}
			return
		}
		var last dropExtractDoneMsg
		successes := 0
		failed := 0
		failedJobs := make([]queuedDropTask, 0, total)
		failedErrors := make([]string, 0, total)
		for i, job := range queue {
			last = runDropExtract(job.Path, job.FPS, job.Opts, func(msg dropProgressMsg) {
				base := (float64(i) / float64(total)) * 100.0
				part := msg.percent / float64(total)
				stage := fmt.Sprintf("queue %d/%d: %s", i+1, total, msg.stage)
				annotated := annotateDropProgress(started, dropProgressMsg{percent: base + part, stage: stage})
				select {
				case progressC <- annotated:
				default:
				}
			})
			if last.err != nil {
				failed++
				failedJobs = append(failedJobs, job)
				failedErrors = append(failedErrors, fmt.Sprintf("%s: %s", job.Path, last.err.Error()))
				if errors.Is(last.err, context.Canceled) {
					last.queueCount = total
					last.queueTotal = total
					last.queueSuccess = successes
					last.queueFailed = failed
					last.queueFailedJobs = failedJobs
					last.queueFailedErrors = failedErrors
					doneC <- last
					return
				}
				continue
			}
			successes++
		}
		last.queueCount = total
		last.queueTotal = total
		last.queueSuccess = successes
		last.queueFailed = failed
		last.queueFailedJobs = failedJobs
		last.queueFailedErrors = failedErrors
		doneC <- last
	}()
	return progressC, doneC
}

func annotateDropProgress(started time.Time, msg dropProgressMsg) dropProgressMsg {
	if started.IsZero() {
		started = time.Now()
	}
	elapsed := time.Since(started)
	if elapsed < 0 {
		elapsed = 0
	}
	msg.elapsed = elapsed
	if msg.percent > 0 && msg.percent < 100 {
		eta := time.Duration((float64(elapsed) * (100.0 - msg.percent)) / msg.percent)
		if eta < 0 {
			eta = 0
		}
		msg.eta = eta
	}
	return msg
}

func waitForDropTaskMsg(progressC <-chan dropProgressMsg, doneC <-chan dropExtractDoneMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-progressC:
			if !ok {
				done, okDone := <-doneC
				if !okDone {
					return dropExtractDoneMsg{err: context.Canceled}
				}
				return done
			}
			return msg
		case done, ok := <-doneC:
			if !ok {
				return dropExtractDoneMsg{err: context.Canceled}
			}
			return done
		}
	}
}

func runDropExtract(path string, fps float64, opts extractDropOptions, progressFn func(dropProgressMsg)) dropExtractDoneMsg {
	started := opts.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}
	finish := func(msg dropExtractDoneMsg) dropExtractDoneMsg {
		msg.videoPath = path
		msg.mode = opts.Mode
		msg.profile = opts.Profile
		msg.startedAt = started
		if msg.endedAt.IsZero() {
			msg.endedAt = time.Now().UTC()
		}
		return msg
	}
	if strings.TrimSpace(opts.RootDir) == "" {
		opts.RootDir = media.DefaultFramesRoot
	}
	if opts.Mode == "audio" {
		progressFn(dropProgressMsg{percent: 5, stage: "audio extract"})
		runName := "Audio_" + time.Now().Format("20060102-150405")
		out := filepath.Join(opts.RootDir, runName)
		voiceDir := filepath.Join(out, "voice")
		audioPath, err := media.ExtractAudioFromVideoWithOptions(media.ExtractAudioOptions{
			Context:   opts.Context,
			VideoPath: path,
			OutDir:    voiceDir,
			Format:    "wav",
			LogFile:   filepath.Join(out, "extract.log"),
			Verbose:   opts.Verbose,
		})
		if err != nil {
			diag, _ := media.WriteDiagnosticsBundle(opts.RootDir, "audio_extract", path, out, filepath.Join(out, "extract.log"), err)
			return finish(dropExtractDoneMsg{err: err, logPath: filepath.Join(out, "extract.log"), diagnosticsPath: diag})
		}
		if opts.Voice {
			progressFn(dropProgressMsg{percent: 80, stage: "transcription"})
			if _, _, err := media.TranscribeAudioWithOptions(media.TranscribeOptions{
				Context:   opts.Context,
				AudioPath: audioPath,
				OutDir:    voiceDir,
				Verbose:   opts.Verbose,
			}); err != nil {
				diag, _ := media.WriteDiagnosticsBundle(opts.RootDir, "transcription", path, out, filepath.Join(out, "extract.log"), err)
				return finish(dropExtractDoneMsg{err: err, logPath: filepath.Join(out, "extract.log"), diagnosticsPath: diag})
			}
		}
		progressFn(dropProgressMsg{percent: 100, stage: "done"})
		return finish(dropExtractDoneMsg{out: out, count: 0, audioPath: audioPath, transcriptPath: filepath.Join(voiceDir, "transcript.txt"), logPath: filepath.Join(out, "extract.log")})
	}

	outDir := ""
	if strings.TrimSpace(opts.RootDir) != "" && strings.TrimSpace(opts.RootDir) != media.DefaultFramesRoot {
		outDir = filepath.Join(opts.RootDir, "Run_"+time.Now().Format("20060102-150405"))
	}
	logFile := ""
	if strings.TrimSpace(outDir) != "" {
		logFile = filepath.Join(outDir, "extract.log")
	}
	progressFn(dropProgressMsg{percent: 5, stage: "frame extraction"})
	res, err := media.ExtractMedia(media.ExtractMediaOptions{
		Context:      opts.Context,
		VideoPath:    path,
		FPS:          fps,
		FrameFormat:  opts.Format,
		HWAccel:      "auto",
		Preset:       opts.Preset,
		ExtractAudio: opts.Voice,
		Verbose:      opts.Verbose,
		OutDir:       outDir,
		LogFile:      logFile,
		ProgressFn: func(p float64) {
			progressFn(dropProgressMsg{percent: 10 + (p * 0.7), stage: "frame extraction"})
		},
	})
	if err != nil {
		diag, _ := media.WriteDiagnosticsBundle(opts.RootDir, "extract_media", path, outDir, logFile, err)
		return finish(dropExtractDoneMsg{err: err, logPath: logFile, diagnosticsPath: diag})
	}
	contactSheet := ""
	if !opts.NoSheet {
		progressFn(dropProgressMsg{percent: 82, stage: "contact sheet"})
		sheetsDir := filepath.Join(res.ImagesDir, "sheets")
		contactSheet, err = media.CreateContactSheetWithContext(opts.Context, res.ImagesDir, opts.SheetCols, filepath.Join(sheetsDir, "contact-sheet.png"), opts.Verbose)
		if err != nil {
			diag, _ := media.WriteDiagnosticsBundle(opts.RootDir, "contact_sheet", path, res.OutDir, filepath.Join(res.OutDir, "extract.log"), err)
			return finish(dropExtractDoneMsg{err: err, logPath: filepath.Join(res.OutDir, "extract.log"), diagnosticsPath: diag})
		}
	}
	count, err := media.CountFrames(res.ImagesDir)
	if err != nil {
		diag, _ := media.WriteDiagnosticsBundle(opts.RootDir, "frame_count", path, res.OutDir, filepath.Join(res.OutDir, "extract.log"), err)
		return finish(dropExtractDoneMsg{err: err, logPath: filepath.Join(res.OutDir, "extract.log"), diagnosticsPath: diag})
	}
	progressFn(dropProgressMsg{percent: 95, stage: "finalizing"})
	transcriptPath := ""
	if opts.Voice {
		transcriptPath = filepath.Join(res.OutDir, "voice", "transcript.txt")
	}
	progressFn(dropProgressMsg{percent: 100, stage: "done"})
	return finish(dropExtractDoneMsg{
		out:            res.OutDir,
		count:          count,
		transcriptPath: transcriptPath,
		contactSheet:   contactSheet,
		audioPath:      res.AudioPath,
		logPath:        filepath.Join(res.OutDir, "extract.log"),
	})
}

func normalizeDroppedPath(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		s = strings.TrimPrefix(s, "file://")
		s = strings.TrimPrefix(s, "file:///")
		s = "/" + strings.TrimLeft(s, "/")
		s = strings.ReplaceAll(s, "%20", " ")
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
	if strings.Contains(s, `\`) && !strings.Contains(s, `:\`) {
		s = strings.ReplaceAll(s, `\ `, " ")
	}
	return strings.TrimSpace(s)
}

func looksLikeVideoPath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	for _, ext := range []string{".mp4", ".mov", ".mkv", ".avi", ".webm", ".m4v"} {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

func rotateChoice(choices []string, current string, delta int) string {
	if len(choices) == 0 {
		return current
	}
	idx := 0
	for i, c := range choices {
		if c == current {
			idx = i
			break
		}
	}
	next := (idx + delta) % len(choices)
	if next < 0 {
		next += len(choices)
	}
	return choices[next]
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func valueOrDash(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func (m *dashboard) applyDropProfile() {
	if len(m.presetProfiles) == 0 {
		m.presetProfiles = appconfig.Default().PresetProfiles
	}
	key := strings.ToLower(strings.TrimSpace(m.dropProfile))
	if p, ok := m.presetProfiles[key]; ok {
		m.dropPreset = p.Preset
		m.dropFormat = p.Format
		m.dropNoSheet = p.NoSheet
		m.dropVoice = p.Voice
		return
	}
	m.dropPreset = "balanced"
	if strings.TrimSpace(m.dropFormat) == "" {
		m.dropFormat = "png"
	}
}

func (m *dashboard) saveCurrentWizardProfile() (string, error) {
	if len(m.presetProfiles) == 0 {
		m.presetProfiles = appconfig.Default().PresetProfiles
	}
	name := strings.ToLower(strings.TrimSpace("custom-" + time.Now().UTC().Format("20060102-150405")))
	for {
		if _, exists := m.presetProfiles[name]; !exists {
			break
		}
		name = name + "-x"
	}
	m.presetProfiles[name] = appconfig.PresetProfile{
		Preset:  strings.ToLower(strings.TrimSpace(m.dropPreset)),
		Format:  strings.ToLower(strings.TrimSpace(m.dropFormat)),
		Voice:   m.dropVoice,
		NoSheet: m.dropNoSheet,
	}
	cfg, err := appconfig.LoadOrDefault()
	if err != nil {
		return "", fmt.Errorf("could not load config: %w", err)
	}
	cfg.PresetProfiles = m.presetProfiles
	if _, err := appconfig.Save(cfg); err != nil {
		return "", fmt.Errorf("could not save profile: %w", err)
	}
	m.dropProfile = name
	return name, nil
}

func (m dashboard) profileChoices() []string {
	if len(m.presetProfiles) == 0 {
		return []string{"fast", "balanced", "high-fidelity"}
	}
	out := make([]string, 0, len(m.presetProfiles))
	for k := range m.presetProfiles {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"balanced"}
	}
	return out
}

func trimForModal(s string, maxLen int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxLen {
		return string(r)
	}
	if maxLen < 4 {
		return string(r[:maxLen])
	}
	return string(r[:maxLen-3]) + "..."
}

func (m *dashboard) persistUIPreferences(welcomeComplete bool) error {
	cfg, err := appconfig.LoadOrDefault()
	if err != nil {
		return fmt.Errorf("could not load config to save UI settings: %w", err)
	}
	cfg.TUIThemePreset = m.themePreset
	cfg.TUISimpleMode = m.simpleMode
	cfg.TUIVimMode = m.vimMode
	if welcomeComplete {
		cfg.TUIWelcomeSeen = true
	}
	if _, err := appconfig.Save(cfg); err != nil {
		return fmt.Errorf("could not save UI settings: %w", err)
	}
	return nil
}

func tickStatus() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg { return statusTickMsg(t) })
}

func tabFromName(name string) detailTab {
	switch name {
	case "Overview":
		return tabOverview
	case "Transcript":
		return tabTranscript
	case "Timeline":
		return tabTimeline
	case "Files":
		return tabFiles
	default:
		return tabOverview
	}
}

func makeStyles(theme string) styles {
	type palette struct {
		bg, fg, title, subtle, border, focusBorder, detail, err, hint, tab, tabActiveFG, tabActiveBG, metricKey, metricValue, badgeFG, badgeBG, badgeActiveBG, statusInfoBG, statusOkBG, statusWarnBG, statusErrBG, statusFG, commandBG, commandFG, modalBG, modalFG string
	}
	p := palette{
		bg: "#121212", fg: "#f5f5f4", title: "#fbbf24", subtle: "#a8a29e",
		border: "#57534e", focusBorder: "#f59e0b", detail: "#fafaf9", err: "#fecaca", hint: "#fbbf24",
		tab: "#e7e5e4", tabActiveFG: "#292524", tabActiveBG: "#fbbf24", metricKey: "#fcd34d", metricValue: "#f5f5f4",
		badgeFG: "#292524", badgeBG: "#fcd34d", badgeActiveBG: "#fb923c",
		statusInfoBG: "#44403c", statusOkBG: "#14532d", statusWarnBG: "#78350f", statusErrBG: "#7f1d1d", statusFG: "#fefce8",
		commandBG: "#292524", commandFG: "#f5f5f4", modalBG: "#1c1917", modalFG: "#f5f5f4",
	}
	if theme == themeHighContrast {
		p = palette{
			bg: "#000000", fg: "#ffffff", title: "#ffd60a", subtle: "#d4d4d4",
			border: "#ffffff", focusBorder: "#ffd60a", detail: "#ffffff", err: "#ffb4b4", hint: "#ffd60a",
			tab: "#ffffff", tabActiveFG: "#000000", tabActiveBG: "#ffd60a", metricKey: "#ffe082", metricValue: "#ffffff",
			badgeFG: "#000000", badgeBG: "#ffd60a", badgeActiveBG: "#ff8f00",
			statusInfoBG: "#262626", statusOkBG: "#006400", statusWarnBG: "#8b5a00", statusErrBG: "#8b0000", statusFG: "#ffffff",
			commandBG: "#111111", commandFG: "#ffffff", modalBG: "#111111", modalFG: "#ffffff",
		}
	}
	if theme == themeLight {
		p = palette{
			bg: "#f8fafc", fg: "#0f172a", title: "#0f766e", subtle: "#475569",
			border: "#94a3b8", focusBorder: "#0f766e", detail: "#0f172a", err: "#b91c1c", hint: "#0f766e",
			tab: "#0f172a", tabActiveFG: "#ffffff", tabActiveBG: "#0f766e", metricKey: "#0f766e", metricValue: "#111827",
			badgeFG: "#ffffff", badgeBG: "#0f766e", badgeActiveBG: "#0369a1",
			statusInfoBG: "#cbd5e1", statusOkBG: "#bbf7d0", statusWarnBG: "#fde68a", statusErrBG: "#fecaca", statusFG: "#0f172a",
			commandBG: "#e2e8f0", commandFG: "#0f172a", modalBG: "#ffffff", modalFG: "#0f172a",
		}
	}
	return styles{
		app:         lipgloss.NewStyle().Background(lipgloss.Color(p.bg)).Foreground(lipgloss.Color(p.fg)).Padding(1, 2),
		title:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.title)),
		subtle:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.subtle)),
		panel:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.border)).Padding(0, 1),
		panelFocus:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.focusBorder)).Padding(0, 1),
		detail:      lipgloss.NewStyle().Foreground(lipgloss.Color(p.detail)),
		error:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.err)).Bold(true),
		keyline:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.hint)),
		tab:         lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color(p.tab)),
		tabActive:   lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color(p.tabActiveFG)).Background(lipgloss.Color(p.tabActiveBG)).Bold(true),
		metricKey:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.metricKey)),
		metricValue: lipgloss.NewStyle().Foreground(lipgloss.Color(p.metricValue)).Bold(true),
		badge:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.badgeFG)).Background(lipgloss.Color(p.badgeBG)).Padding(0, 1),
		badgeActive: lipgloss.NewStyle().Foreground(lipgloss.Color(p.badgeFG)).Background(lipgloss.Color(p.badgeActiveBG)).Padding(0, 1),
		statusInfo:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.statusFG)).Background(lipgloss.Color(p.statusInfoBG)).Padding(0, 1),
		statusOk:    lipgloss.NewStyle().Foreground(lipgloss.Color(p.statusFG)).Background(lipgloss.Color(p.statusOkBG)).Padding(0, 1),
		statusWarn:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.statusFG)).Background(lipgloss.Color(p.statusWarnBG)).Padding(0, 1),
		statusErr:   lipgloss.NewStyle().Foreground(lipgloss.Color(p.statusFG)).Background(lipgloss.Color(p.statusErrBG)).Padding(0, 1).Bold(true),
		commandBar:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.commandFG)).Background(lipgloss.Color(p.commandBG)).Padding(0, 1),
		commandKey:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.title)).Bold(true),
		commandText: lipgloss.NewStyle().Foreground(lipgloss.Color(p.commandFG)),
		modal:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.modalFG)).Background(lipgloss.Color(p.modalBG)).Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color(p.focusBorder)).Padding(1, 2),
		modalTitle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.title)),
		errorPanel:  lipgloss.NewStyle().Foreground(lipgloss.Color(p.modalFG)).Background(lipgloss.Color(p.modalBG)).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.err)).Padding(1, 2),
	}
}

func shortenSentence(in string, limit int) string {
	in = oneLine(in)
	if len(in) <= limit {
		return in
	}
	if limit <= 3 {
		return in[:limit]
	}
	return strings.TrimSpace(in[:limit-3]) + "..."
}

func (m dashboard) selectedRun() (runItem, bool) {
	selected, ok := m.list.SelectedItem().(runItem)
	return selected, ok
}

func toListItems(items []runItem) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, it := range items {
		out = append(out, it)
	}
	return out
}

func loadRuns(rootDir string) ([]runItem, string, string, error) {
	indexPath := media.IndexFilePath(rootDir)
	idx, err := media.ReadRunsIndex(indexPath)
	if err == nil {
		items := scanRunsFromIndex(rootDir, idx)
		hint := fmt.Sprintf("loaded from %s", indexPath)
		if generatedAt, parseErr := time.Parse(time.RFC3339, idx.GeneratedAt); parseErr == nil {
			age := time.Since(generatedAt).Round(time.Minute)
			if age > (12 * time.Hour) {
				hint = fmt.Sprintf("%s (stale: %s old, press r after running `frames index`)", hint, age.String())
			}
		}
		return items, "index", hint, nil
	}

	items, liveErr := scanRunsLive(rootDir)
	if liveErr != nil {
		return nil, "", "", liveErr
	}
	hint := "live scan (tip: run `frames index` for faster startup on large run sets)"
	return items, "live", hint, nil
}

func scanRuns(rootDir string) ([]runItem, error) {
	items, _, _, err := loadRuns(rootDir)
	return items, err
}

func jobHistoryPath(rootDir string) string {
	return filepath.Join(rootDir, "job-history.json")
}

func loadJobHistory(rootDir string) ([]jobHistoryEntry, error) {
	path := jobHistoryPath(rootDir)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []jobHistoryEntry{}, nil
		}
		return nil, err
	}
	var out []jobHistoryEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveJobHistory(rootDir string, rows []jobHistoryEntry) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jobHistoryPath(rootDir), raw, 0o644)
}

func scanRunsLive(rootDir string) ([]runItem, error) {
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []runItem{}, nil
		}
		return nil, err
	}
	out := make([]runItem, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		runPath := filepath.Join(rootDir, e.Name())
		images := filepath.Join(runPath, "images")
		count, updated, ext, firstFrame, lastFrame := frameStats(images)
		transcriptPath := filepath.Join(runPath, "voice", "transcript.txt")
		if _, err := os.Stat(transcriptPath); err != nil {
			transcriptPath = ""
		}
		contactSheet := filepath.Join(runPath, "images", "sheets", "contact-sheet.png")
		if _, err := os.Stat(contactSheet); err != nil {
			contactSheet = ""
		}
		transcript := ""
		if transcriptPath != "" {
			if raw, err := os.ReadFile(transcriptPath); err == nil {
				transcript = strings.TrimSpace(string(raw))
			}
		}
		segments := loadSegments(filepath.Join(runPath, "voice", "transcript.json"))

		md, mdErr := media.ReadRunMetadata(runPath)
		mdPresent := mdErr == nil
		desc := fmt.Sprintf("%d %s frames", count, strings.ToUpper(ext))
		if mdPresent {
			desc = fmt.Sprintf("%d %s @ %.2f fps", count, strings.ToUpper(ext), md.FPS)
		}
		if len(segments) > 0 {
			desc += fmt.Sprintf(" | %d segments", len(segments))
		}

		row := runItem{
			title:           e.Name(),
			description:     desc,
			path:            runPath,
			frameCount:      count,
			updatedAt:       updated,
			transcriptPath:  transcriptPath,
			contactSheet:    contactSheet,
			frameExt:        ext,
			firstFrame:      firstFrame,
			lastFrame:       lastFrame,
			transcript:      transcript,
			segments:        segments,
			metadataPresent: mdPresent,
		}
		if mdPresent {
			row.metadata = &md
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].updatedAt.After(out[j].updatedAt)
	})
	return out, nil
}

func scanRunsFromIndex(rootDir string, index media.RunsIndex) []runItem {
	out := make([]runItem, 0, len(index.Runs))
	for _, row := range index.Runs {
		runPath := row.Path
		if strings.TrimSpace(runPath) == "" {
			runPath = filepath.Join(rootDir, row.Name)
		}
		if _, err := os.Stat(runPath); err != nil {
			runPath = filepath.Join(rootDir, row.Name)
		}

		transcriptPath := filepath.Join(runPath, "voice", "transcript.txt")
		if _, err := os.Stat(transcriptPath); err != nil {
			transcriptPath = ""
		}
		transcript := ""
		if transcriptPath != "" {
			if raw, err := os.ReadFile(transcriptPath); err == nil {
				transcript = strings.TrimSpace(string(raw))
			}
		}

		contactSheet := filepath.Join(runPath, "images", "sheets", "contact-sheet.png")
		if _, err := os.Stat(contactSheet); err != nil {
			contactSheet = ""
		}

		segments := loadSegments(filepath.Join(runPath, "voice", "transcript.json"))
		updated := time.Time{}
		if row.UpdatedAt != "" {
			if ts, err := time.Parse(time.RFC3339, row.UpdatedAt); err == nil {
				updated = ts
			}
		}

		desc := fmt.Sprintf("%d %s frames", row.Frames, strings.ToUpper(row.FrameExt))
		if row.FPS > 0 {
			desc = fmt.Sprintf("%d %s @ %.2f fps", row.Frames, strings.ToUpper(row.FrameExt), row.FPS)
		}
		if len(segments) > 0 {
			desc += fmt.Sprintf(" | %d segments", len(segments))
		}

		mdPresent := row.FPS > 0 || strings.TrimSpace(row.Format) != "" || row.DurationSec > 0
		var md *media.RunMetadata
		if mdPresent {
			md = &media.RunMetadata{
				FPS:         row.FPS,
				FrameFormat: row.Format,
				DurationSec: row.DurationSec,
			}
		}

		firstFrame := ""
		lastFrame := ""
		if row.Frames > 0 && row.FrameExt != "" {
			firstFrame = fmt.Sprintf("frame-%04d.%s", 1, row.FrameExt)
			lastFrame = fmt.Sprintf("frame-%04d.%s", row.Frames, row.FrameExt)
		}

		out = append(out, runItem{
			title:           row.Name,
			description:     desc,
			path:            runPath,
			frameCount:      row.Frames,
			updatedAt:       updated,
			transcriptPath:  transcriptPath,
			contactSheet:    contactSheet,
			frameExt:        row.FrameExt,
			firstFrame:      firstFrame,
			lastFrame:       lastFrame,
			transcript:      transcript,
			segments:        segments,
			metadata:        md,
			metadataPresent: mdPresent,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].updatedAt.After(out[j].updatedAt)
	})
	return out
}

func loadSegments(path string) []transcriptSegment {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var payload media.WhisperJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	out := make([]transcriptSegment, 0, len(payload.Segments))
	for _, seg := range payload.Segments {
		text := strings.TrimSpace(seg.Text)
		if text == "" {
			continue
		}
		out = append(out, transcriptSegment{Start: seg.Start, End: seg.End, Text: text})
	}
	return out
}

func frameStats(imagesDir string) (int, time.Time, string, string, string) {
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return 0, time.Time{}, "png", "", ""
	}
	count := 0
	newest := time.Time{}
	ext := "png"
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "frame-") {
			continue
		}
		count++
		names = append(names, e.Name())
		if info, err := e.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if extValue := strings.TrimPrefix(filepath.Ext(e.Name()), "."); extValue != "" {
			ext = strings.ToLower(extValue)
		}
	}
	sort.Strings(names)
	first := ""
	last := ""
	if len(names) > 0 {
		first = names[0]
		last = names[len(names)-1]
	}
	return count, newest, ext, first, last
}

func segmentFrameRange(seg transcriptSegment, fps float64, frameCount int) (int, int) {
	if fps <= 0 {
		return 0, 0
	}
	start := int(math.Floor(seg.Start*fps)) + 1
	end := int(math.Ceil(seg.End * fps))
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	if frameCount > 0 {
		if start > frameCount {
			start = frameCount
		}
		if end > frameCount {
			end = frameCount
		}
	}
	return start, end
}

func selectedSegmentFramePath(it runItem, segIndex int) (string, error) {
	if it.path == "" {
		return "", fmt.Errorf("run path is missing")
	}
	if len(it.segments) == 0 {
		if it.firstFrame == "" {
			return "", fmt.Errorf("no frames available")
		}
		return filepath.Join(it.path, "images", it.firstFrame), nil
	}
	if segIndex < 0 || segIndex >= len(it.segments) {
		segIndex = 0
	}
	fps := runFPS(it)
	if fps <= 0 {
		return "", fmt.Errorf("fps metadata missing; cannot map segment to frame")
	}
	startFrame, _ := segmentFrameRange(it.segments[segIndex], fps, it.frameCount)
	if startFrame <= 0 {
		return "", fmt.Errorf("invalid frame estimate")
	}
	imagesDir := filepath.Join(it.path, "images")
	ext := it.frameExt
	if strings.TrimSpace(ext) == "" {
		ext = "png"
	}
	candidate := filepath.Join(imagesDir, fmt.Sprintf("frame-%04d.%s", startFrame, ext))
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	glob, err := filepath.Glob(filepath.Join(imagesDir, fmt.Sprintf("frame-%04d.*", startFrame)))
	if err == nil && len(glob) > 0 {
		return glob[0], nil
	}
	return "", fmt.Errorf("estimated frame not found: %04d", startFrame)
}

func runFPS(it runItem) float64 {
	if it.metadataPresent && it.metadata != nil {
		return it.metadata.FPS
	}
	return 0
}

func openPath(target string) error {
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

func copyTextToClipboard(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("empty text")
	}
	try := func(name string, args ...string) error {
		if _, err := clipboardLookPath(name); err != nil {
			return err
		}
		return clipboardRunCmd(name, args, text)
	}
	if runtimeGOOS == "darwin" {
		return try("pbcopy")
	}
	if runtimeGOOS == "windows" {
		return try("clip")
	}
	if err := try("wl-copy"); err == nil {
		return nil
	}
	if err := try("xclip", "-selection", "clipboard"); err == nil {
		return nil
	}
	if err := try("xsel", "--clipboard", "--input"); err == nil {
		return nil
	}
	return fmt.Errorf("no clipboard tool found (tried wl-copy/xclip/xsel/clip/pbcopy)")
}

func trimText(text string, maxLen int) string {
	if strings.TrimSpace(text) == "" {
		return "(no transcript)"
	}
	if len(text) <= maxLen {
		return text
	}
	return strings.TrimSpace(text[:maxLen]) + "..."
}

func oneLine(text string) string {
	t := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if len(t) > 96 {
		return t[:96] + "..."
	}
	return t
}

func findSegmentMatches(segments []transcriptSegment, query string) []int {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	out := make([]int, 0, len(segments))
	for i, seg := range segments {
		if strings.Contains(strings.ToLower(seg.Text), q) {
			out = append(out, i)
		}
	}
	return out
}

func frameRange(first string, last string) string {
	if first == "" && last == "" {
		return "-"
	}
	return first + " -> " + last
}

func humanDuration(sec float64) string {
	if sec <= 0 {
		return "-"
	}
	d := time.Duration(sec * float64(time.Second)).Round(time.Second)
	return d.String()
}

func humanTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC822)
}

func formatSeconds(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := int(sec) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func wrapLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}
	runes := []rune(line)
	if len(runes) <= width {
		return []string{line}
	}
	out := make([]string, 0, (len(runes)/width)+1)
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		out = append(out, string(runes))
	}
	return out
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
