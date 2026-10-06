package bed

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"unicode/utf8"
)

func testEditor() Model {
	m := New()
	m.CharLimit = 0
	m.MaxWidth = 0
	m.MaxHeight = 0
	m.SetWidth(24)
	m.SetHeight(3)
	m.Focus()
	return m
}
func TestSelectionReplacementAndCut(t *testing.T) {
	m := testEditor()
	m.SetValue("hello 한글")
	m.SetPosition(8)
	for range 2 {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
	}
	if m.SelectedText() != "한글" {
		t.Fatal(m.SelectedText())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("world"), Paste: true})
	if m.Value() != "hello world" {
		t.Fatal(m.Value())
	}
	m.ExtendSelection(func() { m.SetPosition(6) })
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	if m.Value() != "hello " || cmd == nil || cmd() != CopyMsg("world") {
		t.Fatal("cut must return clipboard request")
	}
}
func TestSelectionInvalidation(t *testing.T) {
	m := testEditor()
	m.SetValue("abc")
	m.DocumentKey = "first"
	m.SetPosition(3)
	m.ExtendSelection(func() { m.SetPosition(1) })
	m.DocumentKey = "second"
	if m.SelectedText() != "" {
		t.Fatal("selection crossed document")
	}
	m.ExtendSelection(func() { m.SetPosition(0) })
	m.SetValue("new")
	if m.SelectedText() != "" {
		t.Fatal("stale selection")
	}
}
func TestAtomicSelection(t *testing.T) {
	m := testEditor()
	m.SetValue("a [Paste 1] z")
	m.AtomicTokens = []string{"", "[Paste 1]"}
	m.SetPosition(5)
	m.ExtendSelection(func() { m.SetPosition(7) })
	if m.SelectedText() != "[Paste 1]" {
		t.Fatal(m.SelectedText())
	}
	m.DeleteSelection()
	if m.Value() != "a  z" {
		t.Fatal(m.Value())
	}
}
func TestUnicodeCursorHitTesting(t *testing.T) {
	m := testEditor()
	m.SetValue("한글 hello world wrapping words\nsecond\nthird\nfourth\nfifth")
	for pos := 0; pos <= utf8.RuneCountInString(m.Value()); pos++ {
		m.SetPosition(pos)
		probe := m.Model
		probe.Focus()
		probe.Cursor.Blink = false
		probe.Cursor.Style = cursorProbeStyle
		x, y, ok := widgetCursor(probe.View())
		if !ok {
			t.Fatalf("cursor missing at %d: %q", pos, probe.View())
		}
		if got := m.Point(x-m.GutterWidth, y); got != Position(m) {
			t.Fatalf("pos %d got %d at %d,%d", Position(m), got, x, y)
		}
	}
}
func TestMouseDragAndReplace(t *testing.T) {
	m := testEditor()
	m.SetValue("one two three")
	for i, x := range []int{6, 9, 9} {
		action := tea.MouseActionPress
		if i == 1 {
			action = tea.MouseActionMotion
		}
		if i == 2 {
			action = tea.MouseActionRelease
		}
		m, _ = m.Update(tea.MouseMsg{X: x, Y: 0, Button: tea.MouseButtonLeft, Action: action})
	}
	if m.SelectedText() != "two" {
		t.Fatal(m.SelectedText())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("new")})
	if m.Value() != "one new three" {
		t.Fatal(m.Value())
	}
}
func TestWhitespaceLogicalGutterAndResize(t *testing.T) {
	m := testEditor()
	m.SetValue("hello long line that wraps here\nnext line\nlast")
	m.ShowWhitespace = true
	for _, width := range []int{12, 24, 40} {
		m.SetWidth(width)
		for pos := 0; pos <= utf8.RuneCountInString(m.Value()); pos++ {
			m.SetPosition(pos)
			value := m.Value()
			rows := m.Rows()
			offset := m.ScrollOffset(rows)
			view := m.View()
			for y, line := range strings.Split(ansi.Strip(view), "\n") {
				if y+offset < len(rows) && rows[y+offset].Column > 0 && ansi.Cut(line, 0, 2) != "  " {
					t.Fatalf("wrap numbered %q", line)
				}
			}
			if m.Value() != value || Position(m) != pos {
				t.Fatal("render mutated document")
			}
		}
	}
}
func TestGraphemeSelection(t *testing.T) {
	for _, value := range []string{"e\u0301", "👩‍💻", "🇰🇷"} {
		m := testEditor()
		m.SetValue(value)
		m.SetPosition(utf8.RuneCountInString(value))
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftLeft})
		if m.SelectedText() != value {
			t.Fatal("split grapheme", m.SelectedText())
		}
	}
}
func TestScrollbarAndWheel(t *testing.T) {
	m := testEditor()
	m.SetValue("one\ntwo\nthree\nfour\nfive\nsix")
	m.SetPosition(0)
	if !m.Scroll(true) || Position(m) != 0 || m.ScrollOffset(m.Rows()) == 0 {
		t.Fatal("wheel did not move")
	}
	for _, line := range strings.Split(m.RenderScrollbar(m.View(), 25), "\n") {
		if ansi.StringWidth(line) != 25 {
			t.Fatal("scrollbar width", ansi.StringWidth(line))
		}
	}
}

func TestDefaultControlWordNavigation(t *testing.T) {
	m := testEditor()
	m.SetValue("one two three")
	m.SetPosition(13)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if Position(m) != 8 {
		t.Fatalf("ctrl+left: got %d want 8", Position(m))
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	if Position(m) != 13 {
		t.Fatalf("ctrl+right: got %d want 13", Position(m))
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftLeft})
	if m.SelectedText() != "three" {
		t.Fatalf("ctrl+shift+left: %q", m.SelectedText())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftRight})
	if m.SelectedText() != "" || Position(m) != 13 {
		t.Fatal("reverse word selection did not collapse")
	}
	m.SetPosition(8)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlShiftRight})
	if m.SelectedText() != "three" {
		t.Fatalf("ctrl+shift+right: %q", m.SelectedText())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("last")})
	if m.Value() != "one two last" {
		t.Fatal(m.Value())
	}
}

func TestDefaultAltWordNavigationPreserved(t *testing.T) {
	for _, left := range []tea.KeyMsg{{Type: tea.KeyLeft, Alt: true}, {Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}} {
		m := testEditor()
		m.SetValue("one two")
		m.SetPosition(7)
		m, _ = m.Update(left)
		if Position(m) != 4 {
			t.Fatalf("%s moved to %d", left.String(), Position(m))
		}
	}
	for _, right := range []tea.KeyMsg{{Type: tea.KeyRight, Alt: true}, {Type: tea.KeyRunes, Runes: []rune("f"), Alt: true}} {
		m := testEditor()
		m.SetValue("one two")
		m.SetPosition(4)
		m, _ = m.Update(right)
		if Position(m) != 7 {
			t.Fatalf("%s moved to %d", right.String(), Position(m))
		}
	}
}
