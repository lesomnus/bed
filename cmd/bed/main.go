// bed opens one UTF-8 file for interactive editing.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/bed"
)

type app struct {
	editor    bed.Model
	document  fileDocument
	savedText string
	status    string
	quitArmed bool
	width     int
}
type pasted struct {
	text string
	err  error
}

func newApp(path string) (app, error) {
	d, text, err := openDocument(path)
	if err != nil {
		return app{}, err
	}
	m := app{editor: bed.New(), document: d, savedText: text, width: 80}
	m.editor.IndentCharacters = " " + string(d.tab)
	m.editor.CharLimit = 0
	m.editor.MaxHeight = 0
	m.editor.MaxWidth = 0
	m.editor.Placeholder = ""
	m.editor.SetWidth(79)
	m.editor.SetHeight(20)
	m.editor.SetValue(text)
	if m.editor.Value() != text {
		return app{}, fmt.Errorf("file exceeds the editor's supported text/line limits")
	}
	m.editor.SetPosition(0)
	m.editor.Focus()
	return m, nil
}
func (m app) Init() tea.Cmd { return m.editor.Focus() }
func (m app) dirty() bool   { return m.editor.Value() != m.savedText }
func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(4, v.Width)
		m.editor.SetWidth(m.width - 1)
		m.editor.SetHeight(max(1, v.Height-2))
		m.editor, _ = m.editor.UpdateText(nil)
		return m, nil
	case tea.KeyMsg:
		if !v.Paste {
			switch v.String() {
			case "ctrl+s":
				m.editor.BreakUndoGroup()
				m.quitArmed = false
				if err := m.document.save(m.editor.Value()); err != nil {
					m.status = "Save failed: " + err.Error()
				} else {
					m.savedText = m.editor.Value()
					m.status = "Saved"
				}
				return m, nil
			case "ctrl+q":
				if m.dirty() && !m.quitArmed {
					m.quitArmed = true
					m.status = "Unsaved changes: Ctrl+S to save, Ctrl+Q again to discard"
					return m, nil
				}
				return m, tea.Quit
			}
		}
		m.quitArmed = false
		m.status = ""
		if !v.Paste && key.Matches(v, m.editor.KeyMap.Paste) {
			return m, func() tea.Msg { text, err := clipboard.ReadAll(); return pasted{text, err} }
		}
		if v.Type == tea.KeyRunes {
			text, err := m.document.encodeInput(string(v.Runes))
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			v.Runes = []rune(text)
		}
		msg = v
	case pasted:
		if v.err != nil {
			m.status = "Paste failed: " + v.err.Error()
			return m, nil
		}
		return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(v.text), Paste: true})
	case bed.EditErrorMsg:
		m.status = "Edit failed: " + v.Err.Error()
		return m, nil
	case bed.CopyMsg:
		if err := clipboard.WriteAll(string(m.document.decode(string(v)))); err != nil {
			m.status = "Clipboard failed: " + err.Error()
		}
		return m, nil
	case tea.MouseMsg:
		m.quitArmed = false
		v.Y-- // header occupies one row
		msg = v
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}
func (m app) View() string {
	name := strconv.Quote(m.document.path)
	if m.dirty() {
		name += " *"
	}
	footer := "Ctrl+S save · Ctrl+Q quit · Ctrl+Z undo · Ctrl+Y redo"
	if m.status != "" {
		footer = m.status
	}
	view := m.editor.RenderScrollbar(m.editor.View(), m.width)
	view = strings.ReplaceAll(view, string(m.document.tab), "⇥")
	return ansi.Truncate(name, m.width, "…") + "\n" + view + "\n" + ansi.Truncate(footer, m.width, "…")
}
func run(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("Usage: bed <file>\nCtrl+S: save · Ctrl+Q: quit (press twice to discard unsaved changes)")
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: bed <file>")
	}
	m, err := newApp(args[0])
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run()
	return err
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bed:", err)
		os.Exit(1)
	}
}
