# Changelog

All notable changes to i-mux are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.12.0] - 2026-11-07

### Added

- **Plain terminals:** the **+** next to Agents, the tab bar's **+**, `prefix c`, `prefix v` and
  `prefix -` now end with **Terminal (shell)**, which opens your shell in the project folder
  without an agent. `imux split shell` and `imux tab shell` do the same.

## [0.11.0] - 2026-11-07

### Added

- **Agent menu:** right-click an agent in the sidebar (or click the one already selected) to go to
  it, **rename** it or **close** it. The name stays when the chat moves to another CLI; an empty
  name brings back the agent's own. Closing the last agent of a tab closes the tab; both ask first.

## [0.10.0] - 2026-11-07

### Added

- **Recent projects reopen as you left them:** the folder picker lists RECENT projects first, with
  their tabs, agents and when you last used them; choosing one brings back its tabs, pane names,
  layout and agents, each resuming its own conversation. Opening a project folder in any other way
  (`imux` in the folder, `imux open`) does the same.
- **Restore last session:** after a restart (when the tmux server went away), the picker offers to
  bring back every project that was open.
- `imux open -d <folder>` opens a project without showing it.

## [0.9.0] - 2026-11-07

### Added

- **Folder picker:** opening a project (the **+** next to Projects, `o`, `imux open`) shows a list
  that narrows as you type: open and recent projects first, then the git repositories in your
  home folder. Type a path starting with `~/` or `/` to browse folders; Tab goes into one.
- `imux` in the home folder asks which project to open instead of making the home folder one.
- **Close projects and tabs:** click the current project (or right-click any) for its menu with
  **Close project…**; the current tab has a **×**; both ask first. `imux close` from the command
  line. The client moves to another open project.

### Fixed

- Clicks on the tab bar's **+** and **×** (and the agent bar) could open a menu that closed at once
  when the mouse button was released after it appeared; they now act on release.
- The tabs after an agent no longer shift when it starts or stops working.

## [0.8.0] - 2026-11-07

### Added

- **Tabs:** a tab bar at the top shows the project's tabs (tmux windows) with their names and
  states; click one to switch, **+** (or `prefix c`) opens a new tab with the agent you pick, and
  `prefix ,` names it. `imux tab <agent>` does the same from the command line. `[tmux] tabs =
  false` keeps the status line at the bottom.

### Fixed

- After an upgrade, the background watcher and the sidebars of running sessions kept running the
  old version. They now restart with the new binary on their own, and the tmux config is
  refreshed.
- The sidebar takes its width back when a bigger terminal attaches.

## [0.7.0] - 2026-11-07

### Changed

- **New sidebar:** **Projects** with a colored letter, branch, a dot for uncommitted changes and
  the most urgent state of their agents; **Agents** of every project grouped by project, each with
  its icon, name, how long ago its state changed and its state; **Continue this chat in** as
  buttons; a toolbar with Rename, Changes (a menu of changed files with their diffs), History,
  Limits and Help. The default width is 34 columns.
- Agents are named after their terminal title when they set one (Claude Code does), unless you
  name them.

### Added

- **Needs you:** an agent that asks a question (a permission prompt, a numbered Yes choice, y/n)
  is marked in the sidebar.
- **Done:** an agent that finished while you looked elsewhere gets a green dot until you look.
- A working agent's spinner turns; it costs no tmux calls.

## [0.6.0] - 2026-11-07

### Changed

- **A switch shows one short message instead of the whole conversation.** i-mux saves the handoff
  in `~/.imux/handoffs` and the new agent gets three lines: who it continues from, a sentence in
  your language, and the file to read. Claude, Copilot and Antigravity get `--add-dir` for that
  folder so reading it asks nothing. The document no longer appears in the process list either.
  `[handoff] deliver = "inline"` restores the old behavior.
- **New sidebar layout:** PROJECTS, AGENTS (cards with name, state and model), CHANGES and MOVE THIS
  CHAT TO (without the agent already in use), separated by space instead of lines; a small toolbar
  (Rename, History, Limits, Help) stays at the bottom. **+ New agent…** opens a pane side by side
  or below, whichever suits the pane's shape.

### Fixed

- Chained handoffs (A → B → C) keep the original request when the middle agent got the short
  message: i-mux follows it back to the saved file.

## [0.5.0] - 2026-11-07

### Added

- **PROJECTS** in the sidebar: every open project with its agents; click an agent to focus it or
  another project to switch to it. **+ Open a project…** (or `imux open <folder>`) opens a folder as
  a project and switches to it.
- **CHANGES** in the sidebar: the branch and the files `git status` lists; click one for its diff.
- **Pane names:** **✎ Name this pane** (or `imux rename <name>`); the name shows in the sidebar and
  the pane border.
- Agents show who makes them (Anthropic, Google, GitHub, OpenAI); headings use plain words.

### Changed

- **After a switch the new agent waits for you.** It confirms in one sentence that it has the
  context instead of starting work (and spending tokens) on its own.
  `[handoff] on_arrival = "continue"` restores the old behavior.

## [0.4.0] - 2026-11-07

### Added

- **Sidebar** on the left of each window, replacing the bottom agent bar as the
  default: the window's panes with their state (working, idle, limited, exited), the agents the
  active pane can continue in, Split right / Split down buttons that ask which agent to start,
  Sessions, Limits and Keys, all clickable, and usable from the keyboard. `prefix b` hides or shows
  it; `prefix v` / `prefix -` split with an agent chooser. It sleeps until something changes (tmux
  hooks and the watcher wake it). `[tmux] sidebar = false` and `agent_bar = true` bring back the old
  bar.
- The watcher records each agent pane's state (`@imux_state`: working or idle).

### Fixed

- A double click on a sidebar button could close the menu it opened and send the next key to the
  sidebar: click actions now wait out tmux's double-click time and a second click is ignored.

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
