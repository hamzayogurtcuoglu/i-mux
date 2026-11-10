# Changelog

All notable changes to i-mux are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.37.0] - 2026-11-10

### Changed

- **Skills: one ✨ New skill… button and a dialog.** The ✨ Write with Agent… and ➕ buttons, and
  tmux's one-line prompt in the status line, are gone. ✨ New skill… opens a popup that asks who
  writes the skill: an agent in a new pane (Claude, Antigravity, Copilot, Codex or OpenCode; the
  current pane's agent first), the agent of the current pane (it knows the conversation), or you,
  from a blank `SKILL.md`. Then it asks what the skill should do in a real text box (wraps, cursor
  keys, Home/End, paste with newlines). The agent starts beside the current pane, named
  "new skill", with i-mux's skill-writing guide as its first message and your description after
  it; left empty, the agent asks you first. A blank skill is named in the same popup (its path is
  shown, a taken name is refused) and opens in the editor there. Right-clicking the Skills view's
  empty space offers New skill… and Blank SKILL.md….
## [0.36.1] - 2026-11-09

### Fixed

- **Sidebars come back.** When imux was killed (on 2026-11-09 a build copied over the running
  `imux` made macOS kill every imux process: Code Signature Invalid), the sidebars' panes closed
  and stayed gone. The watcher now puts a sidebar back in any window that lost one, unless it was
  turned off (prefix b, which is now remembered per tab). `imux update` and the installer were
  never affected: they replace the file instead of writing into it.
- The project skills code from 0.36.0 passes the linter again (CI was red).

### Added

- `make install-local` installs a build into `~/.local/bin` the safe way; CONTRIBUTING and a new
  AGENTS.md say never to copy over the running `imux`.

## [0.36.0] - 2026-11-09

### Added

- **🧩 Skills view on the sidebar rail.** Lists the project's `<name>/SKILL.md` skills across every
  agent CLI's folder, shows which of the five agents see each one (`✻▲◉◎▣`) and any front-matter
  problem (`⚠`), and links `.agents/skills/` into `.claude/skills/` automatically so one skill works
  in Claude Code, Antigravity CLI, Copilot CLI, Codex CLI and OpenCode. **✨ Write with Agent…**
  asks what the skill should teach and sends a structured authoring prompt to the active agent;
  **➕** creates a starter `SKILL.md` and opens it in the editor; a skill's menu has **Open**,
  **View**, **Add to Agent**, **Improve with Agent**, **Share with all agents** and **Delete**.

## [0.35.0] - 2026-11-09

### Added

- **Named conversations in History.** An agent renamed in the sidebar names its conversation too
  (every conversation of that pane, also after it moved to another CLI), and History lists it by
  that name: `backend fix · fix the login redirect…`. A conversation's menu in History has
  **Rename…** (an empty name removes it); an open agent running that conversation takes the new
  name. Continuing a named conversation from History gives the agent its name back. Names are kept
  in `state.json` and survive closing the agent, the tab or the project.

## [0.34.0] - 2026-11-09

### Added

- **Settings → Versions and updates** (and `imux versions`): i-mux, tmux and every agent CLI with
  the version installed and the command that updates it (`claude update`, `copilot update`,
  `agy update`, `opencode upgrade`, npm or Homebrew for Codex, the package manager for tmux); a
  letter runs it right there and the list is read again. An older tmux is marked with what a newer
  one adds, and the Settings row says so (`⚠ … (tmux 3.2a is old)`). Where the package manager has
  no newer tmux (Ubuntu 22.04's apt: 3.2a) it says so and points to Homebrew. Nothing is asked of
  the network: each CLI's own updater knows whether a newer version is out.

## [0.33.0] - 2026-11-09

### Changed

- **Closing a project's last agent no longer closes the project.** Its pane stays and says
  "No agents in <project>", with the agents to start there (a key or a click each, a terminal
  too); the sidebar keeps the project and shows "No agents · + to start one", and the + of
  AGENTS starts an agent in that pane. The same happens when the last pane goes another way (a
  terminal's `exit`). Closing the last agent of one of several tabs still closes that tab; the
  × of the last tab and the project's Close still close the project.

### Fixed

- **The + and × of the tab bar work on tmux 3.2 and 3.3** (Ubuntu 22.04 has 3.2a): a click on
  them did nothing there, as they used click ranges only tmux 3.4 has.

## [0.32.0] - 2026-11-09

### Added

- **Drag the panel's edge to make it wider or narrower,** as in the editor. Up to now the panel
  snapped back to its width at once. The width (20-80 columns) is saved in the config
  (`tmux.sidebar_width`) when the drag ends; every tab's panel takes it, new tabs open at it and
  it stays after a restart. A bigger window or a closed agent next to the panel still leave its
  width as it is.

### Changed

- **Source Control shows each changed file's name first, then its folder in gray,** as in the
  editor (`config.py tests/_bdd`). Paths used to be cut at their start (`…_bdd/config.py`,
  `…omation-framework/`), which hid the names. A short row now cuts the folder first, then the
  middle of a long name, so its start and its extension stay (`blim_s…ps.py`).

## [0.31.0] - 2026-11-08

### Added

- **Go to File (Ctrl+P),** the editor's Quick Open: type a few letters of a file's name (the
  letters in order, not necessarily together: `sbrnd` finds `internal/sidebar/render.go`),
  Enter opens it in the editor; `name:42` opens it at line 42. With nothing typed it lists the
  files changed last. Ctrl+P works in every pane, and 📄 in the Files title opens it too;
  `[tmux] quick_open_key_root` changes the key (`""` turns it off). Ctrl+P is then i-mux's in
  the panes: a shell's or OpenCode's previous command is the up arrow.

### Fixed

- After a search result opened a file, the editor lost the first key typed into it: the search
  popup's reader was still waiting on the terminal.

## [0.30.2] - 2026-11-08

### Changed

- On an Intel Mac the installer says that Homebrew builds tmux from source there (10-30 minutes),
  so the wait does not look like a hang.
- Release checks: the Intel macOS job no longer waits 25-50 minutes for Homebrew to build tmux
  (it tests the Intel archive, the installer and PATH), and every check has a time limit.

## [0.30.1] - 2026-11-08

### Fixed

- The explorer's right-click menu opened slowly (about 0.4 s) and showed up behind the panel
  first. On tmux 3.5 and later it opens on the press (about 30 ms), and the sidebar no longer
  redraws an unchanged screen: a dozen redraws went over an open menu each time, and the whole
  sidebar redraws less often everywhere.

## [0.30.0] - 2026-11-08

### Added

- **The explorer's right-click menus are the editor's.** On a file: Open, Open to the Side, Open
  in VS Code, View, Add to Agent, Reveal in Finder / File Explorer (WSL) / Open Containing Folder
  (Linux), Open in Terminal, Select for Compare / Compare with Selected, Cut, Copy, Paste, Copy
  Path, Copy Relative Path, Rename…, Delete (to the Trash, where it can be restored). On a folder:
  New File…, New Folder…, Find in Folder… and the rest; on empty space: also Collapse Folders.
  New, renamed and pasted files are revealed and selected; a taken name gets " copy".
- **Hover:** whatever a click would act on, in every view, lights up under the pointer; the
  explorer's selected file keeps a blue background.
- **Menus take the mouse** on tmux 3.5 and later: an item lights up under the pointer, a click
  chooses it, a click outside closes the menu. On older tmux, a menu item's letter or ↑↓ and
  Enter.

### Fixed

- A project folder with "#" in its name opened popups (files, search, git) in the wrong folder:
  tmux reads a popup's folder as a format.

## [0.29.1] - 2026-11-08

### Fixed

- **OpenCode 1 is reported:** a new OpenCode tab with OpenCode 1 (what its install script and
  npm still give; Homebrew gives 2) dropped to a plain shell without a word. `imux doctor` now
  says so, with the fix: `opencode upgrade --major`. The install hint says so too.

## [0.29.0] - 2026-11-08

### Added

- **Pasting images into any agent.** Copy a screenshot (or image files) and press Ctrl+V in an
  agent's pane, as often as you like: each paste is saved as a new file under `~/.imux/images/`
  (kept 30 days) and the agent gets its path as a bracketed paste, the way a dropped file arrives
  (`@path` for Antigravity). Alt+V and the sidebar's 📎 do the same where the terminal keeps
  Ctrl+V (Windows Terminal). With no image, or outside an agent's pane, Ctrl+V is untouched.
  Reads the macOS clipboard, Wayland (`wl-paste`), X11 (`xclip`) and, from WSL, the Windows
  clipboard. `[paste] images` (a Settings row) turns it off; `[paste] command` brings your own
  reader; `imux doctor` shows the one in use.

## [0.28.0] - 2026-11-08

### Added

- **Notifications** when an agent finishes (after working 10 s or more) or asks you something,
  unless you are looking at it: macOS's notification center, the Linux desktop's (`notify-send`,
  else D-Bus), or, with no desktop (SSH, a server), the terminal's own (OSC 9 for iTerm2, OSC 777
  for Ghostty, WezTerm, kitty, foot and GNOME Terminal). They come with the terminal closed too.
  Settings turns them and their sound on or off; `[notify] command` sends them anywhere.

## [0.27.3] - 2026-11-08

### Fixed

- A wide character (an emoji like ✨, or Chinese / Japanese / Korean text) in a commit subject, a
  file name or an agent name made its row wider than the sidebar: the row wrapped and the rail
  broke. Widths follow the terminal's wide characters now, text is cut by columns, and every row
  is cut to the sidebar's width before it is drawn.

## [0.27.2] - 2026-11-08

0.27.0 and 0.27.1 were tagged, but their builds could not build the release tool (it needs a
newer Go than the runner had); they were never released. This release holds the same changes.

### Added

- **Update from Settings:** "Update i-mux" installs the newest release (or `imux update`). i-mux
  looks once a day whether a newer one is out; then Settings on the rail turns yellow and its
  first row says which version.

### Changed

- **Icons draw everywhere:** the editor's icons are used only when your terminal's font has
  them (Terminal.app, iTerm2 and VS Code with a Nerd Font; Ghostty, WezTerm, kitty); otherwise
  emoji. Before, an installed Nerd Font was enough, and a terminal using another font showed "?".
- The rail says **Settings** (it is 2 columns wider; the default sidebar width is 43).
- The top right of the tab bar is empty: the agents' states and the clock were already in the
  sidebar.
- Releases no longer fail when GitHub's download of the release tool times out (0.24.0, 0.25.0
  and 0.25.1 were never published; their changes are in this release).

## [0.26.1] - 2026-11-08

### Fixed

- Renaming some agents said "no such pane": tmux's prompt turned `%1` in the command into the
  typed name, so panes numbered %1, %10-%19, %100 and up could not be renamed (often a Claude
  pane opened later). Any pane can be renamed now.

## [0.26.0] - 2026-11-08

### Changed

- **Rename from the agent's row only.** The sidebar's Rename button named the window's current
  pane, not the agent you meant (naming Claude could name OpenCode); it is gone. A click on the
  selected agent (or a right-click on any agent) opens its menu with Rename….
- **Pins are in the History list:** "📌 Pin or unpin…" (key `p`) shows the conversations for
  pinning. The agent column lines up (Antigravity no longer shifts the rest), and previews drop
  `<pasted_content>` tags.

## [0.25.1] - 2026-11-08

### Fixed

- A conversation picked in History and continued in another agent went back to the wrong chat:
  choosing its agent again (for example Continue this chat in → Claude) resumed an older chat
  of that agent in the pane. It now resumes the picked conversation, with what happened in the
  other agent since.

## [0.25.0] - 2026-11-08

### Added

- **Settings** (🔧 Config on the rail, `imux settings`, `,` in the sidebar): AI completion in the
  editor, icons, who writes commit messages, what a new agent does after a handoff, what happens at
  a limit, and the config file. A row changes only its line in `~/.imux/config.toml`; sidebars and
  the watcher pick the change up at once.
- **AI completion in the editor**, like Copilot in VS Code: when it is on, files you edit open in
  Neovim with GitHub's copilot.vim, in a profile i-mux keeps (`imux-nvim`; your own Neovim setup is
  not touched). Tab accepts a suggestion, Ctrl+S saves, Ctrl+Q quits. Turning it on installs Neovim
  (Homebrew) and the plugin when missing and offers the one-time GitHub sign-in.

## [0.24.0] - 2026-11-08

### Added

- **Pin conversations in History.** A conversation's menu has Pin to the top (key `t`); pinned
  ones come first, marked 📌, even when they are older than the 50 newest; Unpin puts them back.

## [0.23.1] - 2026-11-08

0.23.0 was tagged, but its build failed on a network error at GitHub; it was never released.
This release holds the same changes.

### Added

- **✨ Commit message suggestions** in Source Control, like the editor's: the agent of the pane
  you work in (or `git.commit_agent`, with `git.commit_model`) gets the staged changes, or with
  nothing staged all of them, in its print mode and writes a one-line message in the style of
  your recent commits. A menu offers Commit, Edit before committing… (your editor), Write another
  and Cancel; nothing is committed without you. It asks once before the changes go to an agent.
  Claude, Codex, Copilot, Antigravity and OpenCode (for example with a free NVIDIA or Groq key).

## [0.22.0] - 2026-11-08

### Changed

- **Buttons you can see and hit.** The sidebar's buttons use the rail's icons: with a Nerd Font the
  editor's own (add, discard, refresh, pull, push, sync, check, branch, chevrons, edit, history,
  search),
  otherwise emoji that every font draws at full size. Each takes at least 3 columns; before, they
  were 1-column signs (⟳ ↶ ⇅ ⌕ + −) in 2-column cells. The tab bar's × takes 3 columns too.

## [0.21.0] - 2026-11-08

### Added

- **Close the side panel from the rail**, as in the editor: a click on the open view's icon
  narrows the sidebar to the rail and gives the agent the room; a click on any view's icon opens it
  again with that view. Each tab keeps its own state.

## [0.20.0] - 2026-11-08

### Added

- **OpenCode is back** as the fifth agent (key `e`), as in 0.18: chats move to and from it with
  their context, it resumes its own sessions, and its sessions are in History. Add an NVIDIA or
  Groq key inside OpenCode with `/connect` (no account needed) and pick the model with `/models`.

### Fixed

- After a round trip (for example Claude → OpenCode → Claude → Codex) the next handoff carried the
  whole "Welcome back" document, instructions included, as the user's last request. It now shows
  what happened meanwhile in the other agent as a short block, and the last request is a real one.
- Reading the handoff file no longer shows up as work in the next handoff ("Files touched", a
  conversation line).

## [0.19.0] - 2026-11-08

### Removed

- OpenCode support (added in 0.18.0). A pane an earlier version started with it shows under its
  id in the sidebar and can be switched to another agent; it is not restored.

## [0.18.1] - 2026-11-08

### Fixed

- With tmux 3.4 (Ubuntu 24.04) a click on the × of the current tab selected the tab instead of
  asking to close it.
- Under a locale that is not UTF-8, the handoff preview and other pages showed `·` and `→` as
  `<C2><B7>` and `<E2><86><92>`.
- A quick click in the sidebar right after another one on a different row (a file just after the
  Source Control icon) was dropped as a double click.

## [0.18.0] - 2026-11-08

### Added

- **OpenCode** as the fifth agent (key `e`): it works with any model endpoint configured in
  OpenCode (OpenAI-compatible APIs, Anthropic, Ollama ...). Chats move to and from it with their
  context, it resumes its own sessions, its sessions are in History, and the limits panel shows its
  recent tokens. `imux doctor` warns when OpenCode cannot run (a state folder owned by root) and
  when `sqlite3`, used to list its sessions, is missing.

## [0.17.0] - 2026-11-08

### Added

- **The editor's icons on the rail** when the terminal has a Nerd Font (`[tmux] icons = "nerd"`,
  or `"auto"`, the default, which picks them when a Nerd Font is installed or the terminal brings
  its symbols: Ghostty, WezTerm, kitty); emoji otherwise.

### Changed

- A line separates the rail from the view. The default sidebar width is 41.

## [0.16.0] - 2026-11-08

### Added

- **Git like an editor's source control:** staged and unstaged changes, + stage, − unstage and
  ↶ discard (asks first) per file and for all, **Commit…** (with nothing staged it commits every
  change), pull / push / sync with ahead and behind counts, a branch menu (switch, new branch) and
  the recent history (click a commit to see it).
- **Editing:** a file opens in your editor (`editor` in the config, else micro, `$VISUAL`,
  `$EDITOR`, nano, vim with the mouse on); right-click a file to view it or open it in VS Code;
  search results open in the editor at the line.
- Search results fold per file (click the file, ← →) and all at once (Tab).

### Changed

- **Bigger rail:** colored icons with their names under them (🤖 Agents, 📁 Files, 🔍 Search,
  🔀 Git, 📊 Limits, ❓ Help); the default sidebar width is 40.
- The mouse wheel scrolls in the file, diff and limits viewers and in the popups.

### Fixed

- Right-click menus in the sidebar no longer close as soon as they open.

## [0.15.0] - 2026-11-08

### Added

- **Activity bar:** a column of icons on the sidebar's left switches its view, as in an editor:
  ✻ Agents, ▤ Explorer, ⌕ Search, ⑂ Source control, and ◔ Limits and ? Help at the bottom. Keys
  `1`–`4` (and `/` for search) when the sidebar has the focus.
- **Explorer:** the project's files as a tree; folders open on click, files open with line numbers
  (`bat` with colors when installed); files changed in git are colored.
- **Search:** the whole project, results grouped by file as you type, the match highlighted;
  Enter opens the file at the line.
- **Diffs like an editor's:** a changed file opens whole, with line numbers, removed lines on red
  and added ones on green, side by side when the window is wide, starting at the first change.

### Changed

- The default sidebar width is 38 columns (the rail takes 3).
- Source control moved from the Agents view to its own view.

## [0.14.0] - 2026-11-07

### Added

- **Copilot's monthly quota in Limits:** a bar with the share of premium requests used, "N of M
  premium requests used" and the reset date, read from Copilot's own session files (the numbers
  of its last reply).

### Changed

- Clearer limit texts: until an agent has answered once, the panel says to send it a message;
  Antigravity, which keeps its quota to itself, shows only limits it hits.

## [0.13.0] - 2026-11-07

### Added

- **Folding sidebar:** click PROJECTS, AGENTS or SOURCE CONTROL to fold or unfold the section; the
  choice is shared by every sidebar.
- **SOURCE CONTROL** in the sidebar: the branch and the changed files with their added and removed
  lines; click a file for its diff. It replaces the "Changes N" button.
- **Plan percentages in Limits:** a bar per window, e.g. `5-hour ███████░░░ 72% · resets 15:00`
  and `weekly`. Claude's come from its own status line: imux starts Claude with a status line for
  that session only (your settings files are not changed; your own status line still shows).
  Codex's come from its session files. Copilot and Antigravity do not report percentages; the
  panel says so. `[limits] claude_statusline = false` turns Claude's off.

### Changed

- Brighter sidebar text and bold section titles.

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
