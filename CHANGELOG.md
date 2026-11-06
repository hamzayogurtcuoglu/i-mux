# Changelog

All notable changes to ajan are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

First release candidate (v0.1 and the MVP).

### Added

- **Handoff between agent CLIs** (Claude Code, Gemini CLI, GitHub Copilot CLI, Codex CLI): parsers
  for each CLI's session files and a budgeted markdown handoff (original request, task list, files
  touched, repository state, condensed conversation, where it stopped) with best-effort secret
  redaction. `ajan handoff [--print]` works without tmux.
- **Workspace in tmux** on a private socket (`tmux -L ajan`): `ajan [agent]`, `ajan attach`,
  `ajan ls`, `ajan split`, pane labels, and a drop back to the shell when an agent exits.
- **Switching from the UI:** the pane menu (`Ctrl+\`, `prefix a`, right-click on a border), the
  clickable agent bar (tmux ≥ 3.4), and the session picker (`prefix S`) to continue any earlier
  conversation of the project, natively or in another agent. `ajan switch` from the command line.
- **Native resume-back:** returning to an agent that already ran in a pane resumes its own session
  with a short "while you were away" delta; a resume that fails falls back to a new session with
  the full handoff.
- **Limit watcher:** detects usage-limit messages on agent screens and, per `on_limit`, asks
  (`ask`), switches (`auto`) or only shows them (`off`); limits appear in the status bar, the agent
  bar and pane borders.
- **`ajan limits`:** limits seen on screen and local usage per agent (`--json`).
- **Agent bus:** `ajan panes`, `read`, `send`, `wait` and `snippet` let agents work together,
  with sender tags, rate and size limits and an optional confirmation menu.
- `ajan doctor`, `ajan config init|path|show`, `ajan sessions`, `ajan pick`.
- Distribution: GoReleaser builds for macOS and Linux (amd64, arm64), `install.sh`, a Homebrew
  formula, and a release smoke test.
