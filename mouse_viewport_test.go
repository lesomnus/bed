package bed

import (
	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"time"
)

func TestMultiClickAndWordDrag(t *testing.T) {
	m := testEditor()
	m.SetValue("one 한글!\nnext")
	m.SetPosition(0)
	now := time.Now()
	m.mouseClock = func() time.Time { return now }
	click := func(x int) {
		m.HandleMouse(tea.MouseMsg{X: x, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.HandleMouse(tea.MouseMsg{X: x, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
		now = now.Add(50 * time.Millisecond)
	}
	click(4)
	if m.SelectedText() != "" || Position(m) != 4 {
		t.Fatal(m.SelectedText(), Position(m))
	}
	click(4)
	if m.SelectedText() != "한글" {
		t.Fatal(m.SelectedText())
	}
	click(4)
	if m.SelectedText() != "one 한글!\n" {
		t.Fatal(m.SelectedText())
	}
	now = now.Add(time.Second)
	click(1)
	click(1)
	// The second press begins a word-granular drag.
	m.HandleMouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}) // triple
	m.HandleMouse(tea.MouseMsg{X: 1, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	if m.SelectedText() != "one 한글!\nnext" {
		t.Fatal(m.SelectedText())
	}
	m.HandleMouse(tea.MouseMsg{X: 1, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m.MultiClickInterval = 0
	click(1)
	click(1)
	if m.SelectedText() != "" {
		t.Fatal("multiclick disabled")
	}
}
func TestWordDragAndChipAtomicity(t *testing.T) {
	m := testEditor()
	m.SetValue("one two [chip]")
	m.SetChips([]Chip{{ID: "c", Label: "[chip]", From: 8, To: 14}})
	now := time.Now()
	m.mouseClock = func() time.Time { return now }
	for range 2 {
		m.HandleMouse(tea.MouseMsg{X: 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		now = now.Add(time.Millisecond)
	}
	m.HandleMouse(tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	if m.SelectedText() != "one two" {
		t.Fatal(m.SelectedText())
	}
	m.HandleMouse(tea.MouseMsg{X: 10, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	if m.SelectedText() != "one two [chip]" {
		t.Fatal(m.SelectedText())
	}
}
func TestViewportScrollPreservesCursorSelectionAndTimer(t *testing.T) {
	m := testEditor()
	m.SetValue("zero\none\ntwo\nthree\nfour\nfive\nsix\nseven")
	selectRange(&m, 0, 2)
	m.Cursor.BlinkSpeed = time.Millisecond
	cmd := m.Focus()
	if !m.Scroll(true) {
		t.Fatal("not scrolled")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "three") || strings.Contains(view, "zero") {
		t.Fatal(view)
	}
	if Position(m) != 2 || m.SelectedText() != "ze" {
		t.Fatal("wheel moved selection")
	}
	if _, ok := cmd().(cursor.BlinkMsg); !ok {
		t.Fatal("render canceled timer")
	}
	if m.Point(0, 0) != 13 {
		t.Fatal("hit test", m.Point(0, 0))
	}
	m.HandleMouse(tea.MouseMsg{X: 2, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if Position(m) != 15 || !strings.Contains(ansi.Strip(m.View()), "three") {
		t.Fatal("click jumped viewport", Position(m), m.View())
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if m.detached || !strings.Contains(m.Value(), "thXree") {
		t.Fatal(m.Value())
	}
}
func TestViewportWrappedRowsResizeAndIndependentEditors(t *testing.T) {
	m := testEditor()
	m.SetWidth(10)
	m.SetHeight(2)
	m.SetValue("abcdefghij klmnopqrst uvwxyz\nlast")
	m.SetPosition(0)
	m.Scroll(true)
	top := m.ScrollOffset(m.Rows())
	row := m.Rows()[top]
	if m.Point(0, 0) != row.Start {
		t.Fatal("wrapped mapping")
	}
	if Position(m) != 0 {
		t.Fatal("cursor moved")
	}
	other := testEditor()
	other.SetValue("independent")
	before := other.View()
	m.View()
	if other.View() != before {
		t.Fatal("shared viewport")
	}
	m.SetHeight(30)
	if m.ScrollOffset(m.Rows()) != 0 {
		t.Fatal("resize clamp")
	}
	m.SetValue("new")
	if m.detached {
		t.Fatal("document reset")
	}
}

func TestLayoutQueriesDoNotMoveLiveViewport(t *testing.T) {
	m := testEditor()
	m.SetValue("zero\none\ntwo\nthree\nfour\nfive\nsix")
	m.SetPosition(len([]rune(m.Value())))
	before := m.Model.View()
	m.Rows()
	m.Point(0, 0)
	if after := m.Model.View(); before != after {
		t.Fatal("layout changed live viewport", ansi.Strip(before), ansi.Strip(after))
	}
	m.Scroll(false)
	m.View()
	m.FollowCursor()
	if view := ansi.Strip(m.View()); !strings.Contains(view, "six") {
		t.Fatal("FollowCursor did not reveal cursor", view)
	}
}
