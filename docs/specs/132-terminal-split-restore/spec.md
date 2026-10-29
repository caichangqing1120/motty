# Restore terminal output after closing a split

Source: user bug report and correction (2026-10-29), issue #132.

## Required Behavior

- Opening and closing a terminal split must not permanently discard the right
  side of existing, untouched main-screen output.
- Preserve cells, colors, wide characters, combining marks and OSC 8 links.
- Output overwritten or erased while narrow must not resurrect stale content.
- Retained content must survive scrolling into history and returning to the screen.
- Keep the existing non-reflow model: do not reformat command output or change
  alternate-screen truncation semantics for full-screen programs.
- After either split direction closes, the remaining terminal and its shell
  receive the available size immediately, including closing an empty pane.

## Verification And Boundaries

Reproduced in installed Motty 0.1.7 on macOS with populated SSH output: old
wide rows lose their right half, whereas newly generated output uses full width.
Use deterministic generated output for verification, not a blank terminal.
Remote commands are read-only. Preserve existing sessions and the installed app.
Issue creation and claim were authorized initially. The follow-up request on
2026-10-29 authorizes committing, pushing the fix branch to the contributor's
fork, and opening a PR against `t8y2/motty:main` linked to #132. Installation,
release and merge are not included. Keep the isolated worktree and evidence.
