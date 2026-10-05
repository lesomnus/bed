package bed

import "testing"

func TestComposerCellReplacementPreservesCursorSGR(t *testing.T) {
	input := "❯ a\x1b[7m \x1b[27m한글 "
	got := replaceCells(input, map[int]string{3: "·", 8: "↵"})
	if got != "❯ a\x1b[7m·\x1b[27m한글↵" {
		t.Fatalf("lost cursor/style/cell alignment: %q", got)
	}
}
