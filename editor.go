package bed

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"io"
	"regexp"
	"strings"
)

// Model owns the textarea and its selection/layout state. Configure the embedded
// textarea's key map and styles as usual. Use New rather than the zero value.
// Like textarea, shallow copies are intended for Bubble Tea Update, not for
// maintaining independent documents; create a new Model for each editor.
type Model struct {
	textarea.Model
	// DocumentKey invalidates selections when switching between application documents.
	DocumentKey string
	// AtomicTokens are labels selected as a whole. Their payloads belong to the app.
	AtomicTokens   []string
	ShowWhitespace bool
	GutterWidth    int
	// Gutter returns single-cell characters for a logical line (zero based).
	Gutter                              func(int) string
	SelectionColor                      int
	ScrollbarStyle, ScrollbarTrackStyle lipgloss.Style
	selection                           *selection
	layout                              *layout
}

// CopyMsg asks the host to copy text; bed does not access the system clipboard.
type CopyMsg string

func New() Model {
	m := Model{Model: textarea.New(), GutterWidth: 2, SelectionColor: 240}
	m.ShowLineNumbers = false
	m.Gutter = func(line int) string { return fmt.Sprintf("%d ", (line+1)%10) }
	m.SetPromptFunc(2, func(int) string { return "  " })
	m.ScrollbarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	m.ScrollbarTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	return m
}

func (m *Model) ClearSelection() { m.selection = nil }
func (m *Model) SetValue(value string) {
	m.ClearSelection()
	m.layout = nil
	m.Model.SetValue(value)
}
func (m *Model) Reset() { m.ClearSelection(); m.layout = nil; m.Model.Reset() }

// Update applies editing keys and mouse events relative to the widget origin.
// Applications intercept submit/completion keys before calling Update. Hosts
// with their own dispatch pipeline can use HandleKey and UpdateText separately.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if handled, cmd := m.HandleKey(k); handled {
			return m, cmd
		}
	}
	if v, ok := msg.(tea.MouseMsg); ok {
		v.X -= m.GutterWidth
		m.HandleMouse(v)
		return m, nil
	}
	return m.UpdateText(msg)
}

// UpdateText forwards an event to textarea after the host has called HandleKey.
func (m Model) UpdateText(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	return m, cmd
}

// View decorates textarea with logical line numbers, whitespace and selection.
// Use RenderScrollbar(View(), width) when reserving an extra scrollbar column.
func (m Model) View() string { return m.RenderSelection(m.RenderDisplay(m.Model.View())) }

var sgrPattern = regexp.MustCompile("\x1b\\[([0-9;]*)m")
var cursorProbeStyle = func() lipgloss.Style {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	return r.NewStyle()
}()

func widgetCursor(view string) (x, y int, ok bool) {
	for row, line := range strings.Split(view, "\n") {
		for _, match := range sgrPattern.FindAllStringSubmatchIndex(line, -1) {
			for _, code := range strings.Split(line[match[2]:match[3]], ";") {
				if code == "7" {
					return ansi.StringWidth(line[:match[0]]), row, true
				}
			}
		}
	}
	return 0, 0, false
}
func indexedBackground(line string, index int) string {
	if lipgloss.ColorProfile().Name() == "Ascii" {
		return line
	}
	background := fmt.Sprintf("\x1b[48;5;%dm", index)
	return background + sgrPattern.ReplaceAllStringFunc(line, func(s string) string { return s + background }) + "\x1b[0m"
}
