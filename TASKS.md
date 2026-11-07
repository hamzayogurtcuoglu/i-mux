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
