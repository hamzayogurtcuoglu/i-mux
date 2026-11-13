# Changelog

All notable changes to i-mux are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.55.0] - 2026-11-13

### Added

- **Source Control has VS Code's commit message box.** Above the Commit button a box says
  "Message (Enter to commit)": click it and write the message there, as in VS Code, instead of
  in the prompt at the bottom of the screen. It wraps and grows up to six rows; **Enter**
  commits, **Ctrl+J** (or Alt+Enter, where the terminal lets it through) starts a new line, and
  **Esc** leaves the box with the message kept. Commit with nothing typed puts the cursor in the
  box. The commit items of ▾ and ⋯ › Commit (Commit & Push, Commit & Sync, Commit Staged, Commit
  All, the signed-off ones) commit its message too; with none, the box asks for it and its hint
  names the command ("Message (Commit & Push)"). Amend with an empty box keeps the last commit's
  message. While it is written, the message is kept per project in `~/.imux/commit/`: every
  sidebar of the project shows it, a restart keeps it, and a commit made with it clears it.

### Removed

- **✨ AI commit messages.** The ✨ button, the Settings row "Commit messages by" and the
  `[git] commit_agent` / `commit_model` keys are gone; a config that still has them loads
  without a warning.

## [0.54.0] - 2026-11-13

### Added

- **The diff shows where its changes are, beside the scroll bar, as VS Code does.** Two thin
  lanes left of the diff viewer's scroll bar map the whole file: red where lines were removed,
  green where lines were added (a replaced line has both). Click a mark to go straight to that
  change instead of scrolling through the file; **]** and **[** go to the next and previous
  change, and the bottom row says which ("change 2 of 5"). The whole file still shows.

## [0.53.1] - 2026-11-13

### Changed

- **The explorer's title has no Search button any more**, and its ⋯ no Search…: the rail's 🔍
  opens search, as it always did. Go to File stays in ⋯, and its button comes back to the title
  wherever there is room (from 43 columns with the editor's icons, the default width, and from
  47 with emoji).

## [0.53.0] - 2026-11-13

### Added

- **The explorer's title has VS Code's buttons.** New File…, New Folder…, Refresh Explorer and
  Collapse Folders sit in the Files view's title as they do in VS Code (the editor's icons with
  a Nerd Font, 📝 📁 🔄 ⊟ otherwise), with **⋯** (More Actions) after them. A new file or folder
  goes into the selected folder (or the selected file's folder), else into the project's, and
  opens or shows selected as from the right-click menu. Refresh reads the files and their git
  colors at once; Collapse Folders closes every open folder. While the pointer is on a button,
  the title says what it does.
- **Show Hidden Files.** ⋯ → **Show Hidden Files** shows the files and folders whose names start
  with a dot (`.github`, `.gitlab-ci.yml`, `.vscode`, `.env`); ✓ marks it while on, and choosing
  it again hides them. It is saved in the config (`[explorer] show_hidden`), so it stays after a
  restart, and every sidebar follows at once. `.git`, `.svn`, `.hg`, `CVS`, `.DS_Store` and
  `Thumbs.db` stay hidden, and `node_modules` and the other folders the explorer skips stay out
  of the tree, as in VS Code.

### Changed

- **Search and Go to File look into hidden folders** while Show Hidden Files is on, outside git
  repositories too (in one, they already found files such as `.github/…`, and still do). They
  never list `.git`, `.DS_Store` or `Thumbs.db`.
- **Go to File and Search moved into ⋯** in the explorer's title at the default width (Ctrl+P
  and 🔍 on the rail still open them); a sidebar wide enough (46 columns with the editor's icons,
  51 with emoji) shows their buttons again.

## [0.52.0] - 2026-11-12

### Changed

- **The ⏰ Schedule tab on the sidebar rail is now named Jobs (`İşler`), and the separate 🧩 Skills rail tab is removed.** Jobs already lists and runs the project's skills alongside its agents, both on demand (🚀) and on a schedule.

## [0.51.0] - 2026-11-12

### Added

- **Scheduler: your agents and skills, now or on a schedule.** A new ⏰ **Schedule** view on
  the sidebar's rail lists the project's own agents (Claude Code's `.claude/agents/`, Copilot's
  `.github/agents/`, OpenCode's, `.agents/agents/` for any CLI, and your personal ones) and its
  skills. There is no form and no prompt to write: the agent or skill already says what to do.
  🚀 opens one Run panel where everything is chosen already: the installed CLI (Claude,
  Antigravity, Copilot, Codex or OpenCode; the agent's own first), the model (the CLI's list,
  the ones used before, or a name you type), when (now, every 5 or 15 minutes, every hour, every
  day at 09:00, or one such as `10m`, `2h`, `09:00 18:30`; `5 dk` and `2 saat` work too) and the
  access. Enter runs it, and its output opens and follows the run as it goes: each tool the
  agent calls with its result, then its answer; `s` stops it, Esc leaves it running. i-mux
  runs it headless in the project while it is open, gives each run the last report so it can
  tell what is new, and shows each one's time, CLI, model, last report and next run, with a
  spinner and a badge on ⏰ while one runs; a click shows its output. A report notifies you;
  "nothing new" stays quiet. Its menu shows the output and the history, continues a run's
  conversation in a pane, changes the schedule, pauses, opens or removes it.

## [0.50.0] - 2026-11-12

### Removed

- **Native Windows.** The port on psmux that came in 0.49.0 is gone, with its `install.ps1` and the
  Windows release archives. On Windows, run i-mux in WSL 2, as before 0.49: open a WSL terminal
  and run the one-line `install.sh`. Nothing changes on macOS, Linux or WSL. A native install from
  0.49 cannot update itself: delete `%LOCALAPPDATA%\Programs\imux`, remove that folder from your
  user PATH, and `winget uninstall marlocarlo.psmux` if you installed psmux for i-mux.

### Fixed

- **Git actions no longer fail with "index.lock: File exists".** The sidebar's own refresh ran
  `git diff`, which rewrites git's index under `.git/index.lock`, so a stage, commit or checkout
  clicked at that moment could fail. i-mux's background reads take no lock now, and an action
  waits up to two seconds for another program's git (an agent's, an editor's), as VS Code does.

## [0.49.1] - 2026-11-12

### Fixed

- **Add to .gitignore in a project opened through a symlink.** It said the file was outside the
  repository when git spelled the project's folder differently: a folder reached through a
  symlink (on macOS, anything under `/tmp` or `/var`) or, on Windows, by its short (8.3) name.
- **Windows: saving the state and settings waits for a file in use.** Windows refuses for a moment
  to replace a file another process has open, often the virus scanner checking a file just
  written; i-mux's state, settings and usage writes now try again for up to two seconds instead of
  failing.

## [0.49.0] - 2026-11-12

### Added

- **Source Control like VS Code.** **⋯** in the Git view's title opens VS Code's menu: Pull, Push,
  Clone…, Checkout to…, Fetch, Show Git Output, and the Commit (Staged, All, Amend, Signed Off,
  Undo Last Commit, Abort), Changes, Pull/Push (Pull (Rebase), Pull from…, Push to…, Push Tags,
  Fetch (Prune), Fetch From All Remotes), Branch (Merge, Rebase, Create, Create From, Rename,
  Delete, Delete Remote, Publish), Remote, Stash (Include Untracked, Staged; Apply, Pop, Drop,
  View), Tags and Worktrees submenus, with View & Sort (a list or a tree; by name, path or status).
  Branches, tags, stashes, remotes and worktrees are chosen in a Quick Pick that filters as you
  type; in Checkout to…, what you typed names a new branch. The Commit button gets a ▾ (Commit &
  Push, Commit & Sync, Amend) and turns into Continue / Commit Merge during a rebase or merge;
  conflicts get a MERGE CHANGES section; a branch without an upstream shows Publish; a click on the
  branch opens Checkout to…; a right-click on a file or a commit opens its menu; HISTORY is a graph
  with branch and tag labels and ↑ ↓ for outgoing and incoming commits; outside a repository,
  Initialize Repository and Clone Repository…. Every git command run from the view, with what git
  said, goes to `~/.imux/git.log` (Show Git Output).
- **Native Windows (experimental)**, by @yusatll (#2): i-mux runs without WSL on
  [psmux](https://github.com/marlocarlo/psmux), with `install.ps1`, PowerShell in the panes, the Nerd
  Font set up for Windows Terminal and VS Code, and Windows in CI. psmux 3.4.0, its current release,
  stops running i-mux's keys and clicks: build psmux from its main branch
  (`scripts\build-psmux.ps1`) until a newer release. WSL 2 stays the recommended way on Windows.

### Fixed

- **Discarding a file named like a command is safe.** The discard confirmation put the file's path
  unquoted into a shell command line, and the commit prompt and the branch menu put the folder and
  branch names into tmux commands; a name like `$(…)` could run. Paths, names and messages now reach
  imux through tmux options or as arguments, never in a command line.

## [0.48.0] - 2026-11-11

### Removed

- **The Image button** next to History in the sidebar. Ctrl+V in an agent's pane already pastes a
  copied image (Alt+V where the terminal keeps Ctrl+V for itself, as Windows Terminal does).

## [0.47.0] - 2026-11-11

### Fixed

- **Menus take clicks on every tmux.** Before tmux 3.5 (Ubuntu 22.04 has 3.2a, Ubuntu 24.04 3.4) a
  click on a menu closed it without choosing anything, so menus needed the keyboard. There i-mux now
  draws its menus itself, in a small popup where the menu was: the item under the pointer lights
  up; a click, Enter or the item's key chooses; Esc, `q` or ✕ closes it. tmux 3.5+ keeps its own
  menus.
- **The spinner turns only while an agent works.** Claude Code, Codex, Copilot and Antigravity count
  as working only while they show their own busy line (`esc to interrupt`, `ctrl+c interrupt`), so
  moving through a list in the Copilot CLI no longer turns it.

### Changed

- **A short HTML menu.** A click on an HTML file offers **Open in browser**, **Open in a new tab**,
  **Edit** and **View**, without the 🌐 and the "live" on each line (pages still reload by
  themselves).
- **No dots in the sidebar.** The yellow dot (uncommitted changes) and the green dot (an agent
  finished while you were elsewhere) are gone; a finished agent shows as idle, and its notification
  still says it finished. The Git icon on the rail counts the changed files.
- **tmux 3.4 is no longer "old"** in Versions and updates: with menus clickable everywhere, it has
  every feature.

## [0.46.1] - 2026-11-11

### Fixed

- **A double click on another project or tab stays there** with tmux 3.7 and later (Homebrew's
  tmux on macOS is 3.8): its second press reached the new project's sidebar and switched straight
  back.

## [0.46.0] - 2026-11-11

### Added

- **An Exit button.** **⏻ Exit** at the right of the tab bar opens a small menu: **Back to the
  terminal** takes you back to your shell while the agents keep working (`imux` brings you back to
  them), and **Quit i-mux and stop every agent…** asks once, saves the session, stops the live
  preview servers and closes i-mux (`imux` → **Restore last session** reopens it all). `imux quit`
  does the same from a shell.
- **Pick your editor.** Settings → **Editor**: Auto, micro (got for you when missing), nano, Neovim
  (when it is missing, it says how to install it and offers to) or Neovim + Copilot (AI
  completion). `imux editor use auto|micro|nano|nvim` from a shell.

### Changed

- **Settings rows open a menu.** Enter or a click on a row lists its values with the current one
  marked ●; pick one, or press its number. Rows no longer flip through their values. AI completion
  and getting micro are now in the Editor menu.
- **`imux` asks which project again.** Outside an open project's folder, while projects are open or
  a last session can be restored, `imux` shows the picker: **Restore last session** first, then
  **this folder**, then your recent projects. In an open project's folder it goes straight there.
- **A click on an HTML file asks how to open it:** in a window of its own, live; in a browser tab,
  live; to edit; or to view. The Settings row for it is gone.
- **The tabs start over the terminal**, past the sidebar, and follow it when the panel opens,
  closes or is resized (at the left edge with the pane zoomed or no sidebar).

## [0.45.0] - 2026-11-11

### Added

- **micro comes with i-mux.** Without an editor of your own, the first file you open asks once
  whether to get micro (5 MB): an editor with the mouse, Ctrl+S / Ctrl+Q, Ctrl+C / Ctrl+V and
  colors for code. Enter gets it and opens the file in it; **Use nano** keeps nano. Settings →
  **Get the micro editor…** and `imux editor install` get it later.
- **Scroll bars in the editor.** micro shows its scroll bar, and nano (5.0 and later) its
  position indicator at the right edge; nano also takes the mouse.
- **A badge on the Agents icon.** While agents work, the rail's 🤖 icon has their number on a blue
  badge, as an editor's activity bar does, so you see it with the panel on another view or closed.

### Changed

- **Web pages open in your browser, live.** A click on an HTML file serves its project on
  `localhost` and opens the page in a window of its own (Chrome's or Edge's app window: no tabs, no
  address bar; under WSL, Windows' browser), or a tab (Settings). The page reloads by itself when
  it or a file it uses changes, and a click on another page shows it in that window. Pages load
  what `file://` blocks (a report's data next to it). From VS Code's terminal, also over a tunnel,
  they open where VS Code runs. Nothing to download. `imux web` lists the served folders, `imux
  web stop` stops them.

### Removed

- The in-terminal browser of 0.44.0 (Carbonyl), which drew pages too coarsely; its 170 MB folder in
  `~/.imux/tools` is deleted.

## [0.44.0] - 2026-11-10

### Added

- **Web pages in a pane.** A click on an HTML file in the explorer opens it in a pane beside your
  agent, in Carbonyl (a Chromium that draws in the terminal, so it works over SSH and a VS Code
  tunnel too); another page loads in the same pane, a second click reloads it. **+** next to
  Agents → **🌐 Web page…** opens any address (`localhost:3000`, or just `3000`), and HTML and SVG
  files have **Open Preview** and **Open in Browser** in their menu. The preview is on by default,
  but its browser (a 74 MB download) comes only when you first want it: the click offers to
  install it, to open the page in your browser instead, or to edit the file. Settings →
  **🌐 Web preview** installs, removes, turns it off or on, and opens a page; `imux web` does the
  same from a shell, and `[web] browser` runs another browser instead.

## [0.43.0] - 2026-11-10

### Added

- **Search in History.** History's **🔍 Search their text…** (`/`) opens a window with every
  conversation of the folder; type, and it finds the ones in which anything you or the agent said
  has the text, with the messages that do (the match marked). Enter opens the conversation, as
  in History.

## [0.42.0] - 2026-11-10

### Added

- **Scroll bars.** Everything that scrolls has a thin bar on its right edge that shows where you
  are: the left panel's file tree, Source Control and Skills, the search, Go to File, the folder
  picker, and the diff, file and commit viewers. Click or drag it to jump.
- **A clearer search.** Each file has its icon, its folder and a count of its matches; the
  matches are colored; **⊟ Collapse all** / **⊞ Expand all** in the title folds or opens every
  file, and a search that finds more than 5 files starts folded.

### Changed

- The diff, file, commit and limits viewers are i-mux's own pager instead of `less`, with the same
  keys (↑↓, PgUp/PgDn, Space, g/G, / to search, n/N, q) and the scroll bar.

## [0.41.0] - 2026-11-10

### Added

- **Install a missing agent from its button.** A click on an agent that is not installed under
  **Continue this chat in** (or on the agent bar) opens a small window: what to run, **Install
  now** (it runs the vendor's command and shows its output) and **Close**; once installed, Enter
  continues the chat in it. Without npm (Node.js) or curl it says how to get them first.
  `[agents.<name>] install` replaces the command.
- **Choose where a new agent opens.** The **+** next to **Agents** asks for the agent (or a plain
  Terminal), then where: right, left, below or above the current pane, or in a new tab.

### Fixed

- **The time next to an agent no longer restarts after every update:** a restarted watcher keeps
  the states it finds and writes only changes.

## [0.40.0] - 2026-11-10

### Added

- **The AGENTS title counts the agents at work:** a spinner and "2 working" (in every project;
  folded, also the number of agents).

### Fixed

- **Switching on the left is instant.** A click on a project, on an agent in another tab or
  project, on "Continue this chat in", on a file or on anything that opens a popup acts on the
  press (a project took 350 ms, now a few ms). A double click still acts once: its second press
  reaches the left panel only where the first one went, and i-mux's popups ignore it. Only menus
  (on tmux older than 3.5, as on Ubuntu 22.04 and 24.04) and questions in the status line still
  wait a moment.
- **The left panel stays up to date:** while you look at it, it checks the project every 3 s, so
  files an agent or editor wrote show within a few seconds.
- **The spinner turns only while an agent works.** Typing in an agent's input, the panel opening
  or closing (which resizes the agents' panes) and a restart of i-mux's watcher (after every
  update or settings change) all counted as work: a project's spinner turned although its agent
  did nothing, and after an update every agent showed "Done". Now only the agent's own output
  (its progress line, its answer) counts.

## [0.39.1] - 2026-11-10

### Fixed

- **Clicks in the sidebar act at once.** A view on the rail, a folder, a section and an agent of
  the tab now respond in about 30 ms instead of 350 ms. Every click waited for tmux's double-click
  time, because tmux's own double click selected the sidebar again and took the focus back from
  the agent; i-mux now keeps it from selecting a sidebar (other panes keep tmux's). Clicks that
  open a menu, a popup or a prompt, or go to another tab or project, still wait a moment, so a
  double click cannot close what the first click opened.
- **The text cursor no longer flashes in the sidebar.** While agents work the sidebar redraws in
  the background, and the terminal sometimes showed the cursor there for a moment. tmux now sends
  such updates as one synchronized update (DECSET 2026: Windows Terminal, iTerm2, kitty, WezTerm
  and others support it; other terminals ignore it). It takes effect when the terminal attaches
  again: detach (prefix d) and run `imux attach`.

## [0.39.0] - 2026-11-10

### Added

- **File icons in the explorer.** Every file has the icon of its type, as in the editor: with the
  editor's icons the Nerd Font glyphs of its default file icon theme (Seti) in their colors (Go,
  Markdown, JSON, YAML, TOML, shell, JavaScript, TypeScript, Python, Rust and about 50 more),
  with emoji an emoji per kind. Folders show open or closed, test files a beaker, the agents'
  instructions (`AGENTS.md`, `CLAUDE.md`) the rail's agent icon and `SKILL.md` the skills one;
  README, LICENSE, CHANGELOG, Makefile, Dockerfile and lock files have their own. Faint indent
  guides show the levels of open folders.

## [0.38.0] - 2026-11-10

### Added

- **The Nerd Font comes with i-mux.** `imux font install` downloads JetBrainsMono Nerd Font
  (Nerd Fonts v3.5.1, checked against its checksum), installs it for you alone without
  administrator rights, sets Windows Terminal's and VS Code's terminal font to it (their old
  settings stay in `settings.json.imux-backup`, comments and all kept) and switches the sidebar
  to the editor's icons. On WSL it does this on the Windows side, where the terminal draws the
  text. It asks first; `imux font` shows what is in place. The installer offers it (`--yes`
  installs it, `--no-font` skips it), Settings has **Install the Nerd Font for the icons…**, and
  `imux doctor` says whether it is set up.

### Fixed

- Icons `auto` under WSL: the terminal runs on Windows, so i-mux now reads Windows Terminal's
  font and the Windows user's VS Code settings (also when WSL was started from the Start menu,
  which tells it nothing about the terminal).

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
