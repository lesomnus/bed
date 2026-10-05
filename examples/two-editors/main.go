// Two independent editors with application-owned focus and submit handling.
package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lesomnus/bed"
	"os"
	"strings"
)

type app struct {
	editors [2]bed.Model
	active  int
	copied  string
}

func (m app) Init() tea.Cmd { return nil }
func (m app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyMsg:
		switch v.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.editors[m.active].Blur()
			m.active = 1 - m.active
			return m, m.editors[m.active].Focus()
		}
	case tea.WindowSizeMsg:
		for i := range m.editors {
			m.editors[i].SetWidth(max(10, v.Width-2))
		}
		return m, nil
	case bed.CopyMsg:
		m.copied = string(v)
		return m, nil
	case tea.MouseMsg:
		// Each editor occupies four rows after its one-row heading.
		target := -1
		top := 0
		if v.Y >= 1 && v.Y < 5 {
			target = 0
			top = 1
		}
		if v.Y >= 6 && v.Y < 10 {
			target = 1
			top = 6
		}
		if target < 0 {
			return m, nil
		}
		if target != m.active {
			m.editors[m.active].Blur()
			m.active = target
			m.editors[target].Focus()
		}
		v.Y -= top
		msg = v
	}
	var cmd tea.Cmd
	m.editors[m.active], cmd = m.editors[m.active].Update(msg)
	return m, cmd
}
func (m app) View() string {
	return "Notes\n" + m.editors[0].View() + "\nDetails\n" + m.editors[1].View() + "\nTab: switch · Alt+W: whitespace · Ctrl+C: quit\nCopied: " + strings.ReplaceAll(m.copied, "\n", " ")
}
func main() {
	m := app{}
	for i := range m.editors {
		m.editors[i] = bed.New()
		m.editors[i].SetWidth(60)
		m.editors[i].SetHeight(4)
		m.editors[i].CharLimit = 0
	}
	m.editors[0].SetValue("Select text with Shift+arrows or drag the mouse.")
	m.editors[1].SetValue("An independent document.\nEnter inserts a newline.")
	m.editors[0].Focus()
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
