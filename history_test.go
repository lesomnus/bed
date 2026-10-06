package bed

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
	"time"
)

func typeText(m Model, text string) Model {
	for _, r := range text {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}
func TestUndoTypingGroupAndRedoBranch(t *testing.T) {
	m := testEditor()
	m = typeText(m, "hello 한글")
	if !m.Undo() || m.Value() != "" || Position(m) != 0 {
		t.Fatal("typing not grouped", m.Value())
	}
	if !m.Redo() || m.Value() != "hello 한글" || Position(m) != 8 {
		t.Fatal("redo did not restore cursor/text")
	}
	m.Undo()
	m = typeText(m, "new")
	if m.CanRedo() {
		t.Fatal("new edit retained redo branch")
	}
}
func TestUndoGroupPauseAndNavigation(t *testing.T) {
	m := testEditor()
	now := time.Unix(100, 0)
	m.historyClock = func() time.Time { return now }
	m = typeText(m, "one")
	now = now.Add(time.Second)
	m = typeText(m, "two")
	m.Undo()
	if m.Value() != "one" {
		t.Fatal("pause did not split group", m.Value())
	}
	m.Redo()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = typeText(m, "!")
	m.Undo()
	if m.Value() != "onetwo" {
		t.Fatal("navigation did not split group", m.Value())
	}
}
func TestUndoSelectionReplacementRestoresSelection(t *testing.T) {
	m := testEditor()
	m.SetValue("first\n한글 last")
	m.SetPosition(8)
	m.ExtendSelection(func() { m.SetPosition(6) })
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("changed"), Paste: true})
	if m.Value() != "first\nchanged last" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if m.Value() != "first\n한글 last" || m.SelectedText() != "한글" || Position(m) != 6 {
		t.Fatal("selection not restored", m.SelectedText(), Position(m))
	}
	m.Redo()
	if m.Value() != "first\nchanged last" || m.SelectedText() != "" {
		t.Fatal("redo failed")
	}
}
func TestUndoPasteNewlineCutAndProgrammaticInsert(t *testing.T) {
	m := testEditor()
	m = typeText(m, "start")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("\nPASTE\n"), Paste: true})
	m = typeText(m, "end")
	m.Undo()
	if m.Value() != "start\nPASTE\n" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if m.Value() != "start" {
		t.Fatal("paste not isolated", m.Value())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Undo()
	if m.Value() != "start" {
		t.Fatal("newline not isolated")
	}
	m.SetPosition(5)
	m.ExtendSelection(func() { m.SetPosition(0) })
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	m.Undo()
	if m.SelectedText() != "start" {
		t.Fatal("cut undo lost selection")
	}
	m.InsertString("replacement")
	m.Undo()
	if m.Value() != "start" || m.SelectedText() != "start" {
		t.Fatal("API insert not undoable")
	}
}
func TestUndoBackspaceGroup(t *testing.T) {
	m := testEditor()
	m.SetValue("abcdef")
	m.SetPosition(6)
	for range 3 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if m.Value() != "abc" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if m.Value() != "abcdef" {
		t.Fatal("backspace not grouped", m.Value())
	}
}
func TestUndoDocumentResetAndBounds(t *testing.T) {
	m := testEditor()
	m.HistoryLimit = 2
	for _, s := range []string{"a", "b", "c"} {
		m.InsertString(s)
	}
	m.Undo()
	m.Undo()
	if m.Undo() || m.Value() != "a" {
		t.Fatal("history count limit ignored")
	}
	m.SetValue("new")
	if m.CanUndo() || m.CanRedo() {
		t.Fatal("SetValue retained history")
	}
	m.InsertString("x")
	m.DocumentKey = "other"
	if m.CanUndo() {
		t.Fatal("history crossed documents")
	}
	m.InsertString("y")
	m.Model.SetValue("external")
	if m.CanUndo() {
		t.Fatal("raw replacement retained stale history")
	}
	m.HistoryBytes = 8
	m.InsertString(strings.Repeat("z", 10))
	if m.CanUndo() {
		t.Fatal("oversized history retained")
	}
	m.HistoryBytes = 100
	m.HistoryLimit = 0
	m.InsertString("a")
	if m.CanUndo() {
		t.Fatal("disabled history retained")
	}
}
func TestUndoBindingsAndDisplayChanges(t *testing.T) {
	m := testEditor()
	m = typeText(m, "x")
	m.EditorKeys.Undo.SetKeys("f6")
	m.EditorKeys.Redo.SetKeys("f7")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if m.Value() != "" {
		t.Fatal("undo remap")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF7})
	if m.Value() != "x" {
		t.Fatal("redo remap")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune("w")})
	m.SetWidth(40)
	m.Undo()
	if m.Value() != "" || !m.ShowWhitespace {
		t.Fatal("display entered edit history")
	}
}
func TestSplitDispatchSelectionReplacement(t *testing.T) {
	m := testEditor()
	m.SetValue("abc")
	m.SetPosition(3)
	m.ExtendSelection(func() { m.SetPosition(0) })
	k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")}
	handled, _ := m.HandleKey(k)
	if handled {
		t.Fatal("replacement prematurely handled")
	}
	m, _ = m.UpdateText(k)
	m.Undo()
	if m.Value() != "abc" || m.SelectedText() != "abc" {
		t.Fatal("split dispatch did not record replacement atomically")
	}
}
