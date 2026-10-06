// An offline demo of application-owned providers and chip payloads.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/lesomnus/bed"
)

type app struct {
	editor bed.Model
	status string
	chips  int
	width  int
}

func (m app) Init() tea.Cmd { return m.editor.Focus() }
func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(12, v.Width)
		m.editor.SetWidth(m.width)
		m.editor.SetHeight(max(3, v.Height-3))
		return m, nil
	case tea.KeyMsg:
		if !v.Paste {
			switch v.String() {
			case "ctrl+q":
				m.editor.Close()
				return m, tea.Quit
			case "f2":
				m.chips++
				err := m.editor.InsertChip(bed.Chip{ID: fmt.Sprint(m.chips), Label: fmt.Sprintf("[Note %d]", m.chips), Text: "This text belongs to the application.\nIt can replace the chip."})
				if err != nil {
					m.status = err.Error()
				}
				return m, nil
			}
		}
	case bed.ChipActivateMsg:
		m.status = "Preview: " + strings.ReplaceAll(v.Chip.Text, "\n", " / ")
		return m, nil
	case bed.FeatureErrorMsg:
		m.status = v.Err.Error()
		return m, nil
	case bed.CopyMsg:
		m.status = "Cut: " + string(v)
		return m, nil
	case tea.MouseMsg:
		v.Y--
		msg = v
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}
func (m app) View() string {
	return "bed extensions · offline providers\n" + m.editor.View() + "\n" + ansi.Truncate("F2 chip · @ completion · Ctrl+G ghost · Ctrl+Z undo · Ctrl+Q quit", m.width, "…") + "\n" + ansi.Truncate(m.status, m.width, "…")
}
func main() {
	e := bed.New()
	e.SetWidth(80)
	e.SetHeight(12)
	e.MaxWidth = 0
	e.MaxHeight = 0
	e.Focus()
	e.CompletionColumns = 3
	e.CompletionTriggers = "@"
	e.CompletionProvider = func(ctx context.Context, r bed.Request) (bed.CompletionResult, error) {
		select {
		case <-ctx.Done():
			return bed.CompletionResult{}, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
		prefix := string([]rune(r.Text)[:r.Cursor])
		start := strings.LastIndex(prefix, "@")
		if start < 0 {
			return bed.CompletionResult{}, nil
		}
		result := bed.CompletionResult{From: utf8.RuneCountInString(prefix[:start]), To: r.Cursor}
		query := prefix[start+1:]
		for _, name := range []string{"seal", "bird", "fox", "otter", "owl", "wolf"} {
			if strings.HasPrefix(name, query) {
				result.Items = append(result.Items, bed.CompletionItem{Label: name, Detail: "demo", InsertText: "@" + name + " "})
			}
		}
		return result, nil
	}
	e.GhostProvider = func(ctx context.Context, r bed.Request) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		return "Try the next step.", nil
	}
	if _, err := tea.NewProgram(app{editor: e, width: 80}, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
