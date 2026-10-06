package bed

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func candidates() CompletionResult {
	return CompletionResult{From: 0, To: 2, Items: []CompletionItem{{Label: "seal", InsertText: "@seal "}, {Label: "bird", InsertText: "@bird "}, {Label: "fox", InsertText: "@fox "}}}
}
func TestCompletionGridKeyboardMouseAndUndo(t *testing.T) {
	m := testEditor()
	m.SetWidth(40)
	m.SetValue("@s")
	m.SetPosition(2)
	m.CompletionColumns = 2
	if !m.SetCompletions(candidates()) {
		t.Fatal("no completions")
	}
	if !strings.Contains(ansi.Strip(m.View()), "seal") {
		t.Fatal("popup not rendered")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.Value() != "@bird " {
		t.Fatal("wrong completion", m.Value())
	}
	m.Undo()
	if m.Value() != "@s" {
		t.Fatal("completion not one edit")
	}
	m.SetPosition(2)
	m.SetCompletions(candidates())
	_, cell, _, top := m.completionGrid()
	m, _ = m.Update(tea.MouseMsg{X: cell + 1, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.Value() != "@bird " {
		t.Fatal("mouse completion failed")
	}
}
func TestGhostIsPreviewUntilAccepted(t *testing.T) {
	m := testEditor()
	m.SetValue("hello ")
	m.SetPosition(6)
	if !m.SetGhost("world\nnext") {
		t.Fatal("ghost rejected")
	}
	if m.Value() != "hello " || !strings.Contains(ansi.Strip(m.View()), "world") {
		t.Fatal("ghost not virtual")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.Value() != "hello world\nnext" {
		t.Fatal("accept failed")
	}
	m.Undo()
	if m.Value() != "hello " {
		t.Fatal("ghost accept not undoable")
	}
	m.SetGhost("discard")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.ghost != nil || strings.Contains(m.View(), "discard") {
		t.Fatal("typing retained ghost")
	}
}
func TestProviderCancellationStaleResultsAndIsolation(t *testing.T) {
	m := testEditor()
	m.SetValue("@s")
	m.SetPosition(2)
	var ctx context.Context
	m.CompletionProvider = func(c context.Context, r Request) (CompletionResult, error) { ctx = c; return candidates(), nil }
	cmd := m.RequestCompletions(context.Background())
	batch := cmd().(tea.BatchMsg)
	response := batch[0]()
	request := m.completion.request
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if ctx.Err() == nil {
		t.Fatal("obsolete request not cancelled")
	}
	m, _ = m.Update(response)
	if m.completion != nil && !m.completion.loading {
		t.Fatal("stale response accepted")
	}
	other := testEditor()
	other.SetValue("@s")
	other.SetPosition(2)
	other.SetCompletions(candidates())
	if other.completion.request.ID == request.ID {
		t.Fatal("request IDs collide between editors")
	}
	before := other.completion.request.ID
	other, _ = other.Update(response)
	if other.completion.request.ID != before {
		t.Fatal("foreign response installed")
	}
	m.Close()
	other.Close()
}
func TestGhostProviderLoadingAndLateResponse(t *testing.T) {
	m := testEditor()
	m.GhostProvider = func(context.Context, Request) (string, error) { return "answer", nil }
	cmd := m.RequestGhost(context.Background())
	if !strings.Contains(m.View(), "Loading") {
		t.Fatal("missing loading state")
	}
	response := cmd().(tea.BatchMsg)[0]()
	m, _ = m.Update(response)
	if !m.AcceptGhost() || m.Value() != "answer" {
		t.Fatal("provider ghost failed")
	}
	m.SetValue("")
	cmd = m.RequestGhost(context.Background())
	response = cmd().(tea.BatchMsg)[0]()
	m.SetValue("other")
	m, _ = m.Update(response)
	if m.ghost != nil {
		t.Fatal("ghost crossed documents")
	}
}
func TestCompletionTriggerAndCustomBindings(t *testing.T) {
	m := testEditor()
	m.CompletionTriggers = "@"
	m.CompletionProvider = func(_ context.Context, r Request) (CompletionResult, error) {
		return CompletionResult{From: 0, To: r.Cursor, Items: []CompletionItem{{Label: "seal", InsertText: "@seal"}}}, nil
	}
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	if m.completion == nil || cmd == nil {
		t.Fatal("trigger did not request candidates")
	}
	m.Close()
	m.SetValue("@s")
	m.SetCompletions(candidates())
	m.EditorKeys.CompletionAccept.SetKeys("f6")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if m.Value() != "@seal " {
		t.Fatal("binding ignored")
	}
}
func TestPathBoundariesAndAutoHeight(t *testing.T) {
	m := testEditor()
	m.WordSeparators = "/\\"
	m.SetValue("/some/path/file")
	m.SetPosition(len(m.Value()))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if Position(m) != 11 {
		t.Fatal("custom boundary ignored", Position(m))
	}
	if h := m.AutoHeight(2, 5); h < 2 || h > 5 {
		t.Fatal("height outside bounds")
	}
}

func TestStaleLoadingTickDoesNotMultiplyTimers(t *testing.T) {
	m := testEditor()
	m.GhostProvider = func(context.Context, Request) (string, error) { return "x", nil }
	m.RequestGhost(context.Background())
	old := m.ghost.request.ID
	m.RequestGhost(context.Background())
	_, cmd := m.receiveFeature(featureTick{old})
	if cmd != nil {
		t.Fatal("obsolete loading timer rescheduled")
	}
	_, cmd = m.receiveFeature(featureTick{m.ghost.request.ID})
	if cmd == nil {
		t.Fatal("active loading animation stopped")
	}
	m.Close()
}
func TestCompletionResizePagingAndEscape(t *testing.T) {
	m := testEditor()
	m.SetValue("@s")
	m.SetPosition(2)
	m.CompletionColumns = 3
	m.CompletionRows = 2
	result := candidates()
	for range 10 {
		result.Items = append(result.Items, result.Items[0])
	}
	m.SetCompletions(result)
	m.SetWidth(12)
	m.SetHeight(2)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.completion.selected != 2 {
		t.Fatal("narrow paging used stale grid")
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if ansi.StringWidth(line) > m.Width() {
			t.Fatal("popup exceeded width")
		}
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.completion != nil || m.Value() != "@s" {
		t.Fatal("dismiss changed text")
	}
}
func TestProviderResultIsOwnedAndInvalidRangesRejected(t *testing.T) {
	m := testEditor()
	m.SetValue("@s")
	m.SetPosition(2)
	result := candidates()
	m.SetCompletions(result)
	result.Items[0].InsertText = "mutated"
	m.AcceptCompletion()
	if m.Value() != "@seal " {
		t.Fatal("caller mutation changed completion")
	}
	if m.SetCompletions(CompletionResult{From: -1, Items: []CompletionItem{{Label: "bad"}}}) {
		t.Fatal("invalid range accepted")
	}
}
func TestFeatureDocumentIsolationAndDisabledBindings(t *testing.T) {
	m := testEditor()
	m.SetGhost("secret")
	m.DocumentKey = "other"
	if strings.Contains(m.View(), "secret") {
		t.Fatal("ghost crossed documents")
	}
	m.SetValue("@s")
	m.SetPosition(2)
	m.SetCompletions(candidates())
	m.EditorKeys.CompletionAccept.SetEnabled(false)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.Value() == "@seal " {
		t.Fatal("disabled accept still worked")
	}
	m.Close()
}

func TestEmptyCompletionsRemainRefreshable(t *testing.T) {
	m := testEditor()
	m.SetValue("@z")
	m.SetPosition(2)
	m.CompletionProvider = func(_ context.Context, r Request) (CompletionResult, error) {
		return CompletionResult{From: 0, To: r.Cursor}, nil
	}
	if !m.SetCompletions(CompletionResult{From: 0, To: 2}) {
		t.Fatal("empty result rejected")
	}
	if !strings.Contains(m.View(), "No matches") {
		t.Fatal("empty result not rendered")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.completion == nil || !m.completion.loading {
		t.Fatal("backspace did not refresh empty result")
	}
	m.Close()
}

func TestControlSpaceTerminalEncoding(t *testing.T) {
	m := testEditor()
	m.CompletionProvider = func(context.Context, Request) (CompletionResult, error) { return CompletionResult{}, nil }
	// Ctrl+Space is encoded as NUL (ctrl+@) by traditional terminals.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt})
	if m.completion == nil || !m.completion.loading {
		t.Fatal("terminal Ctrl+Space not recognized")
	}
	m.Close()
}
func TestBlurCancelsProviders(t *testing.T) {
	m := testEditor()
	var ctx context.Context
	m.GhostProvider = func(c context.Context, r Request) (string, error) { ctx = c; return "x", nil }
	cmd := m.RequestGhost(context.Background())
	_ = cmd().(tea.BatchMsg)[0]()
	m.Blur()
	if ctx.Err() == nil || m.ghost != nil {
		t.Fatal("blur did not cancel provider")
	}
}
