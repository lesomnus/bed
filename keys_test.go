package bed

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

func TestEditorBindingsCanBeRemapped(t *testing.T) {
	m := testEditor()
	m.SetValue("one two")
	m.SetPosition(7)
	m.KeyMap.WordBackward.SetKeys("f2")
	m.EditorKeys.SelectWordLeft.SetKeys("f3")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftLeft})
	if Position(m) != 7 || m.SelectedText() != "" {
		t.Fatal("old selection binding still active")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF3})
	if m.SelectedText() != "two" {
		t.Fatal("remapped selection depends on movement key", m.SelectedText())
	}
	m.ClearSelection()
	m.SetPosition(7)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF2})
	if Position(m) != 4 {
		t.Fatal("base movement remap ignored")
	}
	m.EditorKeys.ToggleWhitespace.SetKeys("f4")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF4})
	if !m.ShowWhitespace {
		t.Fatal("toggle remap ignored")
	}
	m.EditorKeys.ToggleWhitespace.SetEnabled(false)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF4})
	if !m.ShowWhitespace {
		t.Fatal("disabled toggle executed")
	}
}
func TestEmptyKeyMapsDisableShortcuts(t *testing.T) {
	m := testEditor()
	m.SetValue("one two")
	m.SetPosition(7)
	m.KeyMap = textarea.KeyMap{}
	m.EditorKeys = EditorKeyMap{}
	for _, k := range []tea.KeyMsg{{Type: tea.KeyCtrlLeft}, {Type: tea.KeyShiftLeft}, {Type: tea.KeyHome}, {Type: tea.KeyRunes, Alt: true, Runes: []rune("w")}} {
		if handled, _ := m.HandleKey(k); handled {
			t.Fatal("disabled shortcut handled", k)
		}
	}
	if m.ShowWhitespace || m.SelectedText() != "" || Position(m) != 7 {
		t.Fatal("disabled bindings changed state")
	}
}
func TestRemappedCutAndPasteLiteral(t *testing.T) {
	m := testEditor()
	m.SetValue("abc")
	m.SetPosition(3)
	m.ExtendSelection(func() { m.SetPosition(1) })
	m.EditorKeys.Cut = key.NewBinding(key.WithKeys("f5"))
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if m.Value() != "a" || cmd == nil || cmd() != CopyMsg("bc") {
		t.Fatal("cut remap failed")
	}
	m.EditorKeys.ToggleWhitespace.SetKeys("x")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Paste: true})
	if m.ShowWhitespace || m.Value() != "ax" {
		t.Fatal("paste executed a shortcut")
	}
}
