# Notes for coding agents working on i-mux

Read [CONTRIBUTING.md](CONTRIBUTING.md) first; [ARCHITECTURE.md](ARCHITECTURE.md) is the design and
[TASKS.md](TASKS.md) the plan.

- **Installing a build: only `make install-local`.** Never `cp bin/imux ~/.local/bin/imux` or
  `go build -o ~/.local/bin/imux`: i-mux is usually running from that file (this very session's
  sidebars), and on macOS overwriting it in place kills every imux process at once.
- Docs first: describe a change in ARCHITECTURE.md and TASKS.md and commit that, then the code.
- Commits and releases (see `.agents/skills/commit-and-release/SKILL.md`): commit messages and
  CHANGELOG entries are always in English, tags use `vX.Y.Z`, and commits must **never** include
  `Co-Authored-By:` trailers for Claude or any other AI agent.
- Checks before a commit: `make test`, `make lint`, `make e2e` (macOS and Linux).
- Tests use synthetic fixtures only: never read the real `~/.claude`, `~/.gemini`, `~/.copilot`,
  `~/.codex` or `~/.imux`, and never start an agent CLI with a real prompt.
