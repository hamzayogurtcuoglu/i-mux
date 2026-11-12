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
- **Before a commit that will be pushed: `make ci`** (`.agents/skills/check-before-push/SKILL.md`):
  what CI checks, here: lint for the Linux, Windows and macOS builds, the unit tests on Linux,
  with macOS's symlinked temp folder and on Windows (from WSL), and e2e. Unit tests passing on
  Linux are not enough (v0.49.0 failed CI on macOS and Windows). Do not commit, tag or hand over
  a push while it fails, and say which checks could not run here.
- Tests use synthetic fixtures only: never read the real `~/.claude`, `~/.gemini`, `~/.copilot`,
  `~/.codex` or `~/.imux`, and never start an agent CLI with a real prompt.
