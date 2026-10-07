package bed

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"reflect"
	"strings"
	"testing"
)

func multiEditor(text string, positions ...int) Model {
	m := New()
	m.CharLimit = 0
	m.MaxHeight = 0
	m.SetWidth(40)
	m.SetHeight(10)
	m.SetValue(text)
	m.Focus()
	var ss []Selection
	for _, p := range positions {
		ss = append(ss, Selection{Anchor: p, Head: p, Column: -1})
	}
	if err := m.SetSelections(ss, 0); err != nil {
		panic(err)
	}
	return m
}
func TestMultiTypingUndoAndSelection(t *testing.T) {
	m := multiEditor("one\ntwo", 0, 4)
	before := m.Selections()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.Value() != "xyone\nxytwo" {
		t.Fatal(m.Value())
	}
	if !m.Undo() || m.Value() != "one\ntwo" || !reflect.DeepEqual(before, m.Selections()) {
		t.Fatalf("undo %q %#v", m.Value(), m.Selections())
	}
	if !m.Redo() || m.Value() != "xyone\nxytwo" {
		t.Fatal("redo")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if m.Value() != "xy!ne\nxy!wo" {
		t.Fatal(m.Value())
	}
}
func TestMultiNormalizationAndRejection(t *testing.T) {
	m := multiEditor("abcdef", 0, 3)
	err := m.SetSelections([]Selection{{Anchor: 1, Head: 4}, {Anchor: 5, Head: 2}, {Anchor: 6, Head: 6}}, 1)
	if err != nil || len(m.Selections()) != 2 || m.PrimarySelection().Anchor != 5 || m.PrimarySelection().Head != 1 {
		t.Fatalf("%v %#v", err, m.Selections())
	}
	before := m.snapshot()
	m.CharLimit = 6
	if err = m.InsertText(strings.Repeat("x", 10)); err == nil {
		t.Fatal("expected rejection")
	}
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("rejection mutated editor")
	}
	if m.CanUndo() {
		t.Fatal("rejection recorded")
	}
	if err = m.ApplyEdits([]Replacement{{1, 3, "a"}, {2, 4, "b"}}); err == nil {
		t.Fatal("conflict accepted")
	}
}
func TestMultiChipAndExplicitRange(t *testing.T) {
	m := multiEditor("A[tag]Z\nlast", 0)
	if err := m.SetChips([]Chip{{ID: "x", Label: "[tag]", Text: "long", From: 1, To: 6}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetSelections([]Selection{{Anchor: 1, Head: 1}, {Anchor: 6, Head: 6}, {Anchor: 8, Head: 8}}, 2); err != nil {
		t.Fatal(err)
	}
	m.ClearHistory()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "Zlast" || len(m.Chips()) != 0 {
		t.Fatalf("%q %#v", m.Value(), m.Chips())
	}
	m.Undo()
	if len(m.Chips()) != 1 || len(m.Selections()) != 3 {
		t.Fatal("chip undo")
	}
	if err := m.ReplaceRange(8, 12, "next"); err != nil {
		t.Fatal(err)
	}
	if len(m.Selections()) != 3 {
		t.Fatal("explicit edit lost cursors")
	}
}
func TestMultiVerticalMouseAndEscape(t *testing.T) {
	m := multiEditor("abcd\nx\nabcd", 3)
	if err := m.AddCursorVertical(true); err != nil {
		t.Fatal(err)
	}
	if m.PrimarySelection().Head != 6 {
		t.Fatal(m.Selections())
	}
	m.AddCursorVertical(true)
	if m.PrimarySelection().Head != 10 {
		t.Fatal(m.Selections())
	}
	m.AddCursorVertical(true)
	if len(m.Selections()) != 3 {
		t.Fatal(m.Selections())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if len(m.Selections()) != 1 || Position(m) != 10 {
		t.Fatal(m.Selections())
	}
	m.HandleMouse(tea.MouseMsg{X: 0, Y: 0, Alt: true, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(m.Selections()) != 2 {
		t.Fatal(m.Selections())
	}
	m.HandleMouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(m.Selections()) != 1 {
		t.Fatal(m.Selections())
	}
}
func TestMultiNewlineAndUnicode(t *testing.T) {
	m := multiEditor("  a\n b", 3, 6)
	if err := m.InsertNewline(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "  a\n  \n b\n " {
		t.Fatal(m.Value())
	}
	m = multiEditor("e\u0301X\n👩‍💻Y", 2, 7)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "X\nY" {
		t.Fatal(m.Value())
	}
}
func TestMultiLimitAsyncAndCopies(t *testing.T) {
	m := multiEditor("abc", 0)
	m.CursorLimit = 2
	m.AddCursor(1)
	if m.AddCursor(2) == nil || len(m.Selections()) != 2 {
		t.Fatal("limit")
	}
	ss := m.Selections()
	ss[0].Head = 100
	if m.Selections()[0].Head == 100 {
		t.Fatal("alias")
	}
	if m.SetGhost("x") {
		t.Fatal("single ghost with multiple cursors")
	}
	m.SetPosition(3)
	m.SetGhost("x")
	r := m.ghost.request
	m.AddCursor(0)
	m.ClearSecondaryCursors()
	m.SetPosition(3)
	if m.current(r) {
		t.Fatal("stale request revived")
	}
}
func FuzzMultiEditRoundTrip(f *testing.F) {
	f.Add("abc", uint8(1), uint8(3))
	f.Add("한글\n👩‍💻", uint8(2), uint8(5))
	f.Fuzz(func(t *testing.T, text string, a, b uint8) {
		m := New()
		m.CharLimit = 0
		m.MaxHeight = 0
		m.SetValue(text)
		n := len([]rune(m.Value()))
		if n > 1000 {
			return
		}
		p, q := int(a)%(n+1), int(b)%(n+1)
		if err := m.SetSelections([]Selection{{Anchor: p, Head: p}, {Anchor: q, Head: q}}, 0); err != nil {
			t.Fatal(err)
		}
		before := m.snapshot()
		if err := m.InsertText("X"); err != nil {
			return
		}
		after := m.snapshot()
		if !m.Undo() || !reflect.DeepEqual(before, m.snapshot()) {
			t.Fatal("undo")
		}
		if !m.Redo() || !reflect.DeepEqual(after, m.snapshot()) {
			t.Fatal("redo")
		}
	})
}

func TestMultiLegacyAtomicTokens(t *testing.T) {
	m := multiEditor("[paste]\n[paste]", 0)
	m.AtomicTokens = []string{"[paste]"}
	if err := m.SetSelections([]Selection{{Anchor: 7, Head: 7}, {Anchor: 15, Head: 15}}, 0); err != nil {
		t.Fatal(err)
	}
	before := m.snapshot()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.Value() != "\n" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo lost legacy chips")
	}
	m.AddCursor(3)
	if p := m.PrimarySelection().Head; p > 0 && p < 7 {
		t.Fatal("cursor inside token", p)
	}
}
func BenchmarkMultiInsert(b *testing.B) {
	for _, count := range []int{1, 32, 256} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			m := New()
			m.CharLimit = 0
			m.MaxHeight = 0
			m.SetValue(strings.Repeat("abcd\n", count))
			ss := make([]Selection, count)
			for i := range ss {
				ss[i] = Selection{Anchor: 5 * i, Head: 5 * i, Column: -1}
			}
			m.SetSelections(ss, 0)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := m.InsertText("X"); err != nil {
					b.Fatal(err)
				}
				m.Undo()
				m.ClearHistory()
			}
		})
	}
}

func TestMultiActualTerminalKeyNames(t *testing.T) {
	m := multiEditor("a\nb\nc", 0)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlDown, Alt: true})
	if len(m.Selections()) != 2 || m.PrimarySelection().Head != 2 {
		t.Fatal(m.Selections())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlDown, Alt: true})
	if len(m.Selections()) != 3 {
		t.Fatal(m.Selections())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlUp, Alt: true})
	if len(m.Selections()) != 3 || m.PrimarySelection().Head != 2 {
		t.Fatal(m.Selections())
	}
}

func TestLegacyIndentInsideTokenRejects(t *testing.T) {
	m := multiEditor("[paste]", 0)
	m.AtomicTokens = []string{"[paste]"}
	m.SetPosition(3)
	if m.Indent() == nil || m.Value() != "[paste]" {
		t.Fatal("indent replaced label", m.Value())
	}
}

func TestMultiSetSelectionsAfterDocumentSwitch(t *testing.T) {
	m := multiEditor("a\nb", 0)
	m.DocumentKey = "next"
	if err := m.SetSelections([]Selection{{Anchor: 0, Head: 0}, {Anchor: 2, Head: 2}}, 0); err != nil {
		t.Fatal(err)
	}
	_ = m.View()
	if len(m.Selections()) != 2 {
		t.Fatal("render cleared new selections")
	}
}
