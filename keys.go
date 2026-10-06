package bed

import "github.com/charmbracelet/bubbles/key"

// EditorKeyMap configures bed's additional editing actions. Base textarea
// actions remain configurable through Model.KeyMap. A zero map disables these
// actions; individual bindings support SetKeys and SetEnabled.
type EditorKeyMap struct {
	Undo, Redo                                    key.Binding
	SelectLeft, SelectRight, SelectUp, SelectDown key.Binding
	SelectWordLeft, SelectWordRight               key.Binding
	SelectRowStart, SelectRowEnd                  key.Binding
	RowStart, RowEnd                              key.Binding
	Cut, ToggleWhitespace                         key.Binding
}

// DefaultEditorKeyMap returns fresh default bindings for an editor instance.
func DefaultEditorKeyMap() EditorKeyMap {
	bind := func(s string) key.Binding { return key.NewBinding(key.WithKeys(s)) }
	return EditorKeyMap{
		Undo: bind("ctrl+z"), Redo: key.NewBinding(key.WithKeys("ctrl+y", "ctrl+shift+z")),
		SelectLeft: bind("shift+left"), SelectRight: bind("shift+right"),
		SelectUp: bind("shift+up"), SelectDown: bind("shift+down"),
		SelectWordLeft: bind("ctrl+shift+left"), SelectWordRight: bind("ctrl+shift+right"),
		SelectRowStart: bind("shift+home"), SelectRowEnd: bind("shift+end"),
		RowStart: bind("home"), RowEnd: bind("end"), Cut: bind("ctrl+x"), ToggleWhitespace: bind("alt+w"),
	}
}
