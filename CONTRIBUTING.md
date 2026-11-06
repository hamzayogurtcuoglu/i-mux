# Contributing to ajan

Thanks for helping. ajan is small on purpose: one static Go binary, tmux for everything on screen,
and no runtime dependencies besides tmux. Read [ARCHITECTURE.md](ARCHITECTURE.md) for the design
and [TASKS.md](TASKS.md) for the plan.

## Building and testing

You need Go (see `go.mod`), tmux ≥ 3.2 and, for the full check, golangci-lint and shellcheck.

```sh
make build         # bin/ajan
make test          # unit tests
make race          # unit tests with the race detector
make e2e           # end-to-end tests against a real tmux on a private socket
make lint          # golangci-lint (vet, staticcheck, errcheck, gofumpt, misspell)
make generate      # regenerate docs/CONFIG.md; CI fails when it is stale
make fuzz-short    # every fuzz target for 60 s
make check-fixtures
```

On Windows, work inside WSL. End-to-end tests copy the binary to the Linux file system, because
programs under `/mnt/c` start slowly.

The dependency rule is strict: the standard library plus `github.com/BurntSushi/toml`. Do not add
modules (test helpers included; the e2e pty helper uses raw ioctls for this reason).

Every user-facing string goes through `internal/i18n` (a test fails on a missing key). The handoff
document is model-facing and always English.

## Tests never use real data

- **Fixtures are synthetic.** Write fixtures by hand or generate them in the test, with fake paths
  under `/home/test/...`. Never copy, trim or "anonymize" a real session file, screenshot or log
  into the repository: conversations contain code, secrets and personal data.
  `scripts/check-fixtures.sh` runs in CI and rejects home-directory paths other than `/home/test`,
  real-looking API keys and tokens, and email addresses in any `testdata/` directory.
- **Tests never launch a real agent CLI** (`claude`, `gemini`, `copilot`, `codex`), never use the
  network, and never read the developer's real `~/.claude`, `~/.gemini`, `~/.copilot`, `~/.codex`
  or `~/.ajan`. Unit tests call `testutil.Isolate(t)`, which points `HOME`, `AJAN_HOME` and every
  agent home at temp directories. End-to-end tests use the POSIX sh fake agents in
  `e2e/fakeagents` (configured as the agents' `bin`) with a `PATH` that holds no real agent; the
  harness fails a test if an agent ever resolves elsewhere.
- **Real CLIs are checked by hand.** Tasks marked (USER) in TASKS.md need real CLIs, real
  terminals or your quota; their results go into ARCHITECTURE §20/§21 as structure only (flag
  names, key names, counts), never conversation content.

## Pull requests

- One topic per pull request, with tests in the same change; `go vet`, lint and CI green.
- Explain *why* in the description. Mention any new open question in ARCHITECTURE §20.
- Keep the watcher light: it is the only long-running process (budgets in ARCHITECTURE §17; check
  with `scripts/measure-watch.sh`).
