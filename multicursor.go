package bed

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// Selection describes a directed range of rune offsets. Equal offsets are a cursor.
// ID is assigned by bed when zero; Column is the desired terminal-cell column
// for vertical movement (-1 means recalculate).
type Selection struct {
	Anchor, Head int
	ID           uint64
	Column       int
}

func (s Selection) bounds() (int, int) { return min(s.Anchor, s.Head), max(s.Anchor, s.Head) }

// Selections returns a detached, document-ordered copy. PrimarySelection identifies
// the active cursor independently of the order.
func (m *Model) Selections() []Selection {
	if len(m.cursors) > 0 && m.cursorDocument == m.DocumentKey && m.cursorText == m.Value() {
		for _, s := range m.cursors {
			if s.ID == m.primaryCursor && s.Head == Position(*m) {
				return slices.Clone(m.cursors)
			}
		}
	}
	p := Position(*m)
	a := p
	if m.SelectionValid() {
		a = m.selection.anchor
	}
	return []Selection{{Anchor: a, Head: p, ID: 1, Column: -1}}
}
func (m *Model) PrimarySelection() Selection {
	ss := m.Selections()
	for _, s := range ss {
		if s.ID == m.primaryCursor {
			return s
		}
	}
	return ss[0]
}
func (m *Model) cursorLimit() int {
	if m.CursorLimit <= 0 {
		return 1
	}
	return min(m.CursorLimit, 4096)
}

// SetSelections validates all ranges before changing anything. primary is an
// index in the supplied slice, before normalization. Overlaps merge; adjacent
// nonempty selections stay separate. IDs need not be supplied.
func (m *Model) SetSelections(ss []Selection, primary int) error {
	if len(ss) == 0 || primary < 0 || primary >= len(ss) || len(ss) > m.cursorLimit() {
		return fmt.Errorf("invalid selection set or cursor limit exceeded")
	}
	ss = slices.Clone(ss)
	n := utf8.RuneCountInString(m.Value())
	seen := map[uint64]bool{}
	for i := range ss {
		s := &ss[i]
		if s.Anchor < 0 || s.Head < 0 || s.Anchor > n || s.Head > n {
			return fmt.Errorf("invalid selection range")
		}
		if s.ID != 0 && seen[s.ID] {
			return fmt.Errorf("duplicate selection ID")
		}
		seen[s.ID] = true
	}
	next := m.nextCursor
	for i := range ss {
		if ss[i].ID == 0 {
			for {
				next++
				if !seen[next] {
					break
				}
			}
			ss[i].ID = next
			seen[next] = true
		}
		next = max(next, ss[i].ID)
		a, b := ss[i].bounds()
		if a != b {
			a, b = expandedRange(m.chips, a, b)
			if ss[i].Anchor < ss[i].Head {
				ss[i].Anchor, ss[i].Head = a, b
			} else {
				ss[i].Anchor, ss[i].Head = b, a
			}
		} else {
			ss[i].Anchor = m.snapNearest(a)
			ss[i].Head = ss[i].Anchor
		}
	}
	m.nextCursor = next
	m.BreakUndoGroup()
	m.Close()
	m.selectionRevision++
	m.installSelections(ss, ss[primary].ID)
	return nil
}
func (m *Model) installSelections(ss []Selection, primary uint64) {
	ss = slices.Clone(ss)
	slices.SortStableFunc(ss, func(a, b Selection) int { x, _ := a.bounds(); y, _ := b.bounds(); return x - y })
	out := make([]Selection, 0, len(ss))
	for _, s := range ss {
		a, b := s.bounds()
		if len(out) > 0 {
			last := &out[len(out)-1]
			x, y := last.bounds()
			if a < y || (a == y && (a == b || x == y)) {
				reverse := last.Anchor > last.Head
				if s.ID == primary {
					last.ID = primary
					reverse = s.Anchor > s.Head
					last.Column = s.Column
				}
				b = max(b, y)
				if reverse {
					last.Anchor = b
					last.Head = x
				} else {
					last.Anchor = x
					last.Head = b
				}
				continue
			}
		}
		out = append(out, s)
	}
	m.cursors = out
	m.primaryCursor = primary
	m.cursorText = m.Value()
	m.cursorDocument = m.DocumentKey
	p := out[0]
	for _, s := range out {
		if s.ID == primary {
			p = s
			break
		}
	}
	m.primaryCursor = p.ID
	m.setPosition(p.Head)
	m.selection = nil
	if p.Anchor != p.Head {
		m.selection = &selection{value: m.Value(), document: m.DocumentKey, anchor: p.Anchor, head: p.Head}
	}
}
func (m *Model) snapNearest(p int) int {
	for _, c := range m.chips {
		if p > c.From && p < c.To {
			if p-c.From < c.To-p {
				return c.From
			}
			return c.To
		}
	}
	return p
}

// ClearSecondaryCursors retains the primary range and invalidates async results.
func (m *Model) ClearSecondaryCursors() {
	s := m.PrimarySelection()
	m.cursors = nil
	m.selectionRevision++
	m.Close()
	m.BreakUndoGroup()
	m.setPosition(s.Head)
	m.selection = nil
	if s.Anchor != s.Head {
		m.selection = &selection{value: m.Value(), document: m.DocumentKey, anchor: s.Anchor, head: s.Head}
	}
}
func (m *Model) AddCursor(pos int) error {
	ss := m.Selections()
	for i, s := range ss {
		if s.Anchor == pos && s.Head == pos {
			return m.SetSelections(ss, i)
		}
	}
	ss = append(ss, Selection{Anchor: pos, Head: pos, Column: -1})
	return m.SetSelections(ss, len(ss)-1)
}
func (m *Model) vertical(s Selection, down bool) Selection {
	rows := m.Rows()
	i := RowIndex(rows, s.Head)
	r := []rune(m.Value())
	col := s.Column
	if col < 0 {
		col = ansi.StringWidth(string(r[rows[i].Start:s.Head]))
	}
	s.Column = col
	j := i - 1
	if down {
		j = i + 1
	}
	if j < 0 || j >= len(rows) {
		return s
	}
	p := rows[j].Start
	width := 0
	g := uniseg.NewGraphemes(string(r[p:rows[j].End]))
	for g.Next() {
		if width+g.Width() > col {
			break
		}
		p += utf8.RuneCountInString(g.Str())
		width += g.Width()
	}
	s.Head = m.snapNearest(min(p, RowEnd(rows, j)))
	return s
}

// AddCursorVertical uses visual rows, including soft wraps.
func (m *Model) AddCursorVertical(down bool) error {
	s := m.PrimarySelection()
	next := m.vertical(s, down)
	if next.Head == s.Head {
		return nil
	}
	ss := m.Selections()
	for i := range ss {
		if ss[i].ID == s.ID {
			ss[i].Column = next.Column
		}
	}
	for i := range ss {
		if ss[i].Head == next.Head && ss[i].Anchor == next.Head {
			return m.SetSelections(ss, i)
		}
	}
	next.Anchor = next.Head
	next.ID = 0
	ss = append(ss, next)
	return m.SetSelections(ss, len(ss)-1)
}

// Replacement is an edit against the current immutable document (rune offsets).
type Replacement struct {
	From, To int
	Text     string
}

// ApplyEdits atomically applies non-conflicting replacements and remaps every
// selection. Chip intersections expand to complete chips. Conflicting text for
// intersecting edits rejects the entire transaction.
func (m *Model) ApplyEdits(edits []Replacement) error {
	done := m.beginEdit("")
	defer done()
	return m.applyEdits(edits, false)
}
func (m *Model) applyEdits(edits []Replacement, collapse bool) error {
	edits = slices.Clone(edits)
	r := []rune(m.Value())
	for i := range edits {
		e := &edits[i]
		if e.From < 0 || e.To < e.From || e.To > len(r) {
			return fmt.Errorf("invalid edit range")
		}
		e.From, e.To = expandedRange(m.chips, e.From, e.To)
	}
	slices.SortStableFunc(edits, func(a, b Replacement) int { return a.From - b.From })
	merged := make([]Replacement, 0, len(edits))
	for _, e := range edits {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if e.From < last.To || (e.From == last.To && (e.From == e.To || last.From == last.To)) {
				if e.Text != last.Text {
					return fmt.Errorf("conflicting overlapping replacements")
				}
				last.To = max(last.To, e.To)
				continue
			}
		}
		merged = append(merged, e)
	}
	if len(merged) == 0 {
		return nil
	}
	var b strings.Builder
	at := 0
	for _, e := range merged {
		b.WriteString(string(r[at:e.From]))
		b.WriteString(e.Text)
		at = e.To
	}
	b.WriteString(string(r[at:]))
	value := b.String()
	// A fresh textarea validates without changing the live viewport on rejection.
	if err := m.validateValue(value); err != nil {
		return err
	}

	ss := m.Selections()
	primary := m.PrimarySelection().ID
	remap := func(p int) int {
		delta := 0
		for _, e := range merged {
			n := utf8.RuneCountInString(e.Text)
			if p < e.From {
				break
			}
			if p <= e.To {
				return e.From + delta + n
			}
			delta += n - (e.To - e.From)
		}
		return p + delta
	}
	for i := range ss {
		ss[i].Head = remap(ss[i].Head)
		ss[i].Anchor = remap(ss[i].Anchor)
		ss[i].Column = -1
		if collapse {
			ss[i].Anchor = ss[i].Head
		}
	}
	chips := slices.Clone(m.chips)
	for i := len(merged) - 1; i >= 0; i-- {
		e := merged[i]
		chips = mappedChips(chips, e.From, e.To, utf8.RuneCountInString(e.Text))
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

// InsertText broadcasts text to all selections and reports validation errors.
func (m *Model) InsertText(text string) error {
	done := m.beginEdit("")
	defer done()
	var edits []Replacement
	for _, s := range m.Selections() {
		a, b := s.bounds()
		edits = append(edits, Replacement{a, b, text})
	}
	return m.applyEdits(edits, true)
}
func editResult(err error) (bool, tea.Cmd) {
	if err != nil {
		return true, func() tea.Msg { return EditErrorMsg{err} }
	}
	return true, nil
}
func (m *Model) multiKey(k tea.KeyMsg) (bool, tea.Cmd) {
	match := func(bs ...key.Binding) bool { return !k.Paste && key.Matches(k, bs...) }
	if match(m.EditorKeys.AddCursorAbove, m.EditorKeys.AddCursorBelow) {
		return editResult(m.AddCursorVertical(match(m.EditorKeys.AddCursorBelow)))
	}
	if match(m.EditorKeys.ClearCursors) {
		if len(m.Selections()) > 1 {
			m.ClearSecondaryCursors()
			return true, nil
		}
		if m.SelectionValid() {
			m.ClearSelection()
			return true, nil
		}
	}
	if len(m.Selections()) < 2 {
		return false, nil
	}
	if match(m.EditorKeys.Cut) {
		text := m.CopyText()
		return editResultWithCopy(m.CutSelections(), text)
	}
	if match(m.EditorKeys.Undo, m.EditorKeys.Redo, m.EditorKeys.Indent, m.EditorKeys.Outdent, m.EditorKeys.MoveLinesUp, m.EditorKeys.MoveLinesDown, m.EditorKeys.Duplicate, m.EditorKeys.ToggleWhitespace) {
		return false, nil
	}
	if match(m.KeyMap.Paste) {
		return true, func() tea.Msg {
			v, err := clipboard.ReadAll()
			if err != nil {
				return EditErrorMsg{err}
			}
			return multiPasteMsg(v)
		}
	}
	done := m.beginEdit(m.editKind(k))
	defer done()
	ss := m.Selections()
	primary := m.PrimarySelection().ID
	var edits []Replacement
	typing := k.Paste || ((k.Type == tea.KeyRunes || k.Type == tea.KeySpace) && !m.baseBindingMatches(k))
	newline := match(m.KeyMap.InsertNewline)
	delBack := match(m.KeyMap.DeleteCharacterBackward)
	delForward := match(m.KeyMap.DeleteCharacterForward)
	wordBack := match(m.KeyMap.DeleteWordBackward)
	wordForward := match(m.KeyMap.DeleteWordForward)
	deleteBefore := match(m.KeyMap.DeleteBeforeCursor)
	deleteAfter := match(m.KeyMap.DeleteAfterCursor)
	if typing || newline || delBack || delForward || wordBack || wordForward || deleteBefore || deleteAfter {
		for _, s := range ss {
			a, b := s.bounds()
			text := ""
			if typing {
				text = string(k.Runes)
				if k.Type == tea.KeySpace {
					text = " "
				}
			}
			if newline {
				text = m.newlineAt(a)
			}
			if a == b {
				switch {
				case delBack:
					a = GraphemeMove(m.Value(), a, false)
				case delForward:
					b = GraphemeMove(m.Value(), b, true)
				case wordBack:
					a = m.wordTarget(a, false)
				case wordForward:
					b = m.wordTarget(b, true)
				case deleteBefore:
					a = m.logicalEdge(a, false)
				case deleteAfter:
					b = m.logicalEdge(b, true)
				}
			}
			edits = append(edits, Replacement{a, b, text})
		}
		return editResult(m.applyEdits(edits, true))
	}
	shifted := match(m.EditorKeys.SelectLeft, m.EditorKeys.SelectRight, m.EditorKeys.SelectUp, m.EditorKeys.SelectDown, m.EditorKeys.SelectWordLeft, m.EditorKeys.SelectWordRight, m.EditorKeys.SelectRowStart, m.EditorKeys.SelectRowEnd)
	for i, s := range ss {
		p := s.Head
		a, b := s.bounds()
		vertical := false
		switch {
		case match(m.KeyMap.CharacterBackward, m.EditorKeys.SelectLeft):
			if !shifted && a != b {
				p = a
			} else {
				p = GraphemeMove(m.Value(), p, false)
			}
		case match(m.KeyMap.CharacterForward, m.EditorKeys.SelectRight):
			if !shifted && a != b {
				p = b
			} else {
				p = GraphemeMove(m.Value(), p, true)
			}
		case match(m.KeyMap.WordBackward, m.EditorKeys.SelectWordLeft):
			p = m.wordTarget(p, false)
		case match(m.KeyMap.WordForward, m.EditorKeys.SelectWordRight):
			p = m.wordTarget(p, true)
		case match(m.KeyMap.LinePrevious, m.EditorKeys.SelectUp, m.KeyMap.LineNext, m.EditorKeys.SelectDown):
			s = m.vertical(s, match(m.KeyMap.LineNext, m.EditorKeys.SelectDown))
			p = s.Head
			vertical = true
		case match(m.EditorKeys.RowStart, m.EditorKeys.SelectRowStart, m.EditorKeys.RowEnd, m.EditorKeys.SelectRowEnd):
			rows := m.Rows()
			j := RowIndex(rows, p)
			if match(m.EditorKeys.RowEnd, m.EditorKeys.SelectRowEnd) {
				p = RowEnd(rows, j)
			} else {
				p = rows[j].Start
			}
		case match(m.KeyMap.LineStart):
			p = m.logicalEdge(p, false)
		case match(m.KeyMap.LineEnd):
			p = m.logicalEdge(p, true)
		case match(m.KeyMap.InputBegin):
			p = 0
		case match(m.KeyMap.InputEnd):
			p = utf8.RuneCountInString(m.Value())
		default:
			if m.baseBindingMatches(k) {
				return editResult(fmt.Errorf("this action is not supported with multiple cursors"))
			}
			return false, nil
		}
		for _, c := range m.chips {
			if p > c.From && p < c.To {
				if p < s.Head {
					p = c.From
				} else {
					p = c.To
				}
			}
		}
		s.Head = p
		if !shifted {
			s.Anchor = p
		}
		if !vertical {
			s.Column = -1
		}
		ss[i] = s
	}
	m.BreakUndoGroup()
	m.Close()
	m.selectionRevision++
	m.installSelections(ss, primary)
	return true, nil
}
func (m *Model) wordTarget(p int, forward bool) int {
	r := []rune(m.Value())
	if forward {
		for p < len(r) && m.wordSpace(r[p]) {
			p++
		}
		for p < len(r) && !m.wordSpace(r[p]) {
			p++
		}
	} else {
		for p > 0 && m.wordSpace(r[p-1]) {
			p--
		}
		for p > 0 && !m.wordSpace(r[p-1]) {
			p--
		}
	}
	return p
}
func (m *Model) logicalEdge(p int, forward bool) int {
	r := []rune(m.Value())
	if forward {
		for p < len(r) && r[p] != '\n' {
			p++
		}
	} else {
		for p > 0 && r[p-1] != '\n' {
			p--
		}
	}
	return p
}
func (m *Model) newlineAt(p int) string {
	r := []rune(m.Value())
	start := m.logicalEdge(p, false)
	end := start
	if m.AutoIndent {
		for end < p && strings.ContainsRune(m.IndentCharacters, r[end]) {
			end++
		}
	}
	return "\n" + string(r[start:end])
}
func editResultWithCopy(err error, text string) (bool, tea.Cmd) {
	if err != nil {
		return editResult(err)
	}
	return true, func() tea.Msg { return CopyMsg(text) }
}

// CopyText returns selected fragments in document order, separated by newlines.
func (m *Model) CopyText() string { return strings.Join(m.CopyFragments(), "\n") }

func (m *Model) CutSelections() error {
	var edits []Replacement
	for _, s := range m.Selections() {
		a, b := s.bounds()
		if a != b {
			edits = append(edits, Replacement{a, b, ""})
		}
	}
	if len(edits) == 0 {
		lines, starts, blocks := m.affectedLines()
		n := utf8.RuneCountInString(m.Value())
		for _, b := range blocks {
			a, end := starts[b.first], n
			if b.last+1 < len(lines) {
				end = starts[b.last+1]
			} else if a > 0 {
				a--
			}
			edits = append(edits, Replacement{a, end, ""})
		}
	}
	return m.ApplyEdits(edits)
}

type multiPasteMsg string

func (m *Model) renderSelections(view string) string {
	ss := m.Selections()
	primary := m.PrimarySelection().ID
	rows := strings.Split(view, "\n")
	layout := m.Rows()
	offset := m.ScrollOffset(layout)
	r := []rune(m.Value())
	for _, s := range ss {
		a, b := s.bounds()
		a, b = expandedRange(m.chips, a, b)
		for y := range rows {
			if y+offset >= len(layout) {
				break
			}
			row := layout[y+offset]
			start, end := max(a, row.Start), min(b, row.End)
			newline := b > row.End && a <= row.End && row.End < len(r) && r[row.End] == '\n'
			caret := a == b && s.ID != primary && m.Focused() && !m.Cursor.Blink && RowIndex(layout, a) == y+offset
			if start >= end && !newline && !caret {
				continue
			}
			if start > end {
				start = end
			}
			left := m.GutterWidth + ansi.StringWidth(string(r[row.Start:start]))
			right := m.GutterWidth + ansi.StringWidth(string(r[row.Start:end]))
			if newline {
				right++
			}
			if caret {
				right = left + 1
				if a < len(r) && r[a] != '\n' {
					right = left + max(1, ansi.StringWidth(string(r[a:GraphemeMove(m.Value(), a, true)])))
				}
			}
			width := ansi.StringWidth(rows[y])
			if right > width {
				rows[y] += strings.Repeat(" ", right-width)
				width = right
			}
			rows[y] = ansi.Cut(rows[y], 0, left) + indexedBackground(ansi.Cut(rows[y], left, right), m.SelectionColor) + ansi.Cut(rows[y], right, width)
		}
	}
	return strings.Join(rows, "\n")
}

func (m *Model) validateValue(value string) error {
	probe := textarea.New()
	probe.CharLimit = m.CharLimit
	probe.MaxHeight = m.MaxHeight
	probe.SetValue(value)
	if probe.Value() != value {
		return fmt.Errorf("edit exceeds textarea limits or contains unsupported characters")
	}
	return nil
}
