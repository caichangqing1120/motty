# SFTP Navigation P0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fast current-folder filtering, per-side hidden-file visibility, and direct path editing to Motty's existing SFTP side panel.

**Architecture:** Keep the existing `sftpFolder` as the state boundary for the remote and local sides. Each folder retains its complete prepared rows and derives the displayed rows from its own filter and hidden-file state, so filtering and visibility changes never issue remote requests. Reuse the existing validated path editor and add temporary edit mode plus contextual shortcuts rather than replacing the saved breadcrumb/input preference.

**Tech Stack:** Go 1.26, Viva native components and text input, existing Motty SFTP worker and workbench tests.

**Spec:** https://github.com/t8y2/motty/issues/62

## Global Constraints

- Current-folder filtering is local over loaded rows only; do not recursively search or run remote `find`.
- Local and remote filter text, filter visibility, and hidden-file visibility remain independent within the panel session.
- Local hidden items are hidden by default; remote dotfiles remain visible by default for compatibility.
- Visibility and filtering never change transfer source/target paths or trigger a new SFTP listing.
- Reuse the existing path validation, global error toast, breadcrumb setting, theme, icon-button, focus, and native text-input conventions.
- Preserve existing server/local listings, selection behavior, large-list background preparation, cached refreshes, and zero-allocation steady paint when controls are unchanged.
- Baseline is `9a1a243eb154c09c2f46aa941a5849dafbe7b1ef`; Viva is the sibling checkout at `../viva` on `ca0ad1fcdcb3496913fc6378fbdcfa1b47fa868b`.

## Review Focus

- A remote refresh while a filter is active compares against the complete listing, not the filtered subset, and does not cause a refresh loop (Task 1).
- Filter/hide changes clear selections that are no longer visible; uploads/downloads cannot act on a stale hidden row (Task 1/2).
- Unicode and mixed-case names filter without corrupting highlighted byte boundaries; an empty query restores all otherwise-visible rows (Task 1).
- Windows Hidden/System attributes compile into the local hidden predicate while POSIX uses leading dots; remote detection always uses leading dots (Task 2).
- Temporary path edit exits only after a successful navigation, keeps the draft and folder after an error, and does not change the user's saved breadcrumb/input preference (Task 3).

---

### Task 1: Per-side current-folder filter

**Files:** Create `internal/workbench/sftp_filter.go`, `internal/workbench/sftp_filter_test.go`; modify `cmd/motty/main.go`, `internal/workbench/sftp.go`, `internal/workbench/sftp_local.go`, `internal/workbench/sftp_paint.go`, `internal/i18n/i18n.go`, `internal/i18n/english.go`, `internal/i18n/chinese.go`.

**Interfaces:** Add folder-owned complete rows and filter editor state. Add `State.SFTPInput(app.TextInputContext) *app.TextInput`, `State.sftpFilterText(uint32, bool, app.TextEvent) app.TextState`, and private row derivation helpers. Existing listing and transfer method signatures stay unchanged.

- [ ] **Step 1: Write failing tests** for case-insensitive substring filtering, Unicode names, independent local/remote drafts, one-click clear, match/total status, highlighted match runs, selection clearing, no remote request on filter changes, and filtered remote refresh comparison against complete rows.
- [ ] **Step 2: Run** `go test ./internal/workbench -run 'TestSFTPFilter' -count=1`; expected FAIL because filter state, hits, input resolver and derivation are absent.
- [ ] **Step 3: Implement** the complete-row/visible-row model, native input callback, compact search field with clear action, current-row match styling, status counts and contextual configured Find shortcut when a file row/filter owns focus. Keep terminal Find unchanged for terminal focus.
- [ ] **Step 4: Run** focused tests and `go test ./internal/workbench -count=1`; expected PASS.
- [ ] **Step 5: Commit** with `feat(sftp): filter current folder entries`.

### Task 2: Independent hidden-file visibility

**Files:** Create `internal/workbench/sftp_hidden_other.go`, `internal/workbench/sftp_hidden_windows.go`; modify `internal/workbench/sftp.go`, `internal/workbench/sftp_local.go`, `internal/workbench/sftp_filter.go`, `internal/workbench/sftp_paint.go`, `internal/i18n/i18n.go`, `internal/i18n/english.go`, `internal/i18n/chinese.go`; test `internal/workbench/sftp_filter_test.go` and a Windows build-only helper test if needed.

**Interfaces:** Add `localFileHidden(string, os.FileInfo) bool` behind platform build tags. Prepared rows carry a private hidden flag. Each `sftpFolder` owns `showHidden`; remote initializes true and local initializes false.

- [ ] **Step 1: Write failing tests** that local dotfiles are hidden by default, remote dotfiles remain visible, toggles are independent and visible as active state, toggling does not increment remote reads, filter counts respect the visibility boundary, and hidden selected rows cannot survive a toggle.
- [ ] **Step 2: Run** `go test ./internal/workbench -run 'TestSFTPHidden|TestSFTPFilter' -count=1`; expected FAIL before hidden predicates and toggle state exist.
- [ ] **Step 3: Implement** POSIX leading-dot and Windows Hidden/System predicates, mark prepared rows, derive visible rows without I/O, and add a persistent-state icon button with accessible show/hide labels.
- [ ] **Step 4: Run** focused tests, workbench tests and `GOOS=windows GOARCH=amd64 go test ./internal/workbench -c -o /tmp/motty-workbench-windows.test`; expected PASS/build success.
- [ ] **Step 5: Commit** with `feat(sftp): control hidden file visibility`.

### Task 3: Temporary direct path editing and shortcut

**Files:** Modify `internal/workbench/sftp_path.go`, `internal/workbench/sftp_path_test.go`, `internal/workbench/sftp_paint.go`, `internal/workbench/sftp.go`, `internal/workbench/shortcuts.go`, `internal/workbench/shortcut_paint.go`, `internal/workbench/shortcuts_test.go`, `internal/i18n/i18n.go`, `internal/i18n/english.go`, `internal/i18n/chinese.go`.

**Interfaces:** Add `shortcutFilesPath` with default primary+L and saved string `files_path`. Add private `startSFTPPathEditing(*sftpView) ui.EventResult`, a breadcrumb-background hit, and folder-owned temporary edit state. Existing always-input setting remains authoritative and persists unchanged.

- [ ] **Step 1: Write failing tests** for primary+L opening/focusing the active side's editor, clicking breadcrumb background, Escape restoration, successful local/remote exit, invalid-path draft retention, separate local/remote drafts, shortcut persistence and no change to `SFTPPathInput` preference.
- [ ] **Step 2: Run** `go test ./internal/workbench -run 'TestSFTPPath.*(Shortcut|Temporary|Breadcrumb)|TestShortcut.*FilesPath' -count=1`; expected FAIL before temporary mode and shortcut exist.
- [ ] **Step 3: Implement** temporary input visibility and focus routing over the existing submit/error code, exit it only on successful navigation, and leave configured always-input mode unchanged.
- [ ] **Step 4: Run** focused tests and `go test ./internal/workbench -count=1`; expected PASS.
- [ ] **Step 5: Commit** with `feat(sftp): jump directly to folder paths`.

### Task 4: Cross-platform verification and draft PR

**Files:** Update this plan only if verified limitations differ; no unrelated release/version changes.

**Interfaces:** No new production interface.

- [ ] **Step 1: Run** `test -z "$(gofmt -l $(git ls-files '*.go'))"`, `go vet ./...`, `go vet -tags dev ./internal/workbench`, `go test ./... -count=1`, `go test -tags dev ./internal/workbench -count=1`, and `go test -race -p 1 ./... -count=1`.
- [ ] **Step 2: Run** Windows cross-build tests for `./cmd/motty` and `./internal/workbench`; verify no generated binary is tracked.
- [ ] **Step 3: Build and launch an isolated development app/config, inspect local and remote P0 flows in light/dark mode, save screenshots, then close all task-owned app/browser processes. Do not transfer or delete files during validation.
- [ ] **Step 4: Review** the complete diff against #62 and run a fresh whole-branch reviewer; fix Critical/Important findings with RED→GREEN tests.
- [ ] **Step 5: Commit** final evidence/docs if needed, create `codex/issue-62-sftp-navigation`, push it to `caichangqing1120/motty`, and open a draft PR against `t8y2/motty:main` with `Fixes #62`. Attach the PR to this task; do not merge or release.
