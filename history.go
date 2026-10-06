package bed

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"slices"
	"strings"
	"time"
)

type editSnapshot struct {
	chips                []Chip
	text                 string
	cursor, anchor, head int
	selected             bool
}
type editEntry struct{ before, after editSnapshot }
type editHistory struct {
	document, current string
	undo, redo        []editEntry
	kind              string
	at                time.Time
}

func (m *Model) snapshot() editSnapshot {
	s := editSnapshot{text: m.Value(), cursor: Position(*m), chips: slices.Clone(m.chips)}
	if m.SelectionValid() {
		s.selected = true
		s.anchor = m.selection.anchor
		s.head = m.selection.head
	}
	return s
}
func (m *Model) ensureHistory() {
	if m.history == nil || m.history.document != m.DocumentKey || m.history.current != m.Value() {
		m.history = &editHistory{document: m.DocumentKey, current: m.Value()}
	}
}

// ClearHistory discards undo and redo without changing the document.
func (m *Model) ClearHistory() { m.history = &editHistory{document: m.DocumentKey, current: m.Value()} }

// BreakUndoGroup keeps the next edit separate (for example after saving).
func (m *Model) BreakUndoGroup() {
	if m.history != nil {
		m.history.kind = ""
	}
}
func (m *Model) CanUndo() bool { m.ensureHistory(); return len(m.history.undo) > 0 }
func (m *Model) CanRedo() bool { m.ensureHistory(); return len(m.history.redo) > 0 }
func (m *Model) restoreEdit(s editSnapshot) {
	m.selection = nil
	m.layout = nil
	m.Close()
	m.Model.SetValue(s.text)
	m.chips = slices.Clone(s.chips)
	m.chipVersion++
	m.featureText = s.text
	m.featureDocument = m.DocumentKey
	m.SetPosition(s.cursor)
	if s.selected {
		m.selection = &selection{value: s.text, document: m.DocumentKey, anchor: s.anchor, head: s.head}
	}
	m.history.current = m.Value()
}

// Undo restores one edit group, including its cursor and selection.
func (m *Model) Undo() bool {
	m.ensureHistory()
	m.BreakUndoGroup()
	h := m.history
	if len(h.undo) == 0 {
		return false
	}
	e := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, e)
	m.restoreEdit(e.before)
	return true
}

// Redo reapplies an undone group. A new edit discards the redo branch.
func (m *Model) Redo() bool {
	m.ensureHistory()
	m.BreakUndoGroup()
	h := m.history
	if len(h.redo) == 0 {
		return false
	}
	e := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, e)
	m.restoreEdit(e.after)
	return true
}
func (m *Model) beginEdit(kind string) func() {
	if m.editDepth > 0 {
		m.editDepth++
		return func() { m.editDepth-- }
	}
	m.syncFeatures()
	m.ensureHistory()
	before := m.snapshot()
	version := m.chipVersion
	m.editDepth++
	return func() {
		m.editDepth--
		m.reconcileChips(before, version)
		m.featureText = m.Value()
		m.chipCursor = Position(*m)
		m.featureDocument = m.DocumentKey
		after := m.snapshot()
		if before.text != after.text {
			m.Close()
		} else {
			if m.completion != nil && !m.current(m.completion.request) {
				m.DismissCompletions()
			}
			if m.ghost != nil && !m.current(m.ghost.request) {
				m.DismissGhost()
			}
		}
		if before.text == after.text && slices.Equal(before.chips, after.chips) {
			return
		}
		h := m.history
		h.current = after.text
		h.redo = nil
		now := time.Now()
		if m.historyClock != nil {
			now = m.historyClock()
		}
		merge := kind != "" && kind == h.kind && now.Sub(h.at) >= 0 && now.Sub(h.at) <= m.UndoGroupDelay && len(h.undo) > 0 && !before.selected
		if merge {
			last := h.undo[len(h.undo)-1].after
			merge = last.cursor == before.cursor && last.text == before.text && !last.selected
		}
		if merge {
			h.undo[len(h.undo)-1].after = after
		} else {
			h.undo = append(h.undo, editEntry{before, after})
		}
		h.kind = kind
		h.at = now
		// Snapshot storage is bounded even for large file edits. An individual edit
		// exceeding the budget clears history rather than retaining partial records.
		total := 0
		for i := len(h.undo) - 1; i >= 0; i-- {
			e := h.undo[i]
			total += len(e.before.text) + len(e.after.text)
			for _, c := range append(slices.Clone(e.before.chips), e.after.chips...) {
				total += len(c.ID) + len(c.Label) + len(c.Text)
			}
			if len(h.undo)-i > m.HistoryLimit || total > m.HistoryBytes {
				h.undo = append([]editEntry(nil), h.undo[i+1:]...)
				break
			}
		}
	}
}
func (m *Model) editKind(msg tea.Msg) string {
	k, ok := msg.(tea.KeyMsg)
	if !ok || k.Paste || m.SelectedText() != "" {
		return ""
	}
	if key.Matches(k, m.KeyMap.DeleteCharacterBackward) {
		return "backspace"
	}
	if key.Matches(k, m.KeyMap.DeleteCharacterForward) {
		return "delete"
	}
	if (k.Type == tea.KeyRunes || k.Type == tea.KeySpace) && !m.baseBindingMatches(k) && !strings.ContainsAny(string(k.Runes), "\n\r") {
		return "insert"
	}
	return ""
}

// InsertString applies a programmatic insertion as one undoable edit. SetValue
// instead replaces the document and clears history.
func (m *Model) InsertString(text string) {
	done := m.beginEdit("")
	defer done()
	m.DeleteSelection()
	m.Model.InsertString(text)
}
func (m *Model) InsertRune(r rune) { m.InsertString(string(r)) }
