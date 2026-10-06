# bed

A Bubble Tea text editor extracted from [cxz](https://github.com/lesomnus/cxz).
Built on `bubbles/textarea`, with no dependency on cxz, its sessions, RPCs or AI providers.
Requires Go 1.24.2 or newer. The API is experimental.

## Use

```go
editor := bed.New()
editor.SetWidth(60)
editor.SetHeight(6)
editor.Focus()

// In the parent's Update:
editor, cmd = editor.Update(msg)

// In the parent's View:
return editor.View()
```

Run a complete example with two independent editors:

```sh
go run ./examples/two-editors
```

- Ctrl+Left/Right move by words (Alt+Left/Right and Alt+B/F also work).
- Shift+arrows and Ctrl+Shift+Left/Right extend selection.
- Home/End target visible wrapped rows; Shift extends selection to those edges.
- Mouse click moves the cursor; drag selects; wheel moves through long documents.
- Editing replaces the selection; Ctrl+X cuts and returns a `CopyMsg` via a command.
- Alt+W toggles visible spaces/newlines without modifying the text.
- Soft-wrapped rows do not repeat logical line numbers.
- CJK cell widths and grapheme boundaries are used for hit testing and horizontal selection.

The host handles `CopyMsg` to choose its clipboard transport. The host also owns
submission, focus, resize and global shortcuts. Intercept Enter before `Update`
when Enter should submit instead of inserting a newline. Enable Bubble Tea mouse
motion reporting for drag selection and translate mouse events to the editor's
origin before passing them to `Update`.

## Composition

`Model` embeds `textarea.Model`, exposing its styling and base key map. Use
`SetValue`/`Reset` on the bed model, and a new model for each independent editor.
Shallow copies have the same sharing limitations as textarea itself.

`DocumentKey` is an optional opaque identifier for invalidating selection when
an application switches documents while reusing the editor.

`AtomicTokens` lists application-owned labels whose partial selections expand to
the entire label. It does not store payloads or implement chip dialogs, token
creation, or every chip navigation/deletion policy; those remain host concerns.
Autocomplete candidate discovery, AI ghost text generation and application
commands are likewise outside this initial extraction.

`GutterWidth` and `Gutter` control logical line labels. If changing gutter width,
also configure textarea's `SetPromptFunc` with the same width. Gutter strings
must contain only single-cell characters. The default two-cell gutter cycles
single-digit line numbers, matching the compact source widget. Selection color
and scrollbar styles can be customized independently.

For existing applications with layered input dispatch/rendering:

- `HandleKey` handles selection and editor shortcuts; forward unhandled events
  with `UpdateText`. Forward the returned command to Bubble Tea as usual.
- `HandleMouse` takes coordinates relative to the **text area**, excluding the
  gutter. `Update` instead expects coordinates relative to the whole widget.
- `RenderDisplay` and `RenderSelection` decorate the embedded textarea's `View`,
  allowing the host to apply chip/ghost decorations in a controlled order.
- `RenderScrollbar(view, width)` renders a bar in the final column. Reserve one
  extra column in the parent layout so the bar does not overwrite text.
- `Position`, `SetPosition`, `Rows`, `Point` and selection bounds use rune offsets;
  mouse positions and widths use terminal cells.

The wheel currently moves the cursor because textarea's viewport follows it;
this is not an independent scrollback viewport. Avoid independently styling the
textarea with outer borders/padding: apply those around bed and translate mouse
coordinates in the parent.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
```

Originally extracted from cxz at commit
`5ac137a` (`internal/tui/composer_selection.go` and `composer_display.go`).
