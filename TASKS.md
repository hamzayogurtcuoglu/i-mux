# i-mux: Task plan

> Companion to [ARCHITECTURE.md](ARCHITECTURE.md). Section references (§N) point there;
> open questions are `Q1`…`Q29` in ARCHITECTURE §20.
> Last updated 2026-11-06. All tasks are done except the manual (USER) ones: T1.16, T2.13,
> T6.1 and the Q18 part of T5.6.

**Hard requirement:** the user switches a conversation between agents **directly from the UI**,
with one click on the agent bar or one keystroke in the menu. The CLI is secondary. Every
milestone up to v0.1 serves this.

## Conventions

- **Sizes:**
  - **S** ≤ half a day
  - **M** 1–2 days
  - **L** 3–5 days

  Split anything bigger.
- **Task format:** `- [ ] **ID Title** · size · deps`, followed by acceptance criteria (AC).
  A task is done when every AC holds and CI is green.
- **Definition of done (every task):**
  - code has `go vet` / lint clean, with tests in the same PR;
  - UI strings go through `internal/i18n`;
  - no real user data in tests or fixtures;
  - no network access in tests;
  - no real agent CLI is launched by tests.
- **Manual (USER) tasks** need real agent CLIs or a real terminal, and may spend the user's quota.
  The user runs them. Their results update ARCHITECTURE §20/§21.

## Decisions needed before or during M0

| Ref | Decision | Default if not decided |
|---|---|---|
| Q23 | Final name, GitHub owner / Go module path, license, Homebrew tap name | **Decided:** binary `imux`, module `github.com/hamzayogurtcuoglu/i-mux`, MIT license, tap `hamzayogurtcuoglu/homebrew-tap` |
| Q5 | Copilot todos: optional `sqlite3` CLI vs pure-Go SQLite driver | **Decided:** optional `sqlite3` CLI (no driver in the binary) |

## Milestones at a glance

| Milestone | Outcome | Est. |
|---|---|---|
| M0 | Repo, CI, config/state/i18n foundations | ~4 d |
| M1 | All four session parsers + `imux handoff --print` / standalone handoff (F5) | ~3 wk |
| M2 | tmux workspace + **switching from the UI**: clickable agent bar, pane menu, session picker (F1 manual, F3, F6) | ~3 wk |
| **v0.1** | **= M0 + M1 + M2: the user switches a pane's conversation to another agent from the UI, with context** | |
| M3 | Watcher, limit detection, `ask`/`auto`/`off`, limit markers in the agent bar (F1 auto, F2 part 1) | ~1.5 wk |
| **MVP** | **= M0 + M1 + M2 + M3** | |
| M4 | `imux limits` panel (F2 part 2) | ~4 d |
| M5 | Agent bus (F4) | ~1 wk |
| M6 | Native resume-back with delta handoff (D4) | ~1 wk |
| M7 | GoReleaser, install.sh, Homebrew tap | ~4 d |
| M8 | README and docs | ~4 d |

Critical path to v0.1 (UI switching):
T0.2 → T1.1 → T1.2 → T1.5 → T1.12 → T1.14 → T2.1 → T2.4 → T2.6 → T2.8 → T2.8a → T2.8b → T2.8d.

Critical path to the MVP: the v0.1 path → T3.3 → T3.4 → T3.5.

**Acceptance for v0.1:** in a running i-mux workspace, the user clicks "Gemini" on the agent bar
(or presses `Ctrl+\` `g`) while Claude is running. The same pane restarts with Gemini, which
received the handoff, and the agent bar shows "▶ Gemini". The same works from the session picker
for an older conversation. No command is typed.

---

## M0: Repo and CI skeleton

- [x] **T0.1 Record project decisions (Q23)** · S · deps: none
  - AC: the module path, license and tap name are written into ARCHITECTURE §20 (Q23 closed).
  - AC: `LICENSE` file added.
- [x] **T0.2 Go toolchain and CLI skeleton** · S · deps: T0.1
  - `brew install go`, then `go mod init`. Set the `go` directive to the current stable release.
  - Add `cmd/imux/main.go` and the `internal/cli` dispatcher (stdlib `flag`).
  - AC: `go build ./...` succeeds with `CGO_ENABLED=0`.
  - AC: `imux version` prints version, commit and date injected via `-ldflags`.
  - AC: `imux help` lists the commands. Unknown command → exit code 2 with help.
  - AC: the only third-party module in `go.mod` is `github.com/BurntSushi/toml` (D13).
- [x] **T0.3 `internal/paths`** · S · deps: T0.2
  - AC: `IMUX_HOME` overrides `~/.imux`.
  - AC: umask is set to 077 at startup; directories are created 0700 and files 0600. A test asserts the modes.
- [x] **T0.4 `internal/config` (TOML)** · M · deps: T0.3
  - Built-in defaults (§16), merging of user values, validation, plus `imux config init|path|show`.
  - AC: unit tests cover defaults only, partial overrides, per-agent overrides, and an unknown key (warning).
  - AC: an invalid RE2 regex is rejected with its line number. `{prompt}` may appear at most once per template.
  - AC: `config init` writes a commented file and refuses to overwrite it without `--force`.
  - AC: `config show` prints the effective merged config.
- [x] **T0.5 `internal/i18n`** · S · deps: T0.2
  - AC: `T(key, args...)` works with an `en` catalog. Locale comes from config `locale`, then `IMUX_LANG`, then `LANG`.
  - AC: a test fails if code uses a key that is missing from the `en` catalog.
- [x] **T0.6 `internal/state`** · S · deps: T0.3
  - AC: `state.json` writes go temp file → fsync → rename, under `flock(state.lock)`.
  - AC: a test with two concurrent writer processes never produces corrupt JSON or lost updates.
  - AC: unknown top-level fields survive a read-modify-write. A `version` field is present.
- [x] **T0.7 Build tooling** · S · deps: T0.2
  - AC: the Makefile (or justfile) has targets `build`, `test`, `e2e`, `lint`, `bench`, `fuzz-short`.
  - AC: the golangci-lint config enables vet, staticcheck, errcheck, gofumpt and misspell.
- [x] **T0.8 GitHub Actions CI** · M · deps: T0.7
  - AC: lint and `go test -race ./...` run on `ubuntu-latest` and `macos-latest`.
  - AC: an e2e job installs tmux (apt / brew) and runs `go test -tags e2e ./e2e/...`. It is a placeholder until M2.
  - AC: the job fails if the stripped binary exceeds 15 MB.
- [x] **T0.9 Test infrastructure and fixture privacy guard** · S · deps: T0.2
  - AC: `internal/testutil` provides a temp `IMUX_HOME`, fake agent homes and a frozen clock.
  - AC: `scripts/check-fixtures.sh` fails if `testdata/` contains `/Users/` or a `/home/` path other than `/home/test`.
  - AC: it also fails on real-looking secrets (same regexes as T1.11) or email addresses. It runs in CI.

## M1: Session parsers and `imux handoff --print`

Earliest standalone value: works without tmux (F5).

- [x] **T1.1 Session model and Provider interface** · S · deps: T0.2
  - AC: the types from §6 live in `internal/session`, with a registry `session.Providers()`.
  - AC: the doc comments state the "never panic, warnings instead" contract.
- [x] **T1.2 `session/jsonl` bounded reader** · M · deps: T1.1
  - Head window, tail window (starting at the first full line), reverse line scanner, per-line size cap.
  - AC: tests cover window boundaries that split a line, CRLF, an empty file, and a final line
    without a newline (file being written).
  - AC: a 17 MiB line is skipped with a warning.
  - AC: the benchmark reaches ≥ 100 MB/s tail scan on the dev machine.
- [x] **T1.3 `internal/agent` registry and binary resolution** · M · deps: T0.4, T0.6
  - IDs, display names, install hints, default templates (§7.5), and `{prompt}/{id}/{uuid}` expansion.
  - Resolution uses `LookPath` and captures `PATH` into the bin cache (§9.1).
  - AC: config `bin` overrides win. A symlinked binary keeps its *unresolved* dir for `PATH` prepend.
  - AC: version probing (`<bin> --version`, 5 s timeout, the whole process group killed on timeout) is cached in state and **never** run with a prompt.
  - AC: table tests cover template expansion, including a prompt containing quotes, newlines and `$()`.
- [x] **T1.4 `handoff/clean`** · S · deps: T1.1
  - AC: data-driven tag stripping (§7.1 list).
  - AC: `<command-name>`/`<command-args>` become `/name args`.
  - AC: a `<task-notification>` becomes a one-line notice.
  - AC: middle truncation is rune-safe (tests with Turkish `ğüşıöç` and emoji at the cut points), with an omitted-count marker.
- [x] **T1.5 Claude provider** · L · deps: T1.1, T1.2, T1.4
  - Find: by id, by encoded cwd, fallback scan by `"cwd"`; honors `CLAUDE_CONFIG_DIR`.
  - Load: implements every rule in §7.1.
  - AC: synthetic fixtures and tests cover:
    - string and block content;
    - consecutive one-block assistant lines merged by `message.id`;
    - usage counted once per `message.id`;
    - sidechain and meta lines skipped;
    - both error shapes excluded from entries, with shape 1 captured in `Errors`;
    - compaction with and without `preservedMessages`, with the summary several lines after the boundary;
    - the latest `TodoWrite` wins;
    - slash commands and `<local-command-stdout>` dropped;
    - `<synthetic>` model ignored;
    - unknown line types and malformed JSON lines counted in `Warnings`;
    - chained-handoff first message detected.
  - AC: the `FuzzClaudeLoad` fuzz target runs 60 s in `fuzz-short` without panics.
- [x] **T1.6 Gemini provider** · M · deps: T1.1, T1.2
  - Project-id resolution: `projects.json`, then sha256 dir, then `.project_root` scan.
  - Parsers for `.json` and for `.jsonl` (header + `$set` patches + bare message lines, keyed by id).
  - AC: fixtures cover both directory layouts and both file formats.
  - AC: `toolCalls` map to `ToolCall` (status `error` → `Failed`).
  - AC: `info`/`warning` messages become notices; `error` messages go to `Errors`.
  - AC: string vs `[{text}]` content handled; non-text parts become `[attachment]`.
  - AC: a fuzz target exists.
- [x] **T1.7 Copilot provider** · M · deps: T1.1, T1.2
  - `workspace.yaml` top-level scalar reader, events parser, `toolCallId` → success mapping,
    `report_intent` skipped, todos via optional `sqlite3` CLI (Q5).
  - AC: the fixture covers every event type in §7.3.
  - AC: a session dir without `events.jsonl` is skipped by Find.
  - AC: a missing `sqlite3` gives a warning only.
  - AC: `cwd` comes from `workspace.yaml`, else from `session.start.data.context.cwd`.
  - AC: a fuzz target exists.
- [x] **T1.8 Codex provider** · M · deps: T1.1, T1.2
  - Find: date-directory walk newest first with early stop (by `--since`), matching cwd from
    `session_meta`; honors `CODEX_HOME`.
  - AC: the transcript comes from `response_item` only (an `event_msg` duplicate test proves no doubles).
  - AC: `<environment_context>` / `<user_instructions>` user messages are skipped.
  - AC: JSON-string `arguments` are decoded; argv `command` arrays are handled.
  - AC: `token_count.rate_limits` is parsed when present and ignored when absent.
  - AC: a fuzz target exists.
- [x] **T1.9 `handoff/toolsum`** · M · deps: T1.1
  - AC: table-driven tests over the §8.4 rules, using each agent's real tool names:
    - Claude: Bash, Read, Edit, Write, Grep, Glob, WebFetch, WebSearch, TodoWrite, `mcp__srv__tool`;
    - Gemini: the §7.2 list;
    - Copilot: view, rg, bash, create, edit, report_intent;
    - Codex: `shell` argv, apply_patch headers.
  - AC: `FileActivity` classification is correct, failed calls are suffixed, and paths are shown relative to cwd.
- [x] **T1.10 `internal/gitinfo`** · S · deps: T0.2
  - AC: branch, `status --short` (capped at 40 lines plus "+N more") and `diff --stat` (capped at 25).
  - AC: a 2 s timeout per git call. A non-repo and a missing git binary both return a typed result, not an error.
  - AC: tests use a temp repo created in the test.
- [x] **T1.11 `handoff/redact`** · S · deps: T0.2
  - AC: positive and negative tests per pattern (§8.5). Ordinary identifiers such as `skip`, `sk_test_var` and `AKIAlike` in prose are not mangled.
  - AC: output is `[REDACTED:<kind>]`.
- [x] **T1.12 Handoff builder** · L · deps: T1.4, T1.9, T1.10, T1.11
  - Sections, budget allocator (§8.3), newest-first Conversation fill, omitted counts, chained
    carry-over, reason line, delta mode, argv byte guard.
  - AC: golden files (`-update` flag) cover: short session, long session (truncation), compacted
    session, chained handoff, chain of a chain (original request preserved), delta mode, empty
    transcript, and a non-git cwd.
  - AC: property tests (random sessions) show:
    - the rune count is ≤ `budget_chars` and the UTF-8 byte length is ≤ `max_arg_bytes`;
    - line 1 is always the marker; output is valid UTF-8;
    - output is deterministic for a fixed clock.
- [x] **T1.13 `internal/launch` core** · S · deps: T1.3
  - LaunchSpec → argv/env, argv-size check, `syscall.Exec`.
  - AC: argv is never passed through a shell. `PATH` is prepended correctly.
  - AC: a spec over `max_arg_bytes` is rejected before exec.
- [x] **T1.14 `imux handoff` and `imux sessions` commands** · M · deps: T1.5–T1.8, T1.12, T1.13
  - AC: flags as in §13.
  - AC: the file is written to `handoffs/` with mode 0600; `--print` writes to stdout and warnings to stderr.
  - AC: without `--print`, the target CLI is exec'd in the current terminal (F5); in tests it is a fake agent.
  - AC: `--from` defaults to the newest session of any agent for the cwd.
  - AC: `imux sessions` lists agent, id, updated time, size and a first-request preview (40 chars).
  - AC: handoffs older than `keep_days` are pruned at most once a day.
- [x] **T1.15 Large-fixture generator and benchmarks** · S · deps: T1.5, T1.12
  - AC: `scripts/gen-large-fixture` creates a synthetic 64 MB Claude session at test time; it is not committed.
  - AC: `imux handoff --print` on it takes < 1 s with peak RSS < 80 MB (§17).
- [ ] **T1.16 (USER) Quality review on real data** · S · deps: T1.14
  - The user runs `imux handoff --print --from claude` (and gemini/codex) on their own sessions
    and judges whether the result is usable. Nothing is committed from this.
  - AC: findings are filed as issues, and parser or budget tweaks are made.

## M2: tmux workspace and manual switch

- [x] **T2.1 `internal/tmux` wrapper** · M · deps: T0.2
  - Socket and config flags (`IMUX_TMUX_SOCKET`), command batching with `;`, delimiter-based
    output splitting, typed errors (no server / no session / no pane), version parsing.
  - A **command recorder** (`IMUX_TMUX_RECORD=<file>`) is used by tests.
  - AC: unit tests use a fake runner. An integration test runs against a real tmux on a random socket.
- [x] **T2.2 tmux.conf generator** · M · deps: T2.1, T0.4
  - AC: the golden test matches §9.2. The file is regenerated only when its content hash changes, and the
    running server reloads via `source-file`.
  - AC: `tmux -L <test> -f <generated> start-server` loads without errors on CI tmux versions (≥ 3.2).
  - AC: the user's `tmux.local.conf` is sourced last.
- [x] **T2.3 `imux __exec <spec>`** · S · deps: T1.13
  - AC: the spec file is deleted after reading.
  - AC: `IMUX_PANE`, `IMUX_AGENT` and `PATH` are set.
  - AC: e2e: the fake agent receives byte-exact argv, including quotes, newlines, `$()`, backticks and Turkish text.
- [x] **T2.4 Start and attach** (`imux [agent]`, `imux attach`, `imux ls`) · M · deps: T2.1–T2.3, T1.3
  - Session naming (§13), pane options (§9.4), `remain-on-exit on`, and a pre-assigned Claude `--session-id`.
  - Bin cache refresh, `__watch ensure` (a no-op until M3), exec into `tmux attach`.
  - AC: e2e shows the pane options are set and the fake claude got `--session-id <uuid>`.
  - AC: a second `imux` in the same cwd attaches instead of creating a new session.
  - AC: inside i-mux, `imux gemini` opens a new window.
- [x] **T2.5 Pane trail and session discovery for a pane** · M · deps: T2.4, T1.5–T1.8
  - AC: trail JSON is capped at 10 entries.
  - AC: discovery order is pre-assigned id → newest file with mtime ≥ `@imux_start` and matching cwd →
    exclude files claimed by other panes' trails.
  - AC: e2e with two fake-claude panes in the same cwd resolves each correctly.
- [x] **T2.6 `imux switch`** · M · deps: T2.5, T1.12
  - AC: `@imux_lock` blocks a concurrent second switch (with a message).
  - AC: the source is parsed and the handoff file written **before** `respawn-pane -k`. A parse or write
    failure leaves the old agent running.
  - AC: options and trail are updated, and a `display-message` reports the result.
  - AC: e2e: fake claude → fake gemini; the gemini argv contains the handoff with the marker and the expected sections.
- [x] **T2.7 `inject = "paste"` mode** · S · deps: T2.6
  - AC: waits for the screen to be stable (1.5 s, max 20 s), then `load-buffer` + `paste-buffer -p -d` + Enter.
  - AC: e2e: the fake agent reading stdin receives the full document.
  - AC: the prompt does not appear in argv.
- [x] **T2.8 Menu (`__menu`) and key bindings** · M · deps: T2.6, T2.2
  - AC: items follow §9.3. Not-installed agents are disabled; limited agents get the suffix.
  - AC: the recorder shows the expected `display-menu` arguments.
  - AC: the preview popup and the limits popup item are wired (limits shows "coming soon" until M4).
  - AC: Close pane works.
  - AC: menu latency is < 100 ms p95 (§17), measured in e2e.
- [x] **T2.8a Agent bar (clickable second status line)** · M · deps: T2.2, T2.8
  - Generate `status-format[1]` (§9.3) from `order` and the bin cache, with `#[range=user|imux:<x>]` regions.
    Add the `MouseDown1Status` binding that falls back to `switch-client -t =` for other ranges.
  - AC: golden test of the generated format.
  - AC: every range argument is ≤ 15 bytes (a test enforces it).
  - AC: the active pane's agent is highlighted by tmux formats alone; changing focus runs no i-mux process (recorder shows none).
  - AC: not-installed agents are dimmed; `@imux_lim_<agent>` markers render next to the names.
  - AC: with `tmux.agent_bar = false` there is a single status line and no binding.
  - AC: at 80 columns the overflow rule (Q29) applies.
- [x] **T2.8b `imux __click`** · S · deps: T2.8a, T2.6
  - AC: `imux:<agent>` → `switch` in-process for an installed, non-limited agent.
  - AC: a limited agent → a confirm menu; a not-installed agent → an install hint.
  - AC: `imux:cur` / `imux:menu` → pane menu; `imux:sessions` → picker.
  - AC: a plain-shell pane starts the agent there, with context from the trail if available.
- [x] **T2.8c Session picker (`__menu --sessions`, `__menu --session`, `imux pick`, `prefix S`)** · M · deps: T2.8, T1.5–T1.8
  - AC: shows up to 9 rows per page across installed agents for `@imux_cwd`, newest first, with "More…" paging.
  - AC: the preview is the first user request, cleaned, one line, 40 characters.
  - AC: the second-level menu offers resume (own agent, if supported), continue in other agents
    with context, preview, and new-pane variants.
  - AC: `imux switch --from-session <agent>:<id>` builds the handoff from the picked conversation.
  - AC: < 150 ms p95 with 50 synthetic candidate files (§17).
- [x] **T2.8d UI e2e: real client, simulated keys and clicks** · M · deps: T2.8a–T2.8c
  - The test starts a pty-backed `tmux attach` client on the test socket at a fixed size (e.g. 120×40).
    It writes key bytes (`Ctrl+\` = `0x1c`, then `g`) and SGR mouse sequences
    (`ESC[<0;X;YM` / `ESC[<0;X;Ym`) at the computed column of "Gemini" on the agent bar.
  - AC: the click switches the fake-claude pane to fake-gemini, whose argv log holds the handoff.
  - AC: the key path does the same.
  - AC: right-click on a border opens the menu for *that* pane (Q28).
  - AC: picker → row → "Continue in Codex" launches fake-codex with a handoff built from the picked session.
- [x] **T2.9 `imux split`** · S · deps: T2.4
  - AC: `-h`/`-v`, same cwd, pane options set, and the label shows in the border.
- [x] **T2.10 Pane-died handling** · M · deps: T2.4
  - AC: an agent exit falls back to the login shell in `@imux_cwd` with `@imux_agent` cleared and the trail closed.
  - AC: `on_exit = "close"` kills the pane instead.
  - AC: `@imux_fallback` + exit within 10 s with status ≠ 0 respawns from the fallback spec. M6 uses this; it is tested here with a fake agent.
- [x] **T2.11 Scrollback fallback transcript** · S · deps: T2.6
  - AC: when no session is found, the handoff uses `capture-pane -S -3000 -J` text, marked as
    reconstructed, and the user is notified.
  - AC: e2e with a fake agent that writes no session file.
- [x] **T2.12 `imux doctor`** · M · deps: T2.2, T1.3
  - AC: it checks:
    - tmux presence and version (≥ 3.2), and the `tmux-256color` terminfo;
    - each agent: installed, path and version, and an "outdated" hint (e.g. Claude 1.0.x);
    - nvm `PATH` issues and `~/.imux` permissions;
    - watcher status, and whether the imux binary path is writable by others.
  - AC: it prints key hints (VS Code `Ctrl+\`). Exit code is non-zero on a hard failure.
  - AC: it never launches an agent with a prompt.
- [ ] **T2.13 (USER) Manual terminal checklist** · S · deps: T2.8
  - Covers Q1 (`Ctrl+\` in VS Code, iTerm2, Terminal.app, Ghostty), Q27 (clicking the agent bar
    and menus with the mouse in the same terminals), Q29 (layout at 80 columns),
    Q20 (Shift+Enter in Claude and Gemini), Q19 (first-run dialogs keep the prompt) and
    Q22 (switch mid-session loses nothing visible).
  - AC: results are recorded in ARCHITECTURE §20/§21. Defaults change if needed (e.g. key, inject mode).
- [x] **T2.14 M2 e2e suite** · L · deps: T2.3–T2.11
  - AC: covers start, attach, split, manual switch (argv and paste), menu items, pane-died fallback and
    scrollback fallback, on Linux and macOS CI.

## M3: Watcher and limit-triggered switch

- [x] **T3.1 Limit pattern sets** · S · deps: T0.4
  - AC: the §10.1 defaults have `limit` / `warn` / `ignore` severities.
  - AC: config can add patterns or replace the defaults.
  - AC: tests run real-world strings against each severity, including Claude's ` ∙ ` separator and the Opus fallback (ignore).
- [x] **T3.2 Reset-time parser** · M · deps: T0.2
  - AC: table tests cover:
    - `∙ resets 3pm`, `resets 3:45pm (Europe/Istanbul)`, `reset at 15:00`;
    - the legacy `|<epoch>` form;
    - `resets Nov 8, 9am`, `try again in 2 days 3 hours`;
    - rollover past midnight and a DST transition;
    - unparseable text → zero value.
  - AC: `time/tzdata` is embedded. A fuzz target exists.
- [x] **T3.3 Watcher lifecycle** · M · deps: T0.6, T2.1
  - AC: `__watch run|ensure|stop|status`.
  - AC: the singleton uses a pidfile `flock`; `ensure` either starts the daemon (`Setsid`, detached stdio) or sends SIGUSR1.
  - AC: the watcher exits 0 when the i-mux server is gone.
  - AC: `watch.log` rotates at 1 MB.
  - AC: e2e: killing the server ends the watcher within one tick; a second `ensure` does not start a second watcher.
- [x] **T3.4 Tick and detector** · M · deps: T3.1, T3.3
  - Batched list and capture, hash skip, ANSI strip, grace, seen-set, cooldown, idle back-off when detached.
  - AC: unit tests with a fake tmux runner assert ≤ 2 execs per tick and 0 captures when no client is attached.
  - AC: grace, seen and cooldown behave as in §10.
- [x] **T3.5 Actions (`ask` / `auto` / `off`)** · M · deps: T3.4, T2.6, T2.8
  - AC: `ask` shows `display-menu` on the right client (the one showing the session, else the most recently
    active one) with a title containing the reset time.
  - AC: `auto` picks the first installed, non-limited agent in `order`.
  - AC: `off` notifies only. If every agent is limited, it notifies only.
  - AC: the `state.json` limit record follows §15.
- [x] **T3.6 Status bar and pane flag** · S · deps: T3.4
  - AC: `@imux_status` and `@imux_flag` are set only when they change. Expired limits are cleared on the next tick.
  - AC: `warn` shows `◐`.
- [x] **T3.7 Switch reason from the session** · S · deps: T1.5, T2.6
  - AC: a manual switch whose source's last `APIError` is limit-like reports reason "limit" in the handoff.
- [x] **T3.8 M3 e2e** · L · deps: T3.5, T3.6
  - The fake agent prints a limit line after 3 s.
  - AC: `auto`: the fake target's argv holds a handoff whose reason mentions the limit and reset time.
  - AC: `ask`: the recorder shows the expected `display-menu`. A real attached client (pty) on the test socket
    shows the menu (Q24).
  - AC: a limit line printed within the grace period → no action. A repeat line → no second prompt (seen-set + cooldown).
  - AC: the status bar shows the limit, which clears after expiry (with a fake clock).
- [x] **T3.9 Performance measurement** · S · deps: T3.8
  - AC: `scripts/measure-watch.sh` reports max RSS and mean CPU for both the attached and detached cases.
  - AC: the numbers are recorded in `docs/PERF.md` and are within §17. If not, an issue with the control-mode plan exists (Q21).
- [x] **T3.10 The sidebars bring a dead watcher back** · S · deps: T3.3, T10.1; D86
  - The watcher can die with the tmux server living on (an `imux quit` that stopped it before
    failing to close the server, a crash), and a client that stayed attached runs `client-attached`
    never again: every sidebar checks the watcher's lock every 15 s (`IMUX_WATCH_CHECK`) and at
    startup, and starts the watcher when it finds it gone.
  - AC: `NewWindow` ensures the watcher, as Start, Restore and attach do.
  - AC: e2e: a watcher killed with the server alive comes back on a sidebar's check with a new pid.

### ── MVP cut (M0–M3) ──

At this point the core story works end to end: start agents in i-mux, see limits in the status bar,
get the "continue in …" prompt, and switch manually or automatically with context, in the same pane.

## M4: Limits panel

- [x] **T4.1 Claude 5-hour usage** · M · deps: T1.2, T1.5
  - AC: files modified in the window are reverse-scanned until timestamps fall outside it.
  - AC: usage is deduped by `message.id`.
  - AC: the 64 MB fixture is handled in < 300 ms.
- [x] **T4.2 Gemini usage today** · S · deps: T1.6
  - AC: requests and tokens across all project dirs with chats modified today, in the local time zone.
- [x] **T4.3 Copilot premium requests this month** · S · deps: T1.7
  - AC: per session, the latest `session.shutdown` or `session.usage_checkpoint` total, summed per calendar month.
- [x] **T4.4 Codex rate limits** · S · deps: T1.8
  - AC: shows the newest `rate_limits` (percentages and resets) when present, otherwise "n/a (update Codex)".
- [x] **T4.5 `imux limits` rendering** · S · deps: T4.1–T4.4, T3.6
  - AC: table output, `--json` (schema documented) and `--pager`.
  - AC: the tmux popup is wired into the menu.
  - AC: the CodexBar hint appears in the footer.

## M5: Agent bus

- [x] **T5.1 Target resolution and `imux panes`** · M · deps: T2.1
  - AC: `%N` or an agent name, resolved in the caller's session first, then server-wide. Ambiguity → error listing the candidates.
  - AC: idle/busy and "seconds since change" come from two captures 1 s apart, or from the watcher's
    hash timestamps when it is running.
  - AC: `--json` is supported.
- [x] **T5.2 `imux read`** · S · deps: T5.1
  - AC: `-n` lines, ANSI stripped, wrapped lines joined (`-J`).
- [x] **T5.3 `imux send`** · M · deps: T5.1
  - AC: the sender prefix comes from `TMUX_PANE` / `IMUX_AGENT`.
  - AC: a 0600 temp file → `load-buffer` → `paste-buffer -p -d` → Enter.
  - AC: guards per §12 (enabled, size, rate, no self-send, i-mux socket only); `--wait-idle`; `bus.confirm` menu.
- [x] **T5.4 `imux wait`** · S · deps: T5.1
  - AC: exit 0 when idle, 124 on timeout. `--print` outputs the final screen.
- [x] **T5.5 `imux snippet`** · S · deps: T5.3, T5.4
  - AC: prints CLAUDE.md, GEMINI.md and AGENTS.md variants.
  - AC: a test asserts that no code path writes to those files.
- [x] **T5.6 M5 e2e and sandbox check** · M · deps: T5.1–T5.4
  - AC: two fake agents exchange messages, including a multi-line one received intact.
  - AC: the rate limit stops a ping-pong loop.
  - AC: (USER) Q18: whether a sandboxed Codex/Gemini can call `imux send` is recorded.
  - Status (2026-11-06): the automated part is done; the Q18 sandbox check is still open (USER).

## M6: Native resume-back with delta handoff

- [ ] **T6.1 (USER) Verify resume semantics** · S · deps: none (can run any time)
  - Covers Q2 (Gemini `--resume <uuid> -i`), Q6 (Claude 2.x `--resume <id> "<prompt>"`, append vs
    fork), Q14 (Codex `resume <id> "<prompt>"`), Q3 (Copilot flags).
  - AC: the §7.5 template table is updated with the results.
- [x] **T6.2 Delta builder over multi-hop trails** · M · deps: T1.12, T2.5
  - AC: for a trail `A → B → C → A`, the delta concatenates B and C entries after A's end.
  - AC: the delta budget applies. An empty delta means resume without a prompt.
- [x] **T6.3 Resume launch path** · M · deps: T6.1, T6.2, T2.10
  - AC: the resume is chosen only when the trail has the agent with an id, the same cwd and an existing file.
  - AC: the fallback spec (full handoff, new session) is written to `@imux_fallback`.
  - AC: `--no-resume` and per-agent `resume = false` disable it.
- [x] **T6.4 Post-resume session discovery** · S · deps: T6.3
  - AC: the trail entry records the session that is actually written to, whether the CLI appends or forks.
- [x] **T6.5 M6 e2e** · M · deps: T6.3, T6.4
  - AC: fake claude → fake gemini → fake claude gives argv `--resume <id> "<delta>"`, and the delta contains only gemini's part.
  - AC: a resume that fails fast (exit 1 in < 1 s) respawns with the full handoff.

## M7: Distribution

- [x] **T7.1 GoReleaser config** · M · deps: T0.2
  - AC: `CGO_ENABLED=0`, darwin/linux × amd64/arm64, `-trimpath -s -w`, version ldflags, tar.gz archives and `checksums.txt`.
  - AC: `goreleaser release --snapshot --clean` works locally and in CI.
- [x] **T7.2 Release workflow** · S · deps: T7.1, T0.8
  - AC: a `v*` tag publishes a GitHub release with the artifacts.
- [x] **T7.3 `install.sh`** · M · deps: T7.2
  - AC: detects OS and arch (and WSL), finds the latest release through the `/releases/latest`
    redirect (no API rate limit; API as fallback), downloads it, verifies sha256, and installs to
    `~/.local/bin` (or `--prefix`).
  - AC: adds the install directory to PATH in the profile of the user's shell (zsh, bash, fish,
    else `~/.profile`) exactly once; `--no-modify-path` only prints the line.
  - AC: if tmux is missing, it offers to install it via brew / apt / dnf / pacman / apk, with confirmation (`--yes` to skip).
  - AC: shellcheck clean, works under `sh` (POSIX).
  - AC: never runs `sudo` without saying so first.
- [ ] **T7.4 Homebrew tap** · S · deps: T7.1 · (USER: create `hamzayogurtcuoglu/homebrew-tap`, add the
  `HOMEBREW_TAP_GITHUB_TOKEN` secret and the `HOMEBREW_TAP_ENABLED=true` variable; until then the
  release skips the formula upload)
  - AC: the formula is generated by GoReleaser with `depends_on "tmux"`.
  - AC: `brew install OWNER/tap/imux && imux doctor` works on macOS arm64 and amd64.
- [x] **T7.5 Release smoke test** · S · deps: T7.2–T7.4
  - AC: CI runs the README's one-line installer (`curl … | sh`) in a clean environment on macOS
    (arm64 and Intel, zsh), Ubuntu (x86_64 and arm64, bash) and Debian, Ubuntu 22.04, Fedora, Alpine
    and Arch containers; a new shell finds `imux` on PATH; `imux version` and `imux doctor --ci`
    (which skips the agent checks) pass.
  - Note (2026-11-08): on Intel macOS the job installs without tmux. Homebrew has no Intel bottles
    of tmux and its libraries and built them from source, 25-50 minutes every release; the job
    keeps testing the darwin_amd64 archive, the installer and PATH. Every smoke job has a timeout.

## M8: Docs and README

- [x] **T8.1 README** · M · deps: MVP
  - AC: pitch, demo GIF (recorded with fake or real agents, no private content), install
    (brew / curl), quick start, keys, `on_limit` modes, privacy section (cross-provider disclosure, redaction, argv).
  - Status (2026-11-06): written; the demo GIF is still to be recorded (needs a real terminal).
- [x] **T8.2 `docs/CONFIG.md`** · S · deps: T0.4
  - AC: generated from the built-in defaults by `go generate`; CI fails if it is stale.
- [x] **T8.3 `docs/HANDOFF.md`** · S · deps: T1.12
  - AC: the format spec plus a synthetic example document.
- [x] **T8.4 `docs/AGENT-BUS.md`** · S · deps: T5.5
  - AC: commands, target syntax, the snippet, and security notes (prompt injection, `bus.confirm`).
- [x] **T8.5 `docs/TROUBLESHOOTING.md`** · S · deps: T2.12, T2.13
  - AC: covers VS Code `Ctrl+\`, the nvm PATH, extended keys / Shift+Enter, terminfo, outdated CLIs, and "session not found".
- [x] **T8.6 CHANGELOG and CONTRIBUTING** · S · deps: none
  - AC: CONTRIBUTING states the fixture privacy rules and the "never launch real agents in tests" rule.
- [x] **T8.7 Turkish locale (optional)** · M · deps: T0.5
  - AC: a `tr` catalog with full key coverage (test-enforced) and a `README.tr.md`.


## M9: Antigravity CLI (added 2026-11-06)

Google stopped serving personal Google accounts in Gemini CLI on 2026-06-18 (ARCHITECTURE §7.6).

- [x] **T9.1 `antigravity` agent** · M · deps: T1.3, T2.6
  - AC: `agy` is a registered agent (menu key `a`, bar short name `Ag`), after Claude in the
    default order; new conversations start with `agy -i <handoff>`.
- [x] **T9.2 Agent-bar range fits tmux's limit** · S · deps: T2.8a
  - AC: every agent's range is ≤ 15 bytes (`imux:agy` for Antigravity); a test covers every
    registered agent, and the e2e test clicks "Antigravity" with a real mouse event.
- [x] **T9.3 Transcript reader** · M · deps: T1.1, T1.2
  - AC: `internal/session/antigravity` reads `brain/<id>/.system_generated/logs/transcript_full.jsonl`
    (turns, tools with mapped argument names, failed tools, tokens, task list, chained handoff) and
    finds the directory through `history.jsonl`; synthetic fixtures only.
  - AC: e2e round trip Claude → Antigravity → Codex → Antigravity: the handoff from Antigravity is
    built from the transcript (not the scrollback), and the way back resumes with
    `--conversation <id> -i <delta>`.
- [x] **T9.4 Remove Gemini CLI** · M · deps: T9.1
  - AC: no Gemini agent, provider, patterns or doctor check; a config naming `gemini` loads with a
    warning; tests and screenshots use Antigravity.
- [x] **T9.5 Chained handoffs keep headings in the user's text** · S · deps: T1.12
  - AC: reading a handoff back splits only at its own section headings, so a request with `## `
    lines survives every hop whole (unit test, plus a check on a real handoff).
- [ ] **T9.6 (USER) Verify agy for real** · S · deps: T9.1–T9.3
  - One real round trip Claude → Antigravity → Claude → Antigravity (Q30), and note agy's quota
    message when it appears (Q31).

## M10: Sidebar UI (added 2026-11-07)

The user found the bottom agent bar awkward and splitting unclear; the primary UI becomes a
sidebar (ARCHITECTURE D17, §9.3 "Sidebar").

- [x] **T10.1 Sidebar model and rendering** · M
  - AC: a pure `Render(model, width, height)` draws the mockup in §9.3; golden tests for typical,
    narrow, limited, not-installed and many-pane states; Unicode width handled.
- [x] **T10.2 Terminal input** · M
  - AC: raw mode via termios (darwin, linux), alternate screen, SGR mouse; a parser for keys,
    arrows, Enter, Esc and mouse press/release with unit tests; hit-testing maps a click to a row.
- [x] **T10.3 `imux __sidebar` process** · M · deps: T10.1, T10.2
  - AC: redraws on SIGUSR1, SIGWINCH (keeping its width) and a 15 s safety tick; actions focus,
    switch (with context), split (agent chooser menu), sessions, limits, keys; focus returns to
    the target pane; exits when it is the window's last pane.
- [x] **T10.4 Workspace integration** · M · deps: T10.3
  - AC: every new imux window gets a sidebar (`tmux.sidebar`); `prefix b` toggles them;
    `prefix v` / `prefix -` split with an agent chooser; commands and menus given the sidebar pane
    act on the target pane; the sidebar is never listed as a pane by `imux panes`, the bus or the
    watcher.
- [x] **T10.5 Pane state** · S · deps: T3.4
  - AC: the watcher sets `@imux_state` working/idle on change and signals the sidebars.
- [x] **T10.6 E2E with a real client** · M · deps: T10.4
  - AC: the sidebar shows the panes and targets; a real mouse click on "Antigravity" switches the
    target pane with context; "Split right" + an agent opens a new pane with it; `prefix b` hides
    and shows the sidebar; closing the last agent pane closes the window.
- [x] **T10.7 Docs and screenshots** · S · deps: T10.6
  - AC: README keys and screenshots show the sidebar; CONFIG.md lists the new keys.

## M11: Friendlier sidebar (added 2026-11-07)

User feedback on v0.4 (ARCHITECTURE §9.3 "Sidebar v2", §8.2a).

- [x] **T11.1 Wait on arrival** · S
  - AC: `handoff.on_arrival = "wait"` (default) asks the new agent for a one-sentence
    acknowledgement and to wait; `continue` keeps the old text; golden tests for both.
- [x] **T11.2 Makers and plain headings** · S
  - AC: agent rows show Anthropic / Google / GitHub / OpenAI; headings read as plain words.
- [x] **T11.3 Pane names** · M
  - AC: `imux rename [--pane] <name>`; ✎ in the sidebar and `n` ask with `command-prompt`; the name
    shows in the sidebar and the pane border; quotes in names are safe.
- [x] **T11.4 CHANGES** · M
  - AC: git status files (≤ 8 + "+N more") with a diff popup per file; hidden outside git.
- [x] **T11.5 PROJECTS** · M
  - AC: all imux sessions with their agents; click switches project or focuses an agent;
    **+ Open a project…** with `imux open <dir>`.
- [x] **T11.6 E2E and docs** · M
  - AC: real-client tests for project switching, renaming and a diff popup; README, screenshots.
## M12: Quiet handoffs and sidebar v3 (added 2026-11-07)

User feedback on v0.5: the handoff showed as a wall of text, and the sidebar should look more like
an agent workspace with an agents section (ARCHITECTURE D18, §8.2b, §9.3 "Sidebar v3").

- [x] **T12.1 Short handoff message** · M
  - AC: `handoff.deliver = "file"` (default) sends the marker, one localized sentence and the path;
    `--add-dir=` for Claude, Copilot and Antigravity; delta, fallback, paste and `imux handoff`
    use it too; `inline` keeps the old text; unit tests for the message and argv.
- [x] **T12.2 Chained handoffs through the file** · S · deps: T12.1
  - AC: `ParseChained` follows the path on the message's last line (handoff files only); an e2e
    chain Claude → Codex → Claude keeps the original request.
- [x] **T12.3 Sidebar v3** · M
  - AC: PROJECTS, AGENTS cards, + New agent (auto direction), CHANGES, MOVE THIS CHAT TO without
    the current agent, a bottom toolbar of two-button rows with per-button clicks and keyboard
    selection; height trimming as in §9.3; golden tests.
- [x] **T12.4 E2E, docs, screenshots** · M · deps: T12.1–T12.3
  - AC: existing sidebar e2e tests pass on v3; README, CONFIG.md and screenshots updated.

## M13: Agent workspace sidebar (added 2026-11-07)

User request with a reference screenshot (ARCHITECTURE D19, §9.3 "Sidebar v4").

- [x] **T13.1 States: Needs you, Done, times** · M
  - AC: the watcher writes `blocked` for an idle screen that asks a question (unit tests with
    synthetic screens of each agent's prompt and of plain output) and `@imux_state_at` on every
    change; the sidebar writes `@imux_seen` for the pane being looked at and shows Done until then.
- [x] **T13.2 Projects and Agents** · M
  - AC: avatars, branches, dirty dots, roll-up states; agents of all projects grouped by project
    with icons, names (pane name, agent title, agent), times and states; + buttons in the headers.
- [x] **T13.3 Continue block and toolbar** · M
  - AC: switch buttons without the current agent; Rename, changed-files menu with diffs, History,
    Limits, Help; spinner animation without tmux execs; golden tests.
- [x] **T13.4 E2E, docs, screenshots** · M · deps: T13.1–T13.3
  - AC: the sidebar e2e tests on v4 (switch, new agent, rename, open project, diff via the files
    menu, a Needs you state from a fake agent's question); README, CONFIG.md, screenshots.

## M14: Tabs and restart on upgrade (added 2026-11-07)

User request: tabs like the reference workspace, and an old version that kept running after an
upgrade (ARCHITECTURE D20, D21, §9.3 "Tabs").

- [x] **T14.1 Restart on upgrade** · S
  - AC: the watcher and the sidebars exec the new binary when the imux file changed (complete,
    executable, not being written); the watcher rewrites the tmux config on start; unit tests.
- [x] **T14.2 Tab bar** · M
  - AC: `tmux.tabs` (default on): status line at the top with tabs (number, name, state glyph,
    current highlighted) and a **+**; clicks select tabs; the conf golden test covers it.
- [x] **T14.3 New tab, names, states** · M
  - AC: `imux tab <agent>`; **+** and `prefix c` ask for the agent; `prefix ,` names the tab; the
    sidebar keeps `@imux_tab_auto` and `@imux_tab_state` up to date (only on change).
- [x] **T14.4 E2E and docs** · M · deps: T14.2, T14.3
  - AC: a real click on **+** opens a tab with the chosen agent and a sidebar; the bar shows both
    tabs with their names; a click on the first tab goes back; README, CONFIG.md, screenshots.

## M15: Open and close projects easily (added 2026-11-07)

User feedback: "why does i-mux show up on its own", "how do I close a project", "finding the path
in Open project is hard" (ARCHITECTURE D22, D23, §9.3 "Open and close projects").

- [x] **T15.1 Folder candidates** · M
  - AC: recent folders in state (written on start); git repositories under home with the skip list
    and budgets; ranking and path browsing as pure functions with unit tests on a temp tree.
- [x] **T15.2 Picker UI** · M · deps: T15.1
  - AC: a filter line, a list with the selection, Tab completion, Enter, Esc; UTF-8 input,
    Backspace, Ctrl+U; golden render tests; used by the sidebar's **+**, `o`, `imux open` without
    a folder and `imux` in the home folder.
- [x] **T15.3 Close projects and tabs** · M
  - AC: `imux close`; project menu (click the current project, right-click any) with a confirm;
    × on the current tab with a confirm; the client moves to another project first.
- [x] **T15.4 E2E and docs** · M · deps: T15.2, T15.3
  - AC: real-client tests: open a project by typing part of its name in the picker; close a
    project from its menu (the client lands on the other one); close a tab with ×; README.

## M16: Recent projects and restore (added 2026-11-07)

User request: a VS Code-like Recent list that reopens a project as it was last closed
(ARCHITECTURE D24, §9.3 "Recent and restore").

- [x] **T16.1 Snapshots** · M
  - AC: `workspace.json` written by the watcher (30 s, attached) and on Close; open/closed by
    server identity; unit tests with a fake tmux.
- [x] **T16.2 Restore** · M · deps: T16.1
  - AC: tabs, panes, names, layout and native resume per agent; fresh when no session; opening a
    folder with a snapshot restores it; Restore last session restores every project of an earlier
    server.
- [x] **T16.3 Picker sections** · S · deps: T16.1
  - AC: Restore row, RECENT with details, ON THIS MAC; headers not selectable; golden tests.
- [x] **T16.4 E2E and docs** · M · deps: T16.2, T16.3
  - AC: a project with two tabs and a named pane is closed, reopened from the picker and comes
    back with both tabs, the name and each fake agent resumed with its session id; README.

## M17: Agent menu (added 2026-11-07)

User request: rename an agent from its row and close an agent (ARCHITECTURE §9.3 "Open and close
projects", agent menu).

- [x] **T17.1 Agent menu** · M
  - AC: right-click on an agent row, or a click on the selected one, opens Go to / Rename… /
    Close agent…; rename prefills the name; close asks first and closes the tab or project when it
    was the last agent; unit tests for the sidebar actions and the close rules.
- [x] **T17.2 E2E and docs** · S · deps: T17.1
  - AC: real-client tests: right-click → Rename names the pane; right-click → Close removes it;
    README.

## M18: Plain terminals (added 2026-11-07)

User request: the + menus should offer a plain shell too (ARCHITECTURE §9.3, plain terminals).

- [x] **T18.1 Terminal in the menus** · S
  - AC: split and tab menus end with Terminal (shell); `imux split shell [-h|-v]` and
    `imux tab shell` open the shell in the project folder with a sidebar for a new tab; unit
    tests for the menus; an e2e test opens a shell tab and a shell split from the menus.

## M19: Folding sidebar, source control, clear limits (added 2026-11-07)

User feedback with a screenshot (ARCHITECTURE D25, §9.3 "Sidebar v5").

- [x] **T19.1 Folding and contrast** · S
  - AC: section titles fold / unfold on click, shared through `@imux_fold`; brighter colors;
    golden tests.
- [x] **T19.2 SOURCE CONTROL** · M
  - AC: branch, changed files with +/− lines, diff on click, "+N more" opens the full menu; the
    Changes button is gone; unit tests for numstat parsing.
- [x] **T19.3 Claude percentages** · M
  - AC: `--settings=` for Claude launches; `imux __statusline claude` records `rate_limits` and
    chains the user's status line; unit tests with synthetic status-line input (both timestamp
    forms, missing windows).
- [x] **T19.4 Limits panel** · S · deps: T19.3
  - AC: bars and percentages per window for Claude and Codex, plain words for the others;
    golden test; README.

## M20: Copilot quota and clearer limit texts (added 2026-11-07)

- [x] **T20.1 Copilot monthly quota** · S
  - AC: the newest `quotaSnapshots.premium_interactions` of Copilot's session files shows as a
    monthly bar with "N of M premium requests" and the reset date; unlimited plans say so; unit
    tests with synthetic events.
- [x] **T20.2 Clear waiting texts** · S
  - AC: "send it a message in imux: the percentages show after its first reply" for Claude,
    Codex and Copilot; Antigravity: "keeps its quota to itself; a limit hit shows here".

## M21: Activity bar, explorer, search and diffs (added 2026-11-08)

User request with a screenshot of an editor (ARCHITECTURE D26, §9.3 "Sidebar v6").

- [x] **T21.1 Diff viewer** · M
  - AC: whole-file diffs with line numbers and colored lines, side by side when wide; untracked
    and deleted files; starts at the first change; golden tests.
- [x] **T21.2 File viewer** · S
  - AC: `imux __view [--line N] <file>` with bat or less; unit test for the command it builds.
- [x] **T21.3 Search** · M
  - AC: smart-case search over the project's files with the caps; a popup UI with live results
    and Enter to open; unit tests on a temp tree.
- [x] **T21.4 Activity bar and views** · L · deps: T21.1–T21.3
  - AC: rail with Agents, Explorer, Search, Source Control, Limits, Help; explorer tree with
    folding, git colors and scrolling; source control view; `@imux_view`; golden tests.
- [x] **T21.5 E2E and docs** · M · deps: T21.4
  - AC: real clicks: explorer opens a folder and a file; source control opens a diff with the
    whole file; search finds a word and opens it; README, screenshots.

## M22: Bigger rail, full source control, editing, mouse (added 2026-11-08)

User feedback (ARCHITECTURE D27, §9.3 "Sidebar v7").

- [x] **T22.1 Git operations** · M
  - AC: `internal/gitops` (status split, numstat per side, ahead/behind, history, stage, unstage,
    discard, commit with smart staging, checkout, new branch) with tests on temp repos; `imux __git`.
- [x] **T22.2 Source control view** · L · deps: T22.1
  - AC: the layout in §9.3 with every button; discards confirm; push/pull/sync popups; branch menu;
    history with commit view; golden tests.
- [x] **T22.3 Editing** · M
  - AC: editor choice and line argument per editor; explorer click, search Enter and the viewers'
    `v` edit; right-click file menu with VS Code; unit tests for the editor command.
- [x] **T22.4 Rail and mouse** · M
  - AC: emoji rail with labels (compact on short windows); `less --mouse`; wheel in the popups;
    search results fold; golden tests.
- [x] **T22.5 E2E and docs** · M · deps: T22.1–T22.4
  - AC: real clicks: stage a file, commit it with a message, discard a change (confirmed); a file
    opens in the editor; search folds a file; README.

## M23: Editor icons and the rail's line (added 2026-11-08)

- [x] **T23.1 Icon themes** · S
  - AC: `tmux.icons` auto | nerd | emoji; auto from installed Nerd Fonts and the terminal; unit
    tests for the detection and the nerd theme's widths.
- [x] **T23.2 Separator** · S
  - AC: a line between the rail and the view on every row; default width 41; golden tests.

## M24: Closing the side panel (added 2026-11-08)

- [x] **T24.1 Toggle from the rail** · S
  - AC: a click on the active view's icon closes the panel (pane width 6, rail without its line);
    any view's icon opens it with that view; `@imux_collapsed` per window; the width is kept
    while closed; unit tests for the closed rail.
- [x] **T24.2 E2E and docs** · S · deps: T24.1
  - AC: real clicks close and open the panel; the agent pane grows and shrinks; README.

## M25: OpenCode (added 2026-11-08; removed in 0.19, back in 0.20)

User request: an agent that works with any model endpoint (ARCHITECTURE D28, §7.7). Taken out
in 0.19 and put back in 0.20 at the user's request: its `/connect` adds an NVIDIA or Groq key
without an account.

- [x] **T25.1 Agent and launch** · S
  - AC: registry entry (key `e`), templates with an imux-chosen `ses_` id, resume; unit tests.
- [x] **T25.2 Session provider** · M
  - AC: Find via sqlite3 (cwd, since, id), Load via `session export`: text, tools, model, tokens,
    chained handoffs; tests with synthetic exports and a synthetic database.
- [x] **T25.3 Limits, doctor, UI** · S
  - AC: limit patterns; panel row with recent tokens; doctor warns about an unwritable state
    folder; sidebar icon; i18n.
- [x] **T25.4 E2E and docs** · M · deps: T25.1–T25.3
  - AC: a fake opencode; Claude → OpenCode → Claude round trip keeps the context; README.

## M26: Buttons you can see and hit (added 2026-11-08)

- [x] **T26.1 Icons and sizes** · S
  - AC: every sidebar button uses the theme's icon (codicons or emoji) and takes ≥ 3 columns;
    lines keep their width in both themes; unit tests.
- [x] **T26.2 Tab bar and e2e** · S · deps: T26.1
  - AC: the tab's × takes 3 columns (tmux 3.4 gap kept); goldens; e2e clicks still stage,
    discard and close; README.

## M27: Commit message suggestions (added 2026-11-08)

User request: like the editor's "generate commit message" (ARCHITECTURE D29).

- [x] **T27.1 Collect, prompt, clean** · M
  - AC: `internal/commitmsg`: staged or work-tree changes with untracked files, 48 KB cut,
    redaction, recent subjects; the prompt; reply cleaning; unit tests with a temp repository.
- [x] **T27.2 Agents** · S
  - AC: argv / stdin / output file per agent (Claude, Codex, Copilot, Antigravity, OpenCode) from
    their `--help`; agent choice (`git.commit_agent`, `commit_model`); unit tests.
- [x] **T27.3 UI** · M · deps: T27.1, T27.2
  - AC: ✨ button; `__git suggest` with the first-use question, progress message, the offer menu
    (Commit, Edit…, Write another, Cancel) and `__git edit-message`; i18n.
- [x] **T27.4 E2E and docs** · M · deps: T27.3
  - AC: headless modes in the fake agents; real clicks: ✨ → consent → menu → commit with the
    suggested message, the fake got the diff; the real OpenCode with a fake model; README.

## M28: Pinned conversations (added 2026-11-08)

- [x] **T28.1 Pins** · S
  - AC: `state.pinned`; Pin to the top / Unpin in a conversation's menu; the picker lists the
    folder's pinned conversations first (📌, looked up by id), then the rest; unit tests.
- [x] **T28.2 E2E and docs** · S · deps: T28.1
  - AC: pin an old conversation from the menu, it comes first; unpin; README.

## M29: Settings and AI completion in the editor (added 2026-11-08)

User request: a Settings place next to Help with switches, and Copilot-like completion while
editing a file (ARCHITECTURE D30).

- [x] **T29.1 Config writer** · S
  - AC: set one key in config.toml keeping comments and other lines; creates the file from the
    commented default; sidebars and the watcher reload a changed config; unit tests.
- [x] **T29.2 Settings menu** · M · deps: T29.1
  - AC: ⚙ on the rail and `imux settings`; rows with values; cycling and the commit-agent
    submenu; reopens; i18n; unit tests.
- [x] **T29.3 AI completion** · M
  - AC: `[completion]`; the imux-nvim profile and copilot.vim setup with Neovim / Node.js checks
    and the sign-in; edits open Neovim in the profile when on; unit tests with fake commands.
- [x] **T29.4 E2E and docs** · M · deps: T29.2, T29.3
  - AC: real clicks: Settings toggles icons and the arrival mode in the file; with completion on
    an edit runs nvim with the profile (a fake nvim); the real Neovim loads copilot.vim; README.

## M30: Rename on the row, pins you can find (added 2026-11-08)

User report: the toolbar's Rename named the wrong pane ("Claude does not rename"), and the pin
in History was not found.

- [x] **T30.1 Toolbar and rows** · S
  - AC: no Rename in the toolbar (History alone); e2e renames from the agent's row.
- [x] **T30.2 Pins in the list** · S
  - AC: "📌 Pin or unpin…" (p) in the picker opens the rows for pinning; aligned agent column;
    `<pasted_content>` tags out of previews; unit and e2e tests.

## M31: Icons that draw, Settings, updates (added 2026-11-08)

User report: a friend's terminal drew "?" for the icons; "Config" should be "Settings"; people
should not have to update by hand.

- [x] **T31.1 Icons** · S
  - AC: `auto` from the terminal's actual font (Terminal.app, iTerm2, VS Code; Ghostty, WezTerm,
    kitty); `@imux_terminal`; unit tests with sample profiles.
- [x] **T31.2 Rail** · S
  - AC: 9-column rail, "Settings"; default width 43; tests.
- [x] **T31.3 Updates** · M
  - AC: `imux update [--check]`; the watcher's daily check (`update_check`); the rail and the
    Settings row show a newer release; unit tests without network.
- [x] **T31.4 Releases** · S
  - AC: GoReleaser from the module proxy in the release workflow.

## M32: Notifications (added 2026-11-08)

User request: notifications on every platform i-mux supports, switchable in Settings.

- [x] **T32.1 Events** · S
  - AC: finished (after ≥ 10 s of work) and needs-you transitions; not while the user looks
    at the pane; one per pane per 30 s; unit tests.
- [x] **T32.2 Delivery** · M
  - AC: command, osascript, notify-send, gdbus, terminal OSC 9 / 777 with tmux message; argument
    escaping; unit tests per platform with fake commands.
- [x] **T32.3 Settings and e2e** · S · deps: T32.1, T32.2
  - AC: `[notify] enabled, sound, command`; Settings rows; e2e on macOS and Linux: a fake agent
    works then stops → the command gets the title; Linux without a desktop → OSC 777 on the
    client; README.

## M33: Pasting images (added 2026-11-08)

User request: Ctrl+V a copied screenshot into any agent, every time, on macOS, Linux and WSL.

- [x] **T33.1 Clipboard** · M
  - AC: images (and copied image files) from macOS, Wayland, X11 and WSL; a new file per paste;
    unit tests with fake tools; a real X11 clipboard in the Linux container; macOS for real.
- [x] **T33.2 Keys and delivery** · M · deps: T33.1
  - AC: Ctrl+V / Alt+V bindings that fall through outside agent panes or without an image;
    📎 in the sidebar; bracketed paste of the path (`@path` for Antigravity); `[paste] images`
    in Settings; doctor line.
- [x] **T33.3 E2E and docs** · S · deps: T33.2
  - AC: e2e with a fake clipboard tool: Ctrl+V twice gives two files in the agent's input; no
    image passes Ctrl+V through; README.

## M34: Explorer like the editor's (added 2026-11-08)

User request: right-click on files and folders gives the editor's menu (paths, relative paths,
everything similar); what the pointer is over lights up.

- [x] **T34.1 Hover and selection** · M
  - AC: motion reports (`?1003h`) parsed; the item under the pointer drawn with a background in
    every view, redrawn only on change; cleared on the edge columns and when the pointer was on
    its way out (no motion for 150 ms, the last step would leave); the selected explorer row kept;
    unit tests on parsing, painting and the way out.
- [x] **T34.2 Menus and file operations** · L · deps: T34.1
  - AC: file, folder and empty-space menus at the pointer with the items of §9.3 "Explorer
    menus"; `imux __file` (new-file, new-folder, rename, delete → Trash, cut / copy / paste,
    copy-path, copy-relative-path, reveal, terminal, side, compare, add-to-agent, collapse); paths
    through options; `@imux_reveal`; `__diff --compare`; unit tests with temp folders.
- [x] **T34.3 E2E and docs** · M · deps: T34.2
  - AC: e2e on macOS and Linux: hover background on a row, kept at rest, gone on the way out; right-click
    menus list the items; New File, Rename, Delete (into `IMUX_TRASH_DIR`), Copy / Paste, Copy
    Relative Path (`IMUX_COPY_COMMAND` and the tmux buffer) do what they say; README.

## M35: Go to File (added 2026-11-08)

User request: find a file in the open project the way the editor's Ctrl+P does; in the Files
view too. Key chosen by the user: Ctrl+P (it is taken from shells and OpenCode in their panes).

- [x] **T35.1 Finder** · M
  - AC: fuzzy matching and ranking of §9.3 "Go to File" with unit tests (name over folders,
    consecutive, word starts, shorter paths, smart case, parts, `:line`); empty query by change
    time; the popup with ↑↓, wheel, click, Enter, Esc; highlighted letters.
- [x] **T35.2 Key, button, e2e and docs** · S · deps: T35.1
  - AC: `quick_open_key_root` (default `C-p`) bound in tmux.conf; the explorer's title button;
    the pane's project folder (the sidebar's target on the sidebar); e2e: Ctrl+P in an agent pane,
    type, Enter opens the file in the editor at the line; the button; README.

## M36: A panel you can widen, whole file names in Source Control (added 2026-11-09)

User request: in the Git view the changed files show as `…omation-framework/`,
`…ls/.env.example`: paths cut at their start hide the names; and the panel cannot be made wider
by dragging its edge (it snaps back).

- [x] **T36.1 File names** · S
  - AC: §9.3 "Resizing the panel, file names in Source Control": the name first, its folder in
    gray, the folder cut first, then the middle of the name; unit tests at the default width and
    wider; every row exactly the panel's width.
- [x] **T36.2 Dragging the edge** · S · deps: T36.1
  - AC: a dragged width is kept (20-80), saved to `tmux.sidebar_width` once the drag rests and
    taken by the other tabs' panels and new tabs; a resized window, a closed neighbor and the
    closed rail keep the width (unit test against a real tmux); e2e: a real mouse drag on the
    border through tmux; README.

Status: done in b7dbff8 ("Drag the panel wider; file names first in Source Control"); the boxes were ticked on 2026-11-21.

## M37: A project without agents, the tab bar on tmux 3.2 (added 2026-11-09)

User report: closing the agents closed the project ("I did not close the project"); it should
stay and say that it has no agents. And the + of the tab bar does nothing (tmux 3.2a, Ubuntu
22.04's).

- [x] **T37.1 The tab bar's + and × before tmux 3.4** · S
  - AC: §9.3 "A project without agents, the tab bar on tmux 3.2": status-left / status-right
    ranges and their MouseUp1 bindings below 3.4, golden configs; the tab e2e tests pass on tmux
    3.2a.
- [x] **T37.2 "No agents"** · M · deps: T37.1
  - AC: Close agent… on the project's last agent and the last pane going otherwise leave the
    "No agents" pane; its keys and clicks start an agent or a terminal in its place, and so do
    `imux split` and the + of AGENTS; the sidebar lists the project with "No agents" and offers
    nothing to continue; snapshots skip it; another tab's last agent still closes its tab; unit
    tests (drawing, Gather against a real tmux) and e2e.

Status: done in 1236452 ("Closing the last agent keeps the project; tab + on tmux 3.2"); the boxes were ticked on 2026-11-21.

## M38: Versions and updates in Settings (added 2026-11-09)

User request: a place in Settings that shows the versions of what i-mux runs on (tmux was old, a
CLI was old) and updates them, or at least gives the command and a warning.

- [x] **T38.1 The list** · S
  - AC: §9.3 "Versions and updates": i-mux, tmux, the agents in config order with the version read
    now; tmux's thresholds (3.2 needed, 3.4 agent bar clicks, 3.5 menus with the mouse) and the
    package manager's command, apt's candidate compared by upstream version; each agent's own
    updater (Codex: npm or Homebrew by its path; OpenCode 1: `upgrade --major`); unit tests.
- [x] **T38.2 Settings row and popup** · S · deps: T38.1
  - AC: the row (with the tmux warning) opens `imux versions --popup`; a letter runs the command
    with the terminal, the list is read again and the bin cache takes the new versions; e2e with
    the fake agents' updaters; README.

Status: done in 1ee892a ("Versions and updates in Settings"); the boxes were ticked on 2026-11-21.

## M39: Named conversations in History (added 2026-11-09)

User request: agents can be named in the sidebar, but History does not show those names, so a
conversation is hard to find again ("I forget which name I gave"); maybe rename in History too.

- [x] **T39.1 Names on conversations** · S
  - AC: §9.3 "Named conversations": `state.json` names by conversation key; a named pane's trail
    runs take its name (on rename, when History opens, before an agent, tab or project closes, on
    `on_exit = "close"`); History rows show the name before the preview; unit tests.
- [x] **T39.2 Rename in History, the name back** · S · deps: T39.1
  - AC: the conversation menu's Rename… (options, never the prompt's template, carry the key and
    the pane); the open agent running it takes the name; continuing a named conversation names its
    pane; e2e through the real menu and prompt.

Status: done in 77b20cf ("Named conversations in History"); the boxes were ticked on 2026-11-21.

## M41: Sidebars come back (added 2026-11-09)

User report: after v0.36.0 was installed, the left panel was gone everywhere. Cause: the build was
copied over the running binary in place; macOS killed every imux process (Code Signature
Invalid), and nothing restored the sidebars. The skills feature itself was not the cause.

- [x] **T41.1 Restore and remember off** · S
  - AC: `sidebar off` sets `@imux_sidebar_off`, `on` clears it; the watcher restores a sidebar
    in a window seen without one (and not off) in two checks in a row; unit tests; e2e: a killed
    sidebar comes back, one turned off does not, a new tab never gets two.
- [x] **T41.2 Safe local install** · XS
  - AC: `make install-local` (a new file, then `mv`); CONTRIBUTING and AGENTS.md say never to copy
    over a running binary.

## M45: Clicks at once, no cursor in the sidebar (added 2026-11-10)

User report: the text cursor sometimes shows in the left panel, and clicks between panels feel
slow (about a second).

- [x] **T45.1 Clicks at once** · S
  - AC: D36, §9.3 "Clicks at once": a click on a view, a folder, a fold or an agent of the window
    acts at once; menus, popups, prompts and other windows or projects wait `OpenAfter`;
    `DoubleClick1Pane` and `TripleClick1Pane` keep tmux's defaults except over a sidebar (no
    `select-pane`), listed in `@imux_keys`; unit tests (which clicks wait; the config: golden,
    mouse off, loads in a real tmux); e2e: a double click on an agent keeps its pane focused (fails
    without the bindings), a double-clicked menu stays open.
- [x] **T45.2 Synchronized output** · XS
  - AC: D37: `terminal-overrides` gets the DECSET 2026 `Sync` once (guarded by `@imux_sync`, also
    on reload); checked on tmux 3.2a that background updates arrive between `\e[?2026h` and
    `\e[?2026l`; the config test checks it is there once after reloads.

## M111: Approve everything (added 2026-11-26)

User request: a setting that makes Claude, Copilot and the other agents approve every tool call
themselves, in their interactive panes too, not only in Jobs' headless runs (D109).

Shipped in v0.74.0 and removed in v0.74.1 at the user's request (D109): Copilot still asked under
the organization's policy, so it did not help where it was wanted.

- [x] **T111.1 The switch on every launch** · S
  - AC: D109: `auto_approve` (top level, default false) in the config, its template and
    docs/CONFIG.md; `agent.ApproveArgs` gives each CLI's switch (Claude Code and Antigravity
    `--dangerously-skip-permissions`, Copilot `--allow-all`, Codex
    `--dangerously-bypass-approvals-and-sandbox`, OpenCode `--auto`); the workspace's launch spec
    and `imux handoff` put it before the agent's `args` when on, once (not when `args` already
    has it), and nothing when off; Jobs unchanged. Unit tests (each agent's argv on and off, a
    resume, the switch already in `args`, the handoff's argv).
  - Status: done. `Info.ApproveFlag` and `agent.ApproveArgs`; `WS.Spec` and `imux handoff`;
    `TestApproveArgs`, `TestSpecAutoApprove`, `TestHandoffAutoApprove`.
- [x] **T111.2 Settings → Agents → Approve all** · S · deps: T111.1
  - AC: D109: the row (`y`) shows Off or On ⚠; its menu has Off and On ⚠ (approve everything) and
    a line saying it applies to agents started after; `__settings set auto_approve on|off` writes
    the key; i18n (en, tr); unit tests (the row, the menu, the key written).
  - Status: done. `approveLabel`, `SettingMenu("auto_approve")`, `settingKeys`;
    `TestSettingsMenu`, `TestSettingMenus`, `TestSetSetting`.
- [x] **T111.3 Docs** · XS · deps: T111.2
  - AC: GUIDE, README and README.tr, CHANGELOG.
  - Status: done (GUIDE Settings, README and README.tr Configuration, CHANGELOG Unreleased).
- [x] ~~**T111.4 Check each CLI (USER)**~~ · XS · deps: T111.1
  - Dropped with the feature (Q33 closed): Copilot 1.0.116 with `--allow-all` still asked,
    "Organization policy requires confirmation".
- [x] **T111.5 Remove Approve all** · S
  - AC: D109: the Settings row, its menu and i18n keys, `auto_approve`, `agent.ApproveArgs` and
    the launch changes are gone; a config with `auto_approve` loads with no warning
    (`removedKeys`, top-level keys too); GUIDE, README, README.tr, docs/CONFIG.md, CHANGELOG
    (Removed). Unit test: the removed key is quiet, a mistyped one still warns.
  - Status: done. The feature commit reverted; `removedKeys` checked for every undecoded key;
    `TestRemovedKeysQuiet`.

## M110: Several agents together (added 2026-11-25)

User request (from a brainstorm): the agents should work together: several on one task, keeping
the best result (D105); another CLI reviewing what an agent just did (D106); one task going
through several agents in turn (D107); and one prompt to many agents at once (D108).

- [ ] **T110.1 Arena: worktrees and start** · M
  - AC: D105: `imux arena new "<task>" --agents a,b[,c,d] [--name N] [--env E]` takes two to
    four installed agents; each gets `<repo>.worktrees/arena-<name>-<agent>` on a new branch
    `arena/<name>/<agent>` from HEAD and starts there with the task as its first message (and the
    environment, D101); the projects are marked as the arena's; a name in use is refused, and
    outside git the worktree message shows. Unit tests (paths, branches, argv, environment) with
    git in a temp folder; e2e with the fake agents.
- [ ] **T110.2 Arena: the dialog and the group** · M · deps: T110.1
  - AC: D105: Agents' **+** → **⚔ Arena…** opens a popup: the task (several lines), the
    installed agents to tick (a limited one says until when), the name (a slug of the task,
    editable); Enter starts it. PROJECTS lists the contenders under the repository, grouped
    under `⚔ <name>`, each with its state. i18n (en, tr); unit tests (the dialog, the rows).
- [ ] **T110.3 Arena: compare, pick, clean up** · M · deps: T110.1
  - AC: D105: `imux arena compare <name>` and the group's **Compare…**: per contender its state,
    working time and `git diff --stat` against the base (commits and uncommitted changes), and
    `[arena] test`'s result in its worktree when asked; a row opens its diff, two side by side.
    **Pick <agent>…** (asked first) commits its changes in its worktree and merges its branch
    into the base as D72 does, then offers **Remove the others** (worktrees and projects,
    branches kept). Unit tests (git in a temp folder); e2e: two fake agents write different
    files, one is picked and merged, the other removed.
- [ ] **T110.4 Second opinion: the review run** · M
  - AC: D106: `imux review [--pane P] [--reviewer <cli>]` runs the reviewer headless as Jobs
    run (its read-only flags) in the pane's project, with the fixed prompt over `git diff HEAD`
    and the new files (200 KB at most, else "too large"); the result (`IMUX_LGTM`, else the
    problems and their number) is kept per pane with the turn it reviewed; the reviewer is never
    the agent reviewed. Unit tests with fake CLIs.
- [ ] **T110.5 Second opinion: when a turn ends** · S · deps: T110.4
  - AC: D106: Settings → Agents → **Second opinion** Off / Ask / Auto and the reviewer (another
    CLI, or a fixed one), `[review]` in config.toml; on `working` → idle or Done with
    uncommitted changes the watcher notifies and shows `? review` (Ask) or starts the review
    (Auto), once per turn; nothing runs while Off. Unit tests of the trigger; e2e with a fake
    agent that writes a file.
- [ ] **T110.6 Second opinion: the verdict on the row** · S · deps: T110.4
  - AC: D106: `✓ review` or `⚠ N review` on the agent's row, dimmed once a later turn changed
    the project; a click opens the review in the output view with **Send to <agent>** (pasted
    as the bus pastes, tagged `[review from <CLI>]`, once the agent is idle) and **Dismiss**;
    the agent menu's **Second opinion…** starts one at any time. i18n; unit and e2e tests.
- [ ] **T110.7 Recipes: files and runs** · M
  - AC: D107: recipes in `~/.imux/recipes/*.toml` and `.imux/recipes/*.toml` (steps: `agent`,
    `prompt` with `{task}` `{previous}` `{diff}`, `access`, `model`, `until` and `max`; `[run]
    where`), a file's errors reported with their line; `imux recipe ls|run <name> "<task>"`; a
    run is one `imux __auto exec` running the steps in order as Jobs runs a definition, each
    report going into the next prompt, stopping at a failed step; `until` runs a step again
    while its command fails, `max` times at most. Unit tests with fake CLIs.
- [ ] **T110.8 Recipes: in Jobs** · M · deps: T110.7
  - AC: D107: Jobs lists **RECIPES**; 🚀 opens the Run panel with a **Task** box and when (a
    scheduled recipe keeps its task); the output view shows the steps (`── 2/3 Codex ·
    implement ──`); the report and its notification are a Job's; the three samples are written
    once into `~/.imux/recipes`. i18n; unit tests; e2e: a two-step recipe with fake CLIs passes
    the first step's report to the second.
- [ ] **T110.9 Broadcast: `imux broadcast`** · S · deps: T107.1, T107.2
  - AC: D108: `imux broadcast [--to <target>,…] "<text>"` pastes to each target as the bus does,
    a busy one once it is idle (M107's queue); from the user's terminal it is untagged, from an
    agent's pane it is that agent's message (tag, limits, confirmation); the bus's size limit.
    Unit tests; e2e with the fake agents, one of them working.
- [ ] **T110.10 Broadcast: the popup** · S · deps: T110.9
  - AC: D108: AGENTS' title menu and `prefix B` (`broadcast_key_prefix`) open **Broadcast…**:
    the live agents of every project (the current project's ticked, shells not) under a text
    box; Enter sends; a queued pane shows `⏳ queued`; the last selection is kept. i18n; unit
    tests; e2e.
- [ ] **T110.11 Docs** · XS · deps: T110.3, T110.6, T110.8, T110.10
  - AC: README and README.tr, the user guide, docs/AGENT-BUS.md for broadcasts, and the
    CHANGELOG with each release.

## M109: The workspace from a phone (added 2026-11-24)

User request: see the terminal and control the agents from the phone, at home and away, and
turn it on and off from the sidebar (D104).

- [x] **T109.1 A QR code in the terminal** · S
  - AC: D104: `internal/qr` encodes text (byte mode, versions 1–10, level M, every mask
    scored) and draws it with half blocks, black on white with its quiet zone; a test decodes
    what it draws with an independent decoder (outside the repository) and checks the
    patterns against the standard.
  - Status: done. OpenCV decoded versions 1, 6, 8 and 10 (`QR_DUMP` writes them); the unit tests
    keep the verified "HELLO" code and check finders, timing and both format copies for
    versions 1 to 10.
- [x] **T109.2 The phone server, pairing and sealing** · L · deps: T109.1
  - AC: D104: `imux __phone-serve` (started by `imux phone on`, recorded like the web view's
    server, stopped by `imux phone off`) serves the app and its API on the LAN; `imux phone
    pair` makes a one-time token and key valid for two minutes and shows the QR code; pairing
    stores the device; every later message is sealed with AES-256-GCM, its timestamp within a
    minute and its nonce never seen; the agents and the watched pane's screen go out by SSE,
    keys and text come in by POST; read-only devices cannot send; `imux phone devices`,
    `readonly` and `forget`. Unit tests of sealing, pairing, replay, read-only and the API with
    a fake engine.
  - Status: done (`internal/phone`, `imux phone`). The daemon was also run here in a temporary
    home with its own engine socket: the app and its headers served, a pairing by its token,
    the device listed, the server stopped by `imux phone off`.
- [x] **T109.3 The phone app** · L · deps: T109.2
  - AC: D104: an embedded PWA: pairs from the URL's fragment and keeps the key in the phone;
    lists projects and agents with their states; shows a pane live (colours, at its size);
    buttons for Yes / No / Enter / Esc / Ctrl+C and arrows; a text box; reconnects by itself;
    works in iOS Safari and Android Chrome. The ciphers are noble-ciphers, vendored (MIT).
  - Status: written and served (`internal/phone/web`); its syntax checked here. Not yet tried
    on a real iPhone or Android phone: the owner tries it first. noble-ciphers is 2.4.0's
    `aes.js` as jsDelivr bundles it, with its licence beside it.
- [x] **T109.4 Phone on the rail** · M · deps: T109.2
  - AC: D104: **Phone** on the rail, above Limits, opens the Phone view: on / off, the
    address(es), **Pair a phone…** (the QR code in a popup), the devices with read-only and
    remove. English and Turkish. Tests of the view.
  - Status: done as a popup (like Limits) rather than a sidebar view: on / off, pair (the QR
    code), read-only and forget by number. The rail shows labels only when every item fits with
    one, so Help never falls off a short pane now that the rail has one more item.
- [x] **T109.7 A better phone app** · M · deps: T109.3
  - AC: D110: `hello` carries the desktop's theme (dark, and light when it follows the
    terminal), as hex per token, read at each connection; `watch` takes `lines` (0–500) and the
    screen then has that much scrollback; the keys `BTab` and `4`–`9`; the app: summary and
    "needs you" first, project cards, CLI marks, state with its time, quick answers from the
    screen's numbered choices, key bar, growing message box, A− / A+, back to the bottom,
    Earlier, a banner and vibration when an agent starts to wait; English and Turkish. Only
    `internal/phone` and the server's start change. Unit tests (the theme in hello, the
    colors' hex, lines clamped and passed on, the new keys); the app's syntax checked
    (`node --check`) and its choice parser tested with node.
  - Status: done. `phone.Look`, `LookOf`, `Colors` (`look.go`); `watch` with `lines`;
    `web/choices.js`; `TestMore`, `TestLookOf`. The app was checked here in headless Chromium at
    a phone's size against a fake engine (the list, a pane with Copilot's question as buttons);
    not yet on a real phone.

- [x] **T109.8 A friendlier phone app** · S · deps: T109.7
  - AC: D111: quick prompt chips (defaults in English and Turkish, + saves, a long press
    removes, kept in localStorage); a swipe opens the next or previous agent, "n/m" in the
    header; an offline bar and a notice when a send fails; numbered pairing steps; a home-screen
    tip once, not in standalone mode; a double tap fits the font. Only `internal/phone/web`
    changes. Checked with `node --check` and in headless Chromium at a phone's size.
  - Status: done (`web/app.js`, `app.css`, `index.html`). Checked in headless Chromium at
    390×844 against a fake engine: the pairing steps, the tip, a swipe on the header moving
    from 1/3 to 2/3, + saving a prompt. Not yet on a real phone.

- [ ] **T109.5 Anywhere: relay and notifications** · L · deps: T109.3
  - AC: D104: after a hosting decision by the owner: an outbound relay over HTTPS (through the
    system's proxy) for any network, carrying only sealed messages, and web push when an agent
    needs the user.

## M107: Agents ask each other, across projects and CLIs (added 2026-11-24)

User request: an agent can ask another live agent (in another project, in any CLI) a question or
give it a task, and get the answer back (D102). Builds on the agent bus (M5, docs/AGENT-BUS.md).
No new panel in this version.

- [ ] **T107.1 Targets across projects** · S
  - AC: D102: a target is `<project>/<agent>`, a pane's name, an agent name (the one live pane
    in the caller's project, else the only one on the server) or a pane id; an ambiguous target
    fails and lists the matches; `imux panes` shows each pane's address. Unit tests.
- [ ] **T107.2 `imux ask` and `imux task`** · M · deps: T107.1
  - AC: D102: the request goes to the target's pane as the bus sends it (tagged, size and rate
    limits, never one's own pane), after the target is idle (a queue per pane); the reply is the
    target's first complete assistant reply after the request, read from its session file
    (Claude Code, Antigravity, Copilot, Codex, OpenCode); done when the hooks say so (Claude
    Code, Codex) or screen and session stay unchanged; `task` adds `git diff --stat` of the
    target's project; `--json` and `--timeout`; a task chain deeper than two is refused. Unit
    tests with fake session files; e2e with the fake agents.
- [ ] **T107.3 Confirmation across projects** · S · deps: T107.2
  - AC: D102: a request to another project waits for a menu on the target's client (Deliver /
    Refuse); within one project it goes at once; Settings → Agents → **Agent messages** (On /
    Ask first / Off, `[bus]` in config.toml) sets it. Unit tests; e2e of the menu.
- [ ] **T107.4 `imux mcp`: an MCP server for every agent** · M · deps: T107.2
  - AC: D102: `imux mcp` speaks MCP over stdio with `list_agents`, `ask_agent` and
    `delegate_task`, each a thin layer over T107.1–T107.3, and knows its caller's pane;
    `imux mcp install` lists the CLIs found and adds the server to each one's MCP settings only
    when the user says yes for it, keeping a backup; `imux mcp uninstall` takes it out. Unit
    tests of the protocol and of each CLI's settings with fakes; no real agent is started.
- [ ] **T107.5 Docs** · XS · deps: T107.2–T107.4
  - AC: docs/AGENT-BUS.md, the user guide and the README describe asking and tasks, the
    addresses, the MCP install and the confirmation.

## M106: A new agent with an environment (added 2026-11-24)

User request: an agent opened from Agents' **+** should be able to start with an environment,
instead of being opened and then given one from the Env view (which restarts it) (D101).

- [x] **T106.1 The menus list the environments** · S
  - AC: D101: the place, split and New tab menus end with `🔑 With <name>…` (`1`–`9`) for each
    environment, nothing when there are none; `__menu --place|--split|--tab|--new --env <name>`
    opens a menu for it: the title names it, every choice passes `--env` on (the worktree
    prompt's template too), `🔑 Without <name>…` (`0`) goes back; the New agent menu lists none
    and, with `--env`, drops Web page…; the Env view's menu has New agent with it… (`n`); a
    missing name says so; i18n (en, tr); unit tests (items, keys, titles, commands).
  - Status: done. `envItems`, `withEnv` and `envTitle` in `workspace/sidebar.go`, `__menu
    --env`; `TestMenusOfferEnvironments`, `TestEnvRowMenu`.
- [x] **T106.2 Splits, tabs and worktrees start with one** · S · deps: T106.1
  - AC: D101: `imux split|tab <agent|shell> --env <name>` and `imux worktree new … --env
    <name>`: an agent's `Launch.Env` (`@imux_env` in the exec that makes the pane), a terminal's
    shell through `imux __exec` (`shellCmd`) with `@imux_env`, the "No agents" pane's option set
    before it fills; a name with no file stops before anything starts, a bad name is a usage
    error; unit tests (the engine commands, no value on them).
  - Status: done. `newShell` and `shellSpec` (a terminal), `StartEnv`, and `SwitchReq.Env` so
    `split --from-session --env` continues a conversation with it too; `TestNewPanesWithEnv`,
    `TestFillEmptyWithEnv`, `TestEnvFlag`.
- [x] **T106.3 End to end and docs** · S · deps: T106.2
  - AC: e2e: Agents' **+** → Claude → 🔑 With glm… → Right starts Claude once, with the
    variable, `@imux_env` and the border; a terminal split with `--env` has it (`echo`); no
    value on a tmux command line. GUIDE, README and README.tr, CHANGELOG.
  - Status: done. `TestNewAgentWithEnv` (it fails on 0.71.5's binary, which has no 🔑 With
    glm…). `make ci` passed here on Linux; macOS and Windows are checked by CI only.
- [x] **T106.4 A new project's options on its own pane** · XS · deps: T106.2
  - Found in review (the i-mux-core session): `new-session -d` does not make the new pane the
    current one, so `imux worktree new` (since D72), `imux open` or a restore typed in a pane
    set the new agent's pane options, and now `@imux_env`, on the active pane of that window.
  - AC: D101: `launchNew` and `restoreEmpty` give a new session's options its pane
    (`=<session>:`); splits and windows made without `-d` keep the current pane. Unit tests (the
    batch names the session's pane); e2e: `imux worktree new --env` run with a shell pane's
    `TMUX_PANE` leaves that window's panes alone and sets them on the new project's pane.
  - Status: done. `sessionTarget`; `TestStartBatchesOptions` and `TestRestoreWithoutAgents`
    name the target; `TestWorktreeEnvFromAPane` fails before it (the Claude pane beside the
    shell got `@imux_env glm`).

## M105: Signed releases on Windows and macOS (added 2026-11-23)

User report: a company Windows machine would not install i-mux, an unsigned program (D100).

- [x] **T105.1 Windows: Authenticode signing in the release** · M
  - AC: D100: `scripts/sign-windows.sh FILE...` signs `.exe` files with jsign 7.6 (fetched once,
    checked against its SHA-256) from `IMUX_SIGN_*`, timestamped unless `IMUX_SIGN_TSA=none`, and
    does nothing without `IMUX_SIGN_STORETYPE`; GoReleaser's build hooks, `build-installer.sh`
    (binaries and `setup.exe`) and NSIS's `!uninstfinalize` call it; the release workflow passes
    the repository's variables and secret. CI's snapshot signs with a self-signed certificate
    made on the spot, and its Windows job checks that `setup.exe`, `imux.exe`, `imux-core.exe`
    and `uninstall.exe` carry it. shellcheck clean.
  - Status: done; jsign itself runs only in CI (not on the owner's machine), where the
    snapshot's self-signed run checks the whole path. The uninstaller is signed with `--any`:
    NSIS hands `!uninstfinalize` a temporary file.
- [x] **T105.2 macOS: signing and notarization in the release** · S
  - AC: D100: `.goreleaser.yaml`'s `notarize.macos` signs both darwin binaries with
    `MACOS_SIGN_P12` and notarizes them with the App Store Connect key, only when
    `MACOS_SIGN_P12` is set; `goreleaser check` passes; a local snapshot with a self-signed
    certificate signs them (`codesign -dv` shows it).
  - Status: done, but a self-signed certificate cannot test it: quill wants Apple's chain
    ("unable to find remaining chain certificates"). Checked here: `goreleaser check`, and a
    snapshot with `MACOS_SIGN_P12` empty skips the step (GitHub sets an unset secret as an
    empty variable, so `enabled` tests for a non-empty value, not `isEnvSet`). The first
    signed release (T105.3) is its test.
- [ ] **T105.3 (USER) Certificates and secrets** · S · deps: T105.1, T105.2
  - AC: the owner gets a Windows code-signing certificate or service (SignPath Foundation,
    Certum, Azure Trusted Signing …) and an Apple Developer ID, and sets the variables and
    secrets `docs/SIGNING.md` lists; the next release is signed (`Get-AuthenticodeSignature`
    shows Valid, `spctl -a -vv -t install` accepts the macOS binaries).

## M104: Updates, simpler (added 2026-11-23)

User request: the i-mux row should always have its letter, and i-mux-core needs no row of its
own (D99).

- [x] **T104.1 i-mux always has its letter; no i-mux-core row** · XS
  - AC: D99: `versions.Collect` lists i-mux, then the agents; the i-mux row's `u` runs `imux
    update` in every case, its note unchanged; the engine row, its `t` and its messages are
    gone. Unit and e2e tests updated.
  - Status: done. `engineRow`, `Row.Tmux` and four messages removed (en, tr).

## M103: Updates sees a release out today (added 2026-11-23)

User report: Settings → Updates shows no letter on the i-mux row while a newer i-mux is out
(D98).

- [x] **T103.1 Updates asks GitHub when it opens** · XS
  - AC: D98: `imux versions` (and its popup) asks `update.Latest` with `update_check` on and
    remembers the answer; the i-mux row drops its letter only when that live answer says it is
    the newest; without an answer it keeps the letter. Unit tests of the row with and without a
    live answer, and of the environment with the check off and failing (no network in tests).
  - Status: done. `liveLatest` in `commands_versions.go` (`latestRelease` replaced in
    `TestLiveLatest`), `versions.Env.Live`.

## M102: Windows: adding a machine (added 2026-11-23)

User report (Windows 11, a WSL machine at `user@172.x.x.x`): a machine cannot be added; every
`ssh` i-mux starts fails with `getsockname failed: Not a socket`, because Windows' OpenSSH has
no `ControlMaster` (D97).

- [x] **T102.1 Machines on Windows without a master connection** · S
  - AC: D97: on Windows `machine.Options` has no `ControlMaster`, `ControlPath` or
    `ControlPersist`, and Reconnect and Remove run no `ssh -O exit` (`ssh_share_windows.go`;
    macOS and Linux keep them, their tests unchanged); when `imux machine add` or `reconnect`
    signs in but the background check answers `auth`, the message says the machine needs SSH
    key sign-in on Windows (en, tr). Tests: a Windows test that the options hold none of the
    master's and every other one; the POSIX test keeps the master's options; checked by hand
    from Windows 11 against a WSL machine.

## M101: The theme on Copilot's pane, confirmations that stay (added 2026-11-23)

User report: Copilot's pane stays black under a theme, and closing an agent shows a popup that
vanishes when clicked (D96).

- [x] **T101.1 Programs cannot paint over the theme** · S
  - AC: D96: with a theme on its own colors, `ThemeOptions` ends with `set-option -gq
    allow-default-colours off` on i-mux-core (unset quietly with the i-mux and terminal themes;
    nothing on tmux); the generated config writes it; the pin moves to the i-mux-core with E19.
    Unit tests of the commands and their order.
- [x] **T101.2 A confirmation from a menu stays open** · XS
  - AC: D96: the i-mux-core pin with C12.1; checked by hand here: Close agent… from an agent's
    menu, by mouse, opens the confirmation and its Yes closes the agent.
  - Status: done. Pin at i-mux-core dc10d81 (its E19, C12.1, C12.2). Checked here with a pty
    client on a private engine server: a menu opened from a menu item by mouse stays open
    (released at once or 0.6 s later) and its item runs; on the earlier engine and on tmux 3.8
    it closed unchosen. Not yet seen in the user's own window: the running server keeps its
    engine until i-mux restarts.

## M100: Versions that stay in step (added 2026-11-23)

User report: 0.71.x is out, yet the i-mux installed on the WSL machine was a 0.65.0 development
build from a checkout 76 commits behind, and it said it was the latest (D95).

- [x] **T100.1 A development build is offered newer releases** · XS
  - AC: D95: `update.Newer(latest, cur)` reads `cur` in `git describe` form (`vX.Y.Z-N-gHASH`,
    `-dirty`) as `X.Y.Z` plus commits: a release above `X.Y.Z` is newer, the same or older is
    not; `dev` and other forms stay never older; `latest` must be a release. Unit tests.
- [x] **T100.2 No install or release from a stale checkout** · XS
  - AC: D95: `make install-local` fetches the upstream (offline: says so, goes on) and stops
    when HEAD is behind it unless `STALE_OK=1`; the `commit-and-release` and
    `check-before-push` skills fetch first and check HEAD contains `origin/main` and the new
    tag is above every tag on origin. Checked here with a checkout behind its upstream.
  - Status: done. `scripts/fresh-check.sh` checked in a clone 3 commits behind (stops), up to
    date, offline (uses the last fetch), `STALE_OK=1` and a branch without upstream; it runs
    first in `make ci` too.

## M99: The Nerd Font in every install, icons as the terminal draws them (added 2026-11-22)

User report: a fresh install on a company Windows machine showed `?` for every icon (D94).

- [x] **T99.1 The font ships with i-mux** · M
  - AC: D94: `scripts/fetch-font.sh DIR` puts the four styles and `OFL.txt` in `DIR/fonts`,
    from the Nerd Fonts archive checked against its SHA-256; the release workflow runs it; the
    archives carry `fonts/`, `setup.exe` installs it beside imux, `install.ps1` and `install.sh`
    copy it (`<install dir>\fonts`, `<prefix>/../share/imux/fonts`), Homebrew's formula
    installs it under `share/imux/fonts`, and the Windows `imux update` copies it. `imux font
    install` uses those files (each checked against its own SHA-256) before any download.
    Tests: the bundled files found and checked (a wrong one refused, then the download), the
    archive layout, the scripts' copies.
  - Status: done. Checked here: a GoReleaser snapshot's Windows zip holds `fonts/` (the four
    styles, 2.6 MB each, and `OFL.txt`); `install.sh` from a local fake release copied them to
    `~/.local/share/imux/fonts`, and `imux font install --yes` installed them from there with
    the download pointed at nothing. The NSIS installer and `install.ps1` are checked by CI's
    `windows-installer` job and the release smoke tests.
- [x] **T99.2 Every Windows Terminal profile, and icons as drawn** · M
  - AC: D94: `jsonc` reads and writes array elements (`#N` in a path); `imux font install`
    sets Windows Terminal's defaults and each profile with a non-Nerd `font.face` or
    `fontFace` (a `profiles` object or list), keeping the backup as before; `Ready` needs
    them all. `imux` attaching in Windows Terminal passes `WindowsTerminal:<WT_PROFILE_ID>`;
    with `icons = "auto"` the sidebars take the editor's icons only when that profile's font
    (its own, the defaults', else Cascadia Mono) is a Nerd Font and one is installed in the
    user's or the system's fonts; `imux font install` sets `icons = "nerd"` only on Linux.
    Tests: profiles object and list, legacy `fontFace`, the profile in use, no font
    installed.
  - Status: done. `termfont.WTFontSettings` and `WTFace` read the settings, `Terminal.Nerd`
    and `Use` (with `Profiles`) check and set them; tested with fakes here, not yet on a real
    Windows Terminal.
- [x] **T99.3 Every new version checks the font** · S
  - AC: D94: the first interactive start of each version sets the font up when it is not
    ready (`font.auto` holds `done <time> <version>`; a stamp without the version counts as
    older; a failure waits a day as before); unit tests of the rule.
  - Status: done. Every start was considered first and dropped: it would set back, at each
    start, a terminal font the user chose after i-mux set it.

## M98: A simpler Settings, and every system in mind (added 2026-11-22)

User request: Settings is confusing and its texts too long; and a skill so that a change is
thought through for macOS, Linux and Windows alike, not for one of them.

- [x] **T98.1 Settings in three groups, with short rows** · S
  - AC: D93: the menu's rows, headings, aligned values and keys as D93 lists them, in English
    and Turkish; the Theme menu starts with the light/dark switch, the Icons menu ends with the
    Nerd Font install, the Notifications menu has On, Silent and Off (`notifications` sets
    `notify.enabled` and `notify.sound`), the Editor menu's rows are short and hold the Copilot
    sign-in while Neovim + Copilot is chosen; Updates opens the Versions popup. README,
    README.tr and the user guide name the rows as they now read. Unit tests: the menu's rows,
    each submenu, the `notifications` setting.
  - Status: done. Sixteen rows of sentences became nine short rows under three headings; the
    strings for the removed rows are gone from both catalogs.
- [x] **T98.3 Updates: no letter for a row with nothing to run** · XS
  - AC: in the Versions popup (Settings → Updates) a row with no command (i-mux when it is the
    newest, i-mux-core, which comes with i-mux) shows no letter; typing one says why there is
    nothing to do (up to date; updated with i-mux), and a letter no row has says so. Unit tests
    of the rows and the messages.
- [x] **T98.2 A skill for every system** · XS
  - AC: `.agents/skills/every-system/SKILL.md` (linked from `.claude/skills/`) says what to
    check on macOS, Linux (WSL included) and native Windows before and while making any change:
    the questions to ask, the usual differences (paths, shells and argv, process starts,
    signals, consoles and ConPTY, terminals and fonts, clipboard, notifications, file systems),
    where platform code goes (build tags behind small interfaces, D85), and which tests and CI
    jobs show each system; AGENTS.md points to it.

## M97: Engine queries without a process (added 2026-11-22)

User report: on a Windows machine with endpoint security every process start costs about
270 ms, so a click or `imux ls` took about a second (D92).

- [x] **T97.1 Engine queries inside imux** · S
  - AC: D92: `tmux.Client.Run` runs its commands through `tmux.EngineCommand`
    (i-mux-core's `engine.Command`, pinned at its C11.1) when the engine is an existing
    `imux-core` file, the line starts no server and asks no `-V`, and `IMUX_ENGINE_INPROC` is
    not `0`; anything else, and a line the engine leaves to the binary, runs the binary as
    before. `cmd/imux` sets `tmux.EngineCommand`; test binaries do not. Unit tests: which
    lines run in the process and which run the binary, the exit code and stderr reaching
    `Error` as from the binary; the e2e suite runs on it, the imux under test carrying the
    engine.
  - Status: done. `tmux.EngineRunner` is `New`'s runner (the legacy tmux client keeps
    `ExecRunner`); a non-zero code is an `ExitCodeError`. i-mux-core is pinned at e07d98c
    (its C11.1). On macOS `imux ls` against a running server went from 48.5 ms to 13.8 ms
    (20 runs each, `IMUX_ENGINE_INPROC=0` against the default); on Windows each query saved is
    a whole `imux-core.exe` start.

## M96: Tabs like an editor's (added 2026-11-22)

User request: the tab bar does not look like VS Code's; the open tab should be on the panel's
colour.

- [x] **T96.1 The open tab on the panes' colour** · S
  - AC (D91): the tab bar's strip is `tabs_bg`, a shade darker than the panes (`panel_bg`
    darkened until the two can be told apart; 256-color 234 with the i-mux theme); the open tab
    is on `tab_bg`, the panes' own colour (`panel_bg`, the terminal's with the i-mux theme), its
    name bold `text`; the other tabs `subtext0` on the strip; the blank over the sidebar on the
    panes' colour; `terminal` keeps the bar on the terminal's colour and the open tab on
    `surface0`; the first tab at the pane's edge, the tabs' widths unchanged; unit tests for the colours of every built-in
    theme (the strip apart from the panel, the open tab on the panel) and the golden configs.

## M95: Agents in their project's colour (added 2026-11-22)

User request: in the left panel it is hard to tell which project an agent belongs to; the
projects' avatar colours should show in AGENTS too.

- [x] **T95.1 Project colour in AGENTS** · XS
  - AC: §"Agents": a project's group row shows its letter avatar (the same colour as in PROJECTS),
    its name stays muted; every agent row under it starts with `▎` in that colour, with no change
    to the row's width; agents elsewhere (machines) keep their plain row.
  - AC: §"Projects": a project's colour is the one its name hashes to, or, when a project above it
    has that one, the next free one, so open projects never share a colour while there are
    colours enough, and a project keeps its colour; a worktree has its repository's; a theme whose
    accent is one of its colours lists it once.
  - AC: unit tests: the avatar and the bar match, in classic and in a theme; names whose hashes
    collide get different colours; the first project's colour does not move; worktrees.

## M94: Windows: the font, menus under the mouse, themes and switching speed (added 2026-11-22)

User report (Windows 11, i-mux in a classic console window): the Nerd Font does not work, a
popup closes when the mouse moves over it, changing the theme recolours only the side panels,
and switching panes takes at least a second (D89).

- [x] **T94.1 The Nerd Font in the console window** · M
  - AC: on native Windows `imux font` lists "Windows Console" with the font of `HKCU\Console`
    (the defaults and every key with a font of its own); `imux font install` sets them all,
    keeping the old values once in `~/.imux/console-font.imux-backup.reg` (UTF-16, as regedit
    writes it; it puts them back); `imux` started in a console window sets `@imux_terminal` to
    `conhost:<font>` (`GetCurrentConsoleFontEx`) and, when the registry's font is a Nerd Font
    and the window's is not, switches the window to it; a pseudo console (Windows Terminal,
    VS Code, a pane) is not taken for a console window; the font unpacks with Windows' own
    `tar.exe`. Tests: the console among the terminals with a fake setting; the registry in a
    private key of the test's own (never the user's `HKCU\Console`); `termfont` with
    `conhost:` names.
- [x] **T94.2 Menus stay open under a moving mouse** · S
  - AC: i-mux-core C8.10: an SGR motion ending in `m` (the console's report of the mouse moving
    with no button) is a motion, not a release, so a `-M -O` menu neither closes nor chooses;
    parser test and e2e scenario, failing before the fix; i-mux's pin moves to it.
- [x] **T94.3 A theme recolours the agents' panes** · S
  - AC: i-mux-core C8.10: `window-style`, `window-active-style` and `pane-colours` colour a
    pane's cells in the default colours and its palette, in panes already running, as in tmux;
    render test and e2e scenario, failing before the fix; i-mux's pin moves to it.
- [x] **T94.4 One process per pane switch on Windows** · S
  - AC: on Windows the sidebars' hooks are `pane-focus-in`, `after-split-window`,
    `after-kill-pane` and `pane-exited` (`internal/tmux`'s `hooks_windows.go`; macOS and Linux
    keep `window-pane-changed`, and the goldens stay the same everywhere); the web view adds its
    MIME types when it first serves a file, not at every imux start; i-mux-core reuses a pane's
    program name for 250 ms on Windows (a process snapshot per frame stalled its loop); a
    Windows test checks the hooks, one the engine's reuse.
- [x] **T94.5 The Nerd Font comes with the install** · S · deps: T94.1
  - AC: D90: `install.sh` and `install.ps1` install the font with no question (`--no-font` /
    `-NoFont` / `IMUX_NO_FONT=1` skip it; a font in place is left alone); `setup.exe` runs
    `imux font install --yes` unless `/NOFONT`; the first interactive `imux` does it once when
    none of them did (not over SSH, with `icons = "emoji"` or `IMUX_NO_FONT=1`), recorded in
    `~/.imux/font.auto`, a failure tried again a day later and never stopping the start;
    `testutil.Isolate` sets `IMUX_NO_FONT=1`. Tests: the install script runs the font step
    without `--yes`; the first start's rules (unit); the installers' texts.

## M93: A product README (added 2026-11-21)

User request: a short, professional README with Nerd Font icons, ready for a product launch.

- [x] **T93.1 Product README and user guide** · S
  - AC: README.md and README.tr.md are short and scannable: tagline, badges, why i-mux, the
    supported agents, a feature grid, installation per OS (macOS/Linux/WSL, Windows installer
    and PowerShell, from source), the Nerd Font, quick start, key bindings, configuration,
    privacy, uninstall and the docs. Icons are Nerd Font glyphs (Codicons, Font Awesome,
    Material Design) drawn as SVG in `docs/images/icons/`, since GitHub cannot show the font.
    The detailed usage moves, unchanged, to `docs/GUIDE.md`.
  - AC: the screenshots show today's UI with the Nerd Font icons: `scripts/screenshots/shoot.sh`
    runs on i-mux-core at 150x34 with `icons = "nerd"`, and `ansi2svg.py` embeds the Nerd Font in
    each SVG, subset to the characters on screen (`SHOT_FONT`, fontTools), so GitHub shows it.

## M92: i-mux-core only; tmux is gone (added 2026-11-21)

The owner's decision (D87): i-mux runs on i-mux-core on every system, whether tmux is installed or
not; tmux is no longer an engine.

- [x] **T92.1 One engine** · M
  - AC: the engine is always `imux-core` (beside imux, else on `PATH`; `IMUX_TMUX_BIN` still
    wins); `[engine] backend = "tmux"` is ignored with a warning; no engine row in Settings and
    no `imux engine`; doctor and Versions check the engine, not tmux; install.sh and the
    Homebrew formula neither need nor install tmux; CI's e2e runs on i-mux-core only and the
    pinned tmux build is gone; README says so and how to close a tmux server left from before;
    unit and e2e tests pass.
- [x] **T92.2 No tmux compatibility left** · M · deps: T92.1
  - AC: the branches for tmux older than 3.8 (3.2's tab bar ranges, menus without the mouse
    before 3.5, the agent bar before 3.4, the light/dark hooks before 3.6, …) and their golden
    configs are gone; the engine's version is still checked against i-mux's minimum; unit and
    e2e tests pass.
- [x] **T92.3 Updating moves the projects from tmux** · S · deps: T92.1
  - AC: `imux`, started outside tmux, finds the server an earlier i-mux left running on tmux
    (tmux on `PATH`, socket `imux`), records its open projects as interrupted snapshots, closes
    that server and says how many projects moved; Restore last session then reopens them on
    i-mux-core. Nothing happens without tmux or without that server, and nothing from inside
    one of its panes; tmux itself and other servers are never touched; a unit test with a fake
    tmux covers it.
- [x] **T92.4 No tmux left in the docs and messages** · S · deps: T92.1
  - AC: without imux-core the error says to install i-mux again (not tmux), with a unit test;
    the README installs natively on Windows (setup.exe, `install.ps1`) and `go install` builds
    imux-core too; TROUBLESHOOTING drops the tmux-only fixes and explains the move from tmux;
    tests that ran on real tmux are named for the engine and no test leans on tmux's `PATH`.
- [x] **T92.5 imux carries the engine** · S · deps: T92.1
  - AC (D88): `go install …/cmd/imux@latest` alone gives a working i-mux on macOS, Linux and
    Windows; an imux binary named `imux-core` runs as the engine; with no imux-core beside imux
    nor on `PATH`, i-mux makes `~/.imux/engine/imux-core` from itself (a hard link, else a copy)
    and remakes it when imux was replaced; test binaries never do; the README's from-source
    install is the one `go install`; unit tests cover the lookup, the link, the copy and the
    remake with synthetic files; the stripped imux stays under the 15 MB gate.

## M91: CI-green: the watcher's pid and menus after a gesture (added 2026-11-20)

CI has been red on `e2e` since M72: the pane-swap race (fixed with the swap bindings made
synchronous) and, remaining, three failures: a timing bug of i-mux's watcher that the slower
runner exposes on either engine (T91.1), a tmux 3.6 bug that Ubuntu 26.04's runner brought in
(T91.2), and a test that raced tmux's own query timeout on macOS (T91.3).

- [x] **T91.1 The watcher writes its pid without an empty window** · S
  - AC: `internal/watch`: the pid file is space-padded over, never truncated first, so
    `__watch status` cannot read a wiped file and report `pid 0` while the new watcher holds the
    lock (seen on the runner: `status "watcher running (pid 0)"`); a unit test covers the
    overwrite; `TestSidebarRevivesWatcher` waits for the status, not only for the process.
- [x] **T91.2 The tmux 3.6 menu bug is recorded and fenced off** · S
  - AC: `TestSidebarDoubleClick` skips on tmux 3.6 with a pointer to docs/TROUBLESHOOTING.md
    ("Menus close by themselves"), which records the bug and the way out (verified with a
    plain-session probe: no i-mux command, key or mouse involved — 9.3); CI's `e2e` job
    installs a pinned tmux (3.8, built from the release tarball, checksum-checked, cached)
    instead of the runner's, so the suite no longer depends on the distribution's tmux (3.6 on
    Ubuntu 26.04 broke it); the test passes on 3.5a, 3.8 and i-mux-core.
- [x] **T91.3 The theme test waits out tmux's terminal query** · XS
  - AC: `TestThemeTerminalBackground` gives the attach step (`Enfcs`: focus events on and the
    theme's background) more than tmux's 5 s terminal-query timeout: the harness's terminal
    answers no DA query, so tmux writes `Enfcs` only when that timer fires (`TTY_QUERY_TIMEOUT`,
    tty.c; measured 5.01 s after attach on tmux 3.8), while the step waited 5 s from the first
    screen (~0.18 s after attach), a 0.16 s margin that macOS runners lost 4 times in 12 runs. A
    real terminal answers at once; nothing changes for users.

## M90: Native Windows on i-mux-core (added 2026-11-19)

User request: finish the remaining plan — i-mux on Windows (D85). The tasks mirror Phase 9
(C9.1–C9.10) of i-mux-core's plan; the engine's own Windows work is done (its Phase 8), and its
e2e suite passes on Windows 11 (its Windows Terminal check, C8.9, waits for the owner; this
milestone starts before it at the owner's word). All through this, `make ci` keeps passing with
tmux on macOS and Linux, exactly as before.

- [x] **T90.1 Docs** · XS
  - AC: D85 supersedes D60's rule; this milestone; AGENTS.md's rule replaced (Windows code in
    build-tagged files behind small interfaces, Windows CI required, tmux still the default on
    macOS and Linux); CONTRIBUTING's dependency rule and the check-before-push skill follow.
- [x] **T90.2 i-mux builds for Windows** · L
  - AC: `GOOS=windows go build ./...` and `go vet` are clean; lint runs for `GOOS=windows` in
    CI and in `make ci`; every POSIX-only use is behind a small interface in build-tagged files
    (`syscall.Exec`, `Setsid`, `SIGTERM`/`SIGHUP`/`SIGWINCH`, `flock` → `LockFileEx`, Unix file
    modes, `/dev/null`, symlink assumptions, `$HOME` → `USERPROFILE`); the engine is `core` on
    Windows (whatever `backend` says, no engine row in Settings; D85); all existing tests pass
    on macOS and Linux unchanged.
  - Status: `internal/sysx` (D85) holds every call that differs, and the sidebar's terminal on
    Windows is the console through it (`term_windows.go`); `paths.UserHome` and the agents'
    stores read `%USERPROFILE%` there, and `testutil.Isolate` sets it. `make lint-all` and CI's
    `lint-windows` job lint and vet the Windows build (the e2e tests join with T90.9). The
    `sysx` tests pass on Windows 11 as well (run through WSL), and `imux.exe` starts there.
    `/dev/null` remains only in the command lines T90.3 replaces. Symlinks (a skill's link for
    Claude) need Windows' Developer Mode; without it the link is not made, as when its place is
    taken.
- [x] **T90.3 Command strings without a POSIX shell** · M · deps: T90.2
  - AC: `UICommand`, `Silent`, `ShQuote`, `ShJoin`, the `run-shell` strings in the generated
    config, the popups' commands and every `sh -c` use follow E14 (D85, §9.2a): on Windows the
    argv form of `run-shell`/`if-shell` (i-mux-core's X1; the quiet form for
    `… >/dev/null 2>&1 || true`) and popups'/panes' programs as separate arguments, with no
    `rm -f`, pipe or other shell utility (the handoff preview pages itself); on macOS and Linux
    byte-identical to before (golden tests); `TMUX` and `TMUX_PANE` parsed in their Windows
    form; the shell an agent exits to is the engine's choice there; `make e2e-core-argv` runs the
    e2e suite on i-mux-core with the Windows forms (`IMUX_ARGV_FORM=1`). The `sh -c` uses that
    run what an agent or the user set up go with the tasks for them: Claude's status line and
    the install and fix commands (T90.5), the clipboard and the browser (T90.6), the update
    (T90.7).
  - Status: `tmux.ArgvForm` (from `sysx.POSIXShell`, or `IMUX_ARGV_FORM=1`) picks the form.
    `UICommand` gives the quiet argv form, splitting its sh words as sh would; `ShellLine`,
    `Program` and `QuoteProgram` give the plain argv form and programs as separate words, and
    every popup goes through them (`popmenu.Popup` takes an argv). The handoff preview pages
    itself (`--pager`), a menu's answer file is pruned, the watcher's reap nudge is left out,
    the shell an agent exits to is the engine's (`sysx.ShellArgv`), and `TMUX` is read from the
    right. On Linux the sh form is unchanged (its goldens and the menu tests pass as before);
    `tmux-3.8-argv.conf.golden` holds the argv form, and `make e2e-core-argv` passes in full on
    i-mux-core, where the same run on tmux fails, as it must. On Windows 11 the `sysx` tests
    and `internal/tmux`'s goldens pass; its tests of file modes, engine resolution and a real
    tmux wait for T90.9.
- [x] **T90.4 Paths and agents' homes** · S · deps: T90.2
  - AC: `~/.imux` under `%USERPROFILE%`; each agent's config and session paths on Windows
    (Claude Code, Codex, Copilot, Antigravity, OpenCode), each verified from that agent's own
    documentation and recorded in ARCHITECTURE.md (§7.8); path separators (and case) in
    session names, git paths and the sidebar; unit tests with synthetic Windows-style homes,
    reading no real files.
  - Status: every agent keeps its home under `%USERPROFILE%` with the same overrides (§7.8, from
    each agent's docs or source), so `sysx.HomeEnv` (T90.2) was all their resolution needed.
    A file name can no longer leave its folder through `..\` or name a drive (`fileops`: before
    this, on Windows `..\out` was made outside it); the folder picker takes `C:\…`, `~\…` and a
    trailing `\`; the `~` shortening, pane titles that are paths, a run's paths and the search's
    title use the system's separator; `search.Files` lists with `/` without git as with it; and a
    session's name comes from its folder lower-cased on Windows (`sysx.PathKey`). The new
    Windows tests pass on Windows 11 (through WSL), the `..\` one failing before the fix.
- [x] **T90.5 Agents on Windows** · M · deps: T90.3, T90.4
  - AC: detection, installation, starting, resuming and hooking each agent works: npm's `.cmd`
    and `.ps1` shims and `.exe` files are found; install commands for Windows; hook installation
    into each agent's settings with Windows paths and commands; resume commands; an environment
    (D84) applied to PowerShell or cmd.exe at its prompt (a `.ps1` or `.cmd` apply file; until
    then such a pane is left alone, with a message); unit tests; their e2e on Windows comes with
    T90.9's suite, whose fake agents are ported to Go.
  - Status: each agent installs with its maker's Windows command (run by Windows PowerShell,
    `sysx.InstallShell`) and is found on `PATH`, then in that command's folder or npm's, with
    `PATHEXT`; an npm shim runs as what it runs (`sysx.Command`: node and the package's script,
    or its `.exe`), when an agent starts, is probed, lists its models, runs as a job or updates;
    WinGet's Codex updates with `winget upgrade`; a missing winget says how to get it. Claude's
    hooks use its exec form and its status line imux's path with `/` (8.3 when it has a space),
    and the user's own status line runs in Git Bash or PowerShell (`sysx.UserShell`). The
    versions panel shows the engine in tmux's row, with no package manager. An environment
    (D84) reaches PowerShell through a script block read from a `.ps1` apply file and cmd.exe
    through `call` of a `.cmd` one: only the path is typed, the file deletes itself, cmd.exe
    echoes none of its lines (each starts with `@`), and `Reserved` matches Windows' names
    whatever their case, with its folders and shell kept. Windows tests (`sysx`, `agent`,
    `agenthook`, `envfile`, which runs the apply files in Windows PowerShell and cmd.exe) pass on
    Windows 11 build 26200; Linux's behaviour is unchanged.
- [x] **T90.6 Windows integrations** · L · deps: T90.2
  - AC: §7.10; opening URLs and files (`ShellExecute`); clipboard text and image paste (Win32
    clipboard); notifications (a Windows toast); the Nerd Font install as a per-user font with
    the Windows Terminal profile set to it, with the user's consent; the editor launch; git
    (Git for Windows required, `imux doctor` checks); unit tests with fakes and Windows CI e2e
    where the OS allows.
  - Status: native Windows takes the paths i-mux already had under WSL, with Windows' own
    folders and no `wslpath`: the clipboard through PowerShell, Explorer for pages, files and
    Reveal, the default browser's (or Edge's) app window found under `%ProgramFiles%` and
    `%LOCALAPPDATA%`, a window raised with `AppActivate`, the image paste through PowerShell with
    copied files' paths as they are, and the Nerd Font installed and registered for the user with
    Windows Terminal's and VS Code's settings read and set from `%LOCALAPPDATA%` and `%APPDATA%`.
    New for Windows: the Recycle Bin (.NET's `SendToRecycleBin`), a toast through WinRT, micro's
    `win64` and `win-arm64` zips (checksums as GitHub lists them; `fetch.Unzip`), VS Code's
    `code.cmd`, and doctor's git check (Git for Windows there). The test hooks run in PowerShell.
    Unit tests with fakes run everywhere; on Windows 11 build 26200 the toast's script builds its
    toast (not shown), micro's real zip installs and runs, and the new tests pass. Windows CI's
    e2e comes with T90.9.
- [x] **T90.7 Self-update on Windows** · M · deps: T90.2
  - AC: §9.3 "Updates", D85: GoReleaser ships `imux_<version>_windows_<arch>.zip` with `imux.exe`
    and `imux-core.exe`; Settings → Update (`imux update`) downloads and verifies both against
    `checksums.txt`; replacing a running executable renames the current one to `*.old`, writes the
    new one, deletes `*.old` on the next start; rollback if any step fails; a running engine
    server keeps the old binary until its sessions end (Settings says so); an update e2e on
    Windows CI with a local release fixture.
  - Status: GoReleaser builds `imux.exe` and `imux-core.exe` for `windows/amd64` and
    `windows/arm64` into `.zip` archives listed in `checksums.txt`. On Windows `update.Install`
    calls `update.InstallArchive` (on Unix it runs `install.sh` as before): it downloads
    `checksums.txt` and the release zip, checks the SHA-256 sum, unpacks it and calls
    `update.Replace`, which stages each binary to `<name>.new`, renames the existing executable
    to `<name>.old` and puts `<name>.new` in place, rolling back every binary already moved if
    any step fails; `setup()` runs `update.CleanOld()` on each start to remove `*.old` files
    no longer held open by a running process, and `BinVersion` honours the client's 5 s timeout
    so Windows Defender's first-run scan of a newly extracted `imux-core.exe` does not time out
    when `imux update` checks whether an engine server is running. `TestInstallArchiveAndRollback`
    and `TestSelfUpdateEndToEnd` (with a live running `imux.exe` and `imux-core.exe`, cleanup on
    the next start, the running-engine message, and rollback) pass on Linux and on Windows 11
    build 26200.
- [x] **T90.8 Installer** · L · deps: T90.7
  - AC: §7.11, D85: a per-user NSIS installer needing no admin, cross-built on Linux CI
    (`scripts/installer.nsi`, `scripts/build-installer.sh`): installs `imux.exe` and
    `imux-core.exe` to `%LOCALAPPDATA%\Programs\i-mux`, adds it to the user's `PATH`, adds a Start
    menu entry that opens i-mux in Windows Terminal (conhost as the fallback), and an uninstaller
    in Apps & features; `install.ps1` one-liner; a winget manifest
    (`packaging/winget/hamzayogurtcuoglu.i-mux.yaml`); CI builds it and a Windows job installs it
    silently, runs `imux doctor` and uninstalls cleanly.
  - Status: `scripts/installer.nsi` and `scripts/build-installer.sh` cross-build the per-user NSIS
    installers (`imux_<version>_windows_{amd64,arm64}_setup.exe`) on Linux with `makensis` and
    render `packaging/winget/hamzayogurtcuoglu.i-mux.yaml` with the release's version and SHA-256
    sums; `install.ps1` installs `imux-core.exe` and `imux.exe` from the release zip (or builds
    both with `-FromSource`), adds `%LOCALAPPDATA%\Programs\i-mux` to the user's `PATH` and offers
    the Nerd Font; `imux doctor` skips the Unix terminfo check on Windows (where i-mux-core embeds
    its own terminfo, E15); CHANGELOG explains the SmartScreen prompt until a code-signing
    certificate is configured. Checked on Windows 11 build 26200: the cross-built NSIS installer
    installs silently (`/S`), puts both binaries in `%LOCALAPPDATA%\Programs\i-mux`, adds the
    folder to `HKCU\Environment\Path`, creates the Start menu shortcut (`i-mux.lnk` → `wt.exe`),
    registers the Apps & features uninstaller, `imux doctor --ci` passes, and `uninstall.exe /S`
    removes the files, folder, shortcut, `PATH` entry and registry key cleanly; `TestInstallPS1Windows`
    passes on Windows 11 as well. CI's `release-snapshot` and `windows-installer` jobs run the
    same check.
- [x] **T90.9 Windows CI for i-mux** · S · deps: T90.5, T90.6
  - AC: a `windows` job runs the unit tests and the e2e suite on i-mux-core, with tests that
    cannot apply skipped and each skip listed; the fake agents ported to Go (with T90.5's
    agents: installs, shims, hooks); green and blocking; the Windows lint covers the e2e tests
    too; CONTRIBUTING says how to run them on Windows.
  - Plan (2026-11-20): on Windows the harness builds `e2e/fakeagents/fakeagent`, fake.sh ported
    to Go (the agent is the program's name: `fake-claude.exe`, …; a test's control script,
    `A.sh` or `FAKE_SCRIPT`, runs in Git for Windows' sh, as i-mux-core's suite does), and
    drives its clients through ConPTY (`golang.org/x/sys/windows`, no new module). fake.sh
    stays the fake on macOS and Linux; a unit test runs both on the same arguments and compares
    what they log, so the two cannot drift. Tests that cannot apply on Windows skip through one
    list, `e2e/skips_windows_test.go`, each with its reason. The `windows` job runs
    `go test ./...` and `make e2e-core`'s suite.
  - Status (2026-11-20): done. The `windows` job runs `go test ./...` and the e2e suite on
    i-mux-core: all unit tests pass (76 skip on Windows with `testutil.SkipOnWindows` and a
    reason: sh scripts as fakes, POSIX path fixtures, WSL); e2e, 80 pass and 35 are listed in
    `e2e/skips_windows_test.go` with reasons, 3 of them known issues (T90.11). Found on the way:
    the engine pin had no Windows server (031fbf5); three i-mux bugs, fixed in their own commits
    (a schedule's runs could not move, a `file:///C:/…` URL, removing a worktree its agents work
    in); the tests' homes now include `LOCALAPPDATA` and `APPDATA`, so no test touches the
    owner's Windows Terminal settings or engine sockets.
- [x] **T90.11 The Windows e2e known issues** · M · deps: T90.9
  - AC: the three tests T90.9 lists as known Windows issues pass on the `windows` job and leave
    the list: `TestPaneDiedFallsBackToShell` (the server is gone after the agent exits; on
    macOS and Linux the pane falls back to the shell), `TestDiffMarks` (the diff view does not
    show), `TestThemeTerminalBackground` (a theme change's OSC 11 does not reach the client
    through ConPTY). Each: the cause found, a fix or, if it cannot apply, a reason.
  - Status (2026-11-21): done. The shell after an agent was given to the engine as one word, its
    path, which is a shell command line to it: `C:\Program Files\Git\usr\bin\sh.exe` ran as a
    command, failed and closed the pane, and with it the session (fixed: a Windows shell always
    gets an argument, `-l`, `/a` or `-NoLogo`). The diff test respawned its pane with an sh line
    holding a Windows path and looked for "line 1 " with a space Windows' screen does not have
    (fixed in the test: argv, and the row's numbers). A theme change paints attached terminals by
    writing OSC 11 to their tty devices, which Windows clients do not have: they get the
    background when they attach (D77), so that test stays listed, with that reason.
- [ ] **T90.10 Windows check** · XS · deps: T90.9 · USER
  - AC: the owner installs from the installer, opens from the Start menu, installs an agent,
    checks the sidebar, tabs, popups, menus and mouse, image paste, notifications, updates from
    Settings and uninstalls; signs off or files issues.

## M89: Environments for a pane (added 2026-11-19)

User request: an Env view on the left with saved environment files; one can be saved there, and
applying one to the active terminal makes it take effect there: a running CLI restarts after a
short warning, a shell gets it at once. The first use is Claude Code on a Z.ai Coding Plan beside
the Claude subscription, but nothing is specific to a provider (D84).

- [x] **T89.1 Environment files** · S
  - AC: D84: `internal/envfile`: names, `~/.imux/env` (0700) and `<name>.env` (0600); the parser
    (`export`, comments, quotes, no expansion; bad lines and reserved keys reported with their
    line numbers); list, read, create with the commented template, delete; the shell script that
    applies one (POSIX and fish quoting, `unset` of the keys left behind, deletes itself); unit
    tests and a fuzz test for the parser.
- [x] **T89.2 Launches take the pane's environment** · S · deps: T89.1
  - AC: `@imux_env`; `Launch.Env` adds the variables to the spec (reserved ones never); `respawn`
    takes the pane's; the shell an agent exits to starts through `__exec` with them; snapshots
    keep the name and restore it; a split does not inherit it; unit tests.
- [x] **T89.3 Apply and remove** · M · deps: T89.2
  - AC: `imux env apply|clear`: an agent pane asks (a menu; working: its turn stops) unless
    `--yes`, then restarts its agent resuming its session (fallback: the full handoff; fresh when
    it has none); a shell at its prompt gets the apply file through `send-keys`; the "No agents"
    pane only takes the option; other programs are left alone with a message; `imux env ls`,
    `new`, `edit`, `rm` (unsets it from its panes); the border shows `🔑 <name>`; unit tests.
- [x] **T89.4 The Env view** · M · deps: T89.3
  - AC: 🔑 Env on the rail after Jobs (key `7`); `ENV` and **+**; rows with the number of
    variables, `● this pane` for the target's, ⚠ for a file with bad lines; click applies (on
    the target's own: its menu), right-click opens its menu (Apply / Remove from this pane,
    Edit, Delete…); the empty view; i18n (en, tr); unit tests.
- [x] **T89.5 End to end** · S · deps: T89.4
  - AC: e2e with the fake agents: applying to an agent pane restarts it with the variable in its
    environment and its session resumed; applying to a shell pane sets it there (`echo` shows
    it) and the apply file is gone; removing takes it away; no value in any tmux command.
- [x] **T89.6 Docs** · XS
  - AC: README and README.tr, CHANGELOG.
- [x] **T89.7 The New environment popup** · S · deps: T89.4
  - User request (2026-11-19, after v0.65.0): **+** should open a popup to type the name in,
    not ask at the top of the screen; and an environment is for any program, not Claude only.
  - AC: D84: **+** opens the New environment dialog in a popup in the middle of the client
    (`imux env dialog`, as New skill's): a Name box; under it the name what is typed makes
    (`envfile.Slug`), or ⚠ when it is taken; Enter writes the template and opens the editor in
    the same popup, on the line under it; Esc closes; `imux env new` with no name shows the dialog
    in a terminal; the command prompt, `--from-option` and their options go; the template and the
    empty view speak of any program, Claude Code's endpoints as one example; i18n (en, tr);
    README and README.tr, CHANGELOG; unit tests (the name, the dialog, the popup's command).

## M88: The engine switch: i-mux-core, opt-in (added 2026-11-18)

User request: run i-mux on i-mux-core behind a switch, with tmux the default (D83). The tasks
mirror Phase 7 (C7.2–C7.7) of i-mux-core's plan, whose Gate 6 holds; i-mux's `make ci` keeps
passing with tmux, exactly as before, all through this.

- [x] **T88.1 The e2e harness honours `IMUX_TMUX_BIN`** · S
  - AC: `e2e/harness_test.go` uses `IMUX_TMUX_BIN` when set, else `tmux` on `PATH`, as today;
    `make e2e-core` builds or locates `imux-core` and runs the suite with it; `make e2e` is
    unchanged and passes.
- [x] **T88.2 Engine detection and versions** · S · deps: T88.1
  - AC: `ParseVersion` reads `tmux 3.8 (imux-core X.Y.Z)` as tmux 3.8, as today, and gains an
    `Engine` field set from the bracketed part; `imux doctor` shows the engine and its version
    and warns when an imux-core older than i-mux's minimum is found; unit tests; a doctor e2e
    for both engines.
- [x] **T88.3 The engine switch** · M · deps: T88.2
  - AC: D83: `[engine] backend = "tmux" | "core"`, default `tmux`; with `core`, the imux-core
    next to imux's own binary, else the one on `PATH`; `IMUX_TMUX_BIN` still overrides
    everything; the two engines use different socket directories; switching while sessions
    run is refused with an offer to close them; Settings shows the choice as "Engine (beta)";
    unit tests for the resolution order; e2e: the whole suite passes with the default and with
    `core`.
- [x] **T88.4 Packaging and updates** · M · deps: T88.3
  - AC: GoReleaser builds `imux-core` for darwin and linux (amd64 and arm64) into the same
    archives and the Homebrew formula, from `go.mod`'s pin of i-mux-core through `cmd/imux-core`
    (D83); `make install-local` installs both; the self-update
    (Settings → Update, `imux update`) replaces both checksum-verified as one step, rolling
    back if either fails; a running engine server keeps its old binary until its sessions end,
    and the update message says so; a goreleaser snapshot contains both; an update e2e with a
    local release fixture replaces both.
- [x] **T88.5 CI for both engines** · S · deps: T88.4
  - AC: an `e2e-core` job builds i-mux-core at `go.mod`'s pin (`make build`, D83) and runs
    `make e2e-core` on Linux and macOS; `make e2e-core` defaults to that build
    (`IMUX_CORE_BIN` still picks another, such as the engine's working tree); the job is
    blocking and green.
- [x] **T88.6 Beta release** · S · deps: T88.5
  - AC: the release notes explain the switch and how to go back; an "Engine (beta)" issue
    template asks for the terminal, the OS and `imux doctor` output; the owner approves and
    tags (USER).
  - Status: the notes (CHANGELOG, README and README.tr) and the issue template
    (`.github/ISSUE_TEMPLATE/engine-beta.yml`) are in; the release waits for T88.5's green run
    and the owner's tag.

## M87: Aider is removed (added 2026-11-18)

User request: remove Aider, added in v0.63: "I think it's bad, remove it" (D82).

- [x] **T87.1 Aider out of the code** · S
  - AC: D82: M86's code goes (the agent, its session reader, limit patterns, colors and Jobs
    command, `HandoffArgs` and `--read`, the paste's wait while a question shows, `Asking` back
    in `watch`); `aider` in `config.RemovedAgents`, so a config naming it loads with a warning;
    OpenCode's short name `Oc` stays; `docs/CONFIG.md`; unit tests.
- [x] **T87.2 End to end** · XS
  - AC: e2e: the suite passes without the fake Aider and its test.
- [x] **T87.3 Docs** · XS
  - AC: README and README.tr, docs/LIMITS.md; CHANGELOG.

## M86: Aider (added 2026-11-17)

User request: Aider CLI everywhere an agent is, like Claude, Copilot and the others (D81, §7.8).

- [x] **T86.1 Agent, launch and config** · S
  - AC: `session.Aider` ("Aider"), in `AllAgents`, `config.AgentIDs` and the default order (last);
    registry entry: key `i`, bin `aider`, vendor "any model", install `curl -LsSf
    https://aider.chat/install.sh | sh` (hint names aider-install, uv and pipx too), `Dirs`
    `~/.local/bin`, update `--upgrade`; templates: new with no prompt (pasted), resume
    `--restore-chat-history`; with `deliver = "file"` the handoff is passed as `--read=<file>`;
    sidebar icon and the agent bar's short name; default.toml and docs/CONFIG.md; unit tests.
- [x] **T86.2 A paste waits for a question** · S
  - AC: §9.5: `WaitStable` does not paste into a still screen that asks (yes/no, numbered choice,
    Aider's `(Y)es/(N)o`), up to 10 min; `Asking` moves where `workspace` can use it; unit tests.
- [x] **T86.3 Session provider** · M
  - AC: `internal/session/aider`: Find (the folder's `.aider.chat.history.md` or its repository
    root's; runs as sessions, id = start time, `Since`, `ID`, newest first), Load (user, assistant,
    edits, reads, tokens, model, limit-like errors, chained handoff), Preview; `SharedStore`; a
    path to the history file resolves to Aider; the handoff's "full transcript" line names the
    file and the run; tests with synthetic history files only.
- [x] **T86.4 Limits, doctor, Jobs, i18n** · S
  - AC: limit patterns (provider wording and `RateLimitError`); the panel's Aider row says the
    provider's limits apply; doctor checks it like the others; Jobs run it headless (§7.8) and read
    the reply from the run's history file; i18n (en, tr); unit tests.
- [x] **T86.5 End to end** · M · deps: T86.1–T86.4
  - AC: a fake aider (fake.sh): it asks the `.gitignore` question first, then writes a history
    file; Claude → Aider (the handoff is pasted after the question is answered, `--read` names the
    file) → Claude keeps Aider's replies; doctor and the limits panel list Aider.
- [x] **T86.6 Docs** · XS
  - AC: README and README.tr (agent list, keys, an Aider paragraph); docs/LIMITS.md; CHANGELOG.
- [ ] ~~**T86.7 A real Aider** · USER~~ **Dropped (2026-11-18):** Aider was removed (M87).
  - AC: Q32 with a real Aider and a fake model server; results in §7.8 and §21.

## M85: OpenCode Go's limits (added 2026-11-16)

User report: with OpenCode Go, which has 5-hour, weekly and monthly limits, the limits panel does not
show them (D80).

- [x] **T85.1 Go's windows from OpenCode** · S
  - AC: D80: the OpenCode provider runs `opencode api experimental.usage.limits` (5 s timeout) and
    parses `data.go`; null data, null `go`, a failed command or another shape give nothing; unit
    tests with synthetic output and a fake `opencode`.
- [x] **T85.2 The panel** · S
  - AC: the OpenCode row draws 5-hour, weekly and monthly bars with their reset times; a `limited`
    window marks the row limited until its reset; `imux limits --json` has `usage.opencode_go`
    (docs/LIMITS.md); without Go the row is as before; unit tests.
- [x] **T85.3 End to end** · XS
  - AC: e2e: the fake opencode answers `api experimental.usage.limits`; `imux limits` shows the three
    windows.
- [x] **T85.4 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M84: Agents install from Versions and updates (added 2026-11-16)

User report: in Versions and updates, OpenCode's letter says "Nothing to run"; what installs it
there (D79)?

- [x] **T84.1 The letter installs** · S
  - AC: D79, §9.3 "Versions and updates": a missing agent's row shows the command Install now
    runs and its letter runs it (`sh -c`); without npm, curl or brew the row says how to get it
    and nothing runs; an install that finished with the agent still not found says so; unit
    tests.
- [x] **T84.2 OpenCode 2, and the installers' folders** · S
  - AC: D79: OpenCode's install command is `curl -fsSL https://opencode.ai/v2/install | bash`;
    an agent not on the PATH is found in `~/.opencode/bin` (OpenCode) or `~/.local/bin`
    (Antigravity), with that folder first on its PATH; unit tests.
- [x] **T84.3 End to end** · XS
  - AC: e2e: in the popup, a missing agent's letter runs its install command, and the list is
    read again.
- [x] **T84.4 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M83: Every agent shows as working (added 2026-11-16)

User report: a working Antigravity shows no spinner; check Codex, OpenCode and the others too
(D78).

- [x] **T83.1 Each CLI's busy line** · S
  - AC: D78: Antigravity's status line `esc to cancel` / `Esc disabled during file edits`, a
    progress line `(… esc to cancel)` or `esc to interrupt`, and not while a question shows;
    Codex's `… to interrupt` with any key; OpenCode's `esc interrupt` / `esc again to interrupt`
    (no longer D38); unit tests with the screens of the CLIs as they are now.
- [x] **T83.2 End to end** · S
  - AC: e2e: fake Antigravity and OpenCode panes drawing their screens at work are working; an
    Antigravity question with its hint still showing is Needs you.
- [x] **T83.3 Docs** · XS
  - AC: CHANGELOG (the READMEs' "while the agent itself says so" still holds).

## M82: Themes fill the whole terminal (added 2026-11-16)

User report: with a theme, black bands stay at the top, bottom and edges of the terminal (D77).

- [x] **T82.1 The terminal's background** · S
  - AC: D77, §9.3 "Themes": `terminal-overrides[90]` with OSC 11 at the end of `Enfcs` and OSC
    111 at the start of `Dsfcs` for a theme on its own 24-bit colors without `auto_switch`, only
    the `Dsfcs` part otherwise; written to the attached terminals when the theme changes and
    when tmux.conf reloads, and OSC 111 when i-mux quits; control-mode clients skipped; the
    config loads in a real tmux; unit tests.
- [x] **T82.2 End to end** · S
  - AC: e2e: a client attaching with Nord gets OSC 11 with Nord's panel and OSC 111 when it
    detaches; another theme chosen in Settings reaches the attached terminal at once, and the
    i-mux theme gives the terminal its own background back.
- [x] **T82.3 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M81: Claude shows as working again (added 2026-11-16)

User report: the sidebar's spinner turns for a working Copilot but not for a working Claude
(D76).

- [x] **T81.1 Claude Code's spinner line** · XS
  - AC: D76: Claude's busy line is `esc to interrupt` or its spinner line with the time it has
    worked (`· Discombobulating… (30m 46s · ↓ 139.3k tokens · thinking with high effort)`),
    among the last 16 non-blank lines; a line without the time in parentheses is not; unit
    tests.
- [x] **T81.2 End to end** · XS
  - AC: e2e: a fake Claude drawing Claude Code 2.1's spinner line above its input is working
    and counted in the AGENTS title; typing in its input is not work.
- [x] **T81.3 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M80: Projects without agents come back (added 2026-11-16)

User report: after quitting i-mux and starting it again, only the projects with agents came
back; a project that was open without agents should come back too (D75).

- [x] **T80.1 Every open project in the last session** · S
  - AC: D75, §9.3 "Recent and restore": Restore last session counts and restores the projects
    of the earlier server with or without agents; Restore brings a project with no agent to
    start back with the "No agents" pane (its role, folder and label) and a sidebar when one of
    its tabs had one; opening such a folder on its own still starts the default agent; unit
    tests.
- [x] **T80.2 End to end** · S
  - AC: e2e: a project with an agent and one whose last agent was closed; Quit; `imux`
    elsewhere offers "Restore last session · 2 projects"; Enter: both are open again, the
    second with its "No agents" pane and its sidebar, and no agent started in it.
- [x] **T80.3 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M79: The Remote view (added 2026-11-15)

User request: machines in a view of their own on the rail, after Git, plain as VS Code's Remote
Explorer; no Settings row (D74).

- [x] **T79.1 Remote on the rail** · S
  - AC: rail Agents · Files · Search · Git · Remote · Jobs (keys 1–6); the icon's badge counts
    the machines' working agents, its label yellow while one needs you; unit tests.
- [x] **T79.2 The tree** · M
  - AC: `REMOTE` + add; machine rows with state, fold on click (`machine:<name>` in
    `@imux_fold`), menu on right-click; projects and agents under it, open on click; dimmed
    when not connected; an empty view with Add a machine…; machines gone from PROJECTS, AGENTS
    and Settings; `imux machine add` asks the destination when none is given; unit tests and
    e2e.
- [x] **T79.3 Docs** · XS
  - AC: README and README.tr, CHANGELOG.

## M78: Machines over SSH (added 2026-11-15)

User request: the roadmap's eighth planned feature: other computers' agents in the same sidebar,
with no security hole (D73).

- [x] **T78.1 Machines and SSH** · M
  - AC: `machines.json` (0600); names and destinations checked (none read as an ssh option);
    the ssh options of D73 (batch mode in the background, a master per machine, nothing
    forwarded, host keys as configured); the remote command fixed, its words checked or
    quoted; unit tests.
- [x] **T78.2 The machine's side** · S
  - AC: `imux __remote-state` (JSON) and `imux __remote-attach`; unit tests.
- [x] **T78.3 The watcher asks them** · M
  - AC: every 3 s, each machine on its own; state and projects in `machines/<name>.json`;
    untrusted input limited and cleaned; sidebars redrawn on a change; Needs you and finished
    notified; unit tests.
- [x] **T78.4 Sidebar and menus** · M
  - AC: machine rows with their state, projects and agents (dimmed when not connected); the
    machine's menu; Settings → Machines…; add (connects interactively, checks or installs i-mux),
    remove, reconnect, open a project there, a terminal there; English and Turkish; unit tests.
- [x] **T78.5 Opening one** · M
  - AC: a tab `💻 machine · project` attached over SSH, found again when it exists; the root
    keys go on to it; it reconnects when the connection drops; e2e with a fake `ssh` and a
    second i-mux: the machine's projects and states, a question notified, a project opened,
    the connection lost and back.
- [x] **T78.6 Docs** · XS
  - AC: README and README.tr, CHANGELOG.

## M77: Worktrees for agents (added 2026-11-14)

User request: the roadmap's third planned feature: an agent in a worktree of its own (D72).

- [x] **T77.1 New worktree with an agent** · S
  - AC: the place menu's 🌿 In a new worktree… asks a branch and opens `<project>.worktrees/
    <branch>` (new branch, or the existing one) as a project with the agent; `imux worktree
    new`; unit tests (the prompt); e2e.
- [x] **T77.2 Under its project** · S
  - AC: a linked worktree's project shows under its repository's project, indented with 🌿;
    the main worktree found once per folder; unit tests.
- [x] **T77.3 Diff, merge, remove** · M
  - AC: the worktree project's menu has Diff against the base, Merge into it (asked; refused
    with uncommitted changes) and Remove worktree (asked; force asked again; the project
    closes); `imux worktree diff|merge|remove`; English and Turkish; unit tests; e2e.
- [x] **T77.4 Docs** · XS
  - AC: README and README.tr, CHANGELOG.

## M76: Agents report their state through hooks (added 2026-11-14)

User request: the roadmap's fifth planned feature, agents telling their state instead of the
screen being read (D71).

- [x] **T76.1 Hooks at launch** · S
  - AC: Claude's `--settings` file has the hooks (with the status line when that is on);
    Codex gets `-c notify=…` unless its config has a `notify`; `[hooks] enabled`; unit tests.
- [x] **T76.2 `imux __hook`** · S
  - AC: Claude's events and Codex's notify map to working / blocked / idle on the pane named by
    `IMUX_PANE` when it runs that agent; Claude's session id and file go into the trail; no
    output, exit 0; unit tests.
- [x] **T76.3 The watcher takes them** · S
  - AC: `@imux_hook` decides blocked and idle at once, the busy line still wins, working
    lapses without it; other panes as before; unit tests; e2e: a permission hook shows Needs
    you, a stop hook Done.
- [x] **T76.4 Settings and docs** · XS
  - AC: a Settings row (English, Turkish); CONFIG.md; README; CHANGELOG.

## M75: The whole window in the theme (added 2026-11-14)

User request: changing the theme left the agents' panes in the terminal's colors beside a
themed sidebar (D70).

- [x] **T75.1 Panes in the theme** · S
  - AC: a theme on its own colors sets `window-style` / `window-active-style` to its panel and
    text, `pane-colours[0-15]` (tmux 3.3+) to its 16 colors, the borders on its panel; `i-mux`
    and `terminal` unset them; the sidebar asks OSC 11 only while the panes are on the
    terminal's colors; unit tests; e2e (the panes' style, the terminal's DEC 2031 report on
    tmux 3.6+, back to i-mux).

## M74: Themes reviewed (added 2026-11-14)

User request: themes did not read (white text on a light terminal), and the default should be
i-mux's look from before (D69).

- [x] **T74.1 The i-mux theme** · S
  - AC: `i-mux`, the default: the sidebar's colors and the tab bar's grays exactly as before
    themes, tmux's own borders, messages, popups and menus (unset); `catppuccin-latte` as its
    light sibling; unit tests (its escapes, the generated tmux.conf).
- [x] **T74.2 The theme's background** · S
  - AC: the sidebar and the "No agents" pane paint the theme's background and text color on
    every row (no tmux style, so OSC 11 still tells the terminal's background); agents' panes
    are never painted; e2e.
- [x] **T74.3 Legible themes** · M
  - AC: built-in themes adjusted for 4.5:1 text on every background, 3:1 secondary text, 2.2:1
    hints, 2.5:1 accent and states, a visible selection; tints weaker where they must be; the
    terminal theme's quiet colors in bright black; a test over every theme.

## M73: Color themes, and light/dark following the terminal (added 2026-11-14)

User request: the roadmap's second planned feature: themes and automatic light/dark switching
(D68).

- [x] **T73.1 Palettes** · M
  - AC: `internal/theme`: 18 built-in palettes of the 19 named colors, names and aliases, light
    and dark siblings, color parsing (`#rgb`, `#rrggbb`, `rgb()`, ANSI names, reset), the
    resolution order (theme, custom, mode), appearance from a background color; unit tests.
- [x] **T73.2 Config** · S
  - AC: `[theme]` and `[theme.custom]`, `[theme.custom.light|dark]`; an unknown name warns and
    falls back, a bad color is an error at its line; CONFIG.md; unit tests.
- [x] **T73.3 The UI in the palette** · L
  - AC: the sidebar, pager, diff viewer, menus, Quick Picks and the other popups draw with the
    palette; tmux's tab bar, tab states, borders, messages, copy mode, menus and popups too
    (`@imux_c_*` options and styles); unit tests (a theme's colors reach the output).
- [x] **T73.4 Light/dark** · M
  - AC: `@imux_appearance` from the tmux 3.6+ hooks (`imux __theme appearance`) and from a
    sidebar's OSC 11 query (its reply parsed, never read as keys); with `auto_switch` the theme
    changes and applies at once; unit tests; e2e: the appearance changes the colors.
- [x] **T73.5 Settings** · S
  - AC: a Theme row (its list; choosing turns `auto_switch` off) and a Follow the terminal row;
    English and Turkish; unit tests.
- [x] **T73.6 Docs** · XS
  - AC: README and README.tr, CHANGELOG.

## M72: Pane keys and a searchable help (added 2026-11-14)

User request: the first item of the roadmap's planned features: move, swap and resize panes from
the keyboard and search the keys (D67).

- [x] **T72.1 Swap panes** · S
  - AC: `prefix H/J/K/L` (`tmux.swap_pane_keys`, on) run `imux __pane swap <dir>`: the
    neighbor on that side by the panes' edges, nothing at the window's edge, never a sidebar;
    the focus stays on the moved pane; unit tests (the neighbor, the sidebar, the edge).
- [x] **T72.2 Resize mode** · S
  - AC: `prefix r` (`tmux.resize_key_prefix`) enters the `imux-resize` key table: `h j k l` and
    arrows by 2, `H J K L` and Shift+arrows by 10, a status-line note; `Esc` / `Enter` / `q`
    leave; its keys go in `@imux_keys`; unit test of the generated config; e2e: the pane's
    width changes and Esc leaves.
- [x] **T72.3 Keys without the prefix** · XS
  - AC: `tmux.direct_pane_keys` (off): `Ctrl+arrows` move between panes, `Ctrl+Alt+arrows`
    open the split menu that way (`__menu --split h|v --before`); unit test.
- [x] **T72.4 Searchable help** · M
  - AC: `prefix ?` (`tmux.help_key_prefix`), the sidebar's `?` and the rail's Help open a Quick
    Pick of actions and their configured keys; typing filters; Enter runs the action; English
    and Turkish; unit tests; e2e: typing filters and Enter runs a row.
- [x] **T72.5 Docs** · XS
  - AC: CONFIG.md (generated), README and README.tr's keys, CHANGELOG.

## M71: Fixes from the codebase audit (added 2026-11-14)

User request: fix the bugs of ROADMAP.md's audit that hold, and any other found on the way (D66).

- [x] **T71.1 MERGE CHANGES folds** · XS
  - AC: a click on MERGE CHANGES folds and unfolds it (one list of the foldable sections);
    unit test.
- [x] **T71.2 Help keys** · XS
  - AC: the help popup names `e` (OpenCode), the sidebar's `5 jobs` and `4 git`, and Ctrl+V /
    Alt+V pasting an image; English and Turkish.
- [x] **T71.3 Git paths and locks** · S
  - AC: the line counts of Source Control and the Agents view's changes use `--numstat -z`
    (one parser), so non-ASCII and quoted names get theirs; the Agents view's changes and the
    explorer's colors read unquoted UTF-8 paths (`core.quotePath=false`, an escaped name
    unquoted); the diff viewer's `git diff` and search's `git ls-files` take no lock; Discard of
    an untracked file or folder passes `-d`, as Discard All does; unit tests with a
    Turkish and a quoted file name, and an untracked folder.
- [x] **T71.4 Sidebar scrolling** · M
  - AC: `Screen.List`; the wheel stops at the last page, from the top the list was drawn with;
    ↑/↓ scroll the Explorer, Source Control and Jobs lists at the edge of what is shown; unit
    tests.
- [x] **T71.5 Skills from Jobs** · M
  - AC: ⋯ on every Jobs row; ✨ New skill… in the title, a right-click on the title or empty
    space offers New skill… and Blank SKILL.md…; `⚠` and the problem on a skill's row; a
    skill's menu adds Add to Agent, Improve with Agent, Share with all agents and Delete skill…
    (`__auto skill`); the Skills view's code is removed (a stored `skills` view opens Jobs);
    unit tests.
- [x] **T71.6 A broken config keeps the watcher** · S
  - AC: a config change restarts the watcher only when the new one loads; an error is logged
    and the watcher goes on; unit test.
- [x] **T71.7 Docs** · XS
  - AC: CHANGELOG; README and README.tr where they name the keys.
- [x] **T71.8 A second look** · S
  - AC: a rename in the work tree (` R`, after `git add -N`) is one row, its old path no
    second one; a folder pasted into a folder of its own whose name starts with `..` is
    refused, not copied into itself until the path is too long; the agent bus's Deliver and
    Discard quote the parked file's path for sh (an i-mux home with a space); unit tests.
- [x] **T71.9 A third look** · S
  - AC: a link to a folder is a folder in the explorer (listed with them, opens in place); in
    the pager, n and N go through the matches on the last page and around the ends; a
    Settings change saves into a config.toml without a last line break, and a Windows line
    end stays; unit tests.
- [x] **T71.10 A fourth look** · S
  - AC: sharing a scheduled skill moves its schedule and runs to the shared file's id (they
    were left behind, the watcher skipping every run as "definition gone"); a schedule's
    label keeps its seconds ("90s": every 1 min 30 s); unit tests.
- [x] **T71.11 A fifth look** · S
  - AC: Claude's project folder is found for a cwd with non-ASCII letters (one `-` per UTF-16
    code unit, as Claude Code encodes it) and for one past 200 characters (its folders that
    start with the first 200, each file's cwd checked); the e2e client's cleanup cannot hang
    the run; unit tests.

## M70: The commit message box like VS Code's, no AI commit messages (added 2026-11-13)

User request: remove the AI commit messages from Git (✨), and put a text box above the Commit
button, as VS Code has, where the message is written.

- [x] **T70.1 The box** · M
  - AC: D65, §9.3 "The commit message box": the box between the branch row and Commit (VS
    Code's hint, the cursor, wrapping up to six rows); a click places the cursor; typing, the
    keys, a paste, Enter commits, Ctrl+J / Alt+Enter a new line, Esc, Tab and a click elsewhere
    leave it; focus events hide the cursor; the message in `~/.imux/commit/` while it is written,
    read by every sidebar of the project; unit tests (editing, wrapping, drawing at every width
    in both icon themes, clicks, the file).
- [x] **T70.2 Committing from it** · S
  - AC: Commit, Enter and the commit items of ▾ and ⋯ commit the box's message (`__scm ask`,
    `__git commit --draft`), amend keeps the last message with an empty box; with none, the box
    asks (`@imux_commit_ask`, a yellow hint naming the command); the box empties after a commit
    and keeps a message whose commit failed; unit tests.
- [x] **T70.3 No AI commit messages** · S
  - AC: ✨, `__git suggest`, `__git edit-message`, `internal/commitmsg`, `[git] commit_agent` /
    `commit_model` (an old config with them loads without a warning), the Settings row and
    their texts are removed; `docs/CONFIG.md`.
- [x] **T70.4 End to end** · S
  - AC: e2e: a click on the box, a message typed, Enter: committed, the box empty again; Commit
    with an empty box puts the cursor in the box; ▾ → Commit & Push with an empty box asks in the
    box; Settings without "Commit messages by".
- [x] **T70.5 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M69: The diff's changes beside its scroll bar (added 2026-11-13)

User request: in the diff viewer, show where the changes are beside the scroll bar, as VS Code
does, and go there with a click instead of scrolling through the whole file; the whole file stays.

- [x] **T69.1 The marks** · S
  - AC: D64, §9.3 "Scroll bars": `pager.Marks`; two lanes left of the bar (red removed, green
    added) over the whole text; a click on a mark goes to its change, elsewhere as the bar; `]`
    `[` the next and previous change around the ends, "change 2 of 5"; the diff marks its rows (a
    replaced line side by side: both); unit tests (lanes, widths, clicks, keys).
- [x] **T69.2 End to end** · S
  - AC: e2e: a diff in a pane: `]` goes to the second change and `[` back; a click on its mark
    (found on the screen beside the bar) goes there.
- [x] **T69.3 Docs** · XS
  - AC: README and README.tr; CHANGELOG.

## M68: The explorer's title like VS Code's, and hidden files (added 2026-11-13)

User request (through the sil-simulator session): files starting with "." do not show in the
explorer; add a "…" there with "Show Hidden Files", worded as it should be, and VS Code's title
buttons (New File, New Folder, Refresh, Collapse Folders); keep the look.

- [x] **T68.1 Hidden files** · S
  - AC: D63, §9.3 "Explorer title and hidden files": `[explorer] show_hidden` (off by default):
    off, names starting with "." stay out of the explorer as before; on, they show; search and Go
    to File find what they found before (git's list), and look into hidden folders outside git
    while it is on; `.git`, `.svn`, `.hg`, `CVS`, `.DS_Store`, `Thumbs.db` never, and the skip
    list stays out of the tree; every sidebar lists its tree again at once; `docs/CONFIG.md`; unit
    tests (the tree off and on, search's files in a repository and outside one).
- [x] **T68.2 The title's buttons** · M
  - AC: New File… and New Folder… (in the selected folder, else the project's), Refresh
    Explorer (the git status read anew), Collapse Folders and ⋯, codicons with the editor's icons
    and emoji otherwise; Go to File and Search as buttons only while the title has room, else in
    ⋯; the hovered (or keyboard-selected) button's label in the title; a right-click on the title
    opens the root menu; unit tests (layout and actions in both themes at every width, the label,
    which clicks wait).
- [x] **T68.3 The ⋯ menu** · S
  - AC: Go to File…, Search…, the buttons that do not fit and Show Hidden Files with ✓ while on,
    at the button; `imux __file` operations, no path in a command (D58), that save the setting as
    Settings does; unit tests.
- [x] **T68.4 End to end** · S
  - AC: e2e: a dot file is absent at first; ⋯ → a click on Show Hidden Files shows it and `.git`
    stays hidden; config.toml keeps it and a new tab's explorer shows it; off again hides it;
    New Folder… makes a folder through its prompt; Refresh shows a file written outside i-mux;
    Collapse Folders closes the open folders; ⋯ → Go to File… opens Go to File.
- [x] **T68.5 Docs** · XS
  - AC: README and README.tr; CHANGELOG.
- [x] **T68.6 Search only on the rail** · XS
  - AC: user request ("the rail already has Search; Go to File is enough there"): the explorer's
    title has no Search button and its ⋯ no Search…; Go to File keeps its button where the title
    has room (43 columns with the editor's icons, 47 with emoji) and its ⋯ item; unit and e2e
    tests; CHANGELOG (v0.53.1).

## M67: Jobs on the rail, without a separate Skills tab (added 2026-11-12)

User request: rename the Schedule tab to Jobs (since it also triggers skills and agents on demand
and in batch mode) and remove the separate Skills tab from the left rail.

- [x] **T67.1 Jobs on the rail, no Skills rail tab** · XS
  - AC: D62: the ⏰ rail tab is named `Jobs` (`İşler` in Turkish) with header `JOBS` (`İŞLER`),
    opened by key `5`; the separate 🧩 Skills item is removed from the activity rail; unit and
    e2e tests updated; README, README.tr and CHANGELOG updated.

## M66: Scheduler: the project's agents and skills on a schedule (added 2026-11-12)

User request: a rail tab that shows scheduled agents (checks, batch jobs) and makes them; no
form and no prompt: the agent or skill already written says what to do, the user picks when,
which CLI (Claude, Antigravity, whatever is installed) and which model, and sees the output.

- [x] **T66.1 Definitions, times and runs** · L
  - AC: D61, §9.3 "Scheduler": `internal/auto` lists the agent definitions and skills of a
    project, parses times (`5m`, `2h`, `hourly`, `09:00 18:30`) and says when one is due, keeps
    `schedules.toml` and the runs (`auto/<id>/`), builds each CLI's headless command line and
    runs it with a timeout, a lock, a log and a result (ok, quiet, failed, timeout, skipped);
    unit tests with synthetic projects and a fake CLI.
- [x] **T66.2 Menus, Quick Picks and the watcher** · M
  - AC: `imux __auto`: the time, CLI and model steps, run now, stop, last output, history,
    continue, access, pause, remove, open; the model list from the CLIs (cached); the watcher
    starts due runs; notifications; no name or path in a menu command; unit tests.
- [x] **T66.3 The view** · M
  - AC: the Schedule rail tab (⏰, codicon watch) with its running badge; SCHEDULED with time,
    CLI, model, state, first line and next run; AGENTS and SKILLS with 🚀 and ⏰; ✨ New agent…
    (the New skill dialog with its own texts); unit tests (render, actions, widths).
- [x] **T66.4 End to end** · S
  - AC: e2e with the fake agents: 🚀 on a project agent → the Run panel → Enter: it runs with
    `--agent` and Claude's event stream, its output opens with the tool step and the answer, its
    report shows in the view and `schedules.toml` keeps the choice; a time typed in Turkish
    units; a skill every 5 minutes from the panel starts at once; a due schedule runs from the
    watcher; a long run is followed again from its row and stopped with `s`; Output and Remove
    schedule from its menu.
- [x] **T66.5 Docs** · XS
  - AC: README and README.tr; CHANGELOG.
- [x] **T66.6 The Run panel and the output, live** · M
  - AC: D61, §9.3 "Scheduler" (user: "I tried it, it did not work": the runs worked, but
    showed nothing until they ended): 🚀 opens one panel (`internal/autodialog`) where the CLI,
    the model, the time and the access are chosen already and Enter runs it (a typed model or
    time, arrows, letters and clicks); the CLIs stream (Claude `stream-json`, Copilot `json`,
    Antigravity `stream-json`) and `auto.Output` reads their events; a run started from the
    panel opens its output (`internal/autoview` in the pager's `Live` mode), which follows the
    run: each tool call with its result's first line, the answer with its Markdown drawn,
    errors; `s` stops it; no escape or control character from the agent reaches the terminal;
    the log is written line by line; unit tests (panel, parser, drawing, live pager).
- [x] **T66.7 No agent writing** · XS
  - AC: user request ("making an agent in the Scheduler takes too long; leave it"): ✨ New
    agent…, the blank agent file and the view's right-click menu go, and the New skill dialog is
    as before (v0.50.0); the empty view says where definitions go.

## M65: No native Windows (added 2026-11-12)

User request: remove the Windows port on psmux (#2, M62); i-mux need not run natively on Windows,
and nothing else may break.

- [x] **T65.1 Revert the port** · M
  - AC: D60: the merge of #2 is reverted: the psmux adapter, `sysx`, `winx`, `pwshshim`,
    `install.ps1`, the PowerShell scripts, `docs/WINDOWS.md`, the Windows builds and archives, the
    Windows CI job and lint, the `key@windows` messages and `golang.org/x/sys` go; every file the
    port changed is as before it, plus what came after (Source Control, `make ci`) without its
    Windows parts: the Quick Pick takes its signals from `os/signal`, the Windows rename retry
    (T64.4) and `make test-windows` go.
- [x] **T65.2 Nothing else breaks** · S
  - AC: `make ci` passes (lint and vet for Linux and macOS, race, test-macos, size, e2e), the
    GoReleaser snapshot builds darwin and linux only, README, README.tr and CONTRIBUTING say WSL
    for Windows; the check-before-push and commit-and-release skills and AGENTS.md lose their
    Windows steps; CHANGELOG.
- [x] **T65.3 Actions do not trip over the sidebar's index lock** · S
  - AC: §18: v0.49.1's macOS e2e job failed: a checkout from the Quick Pick found
    `.git/index.lock` taken by the sidebar's own refresh (`git diff` rewrites the index, even
    with `GIT_OPTIONAL_LOCKS=0`). gitops' reads, gitinfo and the explorer's numstat take no lock;
    actions wait up to 2 s for one; unit tests: a read leaves the index file as it was, an action
    waits for a lock released after a moment; the e2e tests' own git takes no optional lock.

## M64: CI's checks run before a push (added 2026-11-12)

User request: the pipelines failed again (v0.49.0 on macOS and Windows); coding agents must run
the tests here before code is pushed.

- [x] **T64.1 `make ci`** · S
  - AC: D59: `make ci` runs lint-all (golangci-lint and vet for Linux, Windows and macOS),
    generate-check, check-fixtures, shellcheck (says so when it is missing), race, test-macos
    (`TMPDIR` behind a symlink), test-windows (from WSL on the Windows host with a throwaway
    profile; elsewhere it vets the Windows build and says the tests did not run), size and e2e;
    it stops at the first failure. `PKGS=` narrows the test targets.
- [x] **T64.2 The skill** · XS
  - AC: `.agents/skills/check-before-push` (linked from `.claude/skills`): when and how to run
    `make ci`, what to do with a red CI run, how to write tests that pass on macOS and Windows;
    AGENTS.md, CONTRIBUTING.md and the commit-and-release skill point to it.
- [x] **T64.3 Green on macOS and Windows** · XS
  - AC: Add to .gitignore asks git where the folder is in the working tree (no path comparison, so
    symlinks and 8.3 names do not matter); TestShowLoad and TestMenusCarryNoData build what they
    expect with the platform's shell; tests that run their own binary find it with
    `os.Executable`; `make test-macos` and `make test-windows` pass.
- [x] **T64.4 Writes on Windows wait for a file in use** · XS (removed with the port, M65)
  - AC: §18: atomic writes retry a rename Windows refuses while another process has the file open
    (seen by `make test-windows` on a machine with a virus scanner: TestConcurrentWritersLoseNothing
    failed 4 runs in 5); a Windows unit test holds the file open and Rename waits for it.

## M63: Source Control like VS Code (added 2026-11-12)

User request: the Git view should be VS Code's, every feature of it: the ⋯ menu with its
submenus and the rest.

- [x] **T63.1 Git reads and actions** · L
  - AC: D58, §9.3 "Source Control like VS Code": `gitops` reads conflicts, the operation under
    way, local and remote branches, tags, stashes, remotes, worktrees and the graph (refs, ↑ ↓,
    cached by refs); runs every action of the menus; `~/.imux/git.log`; unit tests on real
    repositories.
- [x] **T63.2 Quick Pick** · M
  - AC: `imux __quickpick`: filter (the best match first), groups, keys, mouse, an argv per item
    run in the popup or once it closed, what was typed naming a new branch; unit tests.
- [x] **T63.3 ⋯ and its submenus** · L
  - AC: `imux __scm menu`: VS Code's items and groups, ‹ Back, prompts, confirmations, Checkout
    to…, the commit dropdown, the file and commit menus; no repository data in a command line;
    unit tests (the items, the argv).
- [x] **T63.4 The view** · M
  - AC: ⋯ and ▾, Continue / Commit Merge, ☁ Publish, MERGE CHANGES, U and !, list / tree and the
    sort, the graph, Initialize / Clone outside a repository, right-click menus, the branch row
    opens Checkout to…; unit tests (render).
- [x] **T63.5 End to end** · S
  - AC: e2e: ⋯ → Stash › Stash… stashes; the branch row → Checkout to… → a typed branch switches;
    a right-click on an untracked file → Add to .gitignore.
- [x] **T63.6 Docs** · XS
  - AC: README and README.tr; CHANGELOG (with the Windows port of M62).

## M61: No Image button (added 2026-11-11)

User request: the Image button next to History is needless, since an image is pasted with Ctrl+V
anyway.

- [x] **T61.1 No Image button** · XS
  - AC: D57: the row under "Continue this chat in" shows History alone; the button's action, icon
    and label go; Ctrl+V and Alt+V still paste images (their e2e tests stay); a unit test (no 📎
    in the toolbar); the button's e2e test goes.

## M60: Menus that take the mouse, a simpler sidebar (added 2026-11-11)

User request: a click on a menu closes it without choosing, so it has to be used with the
keyboard; the HTML menu's labels are long and have a needless icon; the sidebar's green and
yellow dots mean nothing to the user; the spinner should turn only while an agent really works
(it turned while the user moved through a list in the Copilot CLI).

- [x] **T60.1 Menus in a popup before tmux 3.5** · M
  - AC: D54, §9.3 "Menus on tmux before 3.5": every menu goes through `ShowMenu`; before 3.5 a
    popup the size of the menu at its place, with the title, items, keys and separators; hover
    selects, a click, Enter or a key chooses, Esc/q/✕ close; the command runs after the popup
    closed, its formats expanded for the menu's pane and client; the sidebar no longer waits
    `OpenAfter` before menus; unit tests (reading a display-menu, the size, the keys and clicks,
    the command the choice writes); e2e on tmux 3.2a and 3.4: a click on ⏻ Exit opens its menu
    and a click on Back to the terminal detaches; a menu opened while a status-line message is
    shown (the commit message's "Write another") shows its items. Versions and updates calls
    tmux old below 3.4, not 3.5 (menus no longer need it).
- [x] **T60.2 Short HTML labels** · XS
  - AC: D51: Open in browser / Open in a new tab / Edit / View, no 🌐 (en, tr); unit test.
- [x] **T60.3 No dots** · XS
  - AC: D56: no dirty dot on project rows, no "Done" dot on project and agent rows, no green
    glyph in the tab bar; unit tests (the rows, the tab state).
- [x] **T60.4 Working means the agent says so** · S
  - AC: D55: for Claude Code, Codex, Copilot and Antigravity a screen change is work only while
    the screen shows their busy line; OpenCode as before; unit tests (a list moved through
    without the busy line, the busy line of each agent, an unknown agent); e2e: the fake agents
    print a busy line while they work.

## M59: Green CI on tmux 3.8 and Arch Linux (added 2026-11-11)

Report: the CI pipelines fail. The e2e jobs failed on most pushes since 2026-11-09, each time in
other tests (macOS more often); the release smoke test fails on Arch Linux. Run against tmux 3.2a,
3.4 and 3.8 (macOS's Homebrew tmux): three tests failed every time on 3.8 and two now and then on
all three.

- [x] **T59.1 A double click's second press on tmux 3.7+** · S
  - AC: D53, §9.3 "A double click's second press": a sidebar click that shows the client another
    project or tab sets `@imux_moved` to that client, tmux clears it 0.5 s later, and
    `MouseDown1Pane` drops that client's presses on a sidebar while it is set; unit tests (the
    binding in the config golden files, the commands of a switch); e2e: a double click on another
    project stays there on tmux 3.8 (it went there and back).
- [x] **T59.2 e2e tests that hold on tmux 3.8 and a slow runner** · S
  - AC: `rowOf` takes a sidebar frame only when it was drawn for the pane's height as well as its
    width (after a client attaches the rail is still packed for the detached size for a moment,
    and a click on 🔀 hit 🔍), and its timeout shows the sidebar; the project row is found by its
    badge (`B  blog`), not by a working spinner after the name; Settings closes with `q` (tmux 3.8
    holds a lone Escape 500 ms while the terminal has not answered its queries, and the test
    terminal never does: the Escape closed the next menu); a double click on a menu button presses
    twice at once (from tmux 3.5 the menu opens on the first press, and a second press after it is
    drawn closes it); the suite passes on tmux 3.2a, 3.4 and 3.8, the flaky ones 8 times in a row
    on each.
- [x] **T59.3 The release smoke test on Arch Linux** · XS
  - AC: the Arch container installs curl with `pacman -Syu` (a partial upgrade, `-Sy`, pulled
    openssl 4 against packages built for openssl 3 and failed).

## M58: HTML asks, the tabs over the terminal (added 2026-11-11)

User request: a click on an HTML file should ask how to open it (then Settings need not); the tab
bar should start above the terminal, not at the far left, also when the panel opens or closes.

- [x] **T58.1 How to open an HTML file** · S
  - AC: D51: a click on an HTML file opens a menu at the click (window, tab, edit, view); the
    Settings row is gone; unit tests (the menu's items and commands); e2e: the menu's window
    item opens the page live.
- [x] **T58.2 The tab bar over the terminal** · S
  - AC: D52: the bar is padded by the sidebar's width and border (none when zoomed or without a
    sidebar); unit tests (the format, the config golden files); e2e: the first tab starts over the
    agent pane, and moves when the panel closes to the rail.

## M57: Settings as menus, the editor among them (added 2026-11-11)

User request: Settings rows should not flip through values; a click or Enter should open a popup
to choose from, for every row; and an editor choice: micro, nano or Neovim.

- [x] **T57.1 Menus of values, the Editor row** · M
  - AC: D50, §9.3 "Settings": every row with values opens a menu (`●` the current value), the
    choice saves and reopens Settings; Editor: Auto / micro (got when missing) / nano / Neovim /
    Neovim + Copilot, replacing the AI completion row; `imux editor use <editor>`; unit tests
    (the rows and their menus, the editor's choices, the saved keys).

## M56: Leaving i-mux, and coming back (added 2026-11-11)

User request: a button that closes i-mux and goes back to the normal terminal (top right); `imux`
no longer offers the workspace it used to.

- [x] **T56.1 The Exit button** · M
  - AC: D48, §9.3 "Leaving i-mux": ⏻ Exit at the right of the tab bar (a user range from tmux
    3.4, the right range before, the + then outside any range); its menu: back to the terminal
    (detach), quit (confirm, snapshot, live preview servers stopped, server killed); `imux quit
    [--yes]`; unit tests (the tab bar on old and new tmux, the bindings, the menus); e2e: Exit →
    Back to the terminal detaches the client and the agents keep running.
- [x] **T56.2 The picker at start** · S
  - AC: D49: `imux` in an open project's folder attaches; elsewhere with projects open or a last
    session to restore, the picker with Restore first and this folder next; unit tests (when it
    asks, the rows); e2e: `imux` in a new folder while a project is open asks, and Enter opens
    the folder.

## M55: Live preview in the browser (added 2026-11-11)

User request: the terminal browser of M52 draws pages badly ("beyond terrible"); something else.
Replaces M52.

- [x] **T55.1 The live server** · M
  - AC: D47, §9.3 "Live preview in the browser": `imux __web-serve` per folder on 127.0.0.1, a
    port from the folder's path, its record in `~/.imux/web`; Host checked, no dot paths, nothing
    outside the folder, folder lists, no caching; `live.js` injected; pages reload when a served
    file changes; idle exit; `imux web [open|stop]`; unit tests (serving, injection, Host, dot
    paths, links out, listing, reload on change, open with and without the token, the port).
- [x] **T55.2 Opening the browser** · M
  - AC: WSL: an app window of the default browser when Chromium-based, else Edge; a tab with
    `explorer.exe`; VS Code's helper; macOS; Linux; no browser: the URL in the message and the
    clipboard; an open page shows the next one and is raised; Settings cycles window / tab /
    editor; the terminal browser removed, its folder deleted; unit tests (the argv per system,
    the settings row); e2e (a fake browser): a click on index.html opens its URL, the page
    reloads when the file changes, a click on another page shows it in the open page without
    a new browser.

## M54: The Agents icon counts the working agents (added 2026-11-11)

User request: the number of working agents on the rail's Agents icon too.

- [x] **T54.1 A badge on the Agents icon** · S
  - AC: D46: a blue number right of the icon while agents work (all projects, limited ones not
    counted), `9+` above nine, none at zero; with the panel closed and on another view; unit
    tests (the rail with 0, 3 and 12 working; emoji and Nerd Font icons keep their width).

## M53: micro comes with i-mux; the editor shows where you are (added 2026-11-11)

User request: files open in nano, and the README says to install micro by hand; the editor should
have a scroll bar too.

- [x] **T53.1 Getting micro, scroll bars in the editors** · M
  - AC: D45, §9.3 "Editing" and "Getting micro": the first edit without an editor asks once (Get
    micro / Use nano, remembered); micro 2.0.15 downloaded, its SHA-256 checked, unpacked into
    `~/.imux/tools`, used before `$VISUAL`; `imux editor [install]`; Settings → Get the micro
    editor…; micro with `-scrollbar true`, nano 5.0+ with `--indicator --mouse`; unit tests
    (assets, the tarball, the editor's argv, the dialog); e2e: a file opened without an editor
    asks, Use nano opens nano's stand-in and is not asked again.

## M52: Web pages in a pane (added 2026-11-10)

User request: see web pages (an HTML file, for example) in i-mux; on by default but downloaded only
when wanted, installed and turned on or off from Settings. Replaced by M55 (v0.45).

- [x] **T52.1 The browser and `imux web`** · M
  - AC: D44, §9.3 "Web preview": `imux web [install|remove|open]`; Carbonyl 0.0.3 downloaded, its
    SHA-256 checked, unpacked into `~/.imux/tools`, `--version` run, missing Linux libraries named;
    `[web] enabled` and `browser`; the pane's `imux __web-pane`; unit tests (assets, URLs, unzip
    with modes and no path outside, install from a test server, the pane's command).
- [x] **T52.2 Opening pages from the UI** · M
  - AC: a click on an HTML file opens its preview beside the target (not installed: the popup); a
    second click loads it in the same pane; Open Preview and Open in Browser in the file menu;
    **🌐 Web page…** in "+"; the Settings menu; unit tests (menus, popup); e2e with a fake browser
    (`[web] browser`): a click on index.html opens a pane running it with the file's URL, a second
    click reuses that pane.

## M51: Search in History (added 2026-11-10)

User request: a search in History, to find a conversation by text said in it.

- [x] **T51.1 History's search popup** · M
  - AC: D43, §9.3 "Search in History": the menu's **🔍 Search their text…**; the popup lists the
    folder's conversations and finds the ones whose messages have the text (byte search first,
    then parsed), with up to three messages each, folding, the bar; Enter opens the
    conversation's menu; unit tests (search, popup); e2e: a word said later in a conversation
    finds it, Enter opens its menu.

## M50: Scroll bars, and a clearer search (added 2026-11-10)

User request: scroll bars wherever things scroll (search, a git diff, ...) that do not spoil the
look; the search results are colorless, all open, with no way to fold them all and no scroll bar.

- [x] **T50.1 The scroll bar** · S
  - AC: D42: `ScrollBar` / `ScrollTo` / `Cut`; the sidebar's lists (a click scrolls, the keyboard
    skips it), Go to File, the folder picker; unit tests (thumb size and place, clicks, widths).
- [x] **T50.2 i-mux's pager** · M
  - AC: D42, §9.3 "Scroll bars": the diff, file, commit and limits viewers; less's keys, search
    keeping colors, the bar (click, drag), `v` edits the file; unit tests; the viewers' e2e.
- [x] **T50.3 The search, drawn** · S
  - AC: icons, folder, count pill, colored matches, the fold-all button, folded when more than 5
    files, the bar (click); unit tests (render, clicks, folding).

## M49: Install a missing agent from its button (added 2026-11-10)

User request: clicking an agent in "Continue this chat in" that is not installed should show a small
warning popup with how to install it.

- [x] **T49.1 The install popup** · S
  - AC: D41, §9.3 "Not installed": the popup (title, why, command, Install now / Close; keys,
    clicks after 400 ms); the program the command needs missing → how to get it, Close only;
    `[agents.<id>] install`; Install now runs it, then Enter continues the chat; unit tests (the
    popup, the command); e2e: click → popup → Install now (a fake install) → Enter → the pane runs
    the agent.

## M48: Choose where a new agent opens (added 2026-11-10)

User request: after choosing the agent with the "+" of Agents, ask up, down, left, right, or a new
tab.

- [x] **T48.1 The place menu** · S
  - AC: D40: "+" → the agent menu (`__menu --new`) → the place menu (`__menu --place <agent>`):
    right, left, down, up (`split --before` for left and up), new tab; Terminal too; the "No
    agents" pane still fills; unit test (menus and commands); e2e: left, down (Terminal), a new
    tab (double click on "+").

## M47: Faster switching on the left (added 2026-11-10)

User report (after v0.39.1): switching on the left is still slow.

- [x] **T47.1 Every click at once** · S
  - AC: D39: projects, agents elsewhere, Continue in, files and popups act on the press; menus
    (tmux < 3.5) and the command prompt wait; `MouseDown1Pane` notes the pane and
    `SecondClick1Pane` / `TripleClick1Pane` reach a sidebar only from the same pane (checked on
    tmux 3.2a); popups ignore clicks for 400 ms; one gather per click; unit test (which clicks
    wait); config tests (golden, mouse off, real tmux); e2e: a double click on another project
    switches once (fails without the bindings), the switch within 300 ms.
- [x] **T47.2 Fresh while shown** · XS
  - AC: a shown sidebar gathers every 3 s with its project's git status ≤ 2 s old; a hidden one
    does not; e2e: a file written outside shows in Source Control (`TestCommitSuggest`).

## M46: The spinner means work; AGENTS counts it (added 2026-11-10)

User report: a project's spinner turned although its agent did nothing; and the AGENTS title could
show how many agents work.

- [x] **T46.1 Work, not typing, resizes or restarts** · S
  - AC: D38: a change of only the input line and below (Claude Code, Copilot, Codex, Gemini
    layouts), a resize of a quiet pane and its redraw, and a pane met for the first time are not
    work; a resize during work still is; a restarted watcher keeps the last one's times
    (`activity.json`); unit tests (`Typing`, the detector over ticks; fails without the change).
- [x] **T46.3 The time survives a restart** · XS (v0.41)
  - AC: a restarted watcher reads the panes' `@imux_state` first and writes only changes (the
    time next to an agent stayed "<1m" after every install); unit test.
- [x] **T46.2 The working count** · XS
  - AC: the AGENTS title shows a spinner and "N working" (all projects, not limited ones; the
    number alone below 30 columns; folded, "· panes"); nothing extra when none works; i18n en/tr;
    unit test.

## M44: File icons in the explorer (added 2026-11-10)

User request: make the Files view nicer, for example with the file types next to the names.

- [x] **T44.1 Icons by file type, folders, indent guides** · S
  - AC: D35, §9.3 "File icons in the explorer": an icon per entry (whole names, README /
    LICENSE, tests, extensions; folders open and closed), Seti's glyphs and colors with the
    editor's icons and an emoji otherwise, every icon of a theme the same width; faint indent
    guides; git colors and the selection unchanged; unit tests (the lookup, widths, a row in
    both themes) and the explorer e2e.

## M43: The Nerd Font comes with i-mux (added 2026-11-10)

User report: the Nerd Font does not work, and a new PC does not come with it. Cause: i-mux never
installed one (the README only named a Homebrew cask); under WSL the font must be in Windows,
where the terminal draws text, and neither Windows Terminal nor VS Code used one; auto-detection
could not see the terminal from WSL.

- [x] **T43.1 Install and set the font** · S
  - AC: `internal/nerdfont` (D34): Nerd Fonts' JetBrainsMono pinned with its SHA-256, four
    styles for the user alone on macOS, Linux and WSL (the Windows user's fonts and registry);
    Windows Terminal's and VS Code's font set with `internal/jsonc` (comments and formatting
    kept) and a backup; `imux font [install] [--yes] [--check]` asks first and never takes no
    answer for yes; icons become `nerd`; unit tests with fake Windows programs.
- [x] **T43.2 Offer it everywhere, see it from WSL** · S · deps: T43.1
  - AC: the installer offers it (`--yes`, `--no-font`, skipped when `--check` passes); Settings
    → Install the Nerd Font for the icons… (popup); `imux doctor` reports it; `auto` reads
    Windows Terminal's and the Windows user's VS Code fonts under WSL (`IMUX_WINDOWS_HOME`);
    e2e for the installer (a fake imux) and the Settings popup; README.

## M40: Project skills in the sidebar (added 2026-11-09)

User request: a Skills tab on the left to see and monitor the project's skills across agents, edit
them, and have the active LLM agent write or improve them.

- [x] **T40.1 Discovery, validation and cross-agent sharing** · S
  - AC: `internal/skills` (D32, §9.3 "Skills"): finds `<name>/SKILL.md` across `.agents/skills`,
    `.claude/skills`, `.github/skills`, `.opencode/skills`, `.agent/skills` and `.codex/skills`;
    parses YAML front matter and reports missing/invalid `name` or `description`; `Sync` links
    `.agents/skills/<name>` into `.claude/skills/<name>`; `Share` moves a single-agent skill into
    `.agents/skills/` and links it; `Create`, `WritePrompt` and `ImprovePrompt`; unit tests.
- [x] **T40.2 Skills view, menus and agent authoring** · S · deps: T40.1
  - AC: 🧩 **Skills** on the activity rail (`ViewSkills`, key `5`); **✨ Write with Agent…** and
    **➕** template creation; each skill shows its status (`●` / `⚠`), per-agent visibility badges
    (`✻▲◉◎▣`), and description or validation warning; skill menu (Open, View, Add to Agent,
    Improve with Agent, Share with all agents, Delete); unit and e2e tests; README.

## M42: One New skill button and dialog (added 2026-11-10)

User request: writing a skill was poor: a ✨ button and a ➕ next to it, and a tiny prompt in the
status line. One button should ask which agent writes the skill, then open a pane with that
agent, starting with i-mux's skill-writing prompt; a separate popup is fine.

- [x] **T42.1 The New skill dialog** · S
  - AC: `internal/skilldialog` (D33, §9.3 "Skills"): step 1 picks an agent for a new pane (the
    current pane's first; not installed ones cannot be chosen), the current pane's agent or a
    blank template, by arrows, letters or a click; step 2 is a wrapped text box (cursor keys,
    Home/End, Delete, Ctrl+W, Ctrl+U, bracketed paste and unbracketed newlines) or the template's
    name with its path and a taken-name warning; the sidebar's input parser learns Home, End,
    Delete, Ctrl+A/E/W and bracketed paste; unit tests.
- [x] **T42.2 One button, the agent in a new pane** · S · deps: T42.1
  - AC: the Skills view's single ✨ New skill… button (and the empty space's menu) opens the
    dialog in a popup sized to the client; `WritePrompt` puts the guide first and the description
    last (empty: the agent asks first, in the UI's language); `workspace.StartPrompt` opens the
    agent beside the target pane (below when narrow, in the "No agents" pane's place), named
    "new skill"; "this pane" pastes it there; the template is created and edited in the same
    popup; tmux's command-prompt is no longer used for skills; e2e with the fake agents; README.

## M62: Native Windows port on psmux (added 2026-11-11)

Removed in v0.50 (M65, D60).

- [x] **T62.1 Platform abstractions and psmux adapter** · L
  - AC: `internal/sysx`, `internal/winx`, `internal/pwshshim` and the `internal/tmux` psmux
    adapter (`%pane~session` routing across per-session psmux servers, PowerShell command lines,
    menus and popups); `install.ps1`, `scripts/build-psmux.ps1`, `scripts/build-windows.ps1`,
    `docs/WINDOWS.md`; unit tests pass on Linux, macOS and Windows (`ci.yml`).

---

## Testing strategy

### Unit tests (every package)

- **Synthetic fixtures only.**
  - Fixtures are hand-written or generated by builders in `internal/testutil`, using fake
    paths under `/home/test/...`.
  - Real session files are never copied, trimmed or "anonymized" into the repo.
  - `scripts/check-fixtures.sh` enforces this in CI (T0.9).
- **Per-format fixture matrix** (`testdata/<agent>/`):
  - minimal session, multi-turn with tools, compaction (Claude);
  - JSON vs JSONL (Gemini);
  - missing `events.jsonl` (Copilot); `event_msg` duplicates (Codex);
  - API error lines, chained handoff as first message;
  - Turkish and emoji text;
  - unknown line types, malformed lines, a truncated final line;
  - an empty file and a huge line.
- **Golden files** for handoff rendering, `tmux.conf` and `docs/CONFIG.md`, updated with `go test ./... -update`.
- **Property tests** for the handoff builder: budget limits, marker, valid UTF-8, determinism.
- **Fuzzing** (`go test -fuzz`) for each provider's `Load`, the reset-time parser, the tool summarizer
  and the tmux format parser. CI runs a 60 s `fuzz-short`; longer runs are manual. Any crash input is
  committed as a regression seed.
- **Fake clock and fake tmux runner** are injected through interfaces. No sleeps in unit tests.

### End-to-end tests (`go test -tags e2e ./e2e/...`)

- **Isolation:**
  - each test uses its own tmux socket (`IMUX_TMUX_SOCKET=imux-test-<rand>`);
  - its own `IMUX_HOME` and fake agent homes in a temp dir;
  - the server is killed in cleanup.
- **Fake agents** (`e2e/fakeagents/fake-{claude,gemini,copilot,codex}`, POSIX sh). Each one:
  - records its argv (NUL-separated) and selected env to `$IMUX_FAKE_LOG/<agent>-<n>.argv`;
  - optionally writes a synthetic session file into the fake agent home, so discovery and
    parsing run for real;
  - prints a scripted screen, e.g. `FAKE_SCRIPT="sleep 3; echo '5-hour limit reached ∙ resets 3pm'"`;
  - echoes stdin lines to a file (bus and paste tests);
  - exits with a configurable status after a configurable delay (pane-died tests).
- **Wiring:** config sets `agents.<x>.bin` to the fake scripts. `PATH` in e2e contains no real agent
  binaries; the harness fails the test if resolution ever returns a path outside `e2e/fakeagents`.
- **Assertions** use the tmux command recorder (`IMUX_TMUX_RECORD`), pane options (`show -p`),
  `capture-pane` output, and fake-agent argv logs.
- **Attached client** (menus, `ask` mode, Q24): a pty-backed `tmux attach` started by the test (a
  test-only pty helper, kept out of the main binary's dependencies).
- **UI input simulation** (T2.8d): the test writes bytes to that client's pty, exactly like a terminal would:
  - keys (`0x1c` for `Ctrl+\`, menu mnemonics, arrows, Enter);
  - SGR mouse press and release sequences at computed agent-bar or border coordinates.

  This exercises the real tmux bindings, menus and ranges end to end, without a real terminal emulator.
- **CI matrix:** ubuntu-latest (apt tmux) and macos-latest (brew tmux).

### Performance tests

- `go test -bench` for the JSONL reader, providers (64 MB generated fixture) and the handoff builder.
- `scripts/measure-watch.sh` for watcher RSS and CPU (§17), run before each release; results go in `docs/PERF.md`.
- A CI binary-size gate (< 15 MB).

### Manual tests (USER), never automated

Some things need real CLIs, real terminals or the user's quota: T1.16, T2.13, T5.6 (sandbox part)
and T6.1. They are explicit tasks, and their outcomes update ARCHITECTURE §20/§21.

### Never in tests

- launching `claude`, `gemini`, `copilot` or `codex`;
- network access;
- reading the developer's real `~/.claude`, `~/.gemini`, `~/.copilot`, `~/.codex` or `~/.imux`.
  Every test sets `HOME`, `IMUX_HOME`, `CLAUDE_CONFIG_DIR` and `CODEX_HOME` to temp dirs.
