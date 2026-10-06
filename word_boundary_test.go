package bed

import (
	tea "github.com/charmbracelet/bubbletea"
	"testing"
	"unicode/utf8"
)

func TestWordLeftAtWhitespaceOnlyPrefix(t *testing.T) {
	for _, text := range []string{"", "\n", "\n\n", "  \n  ", "\n\nword", "  first\n\nlast", "\n한글 단어\n"} {
		for _, key := range []tea.KeyType{tea.KeyCtrlLeft, tea.KeyCtrlShiftLeft} {
			t.Run(text+"/"+tea.KeyMsg{Type: key}.String(), func(t *testing.T) {
				m := testEditor()
				m.SetValue(text)
				m.SetPosition(utf8.RuneCountInString(text))
				for range utf8.RuneCountInString(text) + 3 {
					before := Position(m)
					m, _ = m.Update(tea.KeyMsg{Type: key})
					if after := Position(m); after < 0 || after > before || (before > 0 && after == before) {
						t.Fatalf("no bounded progress: %d -> %d", before, after)
					}
				}
				if Position(m) != 0 || m.Value() != text {
					t.Fatal("word movement changed text or missed start")
				}
				if key == tea.KeyCtrlShiftLeft && m.SelectedText() != text {
					t.Fatalf("selection %q want %q", m.SelectedText(), text)
				}
			})
		}
	}
}

func TestWordRightAtWhitespaceOnlySuffix(t *testing.T) {
	for _, text := range []string{"", "\n\n", "  \n ", "word\n\n", "한글\n \n"} {
		for _, key := range []tea.KeyType{tea.KeyCtrlRight, tea.KeyCtrlShiftRight} {
			m := testEditor()
			m.SetValue(text)
			m.SetPosition(0)
			end := utf8.RuneCountInString(text)
			for range end + 3 {
				before := Position(m)
				m, _ = m.Update(tea.KeyMsg{Type: key})
				after := Position(m)
				if after < before || after > end || (before < end && after == before) {
					t.Fatalf("no bounded progress: %d -> %d in %q", before, after, text)
				}
			}
			if Position(m) != end || m.Value() != text {
				t.Fatal("word movement changed text or missed end")
			}
			if key == tea.KeyCtrlShiftRight && m.SelectedText() != text {
				t.Fatalf("selection %q want %q", m.SelectedText(), text)
			}
		}
	}
}
