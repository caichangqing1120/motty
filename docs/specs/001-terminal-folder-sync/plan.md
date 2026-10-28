# Terminal Folder Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Deliver Issue #109 with automatic shell-to-files following and optional safe reverse synchronization.
**Architecture:** Session-only Bash/Zsh startup shims report OSC 7 and OSC 133 B through the existing SSH PTY. Workbench coalesces forward reports and distinguishes explicit file navigation from background/forward listing updates. A separate default-off setting enables quoted reverse commands only at untouched ready prompts.
**Tech Stack:** Go 1.26, Viva native UI, existing SSH/SFTP clients; no new dependency.
**Spec:** `docs/specs/001-terminal-folder-sync/spec.md` v1.0.0.

## Global Constraints

- No remote startup-file edits; preserve prompt hooks and normal user startup content.
- Automatic Bash/Zsh support; unknown/refused probes use ordinary shell startup. Explicit `Command` is unchanged.
- Existing `sftp_follow` remains default on; new `sftp_terminal_follow` bool defaults off.
- Same-session attached remote navigation only. No reverse commands to dirty/busy/alternate-screen/AI-exec shells, local panes, standalone windows or container terminals.
- Failed listing, background refresh and forward navigation never execute reverse `cd`; no delayed unsafe commands or feedback loops.
- Preserve the user's original checkout and its unrelated documents. Keep worktree/evidence; push and create only the requested feature PR.

## Review Focus

- Shell startup customization: profiles, ZDOTDIR, prompt string/array hooks, unsupported shells and exec refusal must remain usable (Task 1).
- Fragmented protocol and literal path data: percent, hash, quotes, spaces, Unicode and controls must not break parsing or execute text (Tasks 1/2/3).
- Directory changes during initialization, busy listing or editing must coalesce without permanent snap-back (Task 2).
- Input already typed, running builtins/programs, reconnect, alternate screen and assistant captures must not receive automatic commands (Task 3).
- Default-off persistence, local/standalone panes, independent sessions and forward/reverse cycles must not leak effects (Task 3).

### Task 1: Session-only shell integration and prompt protocol

**Files:** Create `internal/sshconn/integration.go`, real-shell/SSH tests; modify `internal/sshconn/shell.go`, `internal/term/cwd.go`, `internal/term/parser.go`, parser tests, `internal/workbench/terminal.go`.
**Interfaces:** `ShellConfig.IntegrateShell bool`; `Terminal.SetPromptReady(func())`; OSC 7 remains unchanged; OSC 133 B invokes readiness callback. `integrationCommand(shell string) string` builds a runtime-only Bash/Zsh wrapper; unsupported input returns empty.

- [ ] Write failing tests: real Bash/Zsh startup with isolated HOME reports `cd` to literal special paths; original prompt hooks still execute; startup files remain byte-identical; temporary files are cleaned; unknown shell returns empty. SSH probe failure/refused exec uses shell request and explicit commands bypass integration.
- [ ] Run `go test ./internal/sshconn ./internal/term -run 'Integration|PromptReady' -count=1`; expect missing behavior/interface failures.
- [ ] Implement bounded optional shell probe before PTY creation; private temporary startup shims, existing profile/hook chaining, URI-byte encoding, cleanup/fallback; enable integration only for normal SSH terminals. Add exact OSC 133 B parsing.
- [ ] Run the focused tests plus existing SSH/terminal suites; expect PASS. Commit `feat(ssh): add session-local shell folder reporting`.

### Task 2: Reliable forward following

**Files:** Create `internal/workbench/sftp_sync.go` and tests; modify `sftp_extra.go`, `terminal.go`, `connection.go`, `sftp.go`.
**Interfaces:** `terminal.followPath string`; `followTerminalDirectory(session *Session)` consumes the latest pending path only when the attached remote view is safe. Existing `terminalDirectory(sessionID uint32, cwd term.Cwd)` queues it. No reverse origin on forward navigation.

- [ ] Write failing tests for reports before panel opening, reports during busy listing/path editing, latest-only coalescing, disabled follow and independent sessions; assert the final displayed path and rows.
- [ ] Run `go test ./internal/workbench -run 'FolderFollow|FolderTheShell' -count=1`; expect panel remains in old directory in new deferred cases.
- [ ] Implement queue/consume logic on Wake and after initial panel creation; suppress local/container/standalone effects; discard same-path targets without repeated navigation.
- [ ] Run the focused tests; expect PASS. Commit `fix(files): retain terminal folder changes until the panel is ready`.

### Task 3: Optional safe reverse sync and delivery verification

**Files:** Extend `sftp_sync.go` and tests; modify explicit navigation call sites in `sftp.go`, `sftp_extra.go`, `terminal_menu.go`; `terminal.go`; settings/model/persistence/search; `internal/i18n/{terminal_tools,english,chinese}.go`; docs.
**Interfaces:** `State.sftpTerminalFollow bool` persisted as `savedState.SFTPTerminalFollow bool` / `sftp_terminal_follow`; `navigateSFTPDirectory(view *sftpView, remotePath string)` tags explicit navigation; listing completion consumes its reverse target. `terminal.promptReady bool` becomes false on accepted input; OSC 133 B re-arms it. `syncTerminalDirectory(view *sftpView)` sends one `cd -- <single-quoted path>\r` only after a successful explicit navigation and all guards pass.

- [ ] Write failing tests for default-off/restoration and visible settings control; explicit successful directory navigation sends one literal quoted command to its owning ready shell, while local/standalone/container/busy/dirty/alt-screen/AI-exec/error/forward-refresh cases send none.
- [ ] Run `go test ./internal/workbench -run 'FolderSync|ReverseFolder' -count=1`; expect missing setting/navigation behavior.
- [ ] Implement independent localized setting/search and persistence, explicit-navigation tagging, successful-listing handling, no-loop/safety guards, visible skip feedback; keep unsupported prompts fail-closed.
- [ ] Run focused tests, `go test ./... -count=1`, dev-tag tests/vet, affected-package race, tracked-Go formatting, native/headless builds. Record exact commands and results, compatibility limitations and remaining manual acceptance.
- [ ] Commit `feat(files): add opt-in safe terminal directory synchronization`.
- [ ] Whole-branch independent review, fix Important/Critical findings with RED -> GREEN tests, then re-run affected verification. Verify remote target/scope, push the feature branch to the user's fork, open upstream PR against `main` with `Closes #109`, attach PR and inspect initial CI status. Do not merge.

## Self-Review

All FR and SC requirements map to Tasks 1-3. Shared interfaces are defined before their consumers; reverse navigation is distinct from `openSFTPDirectory` to avoid loops. Existing listing cache/worker behavior and asynchronous shell startup are retained. No unresolved review-gate condition.
