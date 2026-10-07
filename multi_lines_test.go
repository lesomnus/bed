package bed

import (
	"reflect"
	"testing"
)

func TestMultiLineIndentUniqueAndUndo(t *testing.T) {
	m := multiEditor("ab\ncd\nef", 0, 1, 3)
	before := m.snapshot()
	if err := m.Indent(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "    ab\n    cd\nef" {
		t.Fatal(m.Value())
	}
	if err := m.Outdent(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != before.text {
		t.Fatal(m.Value())
	}
	m.Undo()
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo")
	}
}
func TestMultiMoveDisjointAndEdge(t *testing.T) {
	m := multiEditor("a\nb\nc\nd\ne", 0, 4)
	before := m.snapshot()
	if err := m.MoveLines(true); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "b\na\nd\nc\ne" {
		t.Fatal(m.Value())
	}
	if m.Selections()[0].Head != 2 || m.Selections()[1].Head != 6 {
		t.Fatal(m.Selections())
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo")
	}
	m.MoveLines(false)
	if m.Value() != before.text || m.CanUndo() {
		t.Fatal("edge must be atomic no-op")
	}
}
func TestMultiDuplicateBlocksAndChipIDs(t *testing.T) {
	m := multiEditor("[x]\nb\nc\nd", 0)
	m.SetChips([]Chip{{ID: "x", Label: "[x]", Text: "payload", From: 0, To: 3}})
	m.SetSelections([]Selection{{Anchor: 0, Head: 0}, {Anchor: 6, Head: 6}}, 0)
	before := m.snapshot()
	if err := m.Duplicate(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "[x]\n[x]\nb\nc\nc\nd" {
		t.Fatal(m.Value())
	}
	cs := m.Chips()
	if len(cs) != 2 || cs[0].ID == cs[1].ID || cs[1].Text != "payload" {
		t.Fatal(cs)
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo")
	}
}
func TestMultiClipboard(t *testing.T) {
	m := multiEditor("one\ntwo\nthree", 0, 1, 8)
	if m.CopyText() != "one\nthree" {
		t.Fatal(m.CopyText())
	}
	before := m.snapshot()
	if err := m.CutSelections(); err != nil {
		t.Fatal(err)
	}
	if m.Value() != "two" {
		t.Fatal(m.Value())
	}
	m.Undo()
	if !reflect.DeepEqual(before, m.snapshot()) {
		t.Fatal("undo")
	}
	m = multiEditor("a\nb", 0, 2)
	m.PasteFragments([]string{"X", "Y"}, "unused")
	if m.Value() != "Xa\nYb" {
		t.Fatal(m.Value())
	}
	m.PasteFragments([]string{"ignored"}, "!")
	if m.Value() != "X!a\nY!b" {
		t.Fatal(m.Value())
	}
	m.SetSelections([]Selection{{Anchor: 0, Head: 1}, {Anchor: 4, Head: 4}}, 0)
	if m.CopyText() != "X" {
		t.Fatal("mixed selection")
	}
}
func TestMultiLineLimitRollback(t *testing.T) {
	m := multiEditor("a\nb", 0, 2)
	m.CharLimit = 3
	before := m.snapshot()
	if m.Duplicate() == nil {
		t.Fatal("expected rejection")
	}
	if !reflect.DeepEqual(before, m.snapshot()) || m.CanUndo() {
		t.Fatal("partial mutation")
	}
}
