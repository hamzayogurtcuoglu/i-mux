# i-mux on Windows

i-mux runs natively on Windows (no WSL) on [psmux](https://github.com/marlocarlo/psmux), a
Windows multiplexer written for ConPTY that reads tmux's commands, formats and configuration
(`winget install --id marlocarlo.psmux`). The rest of i-mux was written for tmux; this page
describes the layers that let the same code run on psmux and Windows, and why they are shaped
the way they are. Installing and using it: [README](../README.md#windows).

## The layers

| Layer | Where | What it does on Windows |
|---|---|---|
| Multiplexer binary | `internal/tmux/bin_windows.go` | `tmux.DefaultBin()` finds `psmux.exe` (PATH, then imux's folder, winget's package and link folders, scoop, cargo); `tmux.Name` is `"psmux"`. `IMUX_TMUX_BIN` still overrides it. |
| psmux adapter | `internal/tmux/psmux_windows.go` | Keeps tmux's one-server model on psmux's one-server-per-session model (below). |
| Shell | `internal/tmux/shell_windows.go`, `internal/pwshshim` | Every command line i-mux writes for `run-shell`, `if-shell`, `#()` and panes is PowerShell. |
| Platform calls | `internal/sysx` | Signals, file locks, exec, detached children and the TUI's terminal, built from Windows primitives. |
| Around the app | `internal/doctor`, `internal/versions`, `internal/update`, `internal/nerdfont`, `internal/termfont`, `internal/agent/hints.go`, `install.ps1` | psmux checks and updates, Windows install hints, self-update, the Nerd Font (below). |

macOS and Linux build none of the `_windows.go` files; where a package needs one code path for
both (to be testable everywhere), it switches on `runtime.GOOS` through a variable the tests
replace (`doctor.goos`, `versions.Env.OS`, `nerdfont.GOOS`, `termfont.Native`, `i18n.OS`).

## The psmux adapter

**Why.** tmux has one server per socket: pane ids (`%3`) and window ids (`@2`) are unique on it,
global options and key bindings are shared by every session, and one batch of commands
(`a ; b ; c`) may touch several sessions. psmux starts one server process per session, all in
the `-L` namespace. Pane `%3` exists in every session's server, `set -g` changes one server, and
a command goes to the server named by the control port in `$TMUX`. i-mux stores pane ids in its
state, passes them between processes and runs batches that span projects, so instead of teaching
every caller about psmux, the adapter makes psmux look like tmux at the `tmux.Client` boundary.

**Ids name their session.** Every `#{pane_id}`, `#{window_id}` and `#D` that i-mux asks psmux to
expand is rewritten to `#{pane_id}~#{session_name}`, and `#{client_name}` to
`#{client_name}~#{client_session}` (`GlobalizeFormat`, used for formats and for the generated
config). So i-mux sees `%3~proj-1a2b` where tmux would give `%3`. These ids are opaque to the
rest of i-mux: they are stored in `state.json`, passed in `--pane` and compared as strings, and
nothing outside the adapter looks inside them. `SplitID` takes one apart; `EnvPane` builds one
from `$TMUX_PANE` and psmux's `PSMUX_SESSION`.

**Routing.** A command whose target (`-t`, `-s`, or `-c` where it names a client) carries such
an id is sent to that session's server with the bare id: the child gets a `$TMUX` that names the
session's control port, read from `%USERPROFILE%\.psmux\<namespace>__<session>.port`.

**Globals are broadcast.** `set -g` / `set -s`, `set-hook -g`, `bind`, `unbind`, `source-file`
and `setenv -g` go to every live session's server. The `set -g` / `-s` values are also kept in a
file and replayed into each session created later, which on tmux would simply inherit them.

**Lists are merged.** `list-clients` asks every server; psmux itself merges `list-sessions` and
the `-a` lists.

**Batches.** A command after `split-window`, `new-window` or `new-session` in the same batch,
without a target of its own, acts on the new pane, as tmux does.

**Forms psmux reads.** `display-menu` and `display-popup` are rewritten: psmux's menu knows only
`-T`, `-x` and `-y`, takes every word not starting with `-` as an item and has no disabled items;
its popup takes `-c` as the start directory, not the client. Splits and resizes get the same
treatment where psmux's flags differ. psmux children start without a console window of their
own.

On macOS and Linux `psmux_unix.go` makes all of this the identity: ids stay `%3`, nothing is
routed or broadcast.

## The shell

psmux runs `run-shell`, `if-shell` and `#()` with PowerShell (pwsh, else Windows PowerShell),
and i-mux sets `default-shell` to the same PowerShell, so the panes' shell is PowerShell too.
Every command line i-mux writes is therefore PowerShell: programs are called with the call
operator (`& 'C:\path\imux.exe' args`), words are single-quoted (`''` is a quote inside them),
and output is discarded with `*>&1 | Out-Null; exit 0` (tmux would show any output of
`run-shell` in the pane's view mode). psmux starts popups with `pwsh` by name; on a machine with
only Windows PowerShell, i-mux puts a copy of itself named `pwsh.exe` in a folder of its own and
puts that folder first on psmux's PATH; run under that name, imux runs `powershell.exe` with the
same arguments (`internal/pwshshim`).

## Platform calls

Windows has no `SIGUSR1`, `SIGWINCH`, `flock` or `exec`. `internal/sysx` builds the same behavior
from named events (waking the watcher and the sidebars, asking a process to stop),
`LockFileEx` (state locks), a waited-for child in the same console (where Unix replaces the
process), process trees killed as one, children started without a window and outside the
console, and a polled console size for the TUIs (no `SIGWINCH`). On Unix every function is the
plain syscall.

## Around the app

**Doctor.** `imux doctor` checks psmux (found, its own version, at least the tmux level i-mux
needs) instead of tmux and terminfo; psmux prints both (`tmux 3.4.0` then `psmux 3.4.0 (...)`):
the tmux line gates features and psmux's own number is what is shown (`tmux.ParseVersion`). Unix
permission bits mean nothing on Windows (Go reports made-up modes), so the `chmod` checks are
replaced by one that `~/.imux` is inside the user's own folder, whose ACL keeps other accounts
out. It recommends Windows Terminal when neither `WT_SESSION` nor `TERM_PROGRAM` is set.

**Messages.** A catalog key with a platform variant (`doctor.tmux_missing@windows`) gives that
variant on that platform (`i18n.T`): the same message with Windows' commands (`winget` instead of
`brew` / `apt`) and the same format verbs, which a test checks.

**Versions and updates.** The multiplexer row is psmux, updated with
`winget upgrade --id marlocarlo.psmux -e` (or `scoop update psmux`, `cargo install psmux` by where
it is installed). Windows cannot replace a running `psmux.exe`, and the popup itself runs inside
psmux, so the row shows the command (`psmux -L imux kill-server; winget upgrade ...`) to run in
PowerShell with i-mux closed instead of running it. psmux draws its own menus, so it is flagged as
old only below the tmux level i-mux needs. The agents update with their own updaters
(`claude update`, `copilot update`, `agy update`, `opencode upgrade`) or npm for Codex.

**Install hints.** `internal/agent/hints.go` swaps in the Windows install commands at start-up
(PowerShell installers for Claude Code and Antigravity CLI, npm for the others); every place that
prints a hint reads `agent.Info.InstallHint`.

**Self-update.** `imux update` downloads the release's `install.ps1` and runs it with
`powershell -File` (`update.Installer`). Windows cannot overwrite a running `.exe` but can rename
it, so the installer renames `imux.exe` to `imux.exe.old` (or `imux.exe.old.<n>` when an older
one is still running) before copying the new one, and puts it back if the copy fails;
`update.RemoveOld` deletes those files at the next start of imux once nothing runs them. The
installer is run as a file, not as a downloaded one-liner, and contains no backticks: Windows'
malware scan (AMSI) blocks both when a program other than PowerShell starts the script. A test in
`internal/update` keeps `install.ps1` ASCII and backtick-free.

**Fonts.** `imux font install` puts JetBrainsMono Nerd Font in
`%LOCALAPPDATA%\Microsoft\Windows\Fonts`, lists each style under
`HKCU\Software\Microsoft\Windows NT\CurrentVersion\Fonts` as Windows' own per-user install does
(no administrator rights), loads it into the session and broadcasts `WM_FONTCHANGE`, then sets
Windows Terminal's (`profiles.defaults.font.face`) and VS Code's terminal font, as it does from
WSL. The archive is unpacked with Windows' own `tar.exe` (bsdtar, which reads xz).

## Building and releasing

- `scripts\build-windows.ps1` builds `bin\imux.exe` (`-Arch arm64` for ARM) with the version
  stamps `make build` uses; `make build-windows` cross-builds it from macOS or Linux.
- `install.ps1 -FromSource <checkout>` builds and installs from a checkout; without a release to
  download, `install.ps1` run from a checkout builds that checkout.
- GoReleaser builds `windows/amd64` and `windows/arm64` as zips (`imux_X.Y.Z_windows_amd64.zip`)
  and attaches `install.ps1` to every release. The repository releases are downloaded from is one
  variable at the top of `install.ps1` (`$Repo`); a fork changes it there and in
  `internal/update.Repo`.

## Limits

- psmux is young and its tmux coverage is still growing; features gated on a tmux version follow
  the tmux level psmux reports.
- On consoles other than Windows Terminal (the old console host) colors and icons are drawn
  poorly.
- Over SSH into Windows the clipboard and notifications are on the other machine.
