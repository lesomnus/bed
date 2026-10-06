package bed

import (
	"github.com/charmbracelet/bubbles/cursor"
	"testing"
	"time"
)

func TestLayoutPreservesCursorTimer(t *testing.T) {
	m := New()
	m.SetValue("hello\nworld")
	m.SetWidth(20)
	m.SetHeight(3)
	m.Cursor.BlinkSpeed = time.Millisecond
	cmd := m.Focus()
	for range 3 {
		m.View()
		m.ScrollOffset(m.Rows())
		m.Point(2, 1)
	}
	msg := cmd()
	if _, ok := msg.(cursor.BlinkMsg); !ok {
		t.Fatalf("layout canceled live cursor: %T", msg)
	}
	updated, command := m.UpdateText(msg)
	if !updated.Cursor.Blink || command == nil {
		t.Fatal("cursor chain stopped")
	}
}
