# Troubleshooting

Start with `ajan doctor`: it checks tmux, the agent CLIs, `~/.ajan` and the watcher, and prints the
keys. It never starts an agent with a prompt.

## `Ctrl+\` does nothing (VS Code and other terminals)

Some terminals keep `Ctrl+\` for themselves. **`prefix a`** (`Ctrl+b`, then `a`) always opens the
same menu, and `prefix S` opens the session picker.

- **VS Code:** its integrated terminal sends most keys to the shell, but keybindings can take
  them first. Set `"terminal.integrated.sendKeybindingsToShell": true`, or remove the keybinding
  that uses `Ctrl+\` (`Preferences: Open Keyboard Shortcuts`, search for `ctrl+\`).
- **Pick another key:** in `~/.ajan/config.toml` set `[tmux] menu_key_root = "M-m"` (Alt+m), or
  `""` to turn the root key off. Run `ajan` again to apply it.

## Clicking the agent bar does nothing

- Clickable regions need **tmux 3.4 or newer** (`tmux -V`). On older tmux (Ubuntu 22.04 ships
  3.2a) the second status line only shows the agents; use the menu (`Ctrl+\` or `prefix a`).
  `ajan doctor` says when this applies.
- The terminal must send mouse events: `[tmux] mouse = true` (the default), and in iTerm2
  "Enable mouse reporting" must be on.
- Right-click on a pane border opens that pane's menu. In tmux a border belongs to the pane on its
  left or above it.

## An agent "is not installed" although it works in my shell (nvm)

Agent CLIs installed with npm under nvm are `#!/usr/bin/env node` scripts: they need nvm's `node`
on `PATH`. The tmux server, key bindings and the watcher do not run your shell profile.

ajan resolves each agent when you run `ajan` from an interactive shell and remembers the binary
and its `PATH` in `~/.ajan/state.json`; agents are then launched with that `PATH`. So run `ajan`
(or `ajan doctor`) once from a shell where `claude --version` works. `ajan doctor` warns when an
agent's `node` cannot be found. You can also set the binary explicitly:
`[agents.claude] bin = "/home/me/.nvm/versions/node/v22.15.0/bin/claude"`.

## Shift+Enter, or other modified keys, do not work in an agent

ajan's tmux enables extended keys (`extended-keys on`, `terminal-features *:extkeys`). The outer
terminal must support them too: iTerm2 (Report modifiers using CSI u), Ghostty, kitty and WezTerm
do; VS Code and Terminal.app may not. Most agents also accept another key for a new line (for
example `\` then Enter, or `Ctrl+j`).

## Colors look wrong, or `tmux-256color` is missing

ajan uses the `tmux-256color` terminfo when it is installed, else `screen-256color`. Install it
with `apt install ncurses-term` (Debian/Ubuntu) or `brew install ncurses` (macOS). Truecolor is
enabled for every terminal.

## A switch says "no session file found" or the handoff looks empty

ajan reads the source agent's own session files (`~/.claude/projects`, `~/.gemini/tmp`,
`~/.copilot/session-state`, `~/.codex/sessions`).

- When none is found, the handoff is rebuilt from the pane's terminal scrollback and the message
  says so. Agents that use the alternate screen (Gemini, Copilot) leave only the visible screen.
- Check what ajan finds: `ajan sessions` in the project directory, and `ajan handoff --print`.
- Agents keep sessions per directory: ajan uses the directory the pane started in.
- Custom locations: `[agents.claude] home = "..."` (also `CLAUDE_CONFIG_DIR`, `CODEX_HOME`,
  `COPILOT_HOME`).
- Session formats change between CLI versions. `ajan handoff --print` shows parser warnings on
  stderr; please report them (without session content).

## An outdated agent CLI

`ajan doctor` warns when an agent is older than the version ajan was checked against, or when its
`--help` does not mention the flags of its launch template (`--session-id`, `-i`, `--resume`,
`resume`). Update it (`npm i -g @anthropic-ai/claude-code`, `@google/gemini-cli`,
`@github/copilot`, `@openai/codex`) or adjust `new_args` / `resume_args` in `config.toml`.

## The new agent exits right away after a switch

The pane drops back to your shell with a message naming the handoff file. Common causes: a first-run
dialog (login, folder trust), an unsupported flag, or an expired login. Start the agent once by
hand in that directory, then switch again. The handoff stays in `~/.ajan/handoffs/` and can be
pasted manually. If a *resume* fails within 10 seconds, ajan starts a new session with the full
handoff automatically.

## Limits are not detected

- The watcher runs only while ajan's tmux server exists; `ajan doctor` shows it. It looks at agent
  panes only while a client is attached.
- It reads the last 25 lines of each agent's screen. Messages that appear within 20 seconds of an
  agent starting are ignored (resumed sessions often redraw old ones).
- Add your own patterns: `[agents.gemini] patterns = [{ re = 'quota is used up', severity = "limit" }]`.
- `~/.ajan/watch.log` lists what it matched (pattern ids only, never screen text).

## Windows

ajan needs tmux, so on Windows run it inside WSL. Keep the binary on the Linux file system
(`~/.local/bin`): binaries under `/mnt/c` start much more slowly and are reported as writable by
everyone.

## Starting over

`tmux -L ajan kill-server` closes every ajan pane. `rm -rf ~/.ajan` removes ajan's settings, state,
handoffs and logs; it never touches the agents' own files.
