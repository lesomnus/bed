package bed

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func selectRange(m *Model, a, b int) {
	m.SetPosition(a)
	m.ExtendSelection(func() { m.SetPosition(b) })
}
func TestLineIndentUndoAndBoundary(t *testing.T) {
	m := testEditor()
	m.TabWidth = 2
	m.SetValue("a\n b\nc")
	selectRange(&m, 0, 5)
	if err := m.Indent(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "  a\n   b\nc" {
		t.Fatal(m.Value())
	}
	if !m.Undo() || m.SelectedText() != "a\n b\n" {
		t.Fatal("undo selection", m.SelectedText())
	}
	m.Redo()
	if err := m.Outdent(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "a\n b\nc" {
		t.Fatal(m.Value())
	}
}
func TestLineMoveAndDuplicate(t *testing.T) {
	for _, bounds := range [][2]int{{2, 6}, {6, 2}} {
		m := testEditor()
		m.SetValue("a\nb\nc\nd")
		selectRange(&m, bounds[0], bounds[1])
		if err := m.MoveLines(true); err != nil {
			t.Fatal(err)
		}
		if m.Value() != "a\nd\nb\nc" || m.SelectedText() != "b\nc" {
			t.Fatal(m.Value(), m.SelectedText())
		}
		m.Undo()
		if m.Value() != "a\nb\nc\nd" {
			t.Fatal("undo")
		}
		m.MoveLines(false)
		if m.Value() != "b\nc\na\nd" {
			t.Fatal(m.Value())
		}
	}
	m := testEditor()
	m.SetValue("한글\nlast")
	m.SetPosition(1)
	m.Duplicate()
	if m.Value() != "한글\n한글\nlast" || Position(m) != 4 {
		t.Fatal(m.Value(), Position(m))
	}
	m.Undo()
	selectRange(&m, 0, 2)
	m.Duplicate()
	if m.Value() != "한글한글\nlast" || m.SelectedText() != "한글" {
		t.Fatal(m.Value(), m.SelectedText())
	}
}
func TestLineEditsKeepChipsAndRejectOverflow(t *testing.T) {
	m := testEditor()
	m.SetValue("a\n[X]\nz")
	m.SetChips([]Chip{{ID: "x", Label: "[X]", Text: "payload", From: 2, To: 5}})
	m.SetPosition(2)
	m.MoveLines(false)
	c := m.Chips()[0]
	if c.From != 0 || c.Text != "payload" {
		t.Fatal(c)
	}
	m.Duplicate()
	if len(m.Chips()) != 2 || m.Chips()[0].ID == m.Chips()[1].ID {
		t.Fatal(m.Chips())
	}
	m.Undo()
	if len(m.Chips()) != 1 {
		t.Fatal(m.Chips())
	}
	before := m.Value()
	m.CharLimit = len([]rune(before))
	if err := m.Duplicate(); err == nil || m.Value() != before {
		t.Fatal("non-atomic overflow", err, m.Value())
	}
	m.CharLimit = 0
	selectRange(&m, 0, 3)
	m.Indent()
	if m.Chips()[0].From != 4 {
		t.Fatal(m.Chips())
	}
}
func TestSoftTabsAndAutoIndent(t *testing.T) {
	m := testEditor()
	m.TabWidth = 3
	m.SetValue("한")
	m.SetPosition(1)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.Value() != "한 " {
		t.Fatal(m.Value())
	}
	m.SetValue("    abc")
	selectRange(&m, 5, 7)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Value() != "    a\n    " {
		t.Fatal(m.Value())
	}
	m.Undo()
	if m.Value() != "    abc" || m.SelectedText() != "bc" {
		t.Fatal(m.Value(), m.SelectedText())
	}
	m.SetValue("  a")
	m.SetPosition(3)
	m.AutoIndent = false
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Value() != "  a\n" {
		t.Fatal(m.Value())
	}
	m.SetValue("  a")
	m.SetPosition(3)
	m.AutoIndent = true
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\nb"), Paste: true})
	if m.Value() != "  a\nb" {
		t.Fatal(m.Value())
	}
}
func TestLineBindingsRemapDisableAndChipPriority(t *testing.T) {
	m := testEditor()
	m.SetValue("a\nb")
	m.SetPosition(0)
	m.EditorKeys.MoveLinesDown.SetKeys("ctrl+j")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if m.Value() != "b\na" {
		t.Fatal(m.Value())
	}
	m.EditorKeys.Indent.SetEnabled(false)
	if handled, _ := m.HandleKey(tea.KeyMsg{Type: tea.KeyTab}); handled {
		t.Fatal("disabled tab handled")
	}
	m.EditorKeys.Indent.SetEnabled(true)
	m.SetValue("[x]")
	m.SetChips([]Chip{{ID: "x", Label: "[x]", From: 0, To: 3}})
	selectRange(&m, 0, 3)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := cmd().(ChipActivateMsg); !ok {
		t.Fatal("enter did not activate chip")
	}
}
func TestEmptyLineAndDocumentEdges(t *testing.T) {
	for _, s := range []string{"", "a", "a\n", "\na", "\n\n"} {
		m := testEditor()
		m.SetValue(s)
		m.SetPosition(0)
		m.MoveLines(false)
		if m.Value() != s {
			t.Fatal("top", s)
		}
		m.SetPosition(len([]rune(s)))
		m.MoveLines(true)
		if m.Value() != s {
			t.Fatal("bottom", s)
		}
		m.Duplicate()
		if strings.Count(m.Value(), "\n") != strings.Count(s, "\n")+1 {
			t.Fatal(m.Value())
		}
		m.Undo()
		if m.Value() != s {
			t.Fatal("undo", m.Value(), s)
		}
	}
}
