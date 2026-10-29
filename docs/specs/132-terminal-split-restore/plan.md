# Terminal Split Restoration Implementation Plan

> Execute inline with superpowers:executing-plans and test-driven-development.

**Goal:** Restore untouched output and terminal sizing after split closure.
**Architecture:** Preserve clipped main-screen rows across temporary width
changes, invalidating their saved content when the visible row changes. Keep
history lossless and retain the alternate screen's existing behavior.
**Tech Stack:** Go 1.26, Motty VT emulator, Viva native UI.
**Spec:** `docs/specs/132-terminal-split-restore/spec.md`

## Global Constraints

- Non-reflow behavior and alternate-screen semantics remain unchanged.
- Follow-up authorization permits commit, fork-branch push and a PR against
  `t8y2/motty:main` linked to #132; no installation, release or merge.
- Worktree starts at `2bbddc0`; sibling Viva is `dc4dbb3`.

## Review Focus

- Multiple successive width changes must not replace the original saved tail.
- Edits, erase, reset and recycled rows must not resurrect old content.
- Wide cells, combining marks, links and colors must survive untouched clipping.
- Scrollback movement must retain full rows within the existing memory budget.
- Closing an empty pane must restore dimensions without a window resize.

### Task 1: Preserve Clipped Main-Screen Rows

**Files:** `internal/term/resize_test.go`, `internal/term/resize.go`,
`internal/term/cell.go`, `internal/term/terminal.go`, `internal/term/terminal_test.go`.
**Interfaces:** Preserve `Terminal.Resize(cols, rows int)`, `Row`, history APIs.

- [x] Add literal-output regressions for repeated narrowing/restoration, edited
  rows, metadata, scrollback transfers, reset and alternate-screen behavior.
- [x] Run `go test ./internal/term -run 'TestResize.*(Retain|Restore|Clipped)' -count=1`.
  Expected: missing right-hand text fails before implementation.
- [x] Implement main-screen row retention with version-based invalidation and
  lossless history transfer; retain clipping for alternate-screen buffers.
- [x] Run `go test ./internal/term -count=1`. Expected: all tests pass, including
  allocation and history-budget checks. Adapt the former main-screen truncation
  expectation to cover alternate-screen truncation explicitly.

### Task 2: Exercise The Product Split Path

**Files:** `internal/workbench/workspace_ui_test.go`, `internal/workbench/workbench.go`
only if the empty-pane sizing regression fails.
**Interfaces:** Consume the unchanged `Terminal.Resize` API from Task 1.

- [x] Populate a real terminal grid with wide multi-line output, split right/down,
  close the new pane and assert restored text, dimensions and shell resize.
- [x] Test closing an empty pane; run regressions before any sizing fix.
- [x] Run `go test ./internal/workbench -count=1`. Expected: all tests pass.
- [x] Run formatting, full tests, vet, development-tag tests and affected race
  tests; build a separate native test app with isolated data.
- [x] Verify populated split/close cycles in the test app, preserve the user's
  installed copy, and record evidence. Obtain a fresh final code review.

## Self-Review

The two tasks preserve existing public interfaces. Every requirement and review
focus has a targeted regression; native verification supplements, not replaces,
automated tests. No unresolved product decisions or existing SE gate apply.
