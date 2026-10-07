package bed

import (
	"context"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Request identifies an exact document/cursor snapshot. Providers must honor ctx
// cancellation; responses are also rejected when their request is no longer current.
type Request struct {
	ID             uint64
	Document, Text string
	Cursor         int
	Revision       uint64
	Selections     []Selection
}
type CompletionItem struct{ Label, Detail, InsertText string }
type CompletionResult struct {
	From, To int
	Items    []CompletionItem
}
type CompletionProvider func(context.Context, Request) (CompletionResult, error)
type GhostProvider func(context.Context, Request) (string, error)
type FeatureErrorMsg struct {
	Feature string
	Err     error
}
type completionResponse struct {
	request Request
	result  CompletionResult
	err     error
}
type ghostResponse struct {
	request Request
	text    string
	err     error
}
type featureTick struct{ id uint64 }
type completionState struct {
	request  Request
	result   CompletionResult
	selected int
	loading  bool
	cancel   context.CancelFunc
}
type ghostState struct {
	request Request
	text    string
	loading bool
	cancel  context.CancelFunc
}

func (m *Model) syncFeatures() {
	value := m.Value()
	if m.completion != nil && !m.current(m.completion.request) {
		m.DismissCompletions()
	}
	if m.ghost != nil && !m.current(m.ghost.request) {
		m.DismissGhost()
	}
	if m.featureDocument != m.DocumentKey || m.featureText != value {
		m.DismissCompletions()
		m.DismissGhost()
		// Edits made through bed reconcile chips before this baseline changes.
		// Direct embedded textarea mutation cannot safely retain positional metadata.
		m.cursors = nil
		m.selectionRevision++
		m.chips = nil
		m.chipVersion++
		m.featureDocument = m.DocumentKey
		m.featureText = value
	}
}

var featureRequestID atomic.Uint64

func (m *Model) request() Request {
	m.syncFeatures()
	m.requestID = featureRequestID.Add(1)
	return Request{ID: m.requestID, Document: m.DocumentKey, Text: m.Value(), Cursor: Position(*m), Revision: m.selectionRevision, Selections: m.Selections()}
}
func (m *Model) current(r Request) bool {
	return r.Revision == m.selectionRevision && r.Document == m.DocumentKey && r.Text == m.Value() && r.Cursor == Position(*m)
}
func (m *Model) DismissCompletions() {
	if m.completion != nil && m.completion.cancel != nil {
		m.completion.cancel()
	}
	m.completion = nil
}
func (m *Model) DismissGhost() {
	if m.ghost != nil && m.ghost.cancel != nil {
		m.ghost.cancel()
	}
	m.ghost = nil
}

// Close cancels provider work when the host removes the editor.
func (m *Model) Close() { m.DismissCompletions(); m.DismissGhost() }
func tickFeature(id uint64) tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return featureTick{id} })
}
func (m *Model) RequestCompletions(ctx context.Context) tea.Cmd {
	if len(m.Selections()) > 1 || m.CompletionProvider == nil {
		return nil
	}
	r := m.request()
	m.DismissCompletions()
	m.DismissGhost()
	ctx, cancel := context.WithCancel(ctx)
	m.completion = &completionState{request: r, loading: true, cancel: cancel}
	provider := m.CompletionProvider
	return tea.Batch(func() tea.Msg { v, err := provider(ctx, r); return completionResponse{r, v, err} }, tickFeature(r.ID))
}
func (m *Model) RequestGhost(ctx context.Context) tea.Cmd {
	if len(m.Selections()) > 1 || m.GhostProvider == nil || Position(*m) != len([]rune(m.Value())) {
		return nil
	}
	r := m.request()
	m.DismissGhost()
	m.DismissCompletions()
	ctx, cancel := context.WithCancel(ctx)
	m.ghost = &ghostState{request: r, loading: true, cancel: cancel}
	provider := m.GhostProvider
	return tea.Batch(func() tea.Msg { v, err := provider(ctx, r); return ghostResponse{r, v, err} }, tickFeature(r.ID))
}

// SetCompletions supplies already available candidates; providers are optional.
func (m *Model) SetCompletions(result CompletionResult) bool {
	if len(m.Selections()) > 1 {
		return false
	}
	r := m.request()
	m.DismissCompletions()
	m.DismissGhost()
	return m.installCompletions(r, result)
}
func (m *Model) installCompletions(r Request, result CompletionResult) bool {
	if !m.current(r) || result.From < 0 || result.To < result.From || result.To > len([]rune(r.Text)) {
		return false
	}

	result.Items = append([]CompletionItem(nil), result.Items...)
	m.completion = &completionState{request: r, result: result}
	return true
}

// SetGhost previews insertion at the document end. No buffer changes occur until acceptance.
func (m *Model) SetGhost(text string) bool {
	if len(m.Selections()) > 1 {
		return false
	}
	r := m.request()
	m.DismissGhost()
	m.DismissCompletions()
	if text == "" || r.Cursor != len([]rune(r.Text)) {
		return false
	}
	m.ghost = &ghostState{request: r, text: text}
	return true
}
func (m *Model) AcceptGhost() bool {
	g := m.ghost
	if g == nil || g.loading || !m.current(g.request) {
		return false
	}
	text, pos := g.text, g.request.Cursor
	m.DismissGhost()
	return m.ReplaceRange(pos, pos, text) == nil
}
func (m *Model) AcceptCompletion() bool {
	c := m.completion
	if c == nil || c.loading || !m.current(c.request) || len(c.result.Items) == 0 {
		return false
	}
	item := c.result.Items[c.selected]
	a, b := c.result.From, c.result.To
	m.DismissCompletions()
	return m.ReplaceRange(a, b, item.InsertText) == nil
}
func (m *Model) receiveFeature(msg tea.Msg) (bool, tea.Cmd) {
	switch v := msg.(type) {
	case completionResponse:
		c := m.completion
		if c == nil || c.request.ID != v.request.ID || !m.current(v.request) {
			return true, nil
		}
		m.DismissCompletions()
		if v.err != nil {
			return true, func() tea.Msg { return FeatureErrorMsg{"completion", v.err} }
		}
		m.installCompletions(v.request, v.result)
		return true, nil
	case ghostResponse:
		g := m.ghost
		if g == nil || g.request.ID != v.request.ID || !m.current(v.request) {
			return true, nil
		}
		m.DismissGhost()
		if v.err != nil {
			return true, func() tea.Msg { return FeatureErrorMsg{"ghost", v.err} }
		}
		if v.text != "" && v.request.Cursor == len([]rune(v.request.Text)) {
			m.ghost = &ghostState{request: v.request, text: v.text}
		}
		return true, nil
	case featureTick:
		active := m.ghost != nil && m.ghost.loading && m.ghost.request.ID == v.id || m.completion != nil && m.completion.loading && m.completion.request.ID == v.id
		if active {
			m.featureFrame++
			return true, tickFeature(v.id)
		}
		return true, nil
	}
	return false, nil
}
func displayText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func overlayLine(base string, x, width int, text string) string {
	if x < 0 || x >= width {
		return base
	}
	text = ansi.Truncate(text, width-x, "")
	end := x + ansi.StringWidth(text)
	return ansi.Cut(base, 0, x) + text + ansi.Cut(base, end, width)
}
func (m *Model) RenderGhost(view string) string {
	g := m.ghost
	if g == nil || !m.current(g.request) || g.request.Cursor != len([]rune(m.Value())) {
		return view
	}
	text := g.text
	if g.loading {
		text = "Loading" + strings.Repeat(".", 1+m.featureFrame%3)
	}
	rows := strings.Split(view, "\n")
	layout := m.Rows()
	row := RowIndex(layout, g.request.Cursor)
	y := row - m.ScrollOffset(layout)
	if y < 0 || y >= len(rows) {
		return view
	}
	x := m.GutterWidth + ansi.StringWidth(string([]rune(m.Value())[layout[row].Start:g.request.Cursor]))
	// Preview never overwrites real text: currently restricted to document-end insertion.
	for i, line := range strings.Split(text, "\n") {
		if y+i >= len(rows) {
			break
		}
		left := m.GutterWidth
		if i == 0 {
			left = x
		}
		rows[y+i] = overlayLine(rows[y+i], left, m.Width(), m.GhostStyle.Render(displayText(line)))
	}
	return strings.Join(rows, "\n")
}
