package bed

import (
	"time"
	"unicode"
)

type mouseClick struct {
	at              time.Time
	x, y, count     int
	value, document string
}

func (m *Model) clickCount(x, y int) int {
	now := time.Now()
	if m.mouseClock != nil {
		now = m.mouseClock()
	}
	last := m.click
	n := 1
	if m.MultiClickInterval > 0 && now.Sub(last.at) >= 0 && now.Sub(last.at) <= m.MultiClickInterval && last.x == x && last.y == y && last.value == m.Value() && last.document == m.DocumentKey {
		n = last.count%3 + 1
	}
	m.click = mouseClick{now, x, y, n, m.Value(), m.DocumentKey}
	return n
}
func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' {
		return 1
	}
	return 2
}
func (m *Model) mouseRange(pos, mode int) (int, int) {
	r := []rune(m.Value())
	pos = max(0, min(len(r), pos))
	a, b := pos, pos
	if mode == 3 {
		for a > 0 && r[a-1] != '\n' {
			a--
		}
		for b < len(r) && r[b] != '\n' {
			b++
		}
		if b < len(r) {
			b++
		}
	} else if mode == 2 && len(r) > 0 {
		if pos == len(r) {
			pos--
			a = pos
			b = pos
		}
		class := wordClass(r[pos])
		for a > 0 && r[a-1] != '\n' && wordClass(r[a-1]) == class {
			a--
		}
		for b < len(r) && r[b] != '\n' && wordClass(r[b]) == class {
			b++
		}
	}
	return expandedRange(m.chips, a, b)
}
