package bed

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"strings"
)

// FollowCursor returns a manually scrolled viewport to the editing cursor.
// Keyboard editing/navigation calls this automatically; wheel scrolling does not.
func (m *Model) FollowCursor() {
	if m.detached {
		m.Model, _ = m.Model.Update(nil)
	}
	m.detached = false
}

// RawView renders the viewport before bed's decorations. Hosts composing their
// own rendering pipeline must use this instead of the embedded textarea.View.
func (m Model) RawView() string {
	if !m.detached || m.viewDocument != m.DocumentKey {
		return m.Model.View()
	}
	rows := m.Rows()
	top := m.ScrollOffset(rows)
	// A fresh viewport is essential: textarea copies share a viewport pointer.
	// Rendering a probe must not reposition the live cursor or cancel its timer.
	probe := textarea.New()
	probe.CharLimit = 0
	probe.MaxWidth = 0
	probe.MaxHeight = 0
	probe.FocusedStyle = m.FocusedStyle
	probe.BlurredStyle = m.BlurredStyle
	probe.Placeholder = m.Placeholder
	probe.EndOfBufferCharacter = m.EndOfBufferCharacter
	probe.SetPromptFunc(m.GutterWidth, func(int) string { return strings.Repeat(" ", m.GutterWidth) })
	probe.SetWidth(m.Width() + m.GutterWidth + probe.FocusedStyle.Base.GetHorizontalFrameSize())
	probe.SetHeight(len(rows) + m.Height())
	probe.SetValue(m.Value())
	probe.Cursor.SetMode(cursor.CursorStatic)
	if m.Focused() {
		probe.Focus()
	} else {
		probe.Blur()
	}
	for probe.Line() > m.Line() {
		probe.CursorStart()
		probe.CursorUp()
	}
	probe.SetCursor(m.LineInfo().StartColumn + m.LineInfo().ColumnOffset)
	probe.Cursor.Style = m.Cursor.Style
	probe.Cursor.Blink = m.Cursor.Blink
	rendered := strings.Split(probe.View(), "\n")
	return strings.Join(rendered[top:min(len(rendered), top+m.Height())], "\n")
}
