# Changelog

All notable changes to i-mux are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.3.0] - 2026-11-07

### Fixed

- **Clicking "Antigravity" on the agent bar did nothing.** Its range `imux:antigravity` is 16 bytes
  and tmux drops ranges over 15; it is now `imux:agy`. A test covers every agent's range.
- **Chained handoffs cut the original request at the user's own `## ` headings** (e.g. a pasted
  brief): reading a handoff back now splits only at its own section headings.

### Changed

- **Handoffs from Antigravity come from its transcript**, not the screen:
  `~/.gemini/antigravity-cli/brain/<id>/.system_generated/logs/transcript_full.jsonl` gives the
  user and assistant turns, the commands it ran and the files it read, token usage and its task
  list, without terminal decoration or the account e-mail. Switching back to Antigravity resumes
  its own conversation (`agy --conversation <id>`) with only what happened elsewhere.
- `imux limits` shows Antigravity's requests and tokens today.

### Removed

- **Gemini CLI.** Google stopped serving personal accounts in it on 2026-06-18; use Antigravity CLI.
  A config that still names `gemini` loads with a warning.

## [0.2.0] - 2026-11-06

### Added

- **Antigravity CLI (`agy`)** as a fifth agent: Google's successor to Gemini CLI for personal
  Google accounts. Switching to it passes the handoff with `agy -i`. Its conversations are not
  readable files, so a handoff *from* it is rebuilt from the pane's scrollback and carries its task
  list from `brain/<id>/task.md`. It follows Claude in the default order (`a` in the menu).
- `imux doctor` warns when Gemini CLI is set to sign in with a personal Google account, which
  Google stopped serving in Gemini CLI on 2026-06-18, and points to Antigravity CLI or an API key.
- The release smoke tests now run for every release (they were never triggered for v0.1.0).
- The watcher recognizes Gemini CLI's "no longer supported for Gemini Code Assist for
  individuals" sign-in error and offers to continue in another agent.

## [0.1.0] - 2026-11-06

First release (v0.1 and the MVP). Install with one line on macOS, Linux and WSL:
`curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh`.

### Added

- **Handoff between agent CLIs** (Claude Code, Gemini CLI, GitHub Copilot CLI, Codex CLI): parsers
  for each CLI's session files and a budgeted markdown handoff (original request, task list, files
  touched, repository state, condensed conversation, where it stopped) with best-effort secret
  redaction. `imux handoff [--print]` works without tmux.
- **Workspace in tmux** on a private socket (`tmux -L imux`): `imux [agent]`, `imux attach`,
  `imux ls`, `imux split`, pane labels, and a drop back to the shell when an agent exits.
- **Switching from the UI:** the pane menu (`Ctrl+\`, `prefix a`, right-click on a border), the
  clickable agent bar (tmux ≥ 3.4), and the session picker (`prefix S`) to continue any earlier
  conversation of the project, natively or in another agent. `imux switch` from the command line.
- **Native resume-back:** returning to an agent that already ran in a pane resumes its own session
  with a short "while you were away" delta; a resume that fails falls back to a new session with
  the full handoff.
- **Limit watcher:** detects usage-limit messages on agent screens and, per `on_limit`, asks
  (`ask`), switches (`auto`) or only shows them (`off`); limits appear in the status bar, the agent
  bar and pane borders.
- **`imux limits`:** limits seen on screen and local usage per agent (`--json`).
- **Agent bus:** `imux panes`, `read`, `send`, `wait` and `snippet` let agents work together,
  with sender tags, rate and size limits and an optional confirmation menu.
- `imux doctor`, `imux config init|path|show`, `imux sessions`, `imux pick`.
- Distribution: GoReleaser builds for macOS and Linux (amd64, arm64), `install.sh`, a Homebrew
  formula, and a release smoke test.
