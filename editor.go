package bed

import (
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"io"
	"regexp"
	"strings"
	"time"
)

// Model owns the textarea and its selection/layout state. Configure the embedded
// textarea's key map and styles as usual. Use New rather than the zero value.
// Like textarea, shallow copies are intended for Bubble Tea Update, not for
// maintaining independent documents; create a new Model for each editor.
type Model struct {
	// CursorLimit bounds active cursors (default 256, hard maximum 4096).
	CursorLimit int
	// AddCursorMouse can override Alt+left-click; nil disables cursor addition.
	AddCursorMouse                               func(tea.MouseMsg) bool
	cursors                                      []Selection
	primaryCursor, nextCursor, selectionRevision uint64
	cursorText, cursorDocument                   string
	// IndentCharacters identifies leading indentation; hosts may add an encoded tab rune.
	IndentCharacters string
	// TabWidth controls space-based tab stops; AutoIndent repeats leading spaces.
	TabWidth     int
	AutoIndent   bool
	viewTop      int
	detached     bool
	viewDocument string
	click        mouseClick
	// MultiClickInterval groups consecutive clicks at the same cell; zero disables.
	MultiClickInterval time.Duration
	mouseClock         func() time.Time
	chipCursor         int
	textarea.Model
	EditorKeys                                                      EditorKeyMap
	CompletionProvider                                              CompletionProvider
	GhostProvider                                                   GhostProvider
	CompletionTriggers                                              string
	CompletionColumns, CompletionRows                               int
	WordSeparators                                                  string
	ChipStyle, GhostStyle, CompletionStyle, CompletionSelectedStyle lipgloss.Style
	chips                                                           []Chip
	chipVersion                                                     uint64
	featureDocument, featureText                                    string
	requestID                                                       uint64
	featureFrame                                                    int
	completion                                                      *completionState
	ghost                                                           *ghostState
	// HistoryLimit and HistoryBytes bound retained edits (defaults: 100 and 8 MiB).
	// Set either to zero to disable retention; call ClearHistory to discard it immediately.
	HistoryLimit, HistoryBytes int
	UndoGroupDelay             time.Duration
	history                    *editHistory
	editDepth                  int
	historyClock               func() time.Time
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
	m.KeyMap.WordBackward = key.NewBinding(key.WithKeys("ctrl+left", "alt+left", "alt+b"))
	m.KeyMap.WordForward = key.NewBinding(key.WithKeys("ctrl+right", "alt+right", "alt+f"))
	m.EditorKeys = DefaultEditorKeyMap()
	m.TabWidth = 4
	m.CursorLimit = 256
	m.AddCursorMouse = func(v tea.MouseMsg) bool { return v.Alt }
	m.MultiClickInterval = 400 * time.Millisecond
	m.AutoIndent = true
	m.IndentCharacters = " "
	m.HistoryLimit = 100
	m.HistoryBytes = 8 << 20
	m.UndoGroupDelay = 750 * time.Millisecond
	m.CompletionColumns = 1
	m.CompletionRows = 5
	m.ChipStyle = lipgloss.NewStyle().Background(lipgloss.Color("237"))
	m.GhostStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	m.CompletionStyle = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	m.CompletionSelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("240"))
	m.ShowLineNumbers = false
	m.Gutter = func(line int) string { return fmt.Sprintf("%d ", (line+1)%10) }
	m.SetPromptFunc(2, func(int) string { return "  " })
	m.ScrollbarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	m.ScrollbarTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	return m
}

func (m *Model) Blur() { m.Close(); m.Model.Blur() }

func (m *Model) ClearSelection() {
	m.selection = nil
	m.cursors = nil
	m.selectionRevision++
	m.Close()
}
func (m *Model) SetValue(value string) {
	m.ClearSelection()
	m.detached = false
	m.layout = nil
	m.Model.SetValue(value)
	m.Close()
	m.chips = nil
	m.chipVersion++
	m.featureDocument = m.DocumentKey
	m.featureText = m.Value()
	m.ClearHistory()
}
func (m *Model) Reset() { m.SetValue("") }

// Update applies editing keys and mouse events relative to the widget origin.
// Applications intercept submit/completion keys before calling Update. Hosts
// with their own dispatch pipeline can use HandleKey and UpdateText separately.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	m.syncFeatures()
	if handled, cmd := m.receiveFeature(msg); handled {
		return m, cmd
	}
	if k, ok := msg.(tea.KeyMsg); ok {
		if handled, cmd := m.HandleKey(k); handled {
			return m, cmd
		}
	}
	if v, ok := msg.(tea.MouseMsg); ok {
		if m.CompletionMouse(v) {
			return m, nil
		}
		v.X -= m.GutterWidth
		handled := m.HandleMouse(v)
		if handled && v.Button == tea.MouseButtonLeft && v.Action == tea.MouseActionPress {
			return m, m.Focus()
		}
		return m, nil
	}
	return m.UpdateText(msg)
}

// UpdateText forwards an event to textarea after the host has called HandleKey.
func (m Model) UpdateText(msg tea.Msg) (Model, tea.Cmd) {
	had := m.completion != nil
	cmd := m.updateText(msg)
	return m, tea.Batch(cmd, m.afterInput(msg, had))
}
func (m *Model) updateText(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok && m.Focused() {
		if handled, cmd := m.multiKey(k); handled {
			return cmd
		}
	}
	if p, ok := msg.(multiPasteMsg); ok {
		_, cmd := editResult(m.InsertText(string(p)))
		return cmd
	}
	if _, ok := msg.(tea.KeyMsg); ok && m.Focused() {
		m.FollowCursor()
	}
	kind := m.editKind(msg)
	done := m.beginEdit(kind)
	defer done()
	if k, ok := msg.(tea.KeyMsg); ok && m.Focused() {
		if k.Paste || (key.Matches(k, m.KeyMap.InsertNewline) && !m.AutoIndent) || ((k.Type == tea.KeyRunes || k.Type == tea.KeySpace) && !m.baseBindingMatches(k)) {
			m.DeleteSelection()
		} else if kind == "" {
			m.BreakUndoGroup()
		}
	}

	if k, ok := msg.(tea.KeyMsg); ok && !k.Paste && m.Focused() {
		if key.Matches(k, m.KeyMap.InsertNewline) && m.AutoIndent {
			if err := m.InsertNewline(); err != nil {
				return func() tea.Msg { return EditErrorMsg{err} }
			}
			return nil
		}
		if key.Matches(k, m.KeyMap.WordBackward, m.KeyMap.WordForward) {
			m.ClearSelection()
			m.moveWord(key.Matches(k, m.KeyMap.WordForward))
			return nil
		}
	}
	var cmd tea.Cmd
	beforePos := Position(*m)
	if k, ok := msg.(tea.KeyMsg); ok && !k.Paste && m.Focused() && key.Matches(k, m.KeyMap.DeleteWordBackward, m.KeyMap.DeleteWordForward) {
		m.deleteWord(key.Matches(k, m.KeyMap.DeleteWordForward))
		return nil
	}
	m.Model, cmd = m.Model.Update(msg)
	if m.Value() == m.featureText && Position(*m) != beforePos {
		m.snapPosition(beforePos)
	}
	return cmd
}

// View decorates textarea with logical line numbers, whitespace and selection.
// Use RenderScrollbar(View(), width) when reserving an extra scrollbar column.
func (m Model) View() string {
	m.syncFeatures()
	return m.RenderCompletions(m.RenderGhost(m.RenderSelection(m.RenderChips(m.RenderDisplay(m.RawView())))))
}

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
