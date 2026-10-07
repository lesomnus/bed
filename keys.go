package bed

import "github.com/charmbracelet/bubbles/key"

// EditorKeyMap configures bed's additional editing actions. Base textarea
// actions remain configurable through Model.KeyMap. A zero map disables these
// actions; individual bindings support SetKeys and SetEnabled.
type EditorKeyMap struct {
	AddCursorAbove, AddCursorBelow, ClearCursors                        key.Binding
	Indent, Outdent, MoveLinesUp, MoveLinesDown, Duplicate              key.Binding
	ChipActivate, ChipExpand                                            key.Binding
	Complete, CompletionAccept, CompletionDismiss                       key.Binding
	CompletionNext, CompletionPrevious, CompletionLeft, CompletionRight key.Binding
	CompletionPageNext, CompletionPagePrevious                          key.Binding
	RequestGhost, GhostAccept, GhostDismiss                             key.Binding
	Undo, Redo                                                          key.Binding
	SelectLeft, SelectRight, SelectUp, SelectDown                       key.Binding
	SelectWordLeft, SelectWordRight                                     key.Binding
	SelectRowStart, SelectRowEnd                                        key.Binding
	RowStart, RowEnd                                                    key.Binding
	Cut, ToggleWhitespace                                               key.Binding
}

// DefaultEditorKeyMap returns fresh default bindings for an editor instance.
func DefaultEditorKeyMap() EditorKeyMap {
	bind := func(s string) key.Binding { return key.NewBinding(key.WithKeys(s)) }
	return EditorKeyMap{
		AddCursorAbove: key.NewBinding(key.WithKeys("alt+ctrl+up", "ctrl+alt+up")), AddCursorBelow: key.NewBinding(key.WithKeys("alt+ctrl+down", "ctrl+alt+down")), ClearCursors: bind("esc"),
		Indent: bind("tab"), Outdent: bind("shift+tab"), MoveLinesUp: bind("alt+up"), MoveLinesDown: bind("alt+down"), Duplicate: bind("ctrl+d"),
		ChipActivate: bind("enter"), ChipExpand: bind("alt+enter"),
		Complete: key.NewBinding(key.WithKeys("ctrl+space", "ctrl+@")), CompletionAccept: key.NewBinding(key.WithKeys("enter", "tab")), CompletionDismiss: bind("esc"),
		CompletionNext: bind("down"), CompletionPrevious: bind("up"), CompletionLeft: bind("left"), CompletionRight: bind("right"),
		CompletionPageNext: bind("pgdown"), CompletionPagePrevious: bind("pgup"),
		RequestGhost: bind("ctrl+g"), GhostAccept: bind("right"), GhostDismiss: bind("esc"),
		Undo: bind("ctrl+z"), Redo: key.NewBinding(key.WithKeys("ctrl+y", "ctrl+shift+z")),
		SelectLeft: bind("shift+left"), SelectRight: bind("shift+right"),
		SelectUp: bind("shift+up"), SelectDown: bind("shift+down"),
		SelectWordLeft: bind("ctrl+shift+left"), SelectWordRight: bind("ctrl+shift+right"),
		SelectRowStart: bind("shift+home"), SelectRowEnd: bind("shift+end"),
		RowStart: bind("home"), RowEnd: bind("end"), Cut: bind("ctrl+x"), ToggleWhitespace: bind("alt+w"),
	}
}
