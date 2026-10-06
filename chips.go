package bed

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Chip is an atomic, styled label in the document. Offsets are rune positions.
// Text is the expansion text. ID is opaque to bed; external payloads stay in the host.
type Chip struct {
	ID, Label, Text string
	From, To        int
}

// ChipActivateMsg asks the host to open a preview or perform another action.
type ChipActivateMsg struct{ Chip Chip }

func (m *Model) Chips() []Chip { m.syncFeatures(); return slices.Clone(m.chips) }
func cleanLabel(text string) bool {
	return text != "" && !strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) || r == utf8.RuneError })
}
func (m *Model) InsertChip(chip Chip) error {
	m.syncFeatures()
	if chip.ID == "" || !cleanLabel(chip.Label) {
		return fmt.Errorf("chip requires an ID and a single-line label without control characters")
	}
	for _, c := range m.chips {
		if c.ID == chip.ID {
			return fmt.Errorf("duplicate chip ID %q", chip.ID)
		}
	}
	done := m.beginEdit("")
	defer done()
	a, b := m.SelectionBounds()
	if a == b {
		a = Position(*m)
		b = a
	}
	if err := m.replaceRange(a, b, chip.Label); err != nil {
		return err
	}
	chip.From = a
	chip.To = a + utf8.RuneCountInString(chip.Label)
	m.chips = append(m.chips, chip)
	slices.SortFunc(m.chips, func(a, b Chip) int { return a.From - b.From })
	m.chipVersion++
	return nil
}
func (m *Model) RemoveChip(id string) error {
	m.syncFeatures()
	for _, c := range m.chips {
		if c.ID == id {
			return m.ReplaceRange(c.From, c.To, "")
		}
	}
	return fmt.Errorf("unknown chip %q", id)
}
func (m *Model) ExpandChip(id string) error {
	m.syncFeatures()
	for _, c := range m.chips {
		if c.ID == id {
			return m.ReplaceRange(c.From, c.To, c.Text)
		}
	}
	return fmt.Errorf("unknown chip %q", id)
}

// ReplaceRange replaces a rune range as one undoable edit. Intersecting chips
// expand the replacement range to whole labels. Invalid/unsupported text is rejected.
func (m *Model) ReplaceRange(from, to int, text string) error {
	done := m.beginEdit("")
	defer done()
	return m.replaceRange(from, to, text)
}
func expandedRange(chips []Chip, a, b int) (int, int) {
	for _, c := range chips {
		if (a < c.To && b > c.From) || (a == b && a > c.From && a < c.To) {
			a = min(a, c.From)
			b = max(b, c.To)
		}
	}
	return a, b
}
func mappedChips(chips []Chip, a, b, n int) []Chip {
	out := make([]Chip, 0, len(chips))
	for _, c := range chips {
		if a < c.To && b > c.From {
			continue
		}
		if c.From >= b {
			c.From += n - (b - a)
			c.To += n - (b - a)
		}
		out = append(out, c)
	}
	return out
}
func (m *Model) replaceRange(a, b int, text string) error {
	m.syncFeatures()
	r := []rune(m.Value())
	if a < 0 || b < a || b > len(r) {
		return fmt.Errorf("invalid edit range")
	}
	a, b = expandedRange(m.chips, a, b)
	value := string(r[:a]) + text + string(r[b:])
	old, pos := m.Value(), Position(*m)
	m.Model.SetValue(value)
	if m.Model.Value() != value {
		m.Model.SetValue(old)
		m.SetPosition(pos)
		return fmt.Errorf("edit exceeds textarea limits or contains unsupported characters")
	}
	m.chips = mappedChips(m.chips, a, b, utf8.RuneCountInString(text))
	m.chipVersion++
	m.featureText = value
	m.selection = nil
	m.layout = nil
	m.SetPosition(a + utf8.RuneCountInString(text))
	m.chipCursor = Position(*m)
	return nil
}

// Reconcile textarea edits (including word deletion) with atomic ranges. A
// partial label deletion removes the whole chip instead of leaving a broken label.
func (m *Model) reconcileChips(before editSnapshot, version uint64) {
	if version != m.chipVersion {
		before = editSnapshot{text: m.featureText, cursor: m.chipCursor, chips: slices.Clone(m.chips)}
	}
	if before.text == m.Value() {
		return
	}
	old, next := []rune(before.text), []rune(m.Value())
	a := 0
	limit := min(before.cursor, Position(*m))
	if before.selected {
		limit = min(before.anchor, before.head)
	}
	for a < limit && a < len(old) && a < len(next) && old[a] == next[a] {
		a++
	}
	b, n := len(old), len(next)
	for b > a && n > a && old[b-1] == next[n-1] {
		b--
		n--
	}
	start, end := expandedRange(before.chips, a, b)
	insert := string(next[a:n])
	value := string(old[:start]) + insert + string(old[end:])
	if value != m.Value() {
		m.Model.SetValue(value)
		m.selection = nil
		m.layout = nil
		m.chips = nil
		m.SetPosition(start + utf8.RuneCountInString(insert))
	}
	m.chips = mappedChips(before.chips, start, end, utf8.RuneCountInString(insert))
	m.chipVersion++
}
func (m *Model) SelectedChip() (Chip, bool) {
	a, b := m.SelectionBounds()
	for _, c := range m.chips {
		if c.From == a && c.To == b {
			return c, true
		}
	}
	return Chip{}, false
}
func (m *Model) chipKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if k.Paste {
		return false, nil
	}
	if c, ok := m.SelectedChip(); ok {
		if key.Matches(k, m.EditorKeys.ChipActivate) {
			return true, func() tea.Msg { return ChipActivateMsg{c} }
		}
		if key.Matches(k, m.EditorKeys.ChipExpand) {
			_ = m.ExpandChip(c.ID)
			return true, nil
		}
	}
	if m.SelectedText() != "" {
		return false, nil
	}
	pos := Position(*m)
	for _, c := range m.chips {
		if key.Matches(k, m.KeyMap.CharacterForward) && pos == c.From {
			m.ExtendSelection(func() { m.SetPosition(c.To) })
			return true, nil
		}
		if key.Matches(k, m.KeyMap.CharacterBackward) && pos == c.To {
			m.ExtendSelection(func() { m.SetPosition(c.From) })
			return true, nil
		}
	}
	return false, nil
}
func (m *Model) RenderChips(view string) string {
	rows := strings.Split(view, "\n")
	layout := m.Rows()
	offset := m.ScrollOffset(layout)
	r := []rune(m.Value())
	for y := range rows {
		if y+offset >= len(layout) {
			break
		}
		row := layout[y+offset]
		for _, c := range m.chips {
			a, b := max(row.Start, c.From), min(row.End, c.To)
			if a >= b {
				continue
			}
			x := m.GutterWidth + ansi.StringWidth(string(r[row.Start:a]))
			end := m.GutterWidth + ansi.StringWidth(string(r[row.Start:b]))
			rows[y] = ansi.Cut(rows[y], 0, x) + m.ChipStyle.Render(ansi.Cut(rows[y], x, end)) + ansi.Cut(rows[y], end, ansi.StringWidth(rows[y]))
		}
	}
	return strings.Join(rows, "\n")
}

// SetChips attaches validated chip metadata to labels already present in Value.
// It is undoable; use ClearHistory after loading a saved document if desired.
func (m *Model) SetChips(chips []Chip) error {
	m.syncFeatures()
	next := slices.Clone(chips)
	slices.SortFunc(next, func(a, b Chip) int { return a.From - b.From })
	r := []rune(m.Value())
	seen := map[string]bool{}
	end := 0
	for _, c := range next {
		if c.ID == "" || seen[c.ID] || !cleanLabel(c.Label) || c.From < end || c.From < 0 || c.To <= c.From || c.To > len(r) || string(r[c.From:c.To]) != c.Label {
			return fmt.Errorf("invalid or overlapping chip %q", c.ID)
		}
		seen[c.ID] = true
		end = c.To
	}
	done := m.beginEdit("")
	defer done()
	m.chips = next
	m.chipVersion++
	m.SetPosition(Position(*m))
	return nil
}
