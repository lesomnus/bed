package bed

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TabWidth is the number of columns in a soft tab (defaults to four).
func (m *Model) tabWidth() int { return max(1, min(32, m.TabWidth)) }

type textEdit struct {
	from, to int
	text     string
}

// commitDocument preserves atomic chip metadata and makes an edit all-or-nothing.
func (m *Model) commitDocument(value string, chips []Chip, pos, anchor int, selected bool) error {
	old, oldPos := m.Value(), Position(*m)
	m.Model.SetValue(value)
	if m.Model.Value() != value {
		m.Model.SetValue(old)
		m.SetPosition(oldPos)
		return fmt.Errorf("edit exceeds textarea limits or contains unsupported characters")
	}
	m.chips = chips
	m.chipVersion++
	m.featureText = value
	m.layout = nil
	m.selection = nil
	m.SetPosition(pos)
	m.chipCursor = Position(*m)
	if selected {
		m.selection = &selection{value: value, document: m.DocumentKey, anchor: anchor, head: Position(*m)}
	}
	return nil
}

func lineStarts(lines []string) []int {
	out := make([]int, len(lines))
	n := 0
	for i, s := range lines {
		out[i] = n
		n += utf8.RuneCountInString(s) + 1
	}
	return out
}
func lineAt(starts []int, pos int) int {
	i := 0
	for i+1 < len(starts) && starts[i+1] <= pos {
		i++
	}
	return i
}
func (m *Model) selectedLines() ([]string, []int, int, int) {
	lines := strings.Split(m.Value(), "\n")
	starts := lineStarts(lines)
	a, b := m.SelectionBounds()
	if a == b {
		a = Position(*m)
		b = a
	} else {
		b--
	}
	return lines, starts, lineAt(starts, a), lineAt(starts, b)
}

// Indent indents selected logical lines, or inserts spaces to the next tab stop.
// Outdent removes up to TabWidth leading spaces from each affected logical line.
func (m *Model) Indent() error  { return m.indent(false) }
func (m *Model) Outdent() error { return m.indent(true) }
func (m *Model) indent(out bool) error {
	if len(m.Selections()) > 1 {
		return fmt.Errorf("line command does not yet support multiple cursors")
	}
	done := m.beginEdit("")
	defer done()
	a, b := m.SelectionBounds()
	if !out && a == b {
		pos := Position(*m)
		r := []rune(m.Value())
		start := pos
		for start > 0 && r[start-1] != '\n' {
			start--
		}
		n := m.tabWidth() - ansi.StringWidth(string(r[start:pos]))%m.tabWidth()
		return m.replaceRange(pos, pos, strings.Repeat(" ", n))
	}
	lines, starts, first, last := m.selectedLines()
	var edits []textEdit
	for i := first; i <= last; i++ {
		e := textEdit{from: starts[i], to: starts[i], text: strings.Repeat(" ", m.tabWidth())}
		if out {
			e.text = ""
			for _, r := range lines[i] {
				if e.to == e.from && r != ' ' && strings.ContainsRune(m.IndentCharacters, r) {
					e.to++
					break
				}
				if r != ' ' || e.to-e.from == m.tabWidth() {
					break
				}
				e.to++
			}
		}
		if e.from != e.to || e.text != "" {
			edits = append(edits, e)
		}
	}
	return m.applyLineEdits(edits)
}
func (m *Model) applyLineEdits(edits []textEdit) error {
	r := []rune(m.Value())
	chips := slices.Clone(m.chips)
	pos, anchor, selected := Position(*m), 0, m.SelectionValid()
	if selected {
		anchor = m.selection.anchor
	}
	mapPos := func(p int) int {
		delta := 0
		for _, e := range edits {
			n := utf8.RuneCountInString(e.text)
			if p < e.from {
				break
			}
			if p <= e.to {
				return e.from + delta + n
			}
			delta += n - (e.to - e.from)
		}
		return p + delta
	}
	var value strings.Builder
	at := 0
	for _, e := range edits {
		value.WriteString(string(r[at:e.from]))
		value.WriteString(e.text)
		at = e.to
	}
	value.WriteString(string(r[at:]))
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		for _, c := range chips {
			if e.from < c.To && e.to > c.From {
				return fmt.Errorf("indentation would split chip %q", c.ID)
			}
		}
		chips = mappedChips(chips, e.from, e.to, utf8.RuneCountInString(e.text))
	}
	return m.commitDocument(value.String(), chips, mapPos(pos), mapPos(anchor), selected)
}

// MoveLines moves the current logical line or all lines touched by the selection.
// A selection ending at a line start excludes that line. Document edges are no-ops.
func (m *Model) MoveLines(down bool) error {
	if len(m.Selections()) > 1 {
		return fmt.Errorf("line command does not yet support multiple cursors")
	}
	done := m.beginEdit("")
	defer done()
	lines, starts, first, last := m.selectedLines()
	if (!down && first == 0) || (down && last == len(lines)-1) {
		return nil
	}
	order := make([]int, len(lines))
	for i := range order {
		order[i] = i
	}
	if down {
		copy(order[first+1:last+2], order[first:last+1])
		order[first] = last + 1
	} else {
		copy(order[first-1:last], order[first:last+1])
		order[last] = first - 1
	}
	result := make([]string, len(lines))
	for i, old := range order {
		result[i] = lines[old]
	}
	next := lineStarts(result)
	positions := make([]int, len(lines))
	for i, old := range order {
		positions[old] = next[i]
	}
	mapPos := func(p int) int { l := lineAt(starts, p); return positions[l] + p - starts[l] }
	chips := slices.Clone(m.chips)
	for i, c := range chips {
		chips[i].From = mapPos(c.From)
		chips[i].To = chips[i].From + (c.To - c.From)
	}
	slices.SortFunc(chips, func(a, b Chip) int { return a.From - b.From })
	pos, anchor, sel := Position(*m), 0, m.SelectionValid()
	if sel {
		a, b := m.SelectionBounds()
		aa := mapPos(a)
		bb := min(utf8.RuneCountInString(strings.Join(result, "\n")), mapPos(b-1)+1)
		if m.selection.anchor < m.selection.head {
			anchor, pos = aa, bb
		} else {
			anchor, pos = bb, aa
		}
	} else {
		pos = mapPos(pos)
	}
	return m.commitDocument(strings.Join(result, "\n"), chips, pos, anchor, sel)
}

// Duplicate duplicates the selected text, or the current logical line when empty.
// Cloned chips receive distinct IDs derived from their original IDs.
func (m *Model) Duplicate() error {
	if len(m.Selections()) > 1 {
		return fmt.Errorf("line command does not yet support multiple cursors")
	}
	done := m.beginEdit("")
	defer done()
	a, b := m.SelectionBounds()
	selected := a != b
	r := []rune(m.Value())
	prefix := ""
	if !selected {
		lines, starts, first, _ := m.selectedLines()
		a = starts[first]
		b = a + utf8.RuneCountInString(lines[first])
		prefix = "\n"
	}
	text := prefix + string(r[a:b])
	n := utf8.RuneCountInString(text)
	chips := mappedChips(m.chips, b, b, n)
	ids := map[string]bool{}
	for _, c := range chips {
		ids[c.ID] = true
	}
	for _, c := range m.chips {
		if c.From >= a && c.To <= b {
			id := c.ID
			for i := 1; ; i++ {
				c.ID = fmt.Sprintf("%s-copy-%d", id, i)
				if !ids[c.ID] {
					break
				}
			}
			ids[c.ID] = true
			c.From = b + utf8.RuneCountInString(prefix) + c.From - a
			c.To = c.From + utf8.RuneCountInString(c.Label)
			chips = append(chips, c)
		}
	}
	slices.SortFunc(chips, func(a, b Chip) int { return a.From - b.From })
	pos := b + n
	if !selected {
		pos = b + 1 + Position(*m) - a
	}
	return m.commitDocument(string(r[:b])+text+string(r[b:]), chips, pos, b, selected)
}

// InsertNewline optionally repeats leading spaces; pasted text is never reindented.
func (m *Model) InsertNewline() error {
	if len(m.Selections()) > 1 {
		var es []Replacement
		for _, s := range m.Selections() {
			a, b := s.bounds()
			es = append(es, Replacement{a, b, m.newlineAt(a)})
		}
		done := m.beginEdit("")
		defer done()
		return m.applyEdits(es, true)
	}
	done := m.beginEdit("")
	defer done()
	a, b := m.SelectionBounds()
	if a == b {
		a = Position(*m)
		b = a
	}
	r := []rune(m.Value())
	start := a
	for start > 0 && r[start-1] != '\n' {
		start--
	}
	indent := ""
	if m.AutoIndent {
		end := start
		for end < a && strings.ContainsRune(m.IndentCharacters, r[end]) {
			end++
		}
		indent = string(r[start:end])
	}
	return m.replaceRange(a, b, "\n"+indent)
}
func (m *Model) lineKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if k.Paste {
		return false, nil
	}
	var err error
	switch {
	case key.Matches(k, m.EditorKeys.Indent):
		err = m.Indent()
	case key.Matches(k, m.EditorKeys.Outdent):
		err = m.Outdent()
	case key.Matches(k, m.EditorKeys.MoveLinesUp):
		err = m.MoveLines(false)
	case key.Matches(k, m.EditorKeys.MoveLinesDown):
		err = m.MoveLines(true)
	case key.Matches(k, m.EditorKeys.Duplicate):
		err = m.Duplicate()
	default:
		return false, nil
	}
	if err != nil {
		return true, func() tea.Msg { return EditErrorMsg{err} }
	}
	return true, nil
}

// EditErrorMsg reports a rejected atomic edit without changing the document.
type EditErrorMsg struct{ Err error }
