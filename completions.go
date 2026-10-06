package bed

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) featureKey(k tea.KeyMsg) (bool, tea.Cmd) {
	if k.Paste {
		return false, nil
	}
	if key.Matches(k, m.EditorKeys.Complete) {
		return true, m.RequestCompletions(context.Background())
	}
	if key.Matches(k, m.EditorKeys.RequestGhost) {
		return true, m.RequestGhost(context.Background())
	}
	if c := m.completion; c != nil && m.current(c.request) {
		if key.Matches(k, m.EditorKeys.CompletionDismiss) {
			m.DismissCompletions()
			return true, nil
		}
		if !c.loading {
			if key.Matches(k, m.EditorKeys.CompletionAccept) {
				m.AcceptCompletion()
				return true, nil
			}
			cols, _, rows, _ := m.completionGrid()
			step := 0
			switch {
			case key.Matches(k, m.EditorKeys.CompletionNext):
				step = cols
			case key.Matches(k, m.EditorKeys.CompletionPrevious):
				step = -cols
			case cols > 1 && key.Matches(k, m.EditorKeys.CompletionRight):
				step = 1
			case cols > 1 && key.Matches(k, m.EditorKeys.CompletionLeft):
				step = -1
			case key.Matches(k, m.EditorKeys.CompletionPageNext):
				step = cols * rows
			case key.Matches(k, m.EditorKeys.CompletionPagePrevious):
				step = -cols * rows
			}
			if step != 0 {
				c.selected = max(0, min(len(c.result.Items)-1, c.selected+step))
				return true, nil
			}
		}
	}
	if g := m.ghost; g != nil && m.current(g.request) {
		if key.Matches(k, m.EditorKeys.GhostDismiss) {
			m.DismissGhost()
			return true, nil
		}
		if !g.loading && key.Matches(k, m.EditorKeys.GhostAccept) {
			m.AcceptGhost()
			return true, nil
		}
	}
	return false, nil
}

// CompletionColumns is an upper bound; narrow editors reduce the column count.
func (m *Model) completionGrid() (cols, cell, rows, top int) {
	count := 0
	if m.completion != nil {
		count = len(m.completion.result.Items)
	}
	cols = max(1, min(max(1, m.CompletionColumns), max(1, m.Width()/12)))
	cell = max(1, m.Width()/cols)
	rows = max(1, min(max(1, m.CompletionRows), m.Height(), (count+cols-1)/cols))
	top = m.Height() - rows
	return
}
func (m *Model) RenderCompletions(view string) string {
	c := m.completion
	if c == nil || !m.current(c.request) {
		return view
	}
	lines := strings.Split(view, "\n")
	cols, cell, rows, top := m.completionGrid()
	if c.loading {
		if top < len(lines) {
			lines[top] = m.CompletionStyle.Render(ansi.Truncate("Loading"+strings.Repeat(".", 1+m.featureFrame%3), m.Width(), ""))
		}
		return strings.Join(lines, "\n")
	}
	if len(c.result.Items) == 0 {
		if top < len(lines) {
			lines[top] = m.CompletionStyle.Render(ansi.Truncate("No matches", m.Width(), ""))
		}
		return strings.Join(lines, "\n")
	}
	page := c.selected / (cols * rows) * (cols * rows)
	for y := 0; y < rows && top+y < len(lines); y++ {
		var row strings.Builder
		for x := 0; x < cols; x++ {
			i := page + y*cols + x
			text := ""
			style := m.CompletionStyle
			if i < len(c.result.Items) {
				item := c.result.Items[i]
				label := item.Label
				if label == "" {
					label = item.InsertText
				}
				text = "  " + displayText(label)
				if item.Detail != "" {
					text += " · " + displayText(item.Detail)
				}
				if i == c.selected {
					text = "› " + strings.TrimPrefix(text, "  ")
					style = m.CompletionSelectedStyle
				}
			}
			text = ansi.Truncate(text, cell, "…")
			text += strings.Repeat(" ", max(0, cell-ansi.StringWidth(text)))
			row.WriteString(style.Render(text))
		}
		lines[top+y] = row.String() + strings.Repeat(" ", max(0, m.Width()-cell*cols))
	}
	return strings.Join(lines, "\n")
}

// CompletionMouse handles widget-relative coordinates before text-area hit testing.
func (m *Model) CompletionMouse(v tea.MouseMsg) bool {
	c := m.completion
	if c == nil || c.loading || !m.current(c.request) {
		return false
	}
	cols, cell, rows, top := m.completionGrid()
	if v.X < 0 || v.X >= cols*cell || v.Y < top || v.Y >= top+rows {
		return false
	}
	if len(c.result.Items) == 0 {
		return true
	}
	if v.Button == tea.MouseButtonWheelUp {
		c.selected = max(0, c.selected-cols)
		return true
	}
	if v.Button == tea.MouseButtonWheelDown {
		c.selected = min(len(c.result.Items)-1, c.selected+cols)
		return true
	}
	i := c.selected/(cols*rows)*(cols*rows) + (v.Y-top)*cols + v.X/cell
	if i >= len(c.result.Items) {
		return true
	}
	if v.Action == tea.MouseActionMotion {
		c.selected = i
		return true
	}
	if v.Action == tea.MouseActionPress && v.Button == tea.MouseButtonLeft {
		c.selected = i
		m.AcceptCompletion()
		return true
	}
	return true
}
func (m *Model) afterInput(msg tea.Msg, hadCompletion bool) tea.Cmd {
	k, ok := msg.(tea.KeyMsg)
	if !ok || !m.Focused() || m.CompletionProvider == nil {
		return nil
	}
	if !(k.Type == tea.KeyRunes || k.Type == tea.KeySpace || key.Matches(k, m.KeyMap.DeleteCharacterBackward, m.KeyMap.DeleteCharacterForward)) {
		return nil
	}
	if hadCompletion {
		return m.RequestCompletions(context.Background())
	}
	if !k.Paste && len(k.Runes) > 0 && strings.ContainsRune(m.CompletionTriggers, k.Runes[len(k.Runes)-1]) {
		return m.RequestCompletions(context.Background())
	}
	return nil
}

// AutoHeight returns a bounded suggested height after wrapping at the current width.
func (m *Model) AutoHeight(minimum, maximum int) int {
	return max(1, min(max(1, maximum), max(minimum, len(m.Rows()))))
}

// WordSeparators adds application-specific boundaries to word navigation (e.g.
// "/\\" for paths). Word deletion uses the same boundaries through bed.
func (m *Model) deleteWord(forward bool) {
	pos := Position(*m)
	m.moveWord(forward)
	other := Position(*m)
	_ = m.replaceRange(min(pos, other), max(pos, other), "")
}
