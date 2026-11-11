# Contributing to i-mux

Thanks for helping. i-mux is small on purpose: one static Go binary, tmux for everything on screen,
and no runtime dependencies besides tmux. Read [ARCHITECTURE.md](ARCHITECTURE.md) for the design
and [TASKS.md](TASKS.md) for the plan.

## Building and testing

You need Go (see `go.mod`), tmux ≥ 3.2 and, for the full check, golangci-lint and shellcheck.

```sh
make build         # bin/imux
make test          # unit tests
make race          # unit tests with the race detector
make e2e           # end-to-end tests against a real tmux on a private socket
make lint          # golangci-lint (vet, staticcheck, errcheck, gofumpt, misspell)
make generate      # regenerate docs/CONFIG.md; CI fails when it is stale
make fuzz-short    # every fuzz target for 60 s
make check-fixtures
```

To try a build as your everyday `imux`, run `make install-local`. Never `cp` (or `go build -o`)
onto the `imux` that is running: macOS kills every process of a program whose file changes in
place (`SIGKILL (Code Signature Invalid)`), so the sidebars and the watcher die at once.
`make install-local` writes a new file and moves it over the old one, as `imux update` does.

On Windows, you can work inside WSL (end-to-end tests copy the binary to the Linux file system,
because programs under `/mnt/c` start slowly) or natively with Go and psmux (see
[docs/WINDOWS.md](docs/WINDOWS.md)).

The dependency rule is strict: the standard library plus `github.com/BurntSushi/toml` (and
`golang.org/x/sys` on Windows for console, event and file-lock calls). Do not add other modules
(test helpers included; the e2e pty helper uses raw ioctls for this reason).

Every user-facing string goes through `internal/i18n` (a test fails on a missing key). The handoff
document is model-facing and always English.

## Tests never use real data

- **Fixtures are synthetic.** Write fixtures by hand or generate them in the test, with fake paths
  under `/home/test/...`. Never copy, trim or "anonymize" a real session file, screenshot or log
  into the repository: conversations contain code, secrets and personal data.
  `scripts/check-fixtures.sh` runs in CI and rejects home-directory paths other than `/home/test`,
  real-looking API keys and tokens, and email addresses in any `testdata/` directory.
- **Tests never launch a real agent CLI** (`claude`, `agy`, `copilot`, `codex`), never use the
  network, and never read the developer's real `~/.claude`, `~/.gemini/antigravity-cli`, `~/.copilot`, `~/.codex`
  or `~/.imux`. Unit tests call `testutil.Isolate(t)`, which points `HOME`, `IMUX_HOME` and every
  agent home at temp directories. End-to-end tests use the POSIX sh fake agents in
  `e2e/fakeagents` (configured as the agents' `bin`) with a `PATH` that holds no real agent; the
  harness fails a test if an agent ever resolves elsewhere.
- **Real CLIs are checked by hand.** Tasks marked (USER) in TASKS.md need real CLIs, real
  terminals or your quota; their results go into ARCHITECTURE §20/§21 as structure only (flag
  names, key names, counts), never conversation content.

## Screenshots

The images in `docs/images` are real i-mux screens with demo agents (`scripts/screenshots`), not
mock-ups. After a UI change, regenerate them on Linux or macOS with tmux 3.4+ and python3:

```sh
make build && scripts/screenshots/shoot.sh
```

## Pull requests

- One topic per pull request, with tests in the same change; `go vet`, lint and CI green.
- Explain *why* in the description. Mention any new open question in ARCHITECTURE §20.
- Keep the watcher light: it is the only long-running process (budgets in ARCHITECTURE §17; check
  with `scripts/measure-watch.sh`).
