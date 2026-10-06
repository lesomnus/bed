package bed

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// Decorate cells after layout: soft wraps are not source newlines, and visible
// whitespace must never change the draft, wrapping, chip labels or cursor SGR.
func (m *Model) RenderDisplay(view string) string {
	layout := m.Rows()
	offset := m.ScrollOffset(layout)
	rows := strings.Split(view, "\n")
	value := m.Model.Value()
	var source []rune
	var hidden []bool
	if m.ShowWhitespace {
		source = []rune(value)
		hidden = make([]bool, len(source))
		for _, c := range m.chips {
			for i := c.From; i < c.To && i < len(hidden); i++ {
				hidden[i] = true
			}
		}
		for _, token := range m.AtomicTokens {
			if token == "" {
				continue
			}
			for start := 0; start < len(value); {
				i := strings.Index(value[start:], token)
				if i < 0 {
					break
				}
				i += start
				a := utf8.RuneCountInString(value[:i])
				for j := a; j < a+utf8.RuneCountInString(token); j++ {
					hidden[j] = true
				}
				start = i + len(token)
			}
		}
	}
	for y := range rows {
		cells := map[int]string{}
		for x := 0; x < m.GutterWidth; x++ {
			cells[x] = " "
		}
		if index := y + offset; index < len(layout) {
			row := layout[index]
			if row.Column == 0 && m.Gutter != nil {
				x := 0
				for _, r := range m.Gutter(row.Line) {
					if x >= m.GutterWidth {
						break
					}
					cells[x] = string(r)
					x++
				}
			}
			if m.ShowWhitespace {
				x, pos := m.GutterWidth, row.Start
				g := uniseg.NewGraphemes(string(source[row.Start:row.End]))
				for g.Next() {
					text := g.Str()
					if text == " " && !hidden[pos] {
						cells[x] = "·"
					}
					x += g.Width()
					pos += utf8.RuneCountInString(text)
				}
				// Only real newline characters get a mark, never a soft wrap or EOF.
				if row.End < len(source) && source[row.End] == '\n' {
					cells[x] = "↵"
				}
			}
		}
		rows[y] = replaceCells(rows[y], cells)
	}
	return strings.Join(rows, "\n")
}

func replaceCells(text string, cells map[int]string) string {
	var out strings.Builder
	state := byte(0)
	column := 0
	for len(text) > 0 {
		seq, width, n, next := ansi.DecodeSequence(text, state, nil)
		if n == 0 {
			break
		}
		if replacement, ok := cells[column]; ok && width == 1 {
			out.WriteString(replacement)
		} else {
			out.WriteString(seq)
		}
		column += width
		text, state = text[n:], next
	}
	return out.String()
}
