# Configuration

<!-- Generated from internal/config/default.toml by `go generate ./internal/config`. Do not edit. -->

i-mux reads `~/.imux/config.toml` (or `$IMUX_HOME/config.toml`). Every key is optional; the
values below are the built-in defaults. `imux config init` writes this file with comments,
`imux config show` prints the effective configuration, and `imux config path` its location.
Unknown keys produce a warning; invalid values and regular expressions are rejected with the
line number.

| Key | Default | Description |
|---|---|---|
| `order` | `["claude", "antigravity", "copilot", "codex", "opencode"]` | Agent order: menu order, agent bar order and auto-switch priority. |
| `default_agent` | `""` | Agent started by a plain `imux`. "" = first installed agent in `order`. |
| `on_limit` | `"ask"` | What happens when an agent hits its usage limit: ask  = show a "continue in ..." menu (default) auto = switch to the first installed, non-limited agent in `order` off  = only show the limit in the status bar |
| `on_exit` | `"shell"` | What a pane does when its agent exits: "shell" (drop to your login shell) or "close". |
| `locale` | `""` | UI language: "" = from IMUX_LANG / LANG; "en" or "tr". |
| `editor` | `""` | Editor files open in from the sidebar and search ("micro", "vim", "code" ...). "" = micro, then $VISUAL, $EDITOR, nano, vim. |
| `handoff.budget_chars` | `30000` | Size of the handoff document in characters (runes). |
| `handoff.max_arg_bytes` | `102400` | Upper bound for the handoff when passed as a command-line argument. |
| `handoff.inject` | `"argv"` | "argv" passes the handoff as the agent's initial prompt; "paste" pastes it into the agent after it started (hides it from `ps`). |
| `handoff.on_arrival` | `"wait"` | What the new agent does with the handoff: "wait" = confirm in one sentence and wait for your next message (starts no work, spends no extra tokens); "continue" = carry on with the unfinished request at once. |
| `handoff.deliver` | `"file"` | "file" = the agent gets a two-line message pointing to the saved handoff file and reads it itself (you do not see the whole document); "inline" = the whole document is the agent's first message. |
| `handoff.redact_secrets` | `true` | Best-effort removal of API keys, tokens and private keys. |
| `handoff.include_git` | `true` | Include branch, `git status --short` and `git diff --stat`. |
| `handoff.include_source_path` | `true` | Include the path of the previous session's transcript file. |
| `handoff.keep_days` | `30` | Delete handoff files older than this many days. |
| `handoff.head_bytes` | `1048576` | How much of a session file is read: the head (original request) ... |
| `handoff.tail_bytes` | `8388608` | ... and the tail (recent history, latest task list). |
| `limits.poll_interval` | `"2.5s"` | How often the watcher looks at agent panes while a client is attached. |
| `limits.idle_interval` | `"15s"` | How often it checks for an attached client when none is attached. |
| `limits.grace` | `"20s"` | Limit messages within this time after an agent started are ignored (resumed sessions). |
| `limits.cooldown` | `"10m"` | After a limit prompt, the same pane is not prompted again for this long. |
| `limits.scan_lines` | `25` | Number of last non-blank lines of an agent's screen that are scanned. |
| `limits.default_expiry` | `"5h"` | How long a limit without a parseable reset time is shown. |
| `limits.claude_statusline` | `true` | Start Claude Code with imux's status line (for this session only, your settings files are not changed): it records the 5-hour and weekly percentages Claude reports, for the limits panel. Your own status line still shows. |
| `bus.enabled` | `true` | Allow agents to message each other with `imux send`. |
| `bus.confirm` | `false` | Ask before every message is delivered. |
| `bus.idle_after` | `"5s"` | A pane counts as idle once its screen did not change for this long. |
| `bus.max_message_bytes` | `16384` | Largest message `imux send` delivers. |
| `bus.rate_per_minute` | `20` | Messages per minute per sending pane (stops agent ping-pong loops). |
| `git.commit_agent` | `"auto"` | Who writes a suggested commit message (✨ in Source Control): "auto" = the agent of the pane the sidebar follows (else the first installed), or one of claude, antigravity, copilot, codex, opencode. It gets the changes once, without its UI; nothing is committed until you choose Commit. |
| `git.commit_model` | `""` | Model for commit_agent when it names an agent ("" = that agent's default), e.g. "haiku" for claude or "groq/llama-3.3-70b-versatile" for opencode. |
| `tmux.menu_key_root` | `'C-\'` | Key that opens the pane menu without the prefix. "" disables it. |
| `tmux.menu_key_prefix` | `"a"` | prefix + key opens the pane menu. |
| `tmux.picker_key_prefix` | `"S"` | prefix + key opens the session picker. |
| `tmux.sidebar` | `true` | Sidebar on the left of each window: panes, agents to continue in, split buttons, sessions, limits and keys, all clickable (ARCHITECTURE §9.3). |
| `tmux.sidebar_width` | `41` |  |
| `tmux.sidebar_key_prefix` | `"b"` | prefix + key shows or hides the sidebar. |
| `tmux.split_right_key_prefix` | `"v"` | prefix + key splits the active pane (asks which agent to start). |
| `tmux.split_down_key_prefix` | `"-"` |  |
| `tmux.icons` | `"auto"` | Icons of the sidebar's rail: "nerd" = the editor's icons (needs a Nerd Font in the terminal), "emoji", or "auto" = nerd when a Nerd Font is installed or the terminal brings its symbols (Ghostty, WezTerm, kitty), else emoji. |
| `tmux.tabs` | `true` | Tab bar at the top: each tab (tmux window) with its name and state, and a + that opens a new tab with the agent you pick. |
| `tmux.new_tab_key_prefix` | `"c"` | prefix + key opens a new tab (asks which agent); prefix + key names the tab. |
| `tmux.rename_tab_key_prefix` | `","` |  |
| `tmux.agent_bar` | `false` | Clickable agent bar (second status line), the older alternative to the sidebar. |
| `tmux.mouse` | `true` | Mouse support (clicks on the agent bar, pane borders and menus). |
| `tmux.history_limit` | `50000` | Scrollback lines kept per pane. |
| `agents.claude.bin` | `""` | Binary to run; "" finds it on PATH (an absolute path or a name). |
| `agents.claude.args` | `[]` | Arguments inserted before the template, e.g. ["--model", "opus"]. |
| `agents.claude.new_args` | `["--session-id", "{uuid}", "{prompt}"]` | Template for a new session; the handoff goes in {prompt}. |
| `agents.claude.resume_args` | `["--resume", "{id}", "{prompt}"]` | Template for resuming a native session ({id}) with a delta handoff. |
| `agents.claude.home` | `""` | Where the agent keeps its sessions. "" = $CLAUDE_CONFIG_DIR or ~/.claude |
| `agents.claude.resume` | `true` | Resume the agent's earlier session when switching back to it. |
| `agents.claude.patterns` | `[]` | Extra limit patterns: [{ re = 'my limit text', severity = "limit" }] (limit, warn or ignore; case-insensitive). |
| `agents.claude.replace_default_patterns` | `false` | Use only `patterns`, not the built-in ones. |
| `agents.antigravity.new_args` | `["-i", "{prompt}"]` | Template for a new session; the handoff goes in {prompt}. |
| `agents.antigravity.resume_args` | `["--conversation", "{id}", "-i", "{prompt}"]` | Template for resuming a native session ({id}) with a delta handoff. |
| `agents.antigravity.home` | `""` | Where the agent keeps its sessions. "" = ~/.gemini/antigravity-cli |
| `agents.copilot.new_args` | `["-i", "{prompt}"]` | Template for a new session; the handoff goes in {prompt}. |
| `agents.copilot.resume_args` | `["--resume", "{id}"]` | Template for resuming a native session ({id}) with a delta handoff. |
| `agents.copilot.home` | `""` | Where the agent keeps its sessions. "" = $COPILOT_HOME or ~/.copilot |
| `agents.codex.new_args` | `["{prompt}"]` | Template for a new session; the handoff goes in {prompt}. |
| `agents.codex.resume_args` | `["resume", "{id}", "{prompt}"]` | Template for resuming a native session ({id}) with a delta handoff. |
| `agents.codex.home` | `""` | Where the agent keeps its sessions. "" = $CODEX_HOME or ~/.codex |
| `agents.opencode.new_args` | `["--session", "ses_imux{uuid}", "--prompt", "{prompt}"]` | Template for a new session; the handoff goes in {prompt}. |
| `agents.opencode.resume_args` | `["--session", "{id}", "--prompt", "{prompt}"]` | Template for resuming a native session ({id}) with a delta handoff. |
| `agents.opencode.home` | `""` | Where the agent keeps its sessions. "" = $XDG_DATA_HOME/opencode or ~/.local/share/opencode |

Agent ids are `claude`, `antigravity`, `copilot` and `codex`. Launch templates use the placeholders
`{prompt}` (the handoff, at most once), `{id}` (the session to resume) and `{uuid}` (a new id).
A placeholder with an empty value is dropped together with the flag before it.

## The default file

```toml
# i-mux configuration (~/.imux/config.toml)
#
# Every key is optional; the values below are the built-in defaults.
# Regex patterns use TOML literal strings ('...'), so backslashes need no escaping.

# Agent order: menu order, agent bar order and auto-switch priority.
order         = ["claude", "antigravity", "copilot", "codex", "opencode"]

# Agent started by a plain `imux`. "" = first installed agent in `order`.
default_agent = ""

# What happens when an agent hits its usage limit:
#   ask  = show a "continue in ..." menu (default)
#   auto = switch to the first installed, non-limited agent in `order`
#   off  = only show the limit in the status bar
on_limit      = "ask"

# What a pane does when its agent exits: "shell" (drop to your login shell) or "close".
on_exit       = "shell"

# UI language: "" = from IMUX_LANG / LANG; "en" or "tr".
locale        = ""

# Editor files open in from the sidebar and search ("micro", "vim", "code" ...).
# "" = micro, then $VISUAL, $EDITOR, nano, vim.
editor        = ""

[handoff]
# Size of the handoff document in characters (runes).
budget_chars        = 30000
# Upper bound for the handoff when passed as a command-line argument.
max_arg_bytes       = 102400
# "argv" passes the handoff as the agent's initial prompt; "paste" pastes it
# into the agent after it started (hides it from `ps`).
inject              = "argv"
# What the new agent does with the handoff: "wait" = confirm in one sentence
# and wait for your next message (starts no work, spends no extra tokens);
# "continue" = carry on with the unfinished request at once.
on_arrival          = "wait"
# "file" = the agent gets a two-line message pointing to the saved handoff
# file and reads it itself (you do not see the whole document); "inline" =
# the whole document is the agent's first message.
deliver             = "file"
# Best-effort removal of API keys, tokens and private keys.
redact_secrets      = true
# Include branch, `git status --short` and `git diff --stat`.
include_git         = true
# Include the path of the previous session's transcript file.
include_source_path = true
# Delete handoff files older than this many days.
keep_days           = 30
# How much of a session file is read: the head (original request) ...
head_bytes          = 1048576
# ... and the tail (recent history, latest task list).
tail_bytes          = 8388608

[limits]
# How often the watcher looks at agent panes while a client is attached.
poll_interval  = "2.5s"
# How often it checks for an attached client when none is attached.
idle_interval  = "15s"
# Limit messages within this time after an agent started are ignored (resumed sessions).
grace          = "20s"
# After a limit prompt, the same pane is not prompted again for this long.
cooldown       = "10m"
# Number of last non-blank lines of an agent's screen that are scanned.
scan_lines     = 25
# How long a limit without a parseable reset time is shown.
default_expiry = "5h"
# Start Claude Code with imux's status line (for this session only, your
# settings files are not changed): it records the 5-hour and weekly
# percentages Claude reports, for the limits panel. Your own status line
# still shows.
claude_statusline = true

[bus]
# Allow agents to message each other with `imux send`.
enabled           = true
# Ask before every message is delivered.
confirm           = false
# A pane counts as idle once its screen did not change for this long.
idle_after        = "5s"
# Largest message `imux send` delivers.
max_message_bytes = 16384
# Messages per minute per sending pane (stops agent ping-pong loops).
rate_per_minute   = 20

[git]
# Who writes a suggested commit message (✨ in Source Control): "auto" = the
# agent of the pane the sidebar follows (else the first installed), or one of
# claude, antigravity, copilot, codex, opencode. It gets the changes once,
# without its UI; nothing is committed until you choose Commit.
commit_agent      = "auto"
# Model for commit_agent when it names an agent ("" = that agent's default),
# e.g. "haiku" for claude or "groq/llama-3.3-70b-versatile" for opencode.
commit_model      = ""

[tmux]
# Key that opens the pane menu without the prefix. "" disables it.
menu_key_root     = 'C-\'
# prefix + key opens the pane menu.
menu_key_prefix   = "a"
# prefix + key opens the session picker.
picker_key_prefix = "S"
# Sidebar on the left of each window: panes, agents to continue in, split
# buttons, sessions, limits and keys, all clickable (ARCHITECTURE §9.3).
sidebar           = true
sidebar_width     = 41
# prefix + key shows or hides the sidebar.
sidebar_key_prefix = "b"
# prefix + key splits the active pane (asks which agent to start).
split_right_key_prefix = "v"
split_down_key_prefix  = "-"
# Icons of the sidebar's rail: "nerd" = the editor's icons (needs a Nerd Font in
# the terminal), "emoji", or "auto" = nerd when a Nerd Font is installed or the
# terminal brings its symbols (Ghostty, WezTerm, kitty), else emoji.
icons             = "auto"
# Tab bar at the top: each tab (tmux window) with its name and state, and a
# + that opens a new tab with the agent you pick.
tabs              = true
# prefix + key opens a new tab (asks which agent); prefix + key names the tab.
new_tab_key_prefix    = "c"
rename_tab_key_prefix = ","
# Clickable agent bar (second status line), the older alternative to the sidebar.
agent_bar         = false
# Mouse support (clicks on the agent bar, pane borders and menus).
mouse             = true
# Scrollback lines kept per pane.
history_limit     = 50000

# Per-agent settings. Placeholders in templates:
#   {prompt} = the handoff (at most once), {id} = session id to resume, {uuid} = a new UUID.
# bin = "" resolves the binary on PATH. args are inserted before the template.
# patterns adds limit patterns: [{ re = 'my limit text', severity = "limit" }]
#   (severity: limit | warn | ignore; matching is case-insensitive).
# resume = false disables native resume when switching back to this agent.

[agents.claude]
bin                      = ""
args                     = []
new_args                 = ["--session-id", "{uuid}", "{prompt}"]
resume_args              = ["--resume", "{id}", "{prompt}"]
home                     = ""   # "" = $CLAUDE_CONFIG_DIR or ~/.claude
resume                   = true
patterns                 = []
replace_default_patterns = false

# Antigravity CLI (agy): Google's successor to Gemini CLI for personal Google
# accounts. Its conversations are read from brain/<id>/ under home.
[agents.antigravity]
bin                      = ""   # "" = agy on PATH
args                     = []
new_args                 = ["-i", "{prompt}"]
resume_args              = ["--conversation", "{id}", "-i", "{prompt}"]
home                     = ""   # "" = ~/.gemini/antigravity-cli
resume                   = true
patterns                 = []
replace_default_patterns = false

[agents.copilot]
bin                      = ""
args                     = []
new_args                 = ["-i", "{prompt}"]
resume_args              = ["--resume", "{id}"]
home                     = ""   # "" = $COPILOT_HOME or ~/.copilot
resume                   = true
patterns                 = []
replace_default_patterns = false

[agents.codex]
bin                      = ""
args                     = []
new_args                 = ["{prompt}"]
resume_args              = ["resume", "{id}", "{prompt}"]
home                     = ""   # "" = $CODEX_HOME or ~/.codex
resume                   = true
patterns                 = []
replace_default_patterns = false

# OpenCode: works with any model endpoint you configure in OpenCode itself
# (OpenAI-compatible APIs, Anthropic, Ollama, NVIDIA, Groq ...).
[agents.opencode]
bin                      = ""
args                     = []
new_args                 = ["--session", "ses_imux{uuid}", "--prompt", "{prompt}"]
resume_args              = ["--session", "{id}", "--prompt", "{prompt}"]
home                     = ""   # "" = $XDG_DATA_HOME/opencode or ~/.local/share/opencode
resume                   = true
patterns                 = []
replace_default_patterns = false
```
