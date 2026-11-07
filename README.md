<h1 align="center">i-mux</h1>

<p align="center">
  <b>Hit your usage limit in one AI coding agent? Keep going in another, with the whole conversation.</b><br>
  Claude Code · Antigravity CLI · GitHub Copilot CLI · OpenAI Codex CLI
</p>

<p align="center">
  <a href="https://github.com/hamzayogurtcuoglu/i-mux/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/hamzayogurtcuoglu/i-mux/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="macOS and Linux" src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20WSL-lightgrey.svg">
</p>

<p align="center">
  <img src="docs/images/limit.svg" alt="Claude hits its usage limit and i-mux offers to continue in Antigravity or Codex" width="880">
</p>

i-mux runs your agent CLIs in a terminal workspace. When one hits its limit, or whenever you like,
press one key and the same pane restarts with another agent. The new agent receives a condensed
handoff: the original request, the task list, files touched, repository state and where the work
stopped. No re-explaining, no copy-paste.

It is a single binary, `imux`, built on [tmux](https://github.com/tmux/tmux) (on its own socket, so your
tmux setup is untouched). Everything stays on your machine.

## Install

One line on macOS, Linux, or Windows through WSL (open a WSL terminal):

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

It downloads the release for your OS and CPU, verifies its checksum, installs `imux` to
`~/.local/bin`, adds that directory to your shell's PATH (zsh, bash or fish) and offers to install
tmux (3.2+; 3.4+ for the clickable agent bar) if it is missing. Open a new terminal afterwards.
Options go after `sh -s --`, for example `... | sh -s -- --yes` to install tmux without asking,
`--version v0.1.0`, `--prefix DIR` or `--no-modify-path`.

With Go 1.27+ you can also build it yourself:

```sh
go install github.com/hamzayogurtcuoglu/i-mux/cmd/imux@latest
```

Then install at least one agent CLI and check the setup. For Google's models, install
[Antigravity CLI](https://antigravity.google/docs/cli/) (`agy`):
`curl -fsSL https://antigravity.google/cli/install.sh | bash`. Google stopped serving personal
accounts in Gemini CLI on 2026-06-18, so i-mux supports Antigravity CLI instead of Gemini CLI.

```sh
imux doctor
```

To uninstall, delete `~/.local/bin/imux` and `~/.imux`, and remove the line the installer added to
your shell profile (marked `# Added by the imux installer`).

## Usage

```sh
cd ~/code/my-project
imux            # start the first installed agent (or: imux claude)
```

The **sidebar** on the left shows your projects (branch, uncommitted changes) and every agent
working in them, with what each one is doing: a spinner while it works, **Needs you** when it asks
you something, a green dot when it finished while you were elsewhere, a ring when idle, and how
long ago that changed. Everything is clickable:

| Sidebar / key | What it does |
|---|---|
| click a project · **+** next to **Projects** (or `o`) | switch to it · open another folder next to this one |
| click an agent · **+** next to **Agents** | go to it (in any project) · open a new agent in a pane (`prefix v` / `prefix -` for right / down) |
| click an agent under **Continue this chat in** | the active pane continues in that agent, with the conversation |
| **✎ Rename** (or `n`) | give the agent a name of your own, like "backend fix" (agents that title their terminal name themselves) |
| **± Changes** (or `f`) | the changed files; pick one to see its diff |
| **⧉ History** (or `prefix S`) · **◔ Limits** · **? Help** | earlier conversations · usage limits · every key |
| `prefix b` | hide or show the sidebar |
| `Ctrl+\` (or `prefix a`), then `c` `a` `p` `x` | the pane menu; continue in Claude / Antigravity / Copilot / Codex |

`prefix` is `Ctrl+b`: press it, release, then press the key.

<table>
  <tr>
    <td width="50%"><img src="docs/images/switched.svg" alt="Antigravity continues the session with i-mux's handoff"></td>
    <td width="50%"><img src="docs/images/menu.svg" alt="The i-mux pane menu"></td>
  </tr>
  <tr>
    <td align="center">Antigravity picks up where Claude stopped</td>
    <td align="center">The pane menu (<code>Ctrl+\</code>)</td>
  </tr>
</table>

After a switch you see one short message, not the whole conversation: i-mux saves it in
`~/.imux/handoffs` and tells the new agent to read that file. The agent reads it, says in one
sentence that it has the context, and waits for you: it does not start working (or spend tokens) on
its own. Set `[handoff] deliver = "inline"` to send the whole document as the message instead,
and `[handoff] on_arrival = "continue"` to let it carry on with an unfinished request right away.

Switching back to an agent that already ran in the pane resumes its own session and tells it only
what happened in the meantime. Run `imux help` for the command line (`switch`, `split`, `ls`,
`limits`, `sessions`, `handoff --print`).

## When an agent hits its limit

A small watcher notices limit messages on agent screens and shows them in the status bar. Choose
what happens next in `~/.imux/config.toml` (`imux config init` writes a commented one):

```toml
on_limit = "ask"   # ask (default) | auto: switch to the next agent in `order` | off: only show it
```

## Privacy

- No network calls, no telemetry, no API keys. i-mux reads the agents' local session files and
  `git status`.
- Switching sends the handoff to the next agent's provider. Secrets are redacted on a best-effort
  basis; check with `imux handoff --print`.
- The handoff is passed as a command-line argument, which other users on the machine can see with
  `ps`. On shared machines set `[handoff] inject = "paste"`.
- Details in [docs/HANDOFF.md](docs/HANDOFF.md) and [docs/AGENT-BUS.md](docs/AGENT-BUS.md).

## Documentation

[Configuration](docs/CONFIG.md) · [Handoff format](docs/HANDOFF.md) · [Limits](docs/LIMITS.md) ·
[Agent bus](docs/AGENT-BUS.md) · [Troubleshooting](docs/TROUBLESHOOTING.md) ·
[Architecture](ARCHITECTURE.md) · [Contributing](CONTRIBUTING.md) · [Türkçe](README.tr.md)

## License

[MIT](LICENSE)
