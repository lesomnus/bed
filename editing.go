// Package bed provides a reusable Bubble Tea text editor built on bubbles/textarea.
package bed

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
	"strings"
	"unicode/utf8"
)

type selection struct {
	value, document string
	anchor, head    int
	dragging        bool
}
type Row struct{ Start, End, Line, Column int }
type layout struct {
	value string
	width int
	rows  []Row
}

func Position(in Model) int {
	pos := 0
	lines := strings.Split(in.Value(), "\n")
	for _, line := range lines[:in.Line()] {
		pos += utf8.RuneCountInString(line) + 1
	}
	li := in.LineInfo()
	return pos + li.StartColumn + li.ColumnOffset
}
func (m *Model) SetPosition(pos int) {
	r := []rune(m.Model.Value())
	pos = max(0, min(len(r), pos))
	prefix := string(r[:pos])
	line := strings.Count(prefix, "\n")
	parts := strings.Split(prefix, "\n")
	column := utf8.RuneCountInString(parts[len(parts)-1])
	for m.Model.Line() > line {
		m.Model.CursorStart()
		m.Model.CursorUp()
	}
	for m.Model.Line() < line {
		m.Model.CursorEnd()
		m.Model.CursorDown()
	}
	m.Model.SetCursor(column)
	m.Model, _ = m.Model.Update(nil)
}

func (m *Model) SelectionValid() bool {
	s := m.selection
	if s == nil {
		return false
	}
	if s.value != m.Model.Value() || s.document != m.DocumentKey || s.head != Position(*m) {
		m.selection = nil
		return false
	}
	return true
}
func (m *Model) SelectionBounds() (int, int) {
	if !m.SelectionValid() {
		return 0, 0
	}
	s := m.selection
	a, b := min(s.anchor, s.head), max(s.anchor, s.head)
	if a == b {
		return a, b
	}
	// A chip is one editable object even if its label wraps across rows.
	for _, token := range m.AtomicTokens {
		if token == "" {
			continue
		}
		for offset := 0; offset < len(s.value); {
			i := strings.Index(s.value[offset:], token)
			if i < 0 {
				break
			}
			i += offset
			start := utf8.RuneCountInString(s.value[:i])
			end := start + utf8.RuneCountInString(token)
			if a < end && b > start {
				a = min(a, start)
				b = max(b, end)
			}
			offset = i + len(token)
		}
	}
	return a, b
}
func (m *Model) SelectedText() string {
	a, b := m.SelectionBounds()
	if a == b {
		return ""
	}
	return string([]rune(m.Model.Value())[a:b])
}
func (m *Model) DeleteSelection() bool {
	a, b := m.SelectionBounds()
	if a == b {
		return false
	}
	r := []rune(m.Model.Value())
	m.selection = nil

	m.Model.SetValue(string(r[:a]) + string(r[b:]))
	m.SetPosition(a)
	return true
}

// Home and End work on the row you can see rather than the line behind it: a
// wrapped draft is one line in the value and several rows on screen, and a key
// that jumps past what is visible is a key you cannot aim with. Pressing again
// on an edge steps to the neighbouring row's edge, so the pair walks the draft
// instead of doing nothing on the second press.
func (m *Model) RowEdge(forward bool) {
	rows := m.Rows()
	if len(rows) == 0 {
		return
	}
	pos := Position(*m)
	i := RowIndex(rows, pos)
	target := rows[i].Start
	if forward {
		target = RowEnd(rows, i)
	}
	if target == pos {
		switch {
		case forward && i+1 < len(rows):
			target = RowEnd(rows, i+1)
		case !forward && i > 0:
			target = rows[i-1].Start
		}
	}
	m.SetPosition(target)
}

// RowIndex answers the question the widget answers when it draws: which
// row does this position appear on. A position on a wrap boundary belongs to the
// row that starts there, and one on a line's trailing newline to the row that
// ends there.
func RowIndex(rows []Row, pos int) int {
	for i, row := range rows {
		if pos < row.End {
			return i
		}
		if i+1 < len(rows) && pos < rows[i+1].Start {
			return i
		}
	}
	return len(rows) - 1
}

// RowEnd is the last position that still renders on the row. A soft wrap
// has no character of its own, so the position after a wrapped row's last
// character is also the position before the next row's first, and the widget
// draws it there; stopping one short keeps End on the row it was pressed on.
func RowEnd(rows []Row, i int) int {
	row := rows[i]
	if i+1 < len(rows) && rows[i+1].Line == row.Line {
		return max(row.Start, row.End-1)
	}
	return row.End
}

func GraphemeMove(value string, pos int, forward bool) int {
	g := uniseg.NewGraphemes(value)
	start := 0
	for g.Next() {
		end := start + utf8.RuneCountInString(g.Str())
		if forward && pos < end {
			return end
		}
		if !forward && pos <= end {
			return start
		}
		start = end
	}
	return start
}

// ExtendSelection is a move with the anchor kept. The move runs through
// the same code the unshifted key would use, so a selection can never cover
// something the cursor could not have reached by itself.
func (m *Model) ExtendSelection(move func()) {
	if !m.SelectionValid() {
		m.selection = &selection{value: m.Model.Value(), document: m.DocumentKey, anchor: Position(*m)}
	}
	s := m.selection

	move()
	// Bounds expand partial chip selections to whole objects.
	s.head = Position(*m)
	s.dragging = false

}

func (m *Model) HandleKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if !m.Focused() {
		return false, nil
	}
	if !k.Paste && k.String() == "alt+w" {
		m.ShowWhitespace = !m.ShowWhitespace
		return true, nil
	}
	// Word-wise selection reuses the widget's own word motion, so what
	// ctrl+left selects is exactly what ctrl+left would have walked over.
	moves := map[string]tea.KeyType{
		"shift+left": tea.KeyLeft, "shift+right": tea.KeyRight, "shift+up": tea.KeyUp, "shift+down": tea.KeyDown,
		"ctrl+shift+left": tea.KeyCtrlLeft, "ctrl+shift+right": tea.KeyCtrlRight,
	}
	if direction, ok := moves[k.String()]; ok && !k.Paste {
		m.ExtendSelection(func() {
			if direction == tea.KeyLeft || direction == tea.KeyRight {
				m.SetPosition(GraphemeMove(m.Model.Value(), Position(*m), direction == tea.KeyRight))
			} else {
				m.Model, _ = m.Model.Update(tea.KeyMsg{Type: direction})
			}
		})
		return true, nil
	}
	// Shift turns the edge keys into a selection to the same place they move to,
	// including the second press that steps to the neighbouring row.
	if forward, ok := map[string]bool{"shift+home": false, "shift+end": true}[k.String()]; ok && !k.Paste {
		m.ExtendSelection(func() { m.RowEdge(forward) })
		return true, nil
	}
	if edge := k.String(); (edge == "home" || edge == "end") && !k.Paste {
		m.selection = nil

		m.RowEdge(edge == "end")

		return true, nil
	}
	if !m.SelectionValid() {
		return false, nil
	}
	a, b := m.SelectionBounds()
	if a == b {
		m.selection = nil
		return false, nil
	}
	switch k.String() {
	case "left", "right":
		if !k.Paste {
			pos := a
			if k.Type == tea.KeyRight {
				pos = b
			}
			m.selection = nil
			m.SetPosition(pos)

			return true, nil
		}
	case "backspace", "delete":
		if !k.Paste {
			m.DeleteSelection()
			return true, nil
		}
	case "ctrl+x":
		if !k.Paste {
			text := m.SelectedText()
			m.DeleteSelection()
			return true, func() tea.Msg { return CopyMsg(text) }
		}
	}
	if k.Type == tea.KeyRunes || k.Type == tea.KeySpace || k.Type == tea.KeyEnter || k.String() == "alt+enter" || k.String() == "ctrl+j" || k.Paste {
		m.DeleteSelection()
	} else {
		m.selection = nil
	}
	return false, nil
}

// Use textarea's own wrapping metadata rather than assuming hard wrapping.
// The cache contains only offsets into the draft and is invalidated on resize/edit.
func (m *Model) Rows() []Row {
	value := m.Model.Value()
	if c := m.layout; c != nil && c.value == value && c.width == m.Model.Width() {
		return c.rows
	}
	var rows []Row
	base := 0
	for line, text := range strings.Split(value, "\n") {
		probe := m.Model
		probe.SetValue(text)
		length := utf8.RuneCountInString(text)
		for start := 0; start <= length; {
			probe.SetCursor(start)
			li := probe.LineInfo()
			end := min(length, li.StartColumn+li.Width)
			rows = append(rows, Row{base + li.StartColumn, base + end, line, li.StartColumn})
			if li.RowOffset+1 >= li.Height || li.Width == 0 {
				break
			}
			start = li.StartColumn + li.Width
		}
		base += length + 1
	}
	m.layout = &layout{value, m.Model.Width(), rows}
	return rows
}
func (m *Model) ScrollOffset(rows []Row) int {
	probe := m.Model
	probe.Focus()
	probe.Cursor.Blink = false
	probe.Cursor.Style = cursorProbeStyle
	_, visible, ok := widgetCursor(probe.View())
	if !ok {
		return 0
	}
	li := m.Model.LineInfo()
	for i, row := range rows {
		if row.Line == m.Model.Line() && row.Column == li.StartColumn {
			return max(0, i-visible)
		}
	}
	return 0
}
func (m *Model) Point(x, y int) int {
	rows := m.Rows()
	offset := m.ScrollOffset(rows)
	if y+offset >= len(rows) {
		return utf8.RuneCountInString(m.Model.Value())
	}
	row := rows[max(0, y+offset)]
	r := []rune(m.Model.Value())
	pos := row.Start
	col := 0
	g := uniseg.NewGraphemes(string(r[row.Start:row.End]))
	for g.Next() {
		w := g.Width()
		if x < col+w {
			break
		}
		col += w
		pos += utf8.RuneCountInString(g.Str())
	}
	return pos
}
func (m *Model) HandleMouse(v tea.MouseMsg) bool {
	x, y := v.X, v.Y
	inside := y >= 0 && y < m.Height() && x >= -m.GutterWidth && x < m.Width()-m.GutterWidth
	if v.Button == tea.MouseButtonWheelUp || v.Button == tea.MouseButtonWheelDown {
		return inside && m.Scroll(v.Button == tea.MouseButtonWheelDown)
	}
	dragging := m.SelectionValid() && m.selection.dragging
	if dragging && (v.Action == tea.MouseActionMotion || v.Action == tea.MouseActionRelease) {
		s := m.selection
		m.SetPosition(m.Point(max(0, x), y))
		s.head = Position(*m)
		if v.Action == tea.MouseActionRelease {
			s.dragging = false
		}
		return true
	}
	if v.Action != tea.MouseActionPress || v.Button != tea.MouseButtonLeft || !inside {
		return false
	}
	m.Focus()
	m.SetPosition(m.Point(max(0, x), y))
	pos := Position(*m)
	m.selection = &selection{value: m.Value(), document: m.DocumentKey, anchor: pos, head: pos, dragging: true}
	return true
}
func (m *Model) RenderSelection(view string) string {

	a, b := m.SelectionBounds()
	if a == b {
		return view
	}
	layout := m.Rows()
	offset := m.ScrollOffset(layout)
	r := []rune(m.Model.Value())
	rows := strings.Split(view, "\n")
	for y := range rows {
		if y+offset >= len(layout) {
			break
		}
		row := layout[y+offset]
		start, end := max(a, row.Start), min(b, row.End)
		newline := b > row.End && a <= row.End && row.End < len(r) && r[row.End] == '\n'
		if start >= end && !newline {
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
		rows[y] = ansi.Cut(rows[y], 0, left) + indexedBackground(ansi.Cut(rows[y], left, right), m.SelectionColor) + ansi.Cut(rows[y], right, ansi.StringWidth(rows[y]))
	}
	return strings.Join(rows, "\n")
}

// Scroll moves the view by rows under the wheel. The widget has no
// scroll of its own -- its view follows the cursor -- so the cursor is what
// moves, which also keeps the scrollbar, the selection and the cursor reading
// one position rather than three. A draft that fits has nothing to scroll, and
// says so, rather than swallowing the event.
func (m *Model) Scroll(down bool) bool {
	rows := m.Rows()
	if len(rows) <= m.Model.Height() {
		return false
	}
	step := -wheelRows
	if down {
		step = wheelRows
	}
	pos := Position(*m)
	from := RowIndex(rows, pos)
	to := max(0, min(len(rows)-1, from+step))
	if to == from {
		return false
	}
	m.selection = nil

	// Hold the column where there is one to hold, the way an arrow key does.
	m.SetPosition(min(rows[to].Start+pos-rows[from].Start, RowEnd(rows, to)))

	return true
}

const wheelRows = 3

// RenderScrollbar marks where the draft is when it stops fitting. The composer
// grows to maxComposerRows and then holds, so past that the rest of a long
// message is off screen with nothing on screen to say so. The bar reads the same
// layout the selection and the cursor read, so it cannot disagree with them
// about where the draft is.
func (m *Model) RenderScrollbar(view string, width int) string {
	rows := strings.Split(view, "\n")
	layout := m.Rows()
	if len(rows) == 0 || width < 2 {
		return view
	}
	// The column is always there; a bar is in it only when the draft runs past
	// what the composer shows. Reserving it means the text never rewraps as the
	// bar arrives, which is what made adding a line feel like a stutter.
	start, size := 0, 0
	if len(layout) > len(rows) {
		offset := min(m.ScrollOffset(layout), len(layout)-len(rows))
		visible, total := len(rows), len(layout)
		size = max(1, visible*visible/total)
		span := max(1, total-visible)
		start = min(visible-size, (offset*(visible-size)+span/2)/span)
	}
	width = max(0, width-1)
	for y, row := range rows {
		bar := " "
		switch {
		case size == 0:
		case y >= start && y < start+size:
			bar = m.ScrollbarStyle.Render("│")
		default:
			bar = m.ScrollbarTrackStyle.Render("│")
		}
		text := ansi.Cut(row, 0, width)
		rows[y] = text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text))) + bar
	}
	return strings.Join(rows, "\n")
}
