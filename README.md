<h1 align="center">i-mux</h1>

<p align="center">
  <b>Hit your usage limit in one AI coding agent? Keep going in another, with the whole conversation.</b><br>
  Claude Code · Gemini CLI · GitHub Copilot CLI · OpenAI Codex CLI
</p>

<p align="center">
  <a href="https://github.com/hamzayogurtcuoglu/i-mux/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/hamzayogurtcuoglu/i-mux/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
  <img alt="macOS and Linux" src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20WSL-lightgrey.svg">
</p>

<p align="center">
  <img src="docs/images/limit.svg" alt="Claude hits its usage limit and i-mux offers to continue in Gemini or Codex" width="880">
</p>

i-mux runs your agent CLIs in a terminal workspace. When one hits its limit, or whenever you like,
press one key and the same pane restarts with another agent. The new agent receives a condensed
handoff: the original request, the task list, files touched, repository state and where the work
stopped. No re-explaining, no copy-paste.

It is a single binary, `imux`, built on [tmux](https://github.com/tmux/tmux) (on its own socket, so your
tmux setup is untouched). Everything stays on your machine.

## Install

macOS, Linux, or Windows through WSL. Requires tmux 3.2+ (3.4+ for the clickable agent bar).

```sh
brew install hamzayogurtcuoglu/tap/imux
```

```sh
curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
```

```sh
go install github.com/hamzayogurtcuoglu/i-mux/cmd/imux@latest   # Go 1.27+
```

The install script verifies the checksum, installs to `~/.local/bin` and offers to install tmux if
it is missing. Then install at least one agent CLI and check the setup:

```sh
imux doctor
```

## Usage

```sh
cd ~/code/my-project
imux            # start the first installed agent (or: imux claude)
```

| Key / action | What it does |
|---|---|
| `Ctrl+\` | pane menu: switch agent, sessions, new pane, preview handoff, limits |
| `Ctrl+\` then `c` `g` `p` `x` | continue in Claude / Gemini / Copilot / Codex |
| click an agent on the bottom bar | continue in that agent |
| `prefix S` | pick any earlier conversation of the project |
| `prefix a` | the pane menu, for terminals that keep `Ctrl+\` |

<table>
  <tr>
    <td width="50%"><img src="docs/images/switched.svg" alt="Gemini continues the session with i-mux's handoff"></td>
    <td width="50%"><img src="docs/images/menu.svg" alt="The i-mux pane menu"></td>
  </tr>
  <tr>
    <td align="center">Gemini picks up where Claude stopped</td>
    <td align="center">The pane menu (<code>Ctrl+\</code>)</td>
  </tr>
</table>

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
