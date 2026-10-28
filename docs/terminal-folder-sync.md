# Terminal and File Panel Folder Sync

The attached file panel follows the SSH terminal's working directory by default.
New Bash and Zsh sessions install session-only folder reporting, without editing
the server's existing startup files. Directory reports are kept while the panel
is opening, busy or being edited; only the latest pending directory is applied.

In **Settings > Files**, the two directions are independent:

- **Follow the terminal's folder** is on by default.
- **Terminal follows file navigation** is off by default. Turn it on to also
  change the owning terminal's folder after entering a remote directory.

Reverse sync requires a fresh successful directory read and an untouched,
ready shell prompt belonging to the same session. It skips partial command
lines, running programs, nested SSH prompts, alternate screens and assistant
commands, with feedback in the file panel. A skipped request is never run later;
navigate again once the original shell is ready. Paths are quoted as literal
arguments to `builtin cd`, rather than invoking aliases or shell functions.

Local panes, standalone file windows and container command sessions do not
control terminals. Refreshing a list or following a terminal report never sends
another `cd` back. Both preferences use the existing shared-settings storage.

## Startup Compatibility

- Bash uses a private runtime rc file that sources `/etc/profile` and the first
  readable user login profile, then appends to the existing string or array
  `PROMPT_COMMAND`. It runs interactively, but Bash's `login_shell` flag and
  automatic `.bash_logout` processing differ from a true login shell.
- Zsh uses a private runtime ZDOTDIR, sources the user's startup files in order,
  restores ZDOTDIR, and adds a `precmd_functions` entry without replacing
  `precmd`. Startup files that deliberately disable further startup processing
  can also disable integration.
- The runtime files are removed after startup, with an exit cleanup as a
  fallback. Existing startup files are never rewritten.
- An unsupported shell, refused probe or refused integration exec falls back
  to the normal SSH shell request. The existing manual OSC 7 setup remains
  available for forward following; automatic reverse sync fails closed when
  this session's integration cannot prove prompt readiness.
- To keep ordinary shell startup entirely, disable both directions and
  reconnect. Re-enabling integration for an already open ordinary shell also
  requires reconnecting. There is no command injection into a live prompt to
  install integration.

The automated checks use isolated local Bash/Zsh PTYs, a real loopback SSH PTY,
and the workbench's scripted transports. They do not constitute acceptance on
every server, user profile, shell version or operating system.
