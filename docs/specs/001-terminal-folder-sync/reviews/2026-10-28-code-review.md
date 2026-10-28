# Independent Code Review

Reviewed revision: `840d9b31c56836e212760d9ce49d76e361836c65` against `374809c`.
Reviewer: one independent, read-only agent. Initial verdict: needs changes.
No second review or independent post-fix approval is claimed.

## Findings And Corrections

| Severity | Finding | Correction | Regression evidence |
|---|---|---|---|
| Important | Complete submitted typeahead could re-authorize reverse cd at an intermediate prompt before a queued `read` | Token-bound command-start markers (Bash 4.4+ PS0 / Zsh preexec), pending submission accounting, fail-closed readiness | `TestReverseFolderSyncRejectsCompleteSubmittedTypeahead`; `TestReverseFolderSyncRequiresCommandStartReportingAndCanClearInputDebt` |
| Important | A reconnect reused old prompt authorization and the previous working directory | Rotate token, clear pending/dirty/readiness/capability/input state and cached cwd, invalidate generations | `TestReverseFolderSyncReconnectRequiresFreshShellAuthorization` |
| Important | Private startup-file umask 077 leaked into the user's shell | Save incoming umask, create private runtime files, restore mask before shell launch and fallback | `TestIntegrationReportsRealShellDirectoriesWithoutChangingProfiles` checks real file/dir modes with incoming 022 |
| Important | Probe cancellation did not bound channel-open/exec acknowledgment establishment | Cancellation-aware channel establishment and exec-start wait; close late/canceled command sessions without canceling the SSH connection | `TestIntegrationProbeCancellationWhileExecAcknowledgmentIsWithheld` also verifies the normal shell remains usable |

All four reproductions failed before their fixes and passed afterward. The full
release test suite and release/development vet also passed after this fix pass.
See `verification.md` for the final gate, including race and builds.

## Deliberate Safety / Compatibility Decisions

- Require token-bound prompt-start/end and command-start reporting, never infer
  readiness from visible prompt text. Standard markers from nested SSH sessions
  do not authorize the outer host's file panel.
- Use `builtin cd` with a literal quoted path; aliases/functions cannot turn a
  navigation request into unrelated code. Failed or stale reads never execute.
- Reverse synchronization requires Bash 4.4+ or Zsh. Older Bash still supports
  automatic forward reporting; it does not get a fragile DEBUG-trap override.
- Submitted multiline/program input can conservatively leave an ambiguity.
  The user can press Ctrl+C at an empty prompt to reset it, then navigate again.
  No automatic interrupt or delayed cd is sent.
- The reviewer evaluated the declared Bash interactive-rc / true-login-shell
  differences as a known compatibility boundary, not a separate finding.
  `docs/terminal-folder-sync.md` explains `login_shell`, `.bash_logout`, and the
  option to disable both sync directions and reconnect for ordinary startup.

No Critical or additional Minor findings. Nothing was declined as out of scope
within the requested behavior. Production-host and universal shell/profile/OS
acceptance remain unverified.
