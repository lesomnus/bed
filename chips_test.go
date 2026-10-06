package bed

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func addChip(t *testing.T, m *Model, id string) {
	t.Helper()
	if err := m.InsertChip(Chip{ID: id, Label: "[paste]", Text: "expanded\ntext"}); err != nil {
		t.Fatal(err)
	}
}
func TestChipAtomicEditingAndHistory(t *testing.T) {
	m := testEditor()
	m.SetValue("before ")
	addChip(t, &m, "one")
	if len(m.Chips()) != 1 {
		t.Fatal("chip lost")
	}
	m.SetPosition(9)
	if Position(m) != 7 {
		t.Fatal("cursor entered chip", Position(m))
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.SelectedText() != "[paste]" {
		t.Fatal("arrow did not select chip")
	}
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || cmd().(ChipActivateMsg).Chip.ID != "one" {
		t.Fatal("activation missing")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "before " || len(m.Chips()) != 0 {
		t.Fatal("partial chip deletion", m.Value())
	}
	m.Undo()
	if len(m.Chips()) != 1 || m.SelectedText() != "[paste]" {
		t.Fatal("undo lost chip metadata/selection")
	}
	if err := m.ExpandChip("one"); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "before expanded\ntext" || len(m.Chips()) != 0 {
		t.Fatal("expand failed", m.Value())
	}
	m.Undo()
	if len(m.Chips()) != 1 {
		t.Fatal("expansion not undoable")
	}
	m.Redo()
	if len(m.Chips()) != 0 {
		t.Fatal("redo retained chip")
	}
}
func TestChipBackspaceWithoutSelection(t *testing.T) {
	m := testEditor()
	addChip(t, &m, "one")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "" || len(m.Chips()) != 0 {
		t.Fatal("backspace split chip", m.Value())
	}
	m.Undo()
	if len(m.Chips()) != 1 {
		t.Fatal("undo lost chip")
	}
}
func TestChipDuplicateLabelsMapByPosition(t *testing.T) {
	m := testEditor()
	addChip(t, &m, "first")
	addChip(t, &m, "second")
	m.SetPosition(7)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	chips := m.Chips()
	if len(chips) != 1 || chips[0].ID != "second" || chips[0].From != 0 {
		t.Fatalf("wrong chip removed: %+v %q", chips, m.Value())
	}
	m.SetPosition(0)
	m.InsertString("prefix ")
	chips = m.Chips()
	if chips[0].From != 7 {
		t.Fatal("chip offsets not mapped")
	}
}
func TestChipMouseAndValidation(t *testing.T) {
	m := testEditor()
	addChip(t, &m, "one")
	m.SetPosition(0)
	m, _ = m.Update(tea.MouseMsg{X: m.GutterWidth + 2, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if c, ok := m.SelectedChip(); !ok || c.ID != "one" {
		t.Fatal("click did not select chip")
	}
	if m.InsertChip(Chip{ID: "one", Label: "duplicate"}) == nil {
		t.Fatal("duplicate ID accepted")
	}
	if m.InsertChip(Chip{ID: "bad", Label: "bad\nlabel"}) == nil {
		t.Fatal("invalid label accepted")
	}
	before := m.Value()
	if m.ReplaceRange(0, 1, "\t") == nil || m.Value() != before {
		t.Fatal("unsupported replacement modified buffer")
	}
	m.SetValue("new")
	if len(m.Chips()) != 0 {
		t.Fatal("chips crossed documents")
	}
}

func TestChipWrapRenderingAndWholeSelection(t *testing.T) {
	m := testEditor()
	m.SetWidth(12)
	addChip(t, &m, "one")
	m.SetPosition(0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	if m.SelectedText() != "[paste]" {
		t.Fatal("shift selected partial chip")
	}
	if err := m.ReplaceRange(1, 2, "x"); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "x" || len(m.Chips()) != 0 {
		t.Fatal("range replacement split chip")
	}
	m.Undo()
	if len(m.Chips()) != 1 {
		t.Fatal("replacement undo lost chip")
	}
	_ = m.View()
}
func TestChipMetadataOnlyExpandUndo(t *testing.T) {
	m := testEditor()
	if err := m.InsertChip(Chip{ID: "same", Label: "text", Text: "text"}); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpandChip("same"); err != nil {
		t.Fatal(err)
	}
	if len(m.Chips()) != 0 {
		t.Fatal("expand retained metadata")
	}
	m.Undo()
	if len(m.Chips()) != 1 || m.Value() != "text" {
		t.Fatal("metadata-only edit not undoable")
	}
	m.Redo()
	if len(m.Chips()) != 0 {
		t.Fatal("metadata redo failed")
	}
}

func TestSelectionReplacementMapsFollowingChip(t *testing.T) {
	for _, api := range []bool{false, true} {
		m := testEditor()
		m.SetValue("hello ")
		addChip(t, &m, "tail")
		m.SetPosition(5)
		m.ExtendSelection(func() { m.SetPosition(0) })
		if api {
			m.InsertString("x")
		} else {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
		}
		chips := m.Chips()
		if len(chips) != 1 || chips[0].From != 2 || m.Value() != "x [paste]" {
			t.Fatalf("replacement corrupted following chip: %+v %q", chips, m.Value())
		}
		m.Undo()
		chips = m.Chips()
		if len(chips) != 1 || chips[0].From != 6 || m.Value() != "hello [paste]" {
			t.Fatal("undo mapping wrong")
		}
	}
}

func TestAttachPersistedChips(t *testing.T) {
	m := testEditor()
	m.SetValue("[x] [y]")
	chips := []Chip{{ID: "x", Label: "[x]", From: 0, To: 3}, {ID: "y", Label: "[y]", From: 4, To: 7}}
	if err := m.SetChips(chips); err != nil {
		t.Fatal(err)
	}
	chips[0].Label = "mutated"
	if m.Chips()[0].Label != "[x]" {
		t.Fatal("metadata shares caller slice")
	}
	if err := m.SetChips([]Chip{{ID: "bad", Label: "x] [", From: 1, To: 5}, {ID: "other", Label: "[y]", From: 4, To: 7}}); err == nil {
		t.Fatal("overlap accepted")
	}
	if len(m.Chips()) != 2 {
		t.Fatal("invalid metadata replaced valid chips")
	}
	m.Undo()
	if len(m.Chips()) != 0 {
		t.Fatal("metadata attachment not undoable")
	}
}
