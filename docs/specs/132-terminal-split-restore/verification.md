# Verification: Terminal Split Restoration

Date: 2026-10-29. Issue: #132. Base: `2bbddc0`. Viva dependency: `dc4dbb3`.

## Reproduction And Root Cause

The installed macOS 0.1.7 release was exercised using an authorized SSH
connection, generated multi-line output and wide rows with an `END` marker.
Opening and closing a right split permanently removed the old rows' right-hand
content. New output still occupied the full width, distinguishing content loss
from a remaining narrow page container.

`Terminal.Resize` destructively truncated main-screen rows. A separate
product-path regression found that closing an empty split did not resize the
remaining terminal/PTY: the right split stayed at 71x44 instead of 146x44, and
the down split stayed at 146x20 instead of 146x44.

## Automated Evidence

- Baseline term, workspace and workbench suites passed before editing.
- Restoration tests first failed with missing right-hand text, metadata and
  history; the full term suite passed after preserving untouched clipped rows.
- Empty-pane sizing tests failed for both axes, then passed after fitting the
  remaining workspace terminal and file panel.
- Fresh review reproduced a wide-character history projection/cache mismatch.
  Both emulator and renderer regressions failed, then passed after repairing
  only the returned boundary view, including combining marks and backgrounds.
- The first parallel full-suite run hit the existing local-shell test's 5-second
  startup deadline. Its isolated test passed three consecutive runs. A final
  serialized full-suite run passed without changing local-shell production code.

| Command | Result |
| --- | --- |
| `test -z "$(gofmt -l .)"` and `git diff --check` | Pass |
| `go vet ./...` | Pass |
| `go vet -tags dev ./internal/workbench` | Pass |
| `go test -p 1 ./... -count=1` | Pass, 82 packages with tests |
| `go test -p 1 -tags dev ./cmd/motty ./internal/workbench -count=1` | Pass |
| `go test -race -p 1 ./internal/term ./internal/workspace ./internal/workbench -count=1` | Pass; term 10.475s, workspace 1.622s, workbench 301.271s |

## Native Product Verification

A separate native development app used an explicit isolated data directory.
It did not replace the installed app or share its data writer.

On the final compiled version, a real local PTY printed 75 wide numbered rows.
After both right-split/close and down-split/close cycles, the existing rows'
right-hand `END` markers remained visible. `stty size` reported `50 175` before
the cycles and `50 175` afterward. No window resize or tab switch was needed.
The task-owned test process was stopped; the original installed app and its
SSH connections remained running.

## Review And Limitations

The fresh review's blocking history-view finding was fixed and verified by a
RED-to-GREEN emulator/renderer pair. No second review was performed after that
fix. A nonblocking coverage suggestion remains deferred: explicitly assert the
intermediate narrowed grid size, in addition to the current two-pane, restored
grid/PTY and retained-output assertions.

The model intentionally remains non-reflowing, and alternate-screen truncation
semantics are unchanged. Windows/Linux native acceptance and a patched SSH
session against the original server were not exercised. Follow-up delivery
authorization permits submitting this verified fix through a PR linked to #132.
Installation, release, merge and issue closure remain outside this verification.
