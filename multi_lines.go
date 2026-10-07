package bed

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

type lineBlock struct{ first, last int }

func (m *Model) affectedLines() ([]string, []int, []lineBlock) {
	lines := strings.Split(m.Value(), "\n")
	starts := lineStarts(lines)
	var blocks []lineBlock
	for _, s := range m.Selections() {
		a, b := s.bounds()
		if b > a {
			b--
		}
		block := lineBlock{lineAt(starts, a), lineAt(starts, b)}
		if len(blocks) > 0 && block.first <= blocks[len(blocks)-1].last+1 {
			blocks[len(blocks)-1].last = max(block.last, blocks[len(blocks)-1].last)
		} else {
			blocks = append(blocks, block)
		}
	}
	return lines, starts, blocks
}
func (m *Model) multiIndent(out bool) error {
	lines, starts, blocks := m.affectedLines()
	var edits []Replacement
	for _, block := range blocks {
		for i := block.first; i <= block.last; i++ {
			e := Replacement{From: starts[i], To: starts[i], Text: strings.Repeat(" ", m.tabWidth())}
			if out {
				e.Text = ""
				for _, r := range lines[i] {
					if e.To == e.From && r != ' ' && strings.ContainsRune(m.IndentCharacters, r) {
						e.To++
						break
					}
					if r != ' ' || e.To-e.From == m.tabWidth() {
						break
					}
					e.To++
				}
			}
			for _, c := range m.chips {
				if e.From < c.To && e.To > c.From {
					return fmt.Errorf("indentation would split chip %q", c.ID)
				}
			}
			edits = append(edits, e)
		}
	}
	return m.ApplyEdits(edits)
}

// commitSelections is used for permutations that cannot be described by cursor
// offset deltas. Validation is performed before mutating the live editor.
func (m *Model) commitSelections(value string, chips []Chip, ss []Selection, primary uint64) error {
	if err := m.validateValue(value); err != nil {
		return err
	}
	m.Model.SetValue(value)
	m.chips = chips
	m.chipVersion++
	m.featureText = value
	m.layout = nil
	m.selectionRevision++
	m.Close()
	m.installSelections(ss, primary)
	m.chipCursor = Position(*m)
	return nil
}
func (m *Model) multiMoveLines(down bool) error {
	done := m.beginEdit("")
	defer done()
	lines, starts, blocks := m.affectedLines()
	order := make([]int, len(lines))
	for i := range order {
		order[i] = i
	}
	// If any block touches the requested edge the entire action is a no-op.
	if (!down && blocks[0].first == 0) || (down && blocks[len(blocks)-1].last == len(lines)-1) {
		return nil
	}
	for _, b := range blocks {
		if down {
			copy(order[b.first+1:b.last+2], order[b.first:b.last+1])
			order[b.first] = b.last + 1
		} else {
			copy(order[b.first-1:b.last], order[b.first:b.last+1])
			order[b.last] = b.first - 1
		}
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
	mapPos := func(p int) int { line := lineAt(starts, p); return positions[line] + p - starts[line] }
	ss := m.Selections()
	primary := m.PrimarySelection().ID
	value := strings.Join(result, "\n")
	n := utf8.RuneCountInString(value)
	for i, s := range ss {
		a, b := s.bounds()
		if a == b {
			ss[i].Head = mapPos(s.Head)
			ss[i].Anchor = ss[i].Head
		} else {
			aa, bb := mapPos(a), min(n, mapPos(b-1)+1)
			if s.Anchor < s.Head {
				ss[i].Anchor, ss[i].Head = aa, bb
			} else {
				ss[i].Anchor, ss[i].Head = bb, aa
			}
		}
		ss[i].Column = -1
	}
	chips := slices.Clone(m.chips)
	for i, c := range chips {
		chips[i].From = mapPos(c.From)
		chips[i].To = chips[i].From + c.To - c.From
	}
	slices.SortFunc(chips, func(a, b Chip) int { return a.From - b.From })
	return m.commitSelections(value, chips, ss, primary)
}
func (m *Model) multiDuplicate() error {
	done := m.beginEdit("")
	defer done()
	lines, starts, blocks := m.affectedLines()
	r := []rune(m.Value())
	ss := m.Selections()
	primary := m.PrimarySelection().ID
	// Duplicate complete affected line blocks, moving cursors into the copies.
	var out strings.Builder
	at, delta := 0, 0
	chips := slices.Clone(m.chips)
	ids := map[string]bool{}
	for _, c := range chips {
		ids[c.ID] = true
	}
	for _, b := range blocks {
		a := starts[b.first]
		end := starts[b.last] + utf8.RuneCountInString(lines[b.last])
		text := "\n" + string(r[a:end])
		n := utf8.RuneCountInString(text)
		out.WriteString(string(r[at:end]))
		out.WriteString(text)
		at = end
		for i, s := range m.Selections() {
			lo, hi := s.bounds()
			last := hi
			if hi > lo {
				last--
			}
			if lineAt(starts, lo) >= b.first && lineAt(starts, last) <= b.last {
				ss[i].Anchor = s.Anchor + delta + end + 1 - a
				ss[i].Head = s.Head + delta + end + 1 - a
				ss[i].Column = -1
			}
		}
		// Existing chips after this insertion shift; chips in the block get copies.
		chips = mappedChips(chips, end+delta, end+delta, n)
		for _, c := range m.chips {
			if c.From >= a && c.To <= end {
				original := c.ID
				for i := 1; ; i++ {
					c.ID = fmt.Sprintf("%s-copy-%d", original, i)
					if !ids[c.ID] {
						break
					}
				}
				ids[c.ID] = true
				c.From = end + delta + 1 + c.From - a
				c.To = c.From + utf8.RuneCountInString(c.Label)
				chips = append(chips, c)
			}
		}
		delta += n
	}
	out.WriteString(string(r[at:]))
	slices.SortFunc(chips, func(a, b Chip) int { return a.From - b.From })
	return m.commitSelections(out.String(), chips, ss, primary)
}

// CopyFragments returns selected text, or each distinct logical line when all
// cursors are empty. Mixed sets copy only nonempty ranges. Chip labels remain
// literal; external payload export is the host's responsibility.
func (m *Model) CopyFragments() []string {
	var out []string
	r := []rune(m.Value())
	for _, s := range m.Selections() {
		a, b := s.bounds()
		a, b = expandedRange(m.chips, a, b)
		if a != b {
			out = append(out, string(r[a:b]))
		}
	}
	if len(out) > 0 {
		return out
	}
	lines, _, blocks := m.affectedLines()
	for _, b := range blocks {
		for i := b.first; i <= b.last; i++ {
			out = append(out, lines[i])
		}
	}
	return out
}

// PasteFragments distributes explicit fragments in document order only when
// their count matches; otherwise it broadcasts fallback, without splitting it.
func (m *Model) PasteFragments(fragments []string, fallback string) error {
	ss := m.Selections()
	if len(fragments) != len(ss) {
		return m.InsertText(fallback)
	}
	var edits []Replacement
	for i, s := range ss {
		a, b := s.bounds()
		edits = append(edits, Replacement{a, b, fragments[i]})
	}
	done := m.beginEdit("")
	defer done()
	return m.applyEdits(edits, true)
}
