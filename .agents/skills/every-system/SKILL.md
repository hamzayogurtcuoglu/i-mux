---
name: every-system
description: Makes every i-mux or i-mux-core change work on macOS, Linux (WSL included) and native Windows alike, not only on the system it was written on. Use when planning, writing or reviewing any feature or fix, before docs-first writes its decision, and whenever a change touches files, paths, processes, shells, terminals, fonts, the clipboard, notifications, installers or updates.
---

# Every system

i-mux runs on macOS, Linux, WSL and native Windows (D85, D87). A change that works where it was
written and breaks elsewhere is a bug in the change. Most i-mux bugs on Windows so far were
assumptions from macOS or Linux: a font that never showed, menus closing under the mouse, a
second per click from process starts (D89, D92).

## Before writing code

Answer these for **each** of macOS, Linux, WSL and Windows, in the decision you write first
(ARCHITECTURE.md, TASKS.md):

1. **Does it run there at all?** Which programs, files and APIs does it need, and are they
   there by default? (`tar`, `sh`, `git`, `notify-send`, `xclip`, PowerShell, `reg.exe` …)
2. **What does the user see there?** Which terminal (Terminal.app, iTerm2, GNOME Terminal,
   Windows Terminal, the classic console, VS Code), which font, which shell.
3. **What does it cost there?** A process start is 1–5 ms on macOS and Linux, 20–50 ms on
   Windows and about 270 ms with endpoint security (EDR, DLP). Count the processes per click.
4. **What can fail there that cannot fail here?** No network or a proxy only the system knows,
   files held open, case-insensitive names, no admin rights, a path with spaces or a comma.
5. **How is it installed and updated there?** `install.sh`, `install.ps1`, `setup.exe`,
   Homebrew, `go install`, `imux update`: each must leave the feature working.

If the answer for a system is "it does not apply", say why in the decision.

## The usual differences

| Area | macOS / Linux | Windows | Watch for |
|---|---|---|---|
| Paths | `/`, `$HOME`, `/tmp` (a symlink on macOS) | `\`, `%USERPROFILE%`, `%LOCALAPPDATA%`, drive letters | `filepath`, never string joins; compare resolved paths |
| Executables | mode bits | `.exe`, `PATHEXT` | `sysx.ExeSuffix`, `sysx.Executable` |
| Commands | `sh -c` lines | no POSIX shell: argv form (E14, X1) | `tmux.UICommand`, `QuoteProgram`, never a hand-built shell line |
| Processes | cheap `fork`/`exec`, `setsid` | slow starts, job objects, no signals | fewer processes; `sysx` for signals, locks, detaching |
| Terminal | a pty | ConPTY or the console (K8–K13) | mouse modes, OSC queries, colours ConPTY changes |
| Fonts | the terminal's settings | Windows Terminal per profile, VS Code, `HKCU\Console` | a font installed *and* chosen; emoji when not |
| Network | system proxy in env vars | a proxy only WinINET knows | ship what can be shipped; never depend on a download at run time |
| Files | locks advisory, delete while open | locks mandatory, no delete while open | close before rename/remove; `.old` for a running binary |
| Text | UTF-8, `\n` | UTF-16 APIs, `\r\n` in some tools | the console's code page, `.ps1` kept ASCII |
| Clipboard, notifications, browser | `pbcopy`/`wl-copy`/`xclip`, `osascript`/`notify-send`, `open`/`xdg-open` | PowerShell, a toast, `ShellExecute` | WSL is Linux that reaches Windows' side |

## Where the code goes

- Platform code lives in build-tagged files (`_windows.go`, `_unix.go`, `_darwin.go`,
  `_linux.go`) behind small interfaces (D85). Core files have no `runtime.GOOS` switches.
- The engine's platform code is in i-mux-core (ARCHITECTURE.md §6 there); i-mux moves its pin.
- Strings a user reads go through `internal/i18n`, in English and Turkish.

## How each system is checked

- **Here:** `make ci` lints and vets the Linux, macOS and Windows builds and runs the unit
  tests on this system and with macOS's symlinked temp folder, and e2e on this system
  (`check-before-push`). It cannot run Windows tests or another system's e2e.
- **CI:** the `ubuntu`, `macos` and `windows` jobs, Windows' e2e on i-mux-core, the installer
  job and, on a tag, the release smoke tests. Watch every job after a push; a job red on one
  system only is still red.
- **Tests:** a test for behaviour that differs by system names the system it covers, skips
  where the program it needs is missing, and builds expected paths and commands with the
  code's own helpers. A Windows-only test goes in a `_windows_test.go` file.
- **Say what was not checked.** When a system could not be tried (no Windows machine, a
  terminal CI does not have), say so in the task's status and to the user.
