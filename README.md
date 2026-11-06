# ajan

**Continue the same AI coding conversation in another agent CLI, in the same terminal pane, with one
click or one key.**

You are working in Claude Code and hit your usage limit. Press `Ctrl+\`, then `g`: the pane restarts
with Gemini CLI, which receives a condensed record of the conversation (the original request, the
task list, the files touched, the repository state, the recent exchange and where it stopped) and
carries on. No re-explaining, no copy-paste, no commands to type.

ajan works with **Claude Code**, **Gemini CLI**, **GitHub Copilot CLI** and **OpenAI Codex CLI**. It is
a single small binary that runs the agents in [tmux](https://github.com/tmux/tmux) on its own socket;
your own tmux setup is not touched. ("ajan" is Turkish for "agent".)

```
┌─ Claude · opus ───────────────────────┬─ Gemini ──────────────────────────────┐
│ > fix the login redirect bug           │ > review src/auth.ts                   │
│ ● Reading src/auth.ts …                │ ✦ Looking at the handler …             │
└────────────────────────────────────────┴────────────────────────────────────────┘
 0:proj*                                       Claude ⛔15:00 · Gemini ● · 14:32
 ajan │ ▶ Claude │ Gemini │ Codex │ Copilot (not installed) │ ⧉ Sessions │ ☰
```

## Features

- **Switch from the UI.** Click an agent on the agent bar, or open the pane menu (`Ctrl+\`,
  `prefix a`, right-click on a border) and press its key. The new agent gets the conversation as
  its first prompt.
- **Usage limits.** A small watcher notices limit messages on agent screens. By default it asks
  "continue in …?"; it can also switch on its own, or only show the limit. Limits and reset times
  appear in the status bar; `ajan limits` shows local usage per agent.
- **Back to where you were.** Switching back to an agent that already ran in the pane resumes its
  own session and tells it only what happened in the meantime.
- **Session picker.** `prefix S` lists the project's recent conversations of every agent; continue
  any of them natively or in another agent.
- **Several agents side by side**, each pane labeled with its agent, and an **agent bus** so they
  can message each other (`ajan send gemini "review src/a.ts"`).
- **Light.** Nothing of ajan sits between your keys and the agents; tmux does all terminal work.
  The only long-running process is the watcher (about 10 MB, ~0% CPU). Everything stays local.

## Install

macOS and Linux (on Windows, inside WSL). ajan needs tmux 3.2 or newer; 3.4+ makes the agent bar
clickable.

```sh
brew install hamzayogurtcuoglu/tap/ajan
```

or

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

The script verifies the download's checksum, installs to `~/.local/bin` (`--prefix` to change),
and offers to install tmux if it is missing (it asks first, and says when it would use sudo).

Install at least one agent CLI, then check the setup:

```sh
ajan doctor
```

## Quick start

```sh
cd ~/code/my-project
ajan            # starts the first installed agent (or: ajan claude)
```

ajan opens a tmux session for the project, or attaches to it if it already exists. Inside it:

| Key / action | Does |
|---|---|
| `Ctrl+\` | pane menu: switch agent, sessions, new pane, preview handoff, limits, close |
| `Ctrl+\` then `c` / `g` / `p` / `x` | switch this pane to Claude / Gemini / Copilot / Codex |
| `prefix a` (`Ctrl+b a`) | the same menu (when the terminal keeps `Ctrl+\`) |
| `prefix S` | session picker |
| click an agent on the bar | switch the active pane to it |
| right-click a pane border | that pane's menu |
| `ajan gemini` (in a pane) | open a new window with Gemini |

From the command line: `ajan switch gemini`, `ajan split codex -h`, `ajan ls`, `ajan attach`,
`ajan limits`, `ajan sessions`. Without tmux, `ajan handoff --from claude --to gemini` starts Gemini
in the current terminal with the handoff, and `ajan handoff --print` just prints it.

## When an agent hits its limit

Set `on_limit` in `~/.ajan/config.toml` (`ajan config init` writes a commented file):

| `on_limit` | Behavior |
|---|---|
| `"ask"` (default) | a menu: "Claude hit its limit (resets 15:00): continue in …" |
| `"auto"` | switch to the first installed agent in `order` that is not limited |
| `"off"` | only show the limit in the status bar |

## Privacy

- **Local only.** ajan makes no network calls, has no telemetry, never calls model APIs and never
  reads credential files. It reads the agents' session files and runs `git status` / `git diff
  --stat` when you switch.
- **Switching shares the conversation with the other provider.** The handoff contains excerpts of
  your conversation with one provider plus repository state, and the next agent sends it to its
  own provider. That is why the default for limits is `ask`, and ajan shows a one-time notice on
  the first switch.
- **Secrets** (private keys, cloud and API tokens, `Authorization` headers) are redacted from the
  handoff on a best-effort basis. Review `ajan handoff --print` if a session contained secrets.
- **Process list.** By default the handoff is passed as a command-line argument, which other
  users on the same machine can see with `ps`. Set `[handoff] inject = "paste"` to paste it into
  the agent instead (`ajan doctor` tells you when other users exist).
- Handoffs, logs and state live in `~/.ajan` with private permissions (0700 / 0600); handoffs are
  deleted after 30 days.
- The agent bus can carry prompt injection between agents; see [docs/AGENT-BUS.md](docs/AGENT-BUS.md).

## Documentation

- [docs/CONFIG.md](docs/CONFIG.md): every setting
- [docs/HANDOFF.md](docs/HANDOFF.md): what the next agent receives
- [docs/AGENT-BUS.md](docs/AGENT-BUS.md): agents talking to each other
- [docs/LIMITS.md](docs/LIMITS.md): `ajan limits` and its JSON
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md): keys, nvm, terminals, missing sessions
- [ARCHITECTURE.md](ARCHITECTURE.md), [CONTRIBUTING.md](CONTRIBUTING.md), [CHANGELOG.md](CHANGELOG.md)
- Türkçe: [README.tr.md](README.tr.md)

## License

MIT. See [LICENSE](LICENSE).
