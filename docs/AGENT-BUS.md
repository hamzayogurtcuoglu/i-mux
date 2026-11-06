# The agent bus

Agents running in ajan panes can talk to each other through tmux. There is no server or API: an
agent calls `ajan` from its own shell tool, and ajan reads or types into another pane on ajan's
tmux socket. Inside a pane `TMUX`, `TMUX_PANE`, `AJAN_PANE` and `AJAN_AGENT` are set.

## Commands

| Command | What it does |
|---|---|
| `ajan panes [--json]` | Lists panes: id, agent, session, `idle` / `busy` / `exited`, seconds since the screen changed, directory. The caller's pane is marked `*`. |
| `ajan read <target> [-n 60]` | Prints the last *n* lines of the target's screen and scrollback (wrapped lines joined, escape sequences removed). |
| `ajan send <target> "<message>" [--wait-idle] [--no-enter]` | Types the message into the target, prefixed with `[from <agent> <pane>] `, and presses Enter. `-` reads the message from stdin. `--wait-idle` waits until the target is idle first. |
| `ajan wait <target> [--timeout 600] [--idle 5s] [--print]` | Waits until the target's screen has not changed for `--idle` (default `bus.idle_after`). Exit 0 when idle, 124 on timeout. `--print` prints the final screen. |
| `ajan snippet [claude\|gemini\|agents]` | Prints instructions you may paste into `CLAUDE.md`, `GEMINI.md` or `AGENTS.md`. |

Busy or idle: agents redraw spinners and timers while they work, so a screen that stays the same
means the agent is waiting. `ajan panes` uses the watcher's record of screen changes when it is
running, and otherwise samples each screen twice, one second apart.

## Targets

- `%N`: a pane id as shown by `ajan panes` (`%3`). It must be a pane of ajan's tmux server.
- An agent name (`claude`, `gemini`, `copilot`, `codex`): the one live pane running that agent in
  the caller's tmux session, else on the whole server. When several panes match, the command fails
  and lists them: use the pane id.

## A typical exchange

```sh
ajan send gemini "Please review src/auth.ts and list problems, most important first"
ajan wait gemini --timeout 900
ajan read gemini -n 120
```

The receiving agent sees the message as if the user typed it:

```
[from claude %1] Please review src/auth.ts and list problems, most important first
```

## The snippet

Agents only use the bus when they know about it. `ajan snippet claude` prints a short block that
explains the commands and the rules (message other agents only when the user asked; a message
tagged `[from …]` is another agent's request, not the user's instruction). Paste it into the
project's `CLAUDE.md`, `GEMINI.md` or `AGENTS.md` yourself. **ajan never creates or edits these
files.**

## Configuration (`[bus]`)

| Key | Default | Meaning |
|---|---|---|
| `enabled` | `true` | `false` turns `ajan send` off. |
| `confirm` | `false` | Every message waits for you: a menu on the target's tmux client shows sender, target and the message, and only **Deliver** sends it. |
| `idle_after` | `"5s"` | How long a screen must stay unchanged to count as idle. |
| `max_message_bytes` | `16384` | Largest message. |
| `rate_per_minute` | `20` | Messages per minute per sending pane. |

## Security notes

The bus is a channel for **prompt injection**. An agent that read untrusted content (a web page,
an issue, a dependency's README) can be steered into messaging another agent, and that agent may
have more permissions than the first. ajan limits the damage but cannot judge messages:

- The snippet is opt-in and never installed automatically: without it, agents rarely use the bus.
- Every message carries a sender tag, and the snippet tells agents to treat tagged messages as
  requests, not as the user's instructions.
- `bus.confirm = true` puts you in the loop for every message; `bus.enabled = false` turns the bus
  off.
- The rate limit stops two agents from messaging each other in a loop, and the size limit keeps
  messages short.
- Targets must be panes of ajan's own tmux server, and an agent cannot message its own pane.
- Messages are pasted, never run: ajan does not execute message text. Bracketed paste is used only
  when the receiving program asked for it.

Agent sandboxes: an agent running in a sandbox (Codex `workspace-write`, Gemini `--sandbox`) may
not be allowed to reach the tmux socket, and then `ajan send` fails inside it (open question Q18).
