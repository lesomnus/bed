package bed

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// ProvidersChanged invalidates in-flight results after replacing provider fields.
// Call this even when the new provider happens to produce the same text.
func (m *Model) ProvidersChanged() { m.selectionRevision++; m.Close() }

func (m *Model) validateReplacements(edits []Replacement) error {
	es := slices.Clone(edits)
	n := utf8.RuneCountInString(m.Value())
	for i := range es {
		e := &es[i]
		if e.From < 0 || e.To < e.From || e.To > n {
			return fmt.Errorf("invalid edit range")
		}
		e.From, e.To = expandedRange(m.chips, e.From, e.To)
	}
	slices.SortFunc(es, func(a, b Replacement) int { return a.From - b.From })
	at := 0
	var out strings.Builder
	r := []rune(m.Value())
	for i, e := range es {
		if i > 0 && (e.From < at || (e.From == at && (e.From == e.To || es[i-1].From == es[i-1].To))) {
			return fmt.Errorf("overlapping provider edits")
		}
		out.WriteString(string(r[at:e.From]))
		out.WriteString(e.Text)
		at = e.To
	}
	out.WriteString(string(r[at:]))
	return m.validateValue(out.String())
}

// SetGhostEdits supplies explicit insertion previews. Ranges must be empty and
// outside chips. Rendering shows the first line of each insertion; acceptance
// inserts all text in one undo transaction. Providers use MultiGhostProvider.
func (m *Model) SetGhostEdits(edits []Replacement) bool {
	r := m.request()
	m.Close()
	return m.installGhostEdits(r, edits)
}
func (m *Model) installGhostEdits(r Request, edits []Replacement) bool {
	if !m.current(r) || len(edits) == 0 || m.validateReplacements(edits) != nil {
		return false
	}
	for _, e := range edits {
		if e.From != e.To || e.Text == "" || m.snapNearest(e.From) != e.From {
			return false
		}
	}
	m.ghost = &ghostState{request: r, edits: slices.Clone(edits)}
	return true
}
func (m *Model) renderGhostEdits(view string, g *ghostState) string {
	es := slices.Clone(g.edits)
	if g.loading {
		for _, s := range g.request.Selections {
			es = append(es, Replacement{s.Head, s.Head, "Loading" + strings.Repeat(".", 1+m.featureFrame%3)})
		}
	}
	slices.SortFunc(es, func(a, b Replacement) int { return b.From - a.From })
	rows := strings.Split(view, "\n")
	layout := m.Rows()
	offset := m.ScrollOffset(layout)
	r := []rune(m.Value())
	for _, e := range es {
		i := RowIndex(layout, e.From)
		y := i - offset
		if y < 0 || y >= len(rows) {
			continue
		}
		x := m.GutterWidth + ansi.StringWidth(string(r[layout[i].Start:e.From]))
		text, _, multi := strings.Cut(e.Text, "\n")
		if multi {
			text += "↵"
		}
		text = m.GhostStyle.Render(displayText(text))
		rows[y] = ansi.Truncate(ansi.Cut(rows[y], 0, x)+text+ansi.Cut(rows[y], x, ansi.StringWidth(rows[y])), m.Width(), "")
	}
	return strings.Join(rows, "\n")
}

// InsertChips inserts one host-supplied chip per selection in document order.
// All IDs must be distinct and unused. On error nothing changes. bed owns only
// descriptors; hosts retain external payloads until no live/history references
// remain, so undo can restore old IDs. No callback side effects occur on rollback.
func (m *Model) InsertChips(chips []Chip) error {
	ss := m.Selections()
	if len(chips) != len(ss) {
		return fmt.Errorf("one chip per selection is required")
	}
	chips = slices.Clone(chips)
	ids := map[string]bool{}
	for _, c := range m.chips {
		ids[c.ID] = true
	}
	var edits []Replacement
	for i, c := range chips {
		if c.ID == "" || ids[c.ID] || !cleanLabel(c.Label) {
			return fmt.Errorf("invalid or duplicate chip ID %q", c.ID)
		}
		ids[c.ID] = true
		a, b := ss[i].bounds()
		edits = append(edits, Replacement{a, b, c.Label})
	}
	if err := m.validateReplacements(edits); err != nil {
		return err
	}
	done := m.beginEdit("")
	defer done()
	if err := m.applyEdits(edits, true); err != nil {
		return err
	}
	delta := 0
	for i, c := range chips {
		a, b := edits[i].From, edits[i].To
		c.From = a + delta
		c.To = c.From + utf8.RuneCountInString(c.Label)
		delta += c.To - c.From - (b - a)
		m.chips = append(m.chips, c)
	}
	slices.SortFunc(m.chips, func(a, b Chip) int { return a.From - b.From })
	m.chipVersion++
	return nil
}

// ExpandChips expands a validated set of IDs as one atomic edit.
func (m *Model) ExpandChips(ids []string) error { return m.editChips(ids, true) }

// RemoveChips removes a validated set of IDs as one atomic edit.
func (m *Model) RemoveChips(ids []string) error { return m.editChips(ids, false) }
func (m *Model) editChips(ids []string, expand bool) error {
	var edits []Replacement
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		found := false
		for _, c := range m.Chips() {
			if c.ID == id {
				found = true
				text := ""
				if expand {
					text = c.Text
				}
				edits = append(edits, Replacement{c.From, c.To, text})
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown chip %q", id)
		}
	}
	return m.ApplyEdits(edits)
}
