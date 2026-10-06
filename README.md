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
(default 8 MiB of retained text and chip strings). Oldest groups are discarded first;
a single oversized edit clears retained history. Set either limit to zero to
stop retaining new edits. `ClearHistory()` immediately discards Undo and Redo.
History is in memory and belongs to one document; a changed `DocumentKey`,
`SetValue` or `Reset` clears it. A new edit after Undo discards the redo branch.

`InsertString`, `InsertRune`, `DeleteSelection` and the normal Update pipeline
record edits. `SetValue` is for loading/replacing a document, not an undoable edit.
Direct mutations through the embedded `textarea.Model` bypass recording and
invalidate history when detected. Chip descriptors (including their ranges and expansion text) are restored with
the buffer. External payloads referenced by chip IDs and arbitrary decorations
remain owned by the application. The split `HandleKey` / `UpdateText` pipeline is also
supported; forward unhandled input to `UpdateText` to commit replacement edits.

## Composition

`Model` embeds `textarea.Model`, exposing its styling and base key map. Use
`SetValue`/`Reset` on the bed model, and a new model for each independent editor.
Shallow copies have the same sharing limitations as textarea itself.

`DocumentKey` is an optional opaque identifier for invalidating selection when
an application switches documents while reusing the editor.

`AtomicTokens` is retained for compatibility with hosts that manage their own
chip lifecycle. It only expands selections over matching labels. New code should
use the positional Chip API below, which also handles movement, deletion and history.

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

## Chips

```go
err := editor.InsertChip(bed.Chip{
    ID: "attachment-1", Label: "[report.pdf]", Text: "expanded text",
})
```

A chip is a styled, atomic **label in the text buffer**. `Value()` includes the
label, not its expanded text. IDs must be unique; identical labels with different
IDs are supported. `Chips()` returns a copy of descriptors with rune ranges.
`SetChips` validates and attaches saved descriptors to existing labels. The host
owns uploads, secrets, attachment payloads, preview dialogs and persistence.

Arrows select a neighboring chip as a unit; mouse clicks also select it. A selected
chip's Enter binding emits `ChipActivateMsg` for the host. Alt+Enter expands its
`Text`. `EditorKeys.ChipActivate` / `ChipExpand` can be changed or disabled.
`ExpandChip(id)` and `RemoveChip(id)` expose the same operations programmatically.
Cursor movement skips chip interiors, edits cannot leave partial labels, and
insert/expand/delete/metadata changes each participate in Undo/Redo. `ChipStyle`
controls label styling. Use color/style attributes without padding or borders so
terminal cell positions stay unchanged.

`ReplaceRange(from, to, text)` is an undoable edit API for rune offsets. Intersecting
chips expand the range to whole labels. Invalid ranges, unsupported control
characters and textarea limit overflows return errors without changing text.

## Completion providers

```go
editor.CompletionTriggers = "@"
editor.CompletionColumns = 3 // responsive grid; default is one column
editor.CompletionProvider = func(ctx context.Context, r bed.Request) (bed.CompletionResult, error) {
    prefix := string([]rune(r.Text)[:r.Cursor])
    start := strings.LastIndex(prefix, "@")
    if start < 0 { return bed.CompletionResult{}, nil }
    from := utf8.RuneCountInString(prefix[:start])
    // Query/filter application data here. From/To are rune offsets.
    return bed.CompletionResult{
        From: from, To: r.Cursor,
        Items: []bed.CompletionItem{{Label: "seal", Detail: "session", InsertText: "@seal "}},
    }, nil
}
```

Ctrl+Space requests candidates explicitly; configured trigger characters request
them while typing. An active popup refreshes after typing/deletion, including when
there are no matches. Providers own token detection, filtering and ranking. The
editor owns list/grid layout, paging, loading dots, keyboard and mouse navigation,
and replacement of the supplied range. The popup overlays the bottom rows of the
widget; it does not change the document or the widget's total height.

Arrow keys navigate, PageUp/PageDown page, Enter/Tab accepts, Escape dismisses;
mouse hover/wheel navigates and clicking accepts. These bindings live in
`EditorKeys.Completion*`. `CompletionRows` limits popup height. Style using
`CompletionStyle` and `CompletionSelectedStyle` without layout-changing padding.

For host-managed data, use `SetCompletions(result)`, `AcceptCompletion()` and
`DismissCompletions()`. `RequestCompletions(ctx)` returns a Bubble Tea command.
Providers receive immutable document/cursor snapshots and must honor cancellation.
Responses from obsolete requests/documents/cursors/other editor instances are
ignored. Errors arrive as `FeatureErrorMsg`; route returned messages through the
editor's `Update` and handle error messages in the parent. Call `Close()` when
removing an editor; `Blur()` also cancels requests. For split input dispatch, call
`CompletionMouse` with widget-relative coordinates before `HandleMouse`.

## Ghost text

```go
editor.GhostProvider = func(ctx context.Context, r bed.Request) (string, error) {
    return "next proposed text", nil // host may query an AI service here
}
// Or supply an already available suggestion directly:
editor.SetGhost("next proposed text")
```

Ghost is a virtual **insertion at the end of the document** in this initial API;
it is not an arbitrary mid-document replacement. Multiline previews show as many
lines as fit in the viewport, with overlong lines clipped. Ctrl+G requests a ghost;
Right accepts it; Escape dismisses it. These are `EditorKeys.RequestGhost`,
`GhostAccept`, `GhostDismiss`. `RequestGhost(ctx)` can be called directly.

Typing, cursor movement, document replacement or a competing completion popup
invalidates the preview. Loading dots and `GhostStyle` use the same display area.
`AcceptGhost()` inserts the full text as one undoable edit; merely showing a ghost
does not affect the document, clipboard text, saving, or Undo history. Providers
are optional and use the same cancellation/stale-result rules as completions.
The library makes no network or AI requests of its own.

## Layout and word policy

`AutoHeight(minimum, maximum)` suggests a bounded height based on current wrapping;
the parent chooses when to apply it with `SetHeight`. `WordSeparators` adds
application-specific separators (for example `/` and `\\` for paths) to the
whitespace boundaries used by word movement, selection and deletion.

Run an offline interactive demonstration of all three extensions:

```sh
go run ./examples/extensions
```

F2 inserts a chip, `@` opens sample candidates, Ctrl+G requests a canned ghost,
and Ctrl+Z/Ctrl+Y undo/redo. Select a chip and press Enter to preview or Alt+Enter
to expand it. Ctrl+Q exits. This demo has no network/AI access and does not save
files; `cmd/bed` remains the practical file editor with no default providers.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
```

Originally extracted from cxz at commit
`5ac137a` (`internal/tui/composer_selection.go` and `composer_display.go`).
