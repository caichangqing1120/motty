# 功能规范：Terminal Folder Sync

**功能编号**：`001-terminal-folder-sync`
**当前版本**：`v1.0.0`
**创建时间**：2026-10-28
**最后更新**：2026-10-28
**状态**：已通过评审
**输入**：Issue #109; user confirmed automatic SSH terminal-to-files sync without editing remote startup files; user additionally requested an opt-in reverse-sync switch.
**版本历史**：查看 [CHANGELOG.md](./CHANGELOG.md)

## 版本信息

- **v1.0.0** (2026-10-28) - Confirmed forward sync and optional reverse sync.

## 目标与范围

- Automatically follow SSH Bash/Zsh directory changes in the attached remote file panel without manual setup. Preserve the existing forward-sync switch (default on).
- Add an independent, persisted Files setting for files-to-terminal navigation (default off).
- Do not modify existing remote startup files. Preserve user prompt hooks. Use private, session-scoped startup shims, removed after startup/session exit.
- No local-terminal, container-command, standalone file-window or cross-host reverse navigation. No new UI surface or dependency.

## 仓库现状

- `internal/term/cwd.go`: OSC 7 parsing already reports absolute percent-decoded paths.
- `internal/workbench/sftp_extra.go`: `terminalDirectory(sessionID uint32, cwd term.Cwd)` only reacts when an existing panel is idle; shell setup is manual and replaces prompt hooks.
- `internal/sshconn/shell.go`: `OpenShell(config ShellConfig) (*Shell, error)` starts a login shell using an SSH shell request, or a supplied `Command` using exec.
- Existing Go 1.26 / Viva native UI, `go test ./...`, development-tag tests, vet and race tests remain authoritative.

## 已选方案与权衡

| 方案 | 范围 | 接口/数据差异 | 迁移代价 | YAGNI 砍项 | 是否采用 |
|---|---|---|---|---|---|
| Session shell integration | Bash/Zsh prompt reports over existing PTY | OSC 7 directory + OSC 133 prompt-ready marker | New default-off JSON bool; no schema migration | No daemon, polling, prompt-text inference or remote persistent changes | Yes |
| Parse typed `cd` or prompt text | Guess current folder from visible text | Incorrect for aliases, failed commands, quoting and nested programs | None | Rejected as unreliable | No |
| Poll processes remotely | OS-dependent process lookup | Additional execs and process identity mapping | None | Rejected as unnecessary load and incomplete shell-readiness detection | No |

Selected: session shell integration, retaining the existing manual OSC 7 fallback. Unknown/unsupported shells or denied probes retain the normal shell request; no automatic command is injected into a running prompt. Bash shims source the normal login profiles in order before installing hooks; Zsh shims source the user's startup files and restore ZDOTDIR. Compatibility limits must be documented and tested, not represented as universal shell support.

## 架构与边界

- `sshconn` probes the login shell with a bounded command; only recognized Bash/Zsh starts with the integration wrapper. Failure to probe or a refused exec falls back to the normal shell request.
- `term` parses OSC 7 and OSC 133 prompt readiness without filesystem access. All workbench state is updated on the window thread.
- `workbench` keeps the latest pending forward location until panel initialization/navigation/editing is safe. Explicit file navigation, not background listing refresh or a forward report, may request reverse sync.
- Reverse sync executes only at a positively reported, untouched shell prompt, outside alternate-screen mode and AI exec. Unsafe requests are skipped with visible feedback, never deferred for surprising later execution.

## 数据与状态

| 实体 | 存储（表/键） | 类型 | 唯一性 | 状态机 |
|---|---|---|---|---|
| Forward preference | existing `sftp_follow` | string; `off` disables | Per saved application state | existing behavior |
| Reverse preference | `sftp_terminal_follow` | bool; absent=false | Per saved application state | off / on |
| Prompt readiness | terminal in-memory | bool | Per terminal instance | unknown/busy -> ready on OSC 133 B; any accepted user input -> dirty/busy |
| Forward target | terminal in-memory | absolute path | Latest report per session | pending -> consumed when safely listed; reports coalesce |
| Reverse target | attached file view in-memory | absolute path | Latest explicit navigation | requested -> listed -> sent or skipped; no retries |

## 接口与数据流

| 方法 | 路径/事件 | 请求/载荷 | 响应/错误码 | 调用方 |
|---|---|---|---|---|
| `OpenShell(config ShellConfig)` | SSH PTY startup | new opt-in integration flag; existing `Command` wins | `*Shell, error`; fallback normal shell on unsupported integration | workbench |
| `SetCwdChange(func(Cwd))` | OSC 7 | `file://host/percent-encoded/path` | absolute decoded directory; malformed/control paths rejected | terminal parser |
| `SetPromptReady(func())` | OSC 133 B | exact prompt-ready event | callback; no filesystem access | terminal parser |
| `terminalDirectory(sessionID uint32, cwd term.Cwd)` | shell -> files | same-session path | coalesced, no cross-session navigation | terminal callback |
| explicit remote navigation | files -> shell | successfully listed absolute path | safely single-quoted `cd -- PATH` at an untouched ready prompt, or skipped | attached file panel |

## 用户故事与验收

### 用户故事 1 - Follow the shell (P1)

After connecting with a normal Bash/Zsh profile, `cd` to a directory and see that directory in the attached file panel without pasting any setup command. If the panel is opened later or busy, the latest report is applied when safe. Disable forward sync to browse independently.

### 用户故事 2 - Optional reverse sync (P1)

Files settings exposes a default-off reverse-sync toggle. After enabling it, entering an accessible remote directory at an empty shell prompt sends exactly one safely quoted `cd` to that same session. The setting survives restart. Shell-driven listing changes never send a reverse command.

### 边缘情况

- Paths with spaces, quotes, percent, `#`, Unicode and shell metacharacters remain data, not executable text; reject relative/control-character paths.
- Running programs, partial input, alternate screens, uninitialized/closed shells, local panes, standalone windows and container sessions receive no automatic command.
- Failed directory reads preserve the existing error behavior and do not send a `cd`.
- Reconnect starts fresh readiness/state. Unsupported shells remain usable and can use the manual OSC 7 mechanism.

## 功能需求

- **FR-001**: Automatic session-local Bash/Zsh reporting with percent-encoded OSC 7, preserving existing prompt hooks and startup-file contents.
- **FR-002**: Coalesce forward reports while blocked; opening the panel later follows the latest reported directory without repeated snap-back after manual browsing.
- **FR-003**: Persist `sftp_terminal_follow` independently of `sftp_follow`, default false; English/Chinese labels and settings search.
- **FR-004**: Reverse only explicit successful remote navigation, once, on the owning untouched ready prompt; never loop on forward navigation or refresh.
- **FR-005**: Safe quoting, no injection into running programs/partial input, no cross-session effects, and visible skip feedback.

## 错误、重试、幂等、鉴权

- No new external API/error code. Existing SSH/SFTP error presentation remains.
- Integration probing is bounded; unsupported/refused probes use the ordinary shell. No automatic retry loops or persistent remote configuration changes.
- Latest-target coalescing and separate forward/reverse origins prevent feedback loops. Reverse requests are consumed once, including skipped requests.
- Use only the authenticated session and its existing SFTP access; no new credentials, privilege escalation or cross-host commands.

## 测试策略

- Real Bash/Zsh PTYs with isolated HOME/ZDOTDIR: automatic reports after `cd`, preserved hooks/profiles, special paths, exit status, cleanup. SSH request tests for probe/fallback and custom-command exclusion.
- Parser split/malformed-input tests; workbench fake SSH/SFTP tests for deferred forward reports, defaults/persistence, explicit navigation, readiness/busy/dirty guards, quoting, errors, independent sessions and no loops.
- `go test ./... -count=1`, dev-tag tests, vet, affected-package race, native/headless builds. No production-host mutations or claims of live user acceptance.

## 成功标准

### SC-U

- **SC-U001**: Normal supported SSH shell `cd` updates the attached file panel without setup.
- **SC-U002**: Reverse synchronization is discoverable, off by default and optional.

### SC-T

- **SC-T001**: Real-shell PTY tests parse the exact target from OSC 7 after `cd` while user hooks still run and configuration files remain byte-identical.
- **SC-T002**: Persisted `sftp_terminal_follow=true` restores true; absent restores false; independent forward off remains off.
- **SC-T003**: One explicit successful navigation writes one quoted command only to its ready owning shell; blocked/failing/forward-driven cases write none.

## 名词对照

| 说法 | 含义 | 本章引用 |
|---|---|---|
| OSC 7 | Terminal escape sequence carrying a file URI for the working directory | FR-001 |
| OSC 133 B | Terminal escape sequence marking the end of a shell prompt, when input can begin | FR-004 |

## 假设与未决

- Supported automatic integration targets the Bash/Zsh Unix shells reflected in the existing setup UI and the issue screenshot; other shells retain the manual fallback.
- User confirmation in this chat approves automatic forward sync; the subsequent request adds the independent opt-in reverse switch. No unresolved product decision.
