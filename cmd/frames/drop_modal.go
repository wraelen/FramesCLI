package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dropModalResult struct {
	FPS  float64
	Opts extractWorkflowOptions
}

type dropModalModel struct {
	path       string
	fpsInput   string
	opts       extractWorkflowOptions
	cursor     int
	confirming bool
	done       bool
	cancelled  bool
	errText    string
}

func runDropModal(path string, fps float64, opts extractWorkflowOptions) (dropModalResult, bool, error) {
	m := dropModalModel{
		path:     path,
		fpsInput: fmt.Sprintf("%.2f", fps),
		opts:     opts,
	}
	finalModel, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return dropModalResult{}, false, err
	}
	out := finalModel.(dropModalModel)
	if out.cancelled {
		return dropModalResult{}, true, nil
	}
	parsedFPS, err := strconv.ParseFloat(strings.TrimSpace(out.fpsInput), 64)
	if err != nil || parsedFPS <= 0 {
		return dropModalResult{}, false, fmt.Errorf("invalid fps value")
	}
	return dropModalResult{FPS: parsedFPS, Opts: out.opts}, false, nil
}

func (m dropModalModel) Init() tea.Cmd { return nil }

func (m dropModalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = clampInt(m.cursor-1, 0, 8)
			m.errText = ""
		case "down", "j":
			m.cursor = clampInt(m.cursor+1, 0, 8)
			m.errText = ""
		case "left", "h":
			m.adjustField(-1)
			m.errText = ""
		case "right", "l":
			m.adjustField(1)
			m.errText = ""
		case "backspace":
			if m.cursor == 0 {
				r := []rune(m.fpsInput)
				if len(r) > 0 {
					m.fpsInput = string(r[:len(r)-1])
				}
			}
		case "enter":
			if m.cursor == 7 {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(m.fpsInput), 64)
				if err != nil || parsed <= 0 {
					m.errText = "FPS must be a positive number."
					return m, nil
				}
				m.done = true
				return m, tea.Quit
			}
			if m.cursor == 8 {
				m.cancelled = true
				m.done = true
				return m, tea.Quit
			}
			m.adjustField(1)
			m.errText = ""
		default:
			if m.cursor == 0 && len(msg.Runes) > 0 && !msg.Alt {
				r := msg.Runes[0]
				if (r >= '0' && r <= '9') || r == '.' {
					m.fpsInput += string(r)
				}
			}
		}
	}
	return m, nil
}

func (m *dropModalModel) adjustField(delta int) {
	switch m.cursor {
	case 1:
		m.opts.Voice = !m.opts.Voice
	case 2:
		if m.opts.FrameFormat == "png" {
			m.opts.FrameFormat = "jpg"
		} else {
			m.opts.FrameFormat = "png"
		}
	case 3:
		presets := []string{"laptop-safe", "balanced", "high-fidelity"}
		m.opts.Preset = rotateChoice(presets, m.opts.Preset, delta)
	case 4:
		m.opts.NoSheet = !m.opts.NoSheet
	case 5:
		m.opts.SheetCols = clampInt(m.opts.SheetCols+delta, 1, 24)
	case 6:
		m.opts.Verbose = !m.opts.Verbose
	}
}

func (m dropModalModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fbbf24")).Render("Drop Config")
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color("#a8a29e")).Render("Review options, then press Enter on Start extraction.")
	box := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#f59e0b")).Padding(1, 2)

	fields := []string{
		fmt.Sprintf("FPS: %s", m.fpsInput),
		fmt.Sprintf("Voice transcript: %s", onOff(m.opts.Voice)),
		fmt.Sprintf("Frame format: %s", m.opts.FrameFormat),
		fmt.Sprintf("Performance preset: %s", m.opts.Preset),
		fmt.Sprintf("Skip contact sheet: %s", onOff(m.opts.NoSheet)),
		fmt.Sprintf("Sheet columns: %d", m.opts.SheetCols),
		fmt.Sprintf("Verbose logs: %s", onOff(m.opts.Verbose)),
		"Start extraction",
		"Cancel",
	}
	lines := []string{
		title,
		sub,
		"",
		"Video:",
		trimForModal(m.path, 90),
		"",
	}
	for i, f := range fields {
		marker := "  "
		if i == m.cursor {
			marker = "> "
		}
		lines = append(lines, marker+f)
	}
	lines = append(lines, "", "Keys: [Up/Down] move  [Left/Right] change  [Enter] select  [q] cancel")
	if strings.TrimSpace(m.errText) != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#fb7185")).Bold(true).Render(m.errText))
	}
	return lipgloss.Place(120, 30, lipgloss.Center, lipgloss.Center, box.Render(strings.Join(lines, "\n")))
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

func clampInt(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
