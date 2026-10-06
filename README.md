# bed

A Bubble Tea text editor extracted from [cxz](https://github.com/lesomnus/cxz).
Built on `bubbles/textarea`, with no dependency on cxz, its sessions, RPCs or AI providers.
Requires Go 1.24.2 or newer. The API is experimental.

## Edit a file

```sh
go install github.com/lesomnus/bed/cmd/bed@latest
bed path/to/file.txt
```

Or run from the repository:

```sh
go run ./cmd/bed path/to/file.txt
```

Ctrl+Z undoes; Ctrl+Y (or Ctrl+Shift+Z where supported by the terminal) redoes.
Ctrl+S saves; Ctrl+Q exits. When there are unsaved changes, Ctrl+Q asks for a
second press to discard them. A `*` marks a modified buffer. A missing file is
created on the first save. Save errors keep the buffer open.

The program supports UTF-8 text with LF or CRLF endings and preserves the final
newline, tabs and existing file permissions. Tabs appear as a single `⇥` cell
and are written back as real tab characters; Tab inserts one. Saving uses a
temporary file in the same directory followed by rename and checks for changes
made on disk since opening/saving. Existing symlinks resolve to their target.
The initial version rejects mixed line endings, unsupported control characters
and files beyond textarea's 10,000-line limit instead of silently changing them.
It is a small single-file editor, without syntax highlighting or file tabs.

Ctrl+S/Ctrl+Q and file I/O belong to `cmd/bed`, not to the library.

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

## Key bindings

The library ships defaults, but does not require fixed shortcuts. Basic editing
uses `editor.KeyMap` (textarea's key map); selection, visible-row navigation, cut
and whitespace display use `editor.EditorKeys`. Both use Bubbles `key.Binding`:

```go
editor.KeyMap.WordBackward.SetKeys("ctrl+left", "alt+b")
editor.EditorKeys.SelectWordLeft.SetKeys("ctrl+shift+left")
editor.EditorKeys.ToggleWhitespace.SetKeys("f4")
editor.EditorKeys.Cut.SetEnabled(false)
```

To supply only your own bindings, clear both maps and enable the actions needed:

```go
editor.KeyMap = textarea.KeyMap{}
editor.EditorKeys = bed.EditorKeyMap{}
editor.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("enter"))
editor.EditorKeys.SelectLeft = key.NewBinding(key.WithKeys("shift+left"))
```

Selection invokes word motion independently of the ordinary word-motion binding.
Editor bindings run before textarea bindings; avoid assigning the same key to
unrelated actions. Home/End exist in both maps (bed's visible-row navigation and
textarea's logical-line navigation), so configure both when replacing those keys.
Printable text input and mouse editing remain available with empty key maps.
Save, quit and other application actions should be intercepted by the parent
before calling `editor.Update`.

## Undo and redo

`Undo()` / `Redo()` return whether a group was restored. `CanUndo()` / `CanRedo()`
report availability. Bindings are `EditorKeys.Undo` and `EditorKeys.Redo` and may
be remapped or disabled like other actions.

Text, cursor and selection are restored together. Consecutive typing and repeated
Backspace/Delete group within `UndoGroupDelay` (default 750 ms). Cursor navigation,
paste, newline, cut and selection replacement separate edits. `BreakUndoGroup()`
starts a new group for the next edit; the file editor calls it when saving.
Saving does not clear history, and Undo does not write to disk.

History is bounded by `HistoryLimit` (default 100 groups) and `HistoryBytes`
(default 8 MiB of retained text snapshots). Oldest groups are discarded first;
a single oversized edit clears retained history. Set either limit to zero to
stop retaining new edits. `ClearHistory()` immediately discards Undo and Redo.
History is in memory and belongs to one document; a changed `DocumentKey`,
`SetValue` or `Reset` clears it. A new edit after Undo discards the redo branch.

`InsertString`, `InsertRune`, `DeleteSelection` and the normal Update pipeline
record edits. `SetValue` is for loading/replacing a document, not an undoable edit.
Direct mutations through the embedded `textarea.Model` bypass recording and
invalidate history when detected. App-owned chip payloads and decorations are
not part of text history. The split `HandleKey` / `UpdateText` pipeline is also
supported; forward unhandled input to `UpdateText` to commit replacement edits.

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
