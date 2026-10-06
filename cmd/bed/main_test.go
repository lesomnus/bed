package main

import (
	"bytes"
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(p, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestFileRoundTripAndSave(t *testing.T) {
	for _, body := range []string{"", "one\n\ttwo\n", "한글\r\n\tlast\r\n", "no trailing newline", strings.Repeat("line\n", 150)} {
		t.Run(strings.TrimSpace(body[:min(10, len(body))]), func(t *testing.T) {
			path := fixture(t, body)
			m, err := newApp(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(m.document.decode(m.editor.Value()), []byte(body)) {
				t.Fatal("load changed content")
			}
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
			if next.(app).status != "Saved" {
				t.Fatal(next.(app).status)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != body {
				t.Fatalf("save changed content: %q %v", got, err)
			}
			if runtime.GOOS != "windows" {
				info, _ := os.Stat(path)
				if info.Mode().Perm() != 0o640 {
					t.Fatal("permissions changed")
				}
			}
		})
	}
}
func TestSaveAndQuitBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")
	m, err := newApp(path)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	m = next.(app)
	if !m.dirty() || m.editor.Value() != "hello" {
		t.Fatal("typing failed")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = next.(app)
	if cmd != nil || !m.quitArmed {
		t.Fatal("dirty quit discarded immediately")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(app)
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "hello" || m.dirty() {
		t.Fatalf("save failed %q %v", body, err)
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("clean quit ignored")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("not a quit")
	}
}
func TestDirtyQuitRequiresSecondPress(t *testing.T) {
	m, err := newApp(fixture(t, "original"))
	if err != nil {
		t.Fatal(err)
	}
	m.editor.SetValue("changed")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m = next.(app)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	if cmd == nil {
		t.Fatal("second quit ignored")
	}
	data, _ := os.ReadFile(m.document.path)
	if string(data) != "original" {
		t.Fatal("quit wrote unsaved text")
	}
}
func TestSaveConflictPreservesExternalChanges(t *testing.T) {
	path := fixture(t, "initial")
	m, err := newApp(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.editor.SetValue("mine")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(app)
	data, _ := os.ReadFile(path)
	if string(data) != "external" || !m.dirty() || !strings.Contains(m.status, "Save failed") {
		t.Fatal("conflict not protected")
	}
}
func TestTabsOnInputAndCRLFPaste(t *testing.T) {
	m, err := newApp(fixture(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(app)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a\r\n\tb"), Paste: true})
	m = next.(app)
	if got := string(m.document.decode(m.editor.Value())); got != "\ta\n\tb" {
		t.Fatalf("lost tabs: %q", got)
	}
}
func TestUnsupportedInputDoesNotWrite(t *testing.T) {
	for _, body := range []string{"binary\x00", "\xff", "mixed\r\nand\n", strings.Repeat("x\n", 10001)} {
		path := fixture(t, body)
		if _, err := newApp(path); err == nil {
			t.Fatal("unsupported document accepted")
		}
		data, _ := os.ReadFile(path)
		if string(data) != body {
			t.Fatal("opening modified file")
		}
	}
}
func TestSaveFailureKeepsBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "file.txt")
	m, err := newApp(path)
	if err != nil {
		t.Fatal(err)
	}
	m.editor.SetValue("keep")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(app)
	if !m.dirty() || m.editor.Value() != "keep" || !strings.Contains(m.status, "Save failed") {
		t.Fatal("failed save lost buffer")
	}
}
