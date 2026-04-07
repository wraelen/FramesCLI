package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "github.com/wraelen/framescli/internal/config"
)

type setupFieldKind int

const (
	setupFieldText setupFieldKind = iota
	setupFieldChoice
)

type setupField struct {
	label       string
	help        string
	section     string
	kind        setupFieldKind
	isPath      bool
	choices     []string
	input       textinput.Model
	value       string
	customValue string
}

type setupStep struct {
	title    string
	subtitle string
	fields   []int
}

type setupWizardModel struct {
	fields       []setupField
	step         int
	cursor       int
	status       string
	errText      string
	saved        bool
	cancelled    bool
	cfg          appconfig.Config
	editing      bool
	windowWidth  int
	windowHeight int
	advanced     bool
}

type setupWizardResult struct {
	Config appconfig.Config
}

var setupDirectoryPicker = pickDirectoryCLI

func runSetupWizard(cfg appconfig.Config) (setupWizardResult, bool, error) {
	m := newSetupWizardModel(cfg)
	finalModel, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return setupWizardResult{}, false, err
	}
	out := finalModel.(setupWizardModel)
	if out.cancelled {
		return setupWizardResult{}, true, nil
	}
	return setupWizardResult{Config: out.configFromFields()}, false, nil
}

func newSetupWizardModel(cfg appconfig.Config) setupWizardModel {
	makeInput := func(value string, width int) textinput.Model {
		ti := textinput.New()
		ti.SetValue(value)
		ti.Width = width
		ti.Prompt = ""
		ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24"))
		return ti
	}

	supportedModels := appconfig.SupportedWhisperModels()
	modelChoices := append([]string{}, supportedModels...)
	if !containsString(modelChoices, cfg.WhisperModel) {
		modelChoices = append(modelChoices, "custom")
	}

	fields := []setupField{
		{
			label:   "Frames root directory",
			help:    "Where extracted runs, contact sheets, and metadata will be written.",
			section: "Storage",
			kind:    setupFieldText,
			isPath:  true,
			input:   makeInput(cfg.FramesRoot, 48),
		},
		{
			label:   "Primary recordings directory",
			help:    "The folder FramesCLI should watch first when you want the newest recording quickly.",
			section: "Video Sources",
			kind:    setupFieldText,
			isPath:  true,
			input:   makeInput(cfg.OBSVideoDir, 48),
		},
		{
			label:   "Recent video directories",
			help:    "Optional extra directories searched by 'extract recent'. Comma-separated.",
			section: "Video Sources",
			kind:    setupFieldText,
			isPath:  true,
			input:   makeInput(strings.Join(cfg.RecentVideoDirs, ", "), 48),
		},
		{
			label:   "Recent file extensions",
			help:    "Extensions FramesCLI should treat as recordings.",
			section: "Video Sources",
			kind:    setupFieldText,
			input:   makeInput(strings.Join(cfg.RecentExts, ", "), 24),
		},
		{
			label:   "Default FPS",
			help:    "Lower is faster. Higher keeps more context from longer recordings.",
			section: "Extraction Defaults",
			kind:    setupFieldText,
			input:   makeInput(fmt.Sprintf("%.2f", cfg.DefaultFPS), 10),
		},
		{
			label:   "Default frame format",
			help:    "jpg is smaller and faster; png is lossless.",
			section: "Extraction Defaults",
			kind:    setupFieldChoice,
			choices: []string{"png", "jpg"},
			value:   defaultChoice(cfg.DefaultFormat, []string{"png", "jpg"}),
		},
		{
			label:   "Hardware acceleration",
			help:    "auto is a good first choice on most machines.",
			section: "Extraction Defaults",
			kind:    setupFieldChoice,
			choices: []string{"none", "auto", "cuda", "vaapi", "qsv"},
			value:   defaultChoice(cfg.HWAccel, []string{"none", "auto", "cuda", "vaapi", "qsv"}),
		},
		{
			label:   "Performance mode",
			help:    "balanced is the safest default. fast lowers overhead; safe favors compatibility.",
			section: "Extraction Defaults",
			kind:    setupFieldChoice,
			choices: []string{"safe", "balanced", "fast"},
			value:   defaultChoice(cfg.PerformanceMode, []string{"safe", "balanced", "fast"}),
		},
		{
			label:   "Transcription backend",
			help:    "auto will prefer faster-whisper if available, otherwise whisper.",
			section: "Transcription",
			kind:    setupFieldChoice,
			choices: []string{"auto", "whisper", "faster-whisper"},
			value:   defaultChoice(cfg.TranscribeBackend, []string{"auto", "whisper", "faster-whisper"}),
		},
		{
			label:   "Whisper executable",
			help:    "Binary name or full path for whisper.",
			section: "Transcription",
			kind:    setupFieldText,
			input:   makeInput(cfg.WhisperBin, 28),
		},
		{
			label:   "Faster-Whisper executable",
			help:    "Binary name or full path for faster-whisper.",
			section: "Transcription",
			kind:    setupFieldText,
			input:   makeInput(cfg.FasterWhisperBin, 28),
		},
		{
			label:       "Whisper model",
			help:        "base is a practical default. Choose custom only if you already know the exact model name.",
			section:     "Transcription",
			kind:        setupFieldChoice,
			choices:     append(modelChoices, "custom"),
			value:       choiceValueForModel(cfg.WhisperModel, supportedModels),
			customValue: cfg.WhisperModel,
			input:       makeInput(customModelValue(cfg.WhisperModel, supportedModels), 20),
		},
		{
			label:   "Whisper language",
			help:    "Leave blank to let the backend auto-detect language.",
			section: "Transcription",
			kind:    setupFieldText,
			input:   makeInput(cfg.WhisperLanguage, 16),
		},
	}

	return setupWizardModel{
		cfg:          cfg,
		fields:       fields,
		windowWidth:  110,
		windowHeight: 32,
		status:       "Quick Setup selected. Press Enter to continue, or press A for Advanced Setup.",
	}
}

func (m setupWizardModel) Init() tea.Cmd { return nil }

func (m setupWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.editing {
			return m.updateWhileEditing(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancelled = true
			return m, tea.Quit
		case "a":
			if m.step == 0 {
				m.advanced = !m.advanced
				if m.advanced {
					m.status = "Advanced Setup selected. More options will be shown."
				} else {
					m.status = "Quick Setup selected. FramesCLI will keep sane defaults for the rest."
				}
				m.errText = ""
			}
			return m, nil
		case "s", "ctrl+s":
			if !m.isReviewStep() {
				m.errText = "Save is only available on the review step."
				m.status = "Move to Review, then save."
				return m, nil
			}
			cfg, err := m.validatedConfig()
			if err != nil {
				m.errText = err.Error()
				m.status = "Fix the highlighted issue before saving."
				return m, nil
			}
			m.cfg = cfg
			m.saved = true
			return m, tea.Quit
		case "n":
			m.nextStep()
			return m, nil
		case "b":
			m.prevStep()
			return m, nil
		case "up", "k":
			m.moveCursor(-1)
			return m, nil
		case "down", "j":
			m.moveCursor(1)
			return m, nil
		case "left", "h":
			m.rotateChoice(-1)
			return m, nil
		case "right", "l", " ":
			m.rotateChoice(1)
			return m, nil
		case "p":
			if err := m.pickSelectedPath(); err != nil {
				m.errText = err.Error()
				m.status = "Could not pick a folder for this field."
				return m, nil
			}
			return m, nil
		case "o":
			if err := m.openSelectedPath(); err != nil {
				m.errText = err.Error()
				m.status = "Could not open the selected path."
				return m, nil
			}
			return m, nil
		case "enter":
			if m.step == 0 {
				m.nextStep()
				return m, nil
			}
			if m.isReviewStep() {
				cfg, err := m.validatedConfig()
				if err != nil {
					m.errText = err.Error()
					m.status = "Fix the highlighted issue before saving."
					return m, nil
				}
				m.cfg = cfg
				m.saved = true
				return m, tea.Quit
			}
			field := m.currentField()
			if field == nil {
				m.nextStep()
				return m, nil
			}
			if field.kind == setupFieldChoice {
				m.rotateChoice(1)
				return m, nil
			}
			m.startEditing()
			return m, nil
		case "esc":
			m.errText = ""
			m.status = m.defaultStatus()
			return m, nil
		}
	}
	return m, nil
}

func (m setupWizardModel) updateWhileEditing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	field := m.currentField()
	if field == nil {
		m.editing = false
		return m, nil
	}
	switch msg.String() {
	case "esc":
		m.stopEditing(false)
		m.status = fmt.Sprintf("%s unchanged.", field.label)
		return m, nil
	case "enter":
		m.stopEditing(true)
		m.status = fmt.Sprintf("%s updated.", field.label)
		m.errText = ""
		return m, nil
	}
	var cmd tea.Cmd
	field.input, cmd = field.input.Update(msg)
	return m, cmd
}

func (m *setupWizardModel) nextStep() {
	m.stopEditing(true)
	if m.step < len(m.steps())-1 {
		m.step++
		m.cursor = 0
		m.errText = ""
		m.status = m.defaultStatus()
	}
}

func (m *setupWizardModel) prevStep() {
	m.stopEditing(true)
	if m.step > 0 {
		m.step--
		m.cursor = 0
		m.errText = ""
		m.status = m.defaultStatus()
	}
}

func (m *setupWizardModel) moveCursor(delta int) {
	m.stopEditing(true)
	step := m.currentStep()
	if len(step.fields) == 0 {
		return
	}
	m.cursor = clampInt(m.cursor+delta, 0, len(step.fields)-1)
	m.errText = ""
	m.status = m.defaultStatus()
}

func (m *setupWizardModel) startEditing() {
	field := m.currentField()
	if field == nil || field.kind != setupFieldText {
		return
	}
	field.input.Focus()
	field.input.CursorEnd()
	m.editing = true
	m.errText = ""
	m.status = fmt.Sprintf("Editing %s. Press Enter to apply or Esc to cancel.", strings.ToLower(field.label))
}

func (m *setupWizardModel) stopEditing(apply bool) {
	field := m.currentField()
	if field == nil || field.kind != setupFieldText {
		m.editing = false
		return
	}
	if !m.editing {
		return
	}
	field.input.Blur()
	if !apply && field.label == "Whisper model" {
		field.input.SetValue(field.customValue)
	}
	if field.label == "Whisper model" {
		field.customValue = strings.TrimSpace(field.input.Value())
	}
	m.editing = false
}

func (m *setupWizardModel) rotateChoice(delta int) {
	field := m.currentField()
	if field == nil || field.kind != setupFieldChoice || len(field.choices) == 0 {
		return
	}
	field.value = rotateChoice(field.choices, field.value, delta)
	if field.label == "Whisper model" && field.value == "custom" {
		m.status = "Whisper model set to custom. Press Enter to type a model name."
		m.errText = ""
		return
	}
	m.status = fmt.Sprintf("%s set to %s.", field.label, field.value)
	m.errText = ""
}

func (m setupWizardModel) configFromFields() appconfig.Config {
	cfg := m.cfg
	for _, field := range m.fields {
		var value string
		switch field.label {
		case "Whisper model":
			if field.value == "custom" {
				value = strings.TrimSpace(field.input.Value())
			} else {
				value = strings.TrimSpace(field.value)
			}
		default:
			if field.kind == setupFieldText {
				value = strings.TrimSpace(field.input.Value())
			} else {
				value = strings.TrimSpace(field.value)
			}
		}
		switch field.label {
		case "Frames root directory":
			cfg.FramesRoot = value
		case "Primary recordings directory":
			cfg.OBSVideoDir = value
		case "Recent video directories":
			cfg.RecentVideoDirs = splitCSVPaths(value)
		case "Recent file extensions":
			cfg.RecentExts = splitCSVExts(value)
		case "Default FPS":
			cfg.DefaultFPS = appconfig.ParseFPS(value, cfg.DefaultFPS)
		case "Default frame format":
			cfg.DefaultFormat = value
		case "Hardware acceleration":
			cfg.HWAccel = value
		case "Performance mode":
			cfg.PerformanceMode = value
		case "Transcription backend":
			cfg.TranscribeBackend = value
		case "Whisper executable":
			cfg.WhisperBin = value
		case "Faster-Whisper executable":
			cfg.FasterWhisperBin = value
		case "Whisper model":
			cfg.WhisperModel = value
		case "Whisper language":
			cfg.WhisperLanguage = value
		}
	}
	if !m.advanced && strings.TrimSpace(cfg.OBSVideoDir) != "" && len(cfg.RecentVideoDirs) == 0 {
		cfg.RecentVideoDirs = []string{cfg.OBSVideoDir}
	}
	return cfg
}

func (m setupWizardModel) validatedConfig() (appconfig.Config, error) {
	cfg := m.configFromFields()
	if strings.TrimSpace(cfg.FramesRoot) == "" {
		return cfg, fmt.Errorf("frames root directory cannot be empty")
	}
	if strings.TrimSpace(cfg.OBSVideoDir) == "" {
		return cfg, fmt.Errorf("primary recordings directory cannot be empty")
	}
	fpsField := ""
	for _, field := range m.fields {
		if field.label == "Default FPS" {
			fpsField = strings.TrimSpace(field.input.Value())
			break
		}
	}
	fps, err := strconv.ParseFloat(fpsField, 64)
	if err != nil || fps <= 0 {
		return cfg, fmt.Errorf("default FPS must be a positive number")
	}
	cfg.DefaultFPS = fps
	if strings.TrimSpace(cfg.WhisperModel) == "" {
		return cfg, fmt.Errorf("whisper model cannot be empty")
	}
	return cfg, nil
}

func (m setupWizardModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fbbf24"))
	subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a8a29e"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#cbd5e1"))
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f8fafc"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24")).Bold(true)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#94a3b8"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fb7185")).Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#34d399"))
	hotkeyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#020617")).Background(lipgloss.Color("#67e8f9")).Padding(0, 1).Bold(true)
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a")).Background(lipgloss.Color("#fbbf24")).Padding(0, 1).Bold(true)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#f59e0b")).Padding(1, 2)
	brandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24")).Bold(true)

	headerLines := []string{
		brandStyle.Render(setupWordmark()),
		titleStyle.Render("FramesCLI Setup"),
		subStyle.Render(m.currentStep().title + " - " + m.currentStep().subtitle),
		subStyle.Render(m.stepIndicator()),
		"",
	}

	var body []string
	if m.step == 0 {
		body = m.renderWelcome(hotkeyStyle, helpStyle, valueStyle, tagStyle)
	} else if m.isReviewStep() {
		body = m.renderReview(labelStyle, valueStyle, helpStyle)
	} else {
		body = m.renderFields(labelStyle, valueStyle, cursorStyle, helpStyle, hotkeyStyle, tagStyle)
	}

	footerLines := []string{okStyle.Render(m.footerHelp())}
	if strings.TrimSpace(m.errText) != "" {
		footerLines = append(footerLines, errorStyle.Render(m.errText))
	} else {
		footerLines = append(footerLines, helpStyle.Render(m.status))
	}

	visibleBody, hiddenAbove, hiddenBelow := visibleSetupLines(body, m.cursorLineHint(), m.bodyHeight(len(headerLines), len(footerLines)))
	lines := append([]string{}, headerLines...)
	if hiddenAbove > 0 {
		lines = append(lines, helpStyle.Render(fmt.Sprintf("... %d lines above ...", hiddenAbove)))
	}
	lines = append(lines, visibleBody...)
	if hiddenBelow > 0 {
		lines = append(lines, helpStyle.Render(fmt.Sprintf("... %d lines below ...", hiddenBelow)))
	}
	lines = append(lines, "")
	lines = append(lines, footerLines...)

	content := box.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.placeWidth(), m.placeHeight(), lipgloss.Left, lipgloss.Top, content)
}

func (m setupWizardModel) renderWelcome(hotkeyStyle, helpStyle, valueStyle, tagStyle lipgloss.Style) []string {
	mode := "Quick Setup"
	if m.advanced {
		mode = "Advanced Setup"
	}
	return []string{
		valueStyle.Render("Welcome."),
		helpStyle.Render("This wizard is intentionally shorter now. Quick Setup only asks for the essentials."),
		"",
		tagStyle.Render(mode),
		helpStyle.Render("Quick Setup asks for: where to store outputs, and where your recordings live."),
		helpStyle.Render("Advanced Setup also lets you change extraction and transcription defaults now."),
		"",
		hotkeyStyle.Render("Enter") + " " + helpStyle.Render("start setup"),
		hotkeyStyle.Render("A") + " " + helpStyle.Render("toggle quick/advanced"),
		hotkeyStyle.Render("Q") + " " + helpStyle.Render("cancel"),
	}
}

func (m setupWizardModel) renderFields(labelStyle, valueStyle, cursorStyle, helpStyle, hotkeyStyle, tagStyle lipgloss.Style) []string {
	step := m.currentStep()
	lines := []string{}
	for pos, fieldIndex := range step.fields {
		field := &m.fields[fieldIndex]
		marker := "  "
		if pos == m.cursor {
			marker = cursorStyle.Render("> ")
		}
		display := ""
		switch {
		case field.label == "Whisper model" && field.value == "custom":
			model := strings.TrimSpace(field.input.Value())
			if model == "" {
				model = "(blank)"
			}
			display = valueStyle.Render("custom") + " " + helpStyle.Render("("+model+")")
		case field.kind == setupFieldText && pos == m.cursor && m.editing:
			display = field.input.View()
		case field.kind == setupFieldText:
			current := strings.TrimSpace(field.input.Value())
			if current == "" {
				current = "(blank)"
			}
			display = valueStyle.Render(current)
		default:
			display = valueStyle.Render(field.value)
		}
		lines = append(lines, marker+labelStyle.Render(field.label)+": "+display)
		if pos == m.cursor {
			lines = append(lines, "   "+helpStyle.Render(field.help))
			if field.kind == setupFieldText {
				if m.editing {
					lines = append(lines, "   "+tagStyle.Render("Editing")+" "+helpStyle.Render("Press Enter to apply or Esc to cancel."))
				} else {
					line := "   " + hotkeyStyle.Render("Enter") + " " + helpStyle.Render("edit")
					if field.isPath {
						line += "  " + hotkeyStyle.Render("P") + " " + helpStyle.Render("pick folder")
						line += "  " + hotkeyStyle.Render("O") + " " + helpStyle.Render("open folder")
					}
					lines = append(lines, line)
				}
			} else {
				lines = append(lines, "   "+hotkeyStyle.Render("Left/Right")+" "+helpStyle.Render("change value")+"  "+hotkeyStyle.Render("Enter")+" "+helpStyle.Render("next value"))
			}
		}
	}
	return lines
}

func (m setupWizardModel) renderReview(labelStyle, valueStyle, helpStyle lipgloss.Style) []string {
	cfg := m.configFromFields()
	lines := []string{
		valueStyle.Render("Review your setup"),
		helpStyle.Render("You can go back with B if anything looks wrong."),
		"",
		labelStyle.Render("Mode") + ": " + valueStyle.Render(map[bool]string{true: "Advanced Setup", false: "Quick Setup"}[m.advanced]),
		labelStyle.Render("Frames root") + ": " + valueStyle.Render(blankIfEmpty(cfg.FramesRoot)),
		labelStyle.Render("Primary recordings") + ": " + valueStyle.Render(blankIfEmpty(cfg.OBSVideoDir)),
	}
	if m.advanced {
		lines = append(lines,
			labelStyle.Render("Recent directories")+": "+valueStyle.Render(joinOrBlank(cfg.RecentVideoDirs)),
			labelStyle.Render("Recent extensions")+": "+valueStyle.Render(joinOrBlank(cfg.RecentExts)),
			labelStyle.Render("Default FPS")+": "+valueStyle.Render(fmt.Sprintf("%.2f", cfg.DefaultFPS)),
			labelStyle.Render("Frame format")+": "+valueStyle.Render(blankIfEmpty(cfg.DefaultFormat)),
			labelStyle.Render("Hardware accel")+": "+valueStyle.Render(blankIfEmpty(cfg.HWAccel)),
			labelStyle.Render("Performance mode")+": "+valueStyle.Render(blankIfEmpty(cfg.PerformanceMode)),
			labelStyle.Render("Transcribe backend")+": "+valueStyle.Render(blankIfEmpty(cfg.TranscribeBackend)),
		)
	} else {
		lines = append(lines,
			helpStyle.Render("Everything else will use the current defaults and can be changed later in `framescli setup`."),
		)
	}
	lines = append(lines, "",
		helpStyle.Render("Press Enter or S to save. Press B to go back."),
	)
	return lines
}

func (m setupWizardModel) steps() []setupStep {
	steps := []setupStep{
		{title: "Welcome", subtitle: "Pick how much setup you want right now"},
		{title: "Storage", subtitle: "Choose where FramesCLI writes outputs", fields: []int{0}},
		{title: "Recordings", subtitle: "Tell FramesCLI where your videos live", fields: []int{1}},
	}
	if m.advanced {
		steps = append(steps,
			setupStep{title: "Recent Search", subtitle: "Optional folders and file types for `extract recent`", fields: []int{2, 3}},
			setupStep{title: "Extraction Defaults", subtitle: "Tune extraction quality and performance", fields: []int{4, 5, 6, 7}},
			setupStep{title: "Transcription", subtitle: "Adjust transcript defaults if you need them", fields: []int{8, 9, 10, 11, 12}},
		)
	}
	steps = append(steps, setupStep{title: "Review", subtitle: "Save or go back and change something"})
	return steps
}

func (m setupWizardModel) currentStep() setupStep {
	steps := m.steps()
	if m.step < 0 {
		return steps[0]
	}
	if m.step >= len(steps) {
		return steps[len(steps)-1]
	}
	return steps[m.step]
}

func (m setupWizardModel) currentField() *setupField {
	step := m.currentStep()
	if len(step.fields) == 0 || m.cursor < 0 || m.cursor >= len(step.fields) {
		return nil
	}
	return &m.fields[step.fields[m.cursor]]
}

func (m setupWizardModel) isReviewStep() bool {
	return m.step == len(m.steps())-1
}

func (m setupWizardModel) defaultStatus() string {
	if m.step == 0 {
		if m.advanced {
			return "Advanced Setup selected. Press Enter to continue."
		}
		return "Quick Setup selected. Press Enter to continue."
	}
	if m.isReviewStep() {
		return "Review your settings, then press Enter or S to save."
	}
	return "Use Enter to edit. Use N for next step and B to go back."
}

func (m setupWizardModel) stepIndicator() string {
	parts := make([]string, 0, len(m.steps()))
	for i, step := range m.steps() {
		label := fmt.Sprintf("%d %s", i+1, step.title)
		if i == m.step {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}

func (m setupWizardModel) footerHelp() string {
	if m.step == 0 {
		return "Actions: [enter] continue  [a] toggle quick/advanced  [q] cancel"
	}
	if m.isReviewStep() {
		return "Actions: [enter] save  [s] save  [b] back  [q] cancel"
	}
	field := m.currentField()
	if field != nil && field.isPath {
		return "Actions: [enter] edit/change  [p] pick folder  [o] open folder  [n] next  [b] back  [q] cancel"
	}
	return "Actions: [enter] edit/change  [n] next  [b] back  [q] cancel"
}

func (m setupWizardModel) cursorLineHint() int {
	if m.step == 0 {
		return 0
	}
	if m.isReviewStep() {
		return 0
	}
	return m.cursor * 3
}

func (m setupWizardModel) placeWidth() int {
	if m.windowWidth <= 0 {
		return 110
	}
	return m.windowWidth
}

func (m setupWizardModel) placeHeight() int {
	if m.windowHeight <= 0 {
		return 32
	}
	return m.windowHeight
}

func (m setupWizardModel) bodyHeight(headerLines, footerLines int) int {
	height := m.placeHeight() - (headerLines + footerLines + 7)
	if height < 8 {
		return 8
	}
	return height
}

func visibleSetupLines(lines []string, cursorLine, maxVisible int) ([]string, int, int) {
	if len(lines) <= maxVisible {
		return lines, 0, 0
	}
	start := cursorLine - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(lines) {
		end = len(lines)
		start = end - maxVisible
	}
	if start < 0 {
		start = 0
	}
	return lines[start:end], start, len(lines) - end
}

func (m *setupWizardModel) openSelectedPath() error {
	field := m.currentField()
	if field == nil || !field.isPath {
		return fmt.Errorf("open path is only available for directory fields")
	}
	target := resolveOpenPath(field.input.Value())
	if target == "" {
		return fmt.Errorf("no folder is set yet for this field")
	}
	if err := openPathCLI(target); err != nil {
		return err
	}
	m.errText = ""
	m.status = fmt.Sprintf("Opened %s in your file explorer.", target)
	return nil
}

func (m *setupWizardModel) pickSelectedPath() error {
	field := m.currentField()
	if field == nil || !field.isPath {
		return fmt.Errorf("folder picker is only available for directory fields")
	}
	initial := resolveOpenPath(field.input.Value())
	selected, err := setupDirectoryPicker(initial)
	if err != nil {
		return err
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		m.errText = ""
		m.status = "Folder picker canceled."
		return nil
	}
	if field.label == "Recent video directories" {
		current := splitCSVPaths(field.input.Value())
		if !containsString(current, selected) {
			current = append(current, selected)
		}
		field.input.SetValue(strings.Join(current, ", "))
	} else {
		field.input.SetValue(selected)
	}
	m.errText = ""
	m.status = fmt.Sprintf("%s set to %s.", field.label, selected)
	return nil
}

func resolveOpenPath(raw string) string {
	parts := splitCSVPaths(raw)
	candidates := parts
	if len(candidates) == 0 {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			candidates = []string{trimmed}
		}
	}
	for _, candidate := range candidates {
		target := expandUserHome(candidate)
		if target == "" {
			continue
		}
		if info, err := os.Stat(target); err == nil {
			if info.IsDir() {
				return target
			}
			return filepath.Dir(target)
		}
		if ancestor := nearestExistingDir(target); ancestor != "" {
			return ancestor
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

func nearestExistingDir(path string) string {
	current := filepath.Clean(path)
	for current != "" && current != "." && current != string(filepath.Separator) {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			return current
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	if info, err := os.Stat(string(filepath.Separator)); err == nil && info.IsDir() {
		return string(filepath.Separator)
	}
	return ""
}

func expandUserHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func pickDirectoryCLI(initial string) (string, error) {
	initial = resolveOpenPath(initial)
	if runtime.GOOS == "darwin" {
		return pickDirectoryMac(initial)
	}
	if runtime.GOOS == "windows" {
		return pickDirectoryWindows(initial)
	}
	if _, err := exec.LookPath("wslpath"); err == nil {
		return pickDirectoryWSL(initial)
	}
	return pickDirectoryLinux(initial)
}

func pickDirectoryWSL(initial string) (string, error) {
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		return "", fmt.Errorf("folder picker unavailable: powershell.exe not found")
	}
	winInitial := ""
	if initial != "" {
		out, err := exec.Command("wslpath", "-w", initial).Output()
		if err == nil {
			winInitial = strings.TrimSpace(string(out))
		}
	}
	selected, err := runWindowsFolderPicker(winInitial)
	if err != nil || strings.TrimSpace(selected) == "" {
		return selected, err
	}
	out, err := exec.Command("wslpath", "-u", strings.TrimSpace(selected)).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pickDirectoryWindows(initial string) (string, error) {
	return runWindowsFolderPicker(initial)
}

func runWindowsFolderPicker(initial string) (string, error) {
	script := strings.Join([]string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"$dialog = New-Object System.Windows.Forms.FolderBrowserDialog",
		"$dialog.Description = 'Select a folder for FramesCLI'",
		"$dialog.ShowNewFolderButton = $true",
	}, "; ")
	if strings.TrimSpace(initial) != "" {
		safeInitial := strings.ReplaceAll(initial, "'", "''")
		script += fmt.Sprintf("; $dialog.SelectedPath = '%s'", safeInitial)
	}
	script += "; if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $dialog.SelectedPath }"
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pickDirectoryMac(initial string) (string, error) {
	script := `POSIX path of (choose folder with prompt "Select a folder for FramesCLI")`
	if strings.TrimSpace(initial) != "" {
		safeInitial := strings.ReplaceAll(initial, `"`, `\"`)
		script = fmt.Sprintf(`POSIX path of (choose folder with prompt "Select a folder for FramesCLI" default location POSIX file "%s")`, safeInitial)
	}
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pickDirectoryLinux(initial string) (string, error) {
	if _, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--directory", "--title=Select a folder for FramesCLI"}
		if strings.TrimSpace(initial) != "" {
			args = append(args, "--filename", initial+string(filepath.Separator))
		}
		out, err := exec.Command("zenity", args...).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--getexistingdirectory"}
		if strings.TrimSpace(initial) != "" {
			args = append(args, initial)
		}
		out, err := exec.Command("kdialog", args...).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("folder picker unavailable: install zenity or kdialog, or type the path manually")
}

func defaultChoice(current string, choices []string) string {
	current = strings.TrimSpace(current)
	for _, choice := range choices {
		if current == choice {
			return choice
		}
	}
	if len(choices) == 0 {
		return ""
	}
	return choices[0]
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func choiceValueForModel(model string, supported []string) string {
	if containsString(supported, model) {
		return model
	}
	return "custom"
}

func customModelValue(model string, supported []string) string {
	if containsString(supported, model) {
		return ""
	}
	return model
}

func blankIfEmpty(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "(blank)"
	}
	return v
}

func joinOrBlank(items []string) string {
	if len(items) == 0 {
		return "(blank)"
	}
	return strings.Join(items, ", ")
}

func setupWordmark() string {
	return strings.Join([]string{
		"  ____                              ____ _     ___",
		" / ___|_ __ __ _ _ __ ___   ___  ___/ ___| |   |_ _|",
		"| |  _| '__/ _` | '_ ` _ \\ / _ \\/ __| |   | |    | |",
		"| |_| | | | (_| | | | | | |  __/\\__ \\ |___| |___ | |",
		" \\____|_|  \\__,_|_| |_| |_|\\___||___/\\____|_____|___|",
	}, "\n")
}
