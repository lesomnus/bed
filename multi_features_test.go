package bed

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"reflect"
	"testing"
)

func TestMultiCompletionExplicitEditsAndOwnership(t *testing.T) {
	m := multiEditor("a\nb", 1, 3)
	before := m.snapshot()
	result := CompletionResult{Items: []CompletionItem{{Label: "both"}}, Edits: [][]Replacement{{{1, 1, "X"}, {3, 3, "Y"}}}}
	if !m.SetCompletions(result) {
		t.Fatal("install")
	}
	result.Edits[0][0].Text = "bad"
	if !m.AcceptCompletion() || m.Value() != "aX\nbY" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo")
	}
	if m.SetCompletions(CompletionResult{Items: []CompletionItem{{InsertText: "x"}}}) {
		t.Fatal("implicit replay")
	}
	if m.SetCompletions(CompletionResult{Items: []CompletionItem{{}}, Edits: [][]Replacement{{{0, 2, "x"}, {1, 3, "y"}}}}) {
		t.Fatal("overlap")
	}
}
func TestMultiGhostAtomicPreview(t *testing.T) {
	m := multiEditor("a\nb", 0, 2)
	before := m.snapshot()
	if !m.SetGhostEdits([]Replacement{{0, 0, "X"}, {2, 2, "Y\nZ"}}) {
		t.Fatal("install")
	}
	_ = m.View()
	if m.Value() != before.text {
		t.Fatal("preview mutated")
	}
	if !m.AcceptGhost() || m.Value() != "Xa\nY\nZb" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo")
	}
	if m.SetGhostEdits([]Replacement{{0, 1, "replacement"}}) {
		t.Fatal("replacement preview")
	}
}
func TestMultiProviderRunsOnceAndStaleSet(t *testing.T) {
	m := multiEditor("a\nb", 0, 2)
	calls := 0
	var ctx context.Context
	m.MultiCompletionProvider = func(c context.Context, r Request) (CompletionResult, error) {
		calls++
		ctx = c
		if len(r.Selections) != 2 {
			t.Fatal("request")
		}
		return CompletionResult{Items: []CompletionItem{{}}, Edits: [][]Replacement{{{0, 0, "X"}, {2, 2, "Y"}}}}, nil
	}
	cmd := m.RequestCompletions(context.Background())
	batch := cmd().(tea.BatchMsg)
	msg := batch[0]()
	if calls != 1 {
		t.Fatal(calls)
	}
	m.ClearSecondaryCursors()
	m.AddCursor(2)
	m, _ = m.Update(msg)
	if m.completion != nil {
		t.Fatal("stale accepted")
	}
	if ctx.Err() == nil {
		t.Fatal("not cancelled")
	}
	m.RequestCompletions(context.Background())
	m.ProvidersChanged()
	if m.completion != nil {
		t.Fatal("provider replacement")
	}
}
func TestMultiChipInsertionExpansionAndRollback(t *testing.T) {
	m := multiEditor("a\nb", 0, 2)
	before := m.snapshot()
	if err := m.InsertChips([]Chip{{ID: "x", Label: "[x]", Text: "XX"}, {ID: "y", Label: "[y]", Text: "YY"}}); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "[x]a\n[y]b" || len(m.Chips()) != 2 {
		t.Fatal(m.Value(), m.Chips())
	}
	after := m.snapshot()
	if m.RemoveChips([]string{"x", "missing"}) == nil || !reflect.DeepEqual(after, m.snapshot()) {
		t.Fatal("partial deletion")
	}
	m.ExpandChips([]string{"x", "y"})
	if m.Value() != "XXa\nYYb" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if !reflect.DeepEqual(after, m.snapshot()) {
		t.Fatal("expand undo")
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("insert undo")
	}
	if m.InsertChips([]Chip{{ID: "same", Label: "a"}, {ID: "same", Label: "b"}}) == nil || !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("invalid IDs mutated")
	}
}

func TestMultiCompletionTriggerAndStablePrimary(t *testing.T) {
	m := multiEditor("a\nb", 1, 3)
	m.CompletionTriggers = "@"
	m.MultiCompletionProvider = func(context.Context, Request) (CompletionResult, error) { return CompletionResult{}, nil }
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if m.completion == nil || !m.completion.loading {
		t.Fatal("multi trigger did not request")
	}
	id := m.PrimarySelection().ID
	m.ClearSecondaryCursors()
	if m.PrimarySelection().ID != id {
		t.Fatal("primary identity changed")
	}
}
