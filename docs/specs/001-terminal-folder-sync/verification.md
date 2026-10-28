# Verification - 2026-10-28

Environment: Go 1.26.3, macOS; sibling Viva revision
`bd516513ae6f6c1c4aa3ebbaf205797fed053e8d`. Feature base: `374809c`.
The final gate below ran after the independent review's four regression fixes;
only documentation was edited after those runs.

| Check | Command / scope | Result |
|---|---|---|
| Full release suite | `go test ./... -count=1` | PASS |
| Development components | `go test -tags dev ./internal/workbench -run '^TestDebug' -count=1` | PASS |
| Release vet | `go vet ./...` | PASS |
| Development vet | `go vet -tags dev ./internal/workbench` | PASS |
| Affected-package race | `go test -race -p 1 ./internal/sshconn ./internal/term ./internal/workbench -count=1` | PASS; workbench 169.664s |
| Native executable | `go build ./cmd/motty` with an external output path | PASS |
| Headless development executable | `go build -tags vivahead,dev ./cmd/motty` with an external output path | PASS |
| Formatting | `gofmt -l` for tracked and new Go sources | No output |
| Patch whitespace | `git diff --check` | PASS |
| English Files UI | Actual headless build, Files settings, screenshot and layout diagnostics | 0 problems |
| Chinese Files UI | Actual headless build, Files settings at 1400x900 and 1000x720 | 0 problems |

## Regression Coverage

- Real isolated Bash/Zsh PTYs: automatic directory reports after cd to paths
  containing spaces, quotes, percent/hash/shell metacharacters and Unicode;
  original profile/hook execution; byte-identical profile files; runtime cleanup;
  exit status; inherited 022 mask produces 0644 files and 0755 directories.
- Real loopback SSH PTY: optional shell probing, startup integration and actual cd
  reports over the authenticated SSH channel.
- SSH request paths: unsupported/refused probe, refused exec, untouched explicit
  command, cancellation while exec acknowledgment is withheld, and usable
  ordinary-shell fallback on the same connection.
- Terminal protocol: fragmented markers, token ownership and malformed paths.
- Workbench: coalesced reports during busy/editing/local-pane states, opening and
  reopening the panel, opt-out startup, default-off shared reverse preference,
  literal quoted cd, failed/obsolete cached reads, independent sessions, no loops,
  partial/redrawn/complete queued input, unsupported command-start reporting,
  reconnect authorization reset, nested prompts and excluded terminal types.

## Review And Limits

The single independent review found four Important issues. Each has an observed
RED -> GREEN regression and a fix; see
`reviews/2026-10-28-code-review.md`. No independent post-fix review is claimed.

No production host, packaged application installation, Windows/Linux desktop
runtime, universal shell/profile compatibility or end-user acceptance is
claimed. Automatic forward integration supports Bash/Zsh; reverse needs Bash
4.4+ or Zsh command-start reporting. Submitted multiline or program input is
handled conservatively and may need explicit Ctrl+C at an empty prompt before
another navigation. Bash true-login differences and complete startup opt-out
are documented in `docs/terminal-folder-sync.md`.

The user's original checkout and unrelated documents remain untouched. The
feature worktree and task-owned verification evidence are preserved for PR
follow-up. Nothing is merged or deployed by this task.
