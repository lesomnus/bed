# Multi-cursor editing plan

Status: phase 1 implemented; phases 2–4 in progress. Updated: 2026-10-07.

## Goal and delivery

Add reusable multi-cursor editing to bed, starting with Alt+click and
Ctrl+Alt+Up/Down. Preserve chip atomicity, undo/redo, Unicode handling, existing
single-cursor integrations, and configurable host bindings. This is a bed plan;
cxz adoption is a separate final integration step, not an implicit dependency
upgrade. No search, occurrence matching, or rectangular selection is required.

Deliver in the phases below, with tests and documentation in each phase. bed
changes go directly to main; cxz changes use an isolated worktree and PR.
Do not describe a phase as supported until its behavior and tests are complete.

## Current implementation

- `editor.go` embeds bubbles/textarea and owns one `selection`.
- `editing.go` represents anchor/head as rune offsets and maps display rows to
  document positions. `mouse.go` implements word/line multi-click selection.
- `chips.go` expands intersecting edits to atomic chip boundaries. Chip IDs are
  unique; external payload ownership remains with the host.
- `history.go` snapshots text, one cursor/selection, and chip metadata, with
  bounded history and typing groups.
- `lines.go`, `completions.go`, `features.go`, `display.go`, and `viewport.go`
  currently assume one editing target or one primary display cursor.

Keep the snapshot history strategy initially. Replace the single-target edit
path with a batch transaction rather than repeatedly replaying textarea key
handling at each cursor. Rendering/layout can continue using textarea, but
multi-cursor edits must have one authoritative bed-owned result.

## Shared behavioral contract

### Selection model

Represent every cursor as a selection with anchor/head rune offsets. Equal
positions mean an empty selection. Retain selection direction, a stable internal
identity, and a desired display column for vertical motion. Track one primary
selection independently from document-order sorting.

Normalize duplicate cursors and overlapping ranges deterministically. Adjacent
nonempty ranges remain distinct unless an edit makes them overlap. A cursor
inside a selected range is absorbed; endpoint insert/delete collisions must
also resolve to a single edit. Preserve the primary identity when possible,
otherwise transfer it to the merged range. Normalize again after chip expansion
and after edits. Never insert duplicate text at one position.

Public APIs should expose copies of the selection set and support setting a
validated set, adding a cursor, clearing secondary cursors, and querying the
primary selection. Names/signatures are finalized in phase 1. Invalid explicit
API ranges return an error without mutation; pointer positions clamp to the
visible document. Use rune offsets for API compatibility, terminal-cell columns
for vertical targeting, and grapheme boundaries for user navigation.

### Input and mouse

- Alt+left-click adds a cursor and makes it primary; clicking an existing cursor
  promotes it without toggling it away. It does not start a drag.
- Ctrl+Alt+Up/Down adds one cursor above/below the primary on the adjacent visual
  row and promotes the target. Repeated keys extend the set one row at a time.
- Soft-wrapped rows count as visual rows. Short rows clamp to their end while
  retaining the desired column. Document edges are no-ops. No virtual spaces.
- Normal click replaces the set with one cursor. Existing double/triple-click
  and plain mouse drag replace the set with one word/line/range selection.
- Escape dismisses an active completion/ghost first; otherwise it collapses the
  set to the primary selection. A subsequent Escape clears that selection using
  the existing single-selection behavior.
- Movement and Shift movement operate on every selection. Without Shift, an
  existing selection collapses consistently with current bed navigation.
- Adding/removing cursors breaks a typing undo group but does not itself create
  a text-undo entry. Terminal/OS interception cannot be fixed by bed; expose
  keyboard remapping and configurable/disableable mouse modifiers.

### Editing transactions and chips

Build edits against one immutable pre-edit document. Expand ranges for chips,
resolve collisions, and validate the complete result against character/line
limits and text constraints. Apply replacements from the end of the document
(or construct the result in one pass), remap all selections/chips, and commit
once. Any invalid edit rejects the whole transaction, with one host-visible
error. Do not partially modify text, chips, cursor sets, or undo history.

A cursor cannot live inside a chip. Pointer hits snap to the nearest boundary
(ties consistently choose the trailing edge); directional motion snaps in its
movement direction. Any partial chip selection/edit expands to the whole chip.
Multiple edits targeting one chip remove/replace it once. Existing chip payload
ownership stays with the host; do not implicitly reuse one unique ID for several
insertions.

### Undo, rendering, and compatibility

Snapshots include all anchors/heads, primary identity, desired columns, text,
and chip metadata. One input action at N cursors creates one undoable edit.
Continuous typing groups only while the cursor set and edit kind remain
compatible. Undo/redo restore the entire set; edits after undo drop the redo
branch. History byte accounting must include added selection metadata.

Render secondary cursors and all selections through bed's decoration path.
Keep a single native/primary cursor and a shared blink clock; do not allocate a
native terminal cursor or timer per secondary cursor. Follow the primary when
scrolling; wheel scrolling retains all selections. Resize remaps visual rows,
never document offsets. Hosts composing RawView must have access to the same
selection/cursor decorations as the complete View path.

Keep existing single-position/query APIs about the primary selection. Explicit
legacy SetPosition/Select operations collapse to a single target, so existing
callers remain predictable. Add plural APIs for multi-target manipulation.
Keyboard editing and InsertString/InsertRune apply to the active set; explicit
ReplaceRange addresses exactly its named range and remaps other selections.
SetValue/document replacement clears secondary selections and transient async
features. Document the distinction rather than silently retargeting old APIs.

## Phase 1 — usable first implementation

1. Introduce the selection set, normalization, primary identity, and batch-edit
   primitive. Route single-cursor editing through the same primitive first.
2. Add configurable cursor-add bindings and mouse handling described above.
3. Support typing, IME/bracketed paste, Enter with per-target auto-indent,
   Backspace/Delete, word deletion, navigation, and Shift selection at all
   cursors. Plain paste inserts the same clipboard text at every selection.
4. Enforce existing chip atomicity for all those operations. Keep InsertChip,
   chip activation, and expansion explicitly primary-only in this phase;
   remap secondary selections after their edits.
5. Extend snapshots, grouping, limits, undo/redo, selection painting, cursor
   rendering, focus/blur, and scroll behavior.
6. Disable/cancel completion and ghost requests while multiple cursors exist;
   reject late results created before the cursor-set change. Unsupported line
   commands must return a clear error instead of editing only one location.
7. Basic copy/cut joins nonempty selected fragments in document order with a
   newline. With no selected text, retain existing single-cursor behavior and
   make multi-cursor copy/cut a no-op until phase 2.
8. Update README and the practical cmd/bed file editor with configurable
   bindings/help. Preserve Ctrl+S save and Ctrl+Q quit.

Exit criteria: the two requested gestures are usable for real file editing;
all edits are atomic and undoable; chips cannot be split; existing single-cursor
behavior remains covered. No advertised keyboard action silently edits only the
primary unless its primary-only semantics are explicitly documented.

## Phase 2 — line commands and clipboard

- Indent/outdent unique affected logical lines once, respecting selections
  ending at the next line's start. Preserve selection direction.
- Move/duplicate disjoint line blocks as one edit. Merge overlapping/adjacent
  affected blocks where required; define boundary collisions before mutation.
  Duplicate chips using unique instance IDs with explicit host payload rules.
- Copy/cut with empty cursors operates on distinct logical lines once; define
  mixed empty/nonempty selections as selected fragments only (empty ones do not
  unexpectedly copy whole lines).
- Add an explicit paste-distribution option/API: one fragment per selection
  only when counts match. Otherwise broadcast the whole text. Do not infer
  intent from arbitrary newline counts or depend on private OS clipboard data.
- Keep plain-text copy and chip expansion/payload export separate; hosts can
  opt into structured fragments without exposing external payloads by default.

## Phase 3 — completion, ghost, and multi-target chip APIs

- Add cursor-set/document revisions to async request identity. Cancel and reject
  stale results on edits, cursor changes, undo, document switch, and provider
  changes, even when text happens to remain equal.
- Completion UI is anchored at the primary cursor. Default providers remain
  single-target; multi-target application requires an explicit provider result
  with validated replacement ranges for the captured selection set. Accept all
  replacements in one transaction. Never replay arbitrary primary edits at
  unrelated positions.
- Ghost text follows the same opt-in rule. Single-target ghost stays hidden
  with multiple cursors; explicitly multi-target ghost previews each proposed
  edit and accepts them atomically. No automatic N-fold provider calls.
- Provide explicit multi-chip insertion with a host-supplied instance-ID/payload
  strategy. Define rollback and undo ownership before enabling it. Activation
  remains primary-only; batch expansion/removal uses explicit APIs.
- Preserve zero-config behavior of existing completion/ghost/chip providers.

## Phase 4 — cxz integration and polish

- Upgrade bed in an isolated cxz worktree and submit a PR. Audit composer event
  routing, key precedence, terminal modifiers, selection/copy handling, custom
  rendering, paste chips and external payload maps.
- Verify Enter/send, suggestion acceptance, mention/path completion, approval
  navigation, Escape, and Ctrl+D detach do not bypass the new selection model.
  Product-specific send behavior stays in cxz; it must not send one message per
  cursor. Define a single-action or disabled policy for each composer command.
- Confirm one output payload from all edits, including legacy chip expansion;
  undo/redo must preserve host payload associations. Do not claim cxz support
  solely because the bed dependency was updated.
- Add configurable cursor-count limits, benchmarks for many cursors/chips and
  large documents, and optimize layout/edit passes based on measurements.
  Enforce a safe bound from phase 1 even before benchmark-driven tuning.

## Validation across phases

- Unit and property/fuzz tests: sorted/normalized nonoverlapping ranges, stable
  primary selection, deterministic collisions, valid offsets, and edit→undo→redo
  round trips of text, selections, chips, and metadata.
- Unicode: Korean, combining marks, emoji/ZWJ, wide glyphs, tabs, soft wraps,
  empty lines, end-of-file, narrow viewports, and resize while selected.
- Input: repeated cursor-add keys, duplicate targets, reverse selections,
  dragging, scroll/focus changes, chip boundary hits, mixed selections, paste,
  per-target indentation, and rejected transactions with no partial effects.
- Async features: stale results after selection-only changes and undo; cancellation
  without leaks; independent editors never share cursor/provider state.
- Full Go tests and race checks, Linux/Windows builds and relevant CI, plus an
  interactive/PTY file-editor exercise covering save/reopen and undo/redo.
  Confirm actual terminal delivery of Alt+click and Ctrl+Alt+arrows where possible;
  report environments where the OS/terminal consumes them.

## Deferred decisions

Exact public type names and configurable cursor-count limit are phase 1 design
choices. Multi-target provider result shapes and chip instance/payload ownership
are phase 3 API decisions and must be documented before implementation. Search,
select-next-occurrence, rectangular selection, and language-server edits remain
outside this plan unless separately requested.
