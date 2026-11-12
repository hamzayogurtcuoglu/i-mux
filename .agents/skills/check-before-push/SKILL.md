---
name: check-before-push
description: Runs every check CI runs, here, before i-mux code is committed, tagged or pushed (lint for Linux, Windows and macOS builds, generated files, unit tests on Linux, with macOS's symlinked temp folder and on Windows through WSL, e2e) and fixes what fails. Use after any code change, before commit-and-release, and whenever a CI run is red.
---

# Check before push

CI (`.github/workflows/ci.yml`) runs the unit tests on Linux, macOS and Windows, the e2e tests on
tmux 3.4 (Ubuntu) and 3.8 (macOS), golangci-lint for Linux and Windows, and more. Passing
`make test` on Linux is not enough: v0.49.0 passed it and failed CI on macOS (a temp path compared
with git's: macOS's `/var` is `/private/var`) and on Windows (tests expecting POSIX shell commands;
PowerShell runs them there). A red CI run after a push is a bug in the change; catch it here.

## When

- After any code change, **before the commit that will be pushed**, and always before tagging a
  release (`commit-and-release`).
- When the user reports a red CI run (see "A red CI run" below).
- Documentation-only changes need none of it.

## How

```sh
make ci
```

It runs these in order and stops at the first failure (each is a target of its own):

| Target | What it catches |
|---|---|
| `make lint-all` | golangci-lint for Linux (e2e tests too) and Windows; `go vet` for Linux, e2e, Windows and macOS builds |
| `make generate-check` | a stale `docs/CONFIG.md` (it regenerates it: review and commit it) |
| `make check-fixtures` | real paths, secrets or emails in `testdata/` |
| `make shellcheck` | the shell scripts, as CI checks them (prints that it skipped when shellcheck is missing: `pip install shellcheck-py`) |
| `make race` | the Linux unit tests with the race detector (CI's ubuntu job) |
| `make test-macos` | the unit tests with `TMPDIR` behind a symlink, as macOS's temp folder is |
| `make test-windows` | in WSL: the Windows test binaries on the Windows host (CI's Windows job: PowerShell, 8.3 temp paths, CRLF), with Git for Windows' `sh` and `git` and a throwaway user profile. Elsewhere it only vets the Windows build and says the tests did not run |
| `make size` | the 15 MB binary gate |
| `make e2e` | the end-to-end tests on this machine's tmux |

`make ci` takes about ten minutes. While iterating, run one target on the packages you changed,
then `make ci` before the commit:

```sh
make test-windows PKGS=./internal/gitops/
make test-macos PKGS="./internal/sidebar/ ./internal/cli/"
```

Not in `make ci`, run them when they apply (CI runs them):

- `make fuzz-short` after changing a parser that has a `Fuzz` test.
- `goreleaser release --snapshot --clean` after changing `.goreleaser.yml` or the release workflows.

## Rules

- **Do not commit, tag or hand a push over while `make ci` fails.** Fix the cause; never skip a
  test, loosen it until it passes, or call it flaky without proof.
- **Say what did not run.** If `make test-windows` could not run the Windows tests (no WSL), or
  shellcheck or tmux is missing, tell the user which checks only CI will do.
- **After a push** (yours or the user's): look at the CI run (`gh run list -L 3`,
  `gh run watch`, or the GitHub MCP `actions_list` / `get_job_logs` tools). A red run comes first.

## A red CI run

1. List the failed jobs and read their logs: `gh run view <id> --log-failed`, or the GitHub MCP
   tools (`actions_list` method `list_workflow_jobs`, then `get_job_logs` with `failed_only`).
   Read every failure in every job, not just the last lines.
2. Reproduce it here first: a macOS unit test with `make test-macos PKGS=...`, a Windows one with
   `make test-windows PKGS=...`, a lint failure with `make lint-all`, an e2e test with `make e2e`
   (CI's tmux may be newer than this machine's: say so if it cannot be reproduced).
3. Fix it, run `make ci`, commit (`fix:` for a bug users could hit, `test:` for a test that was
   wrong) and, when the bug shipped in a release, release a patch (`commit-and-release`).

## Tests that pass everywhere

What broke CI before, and how to write it instead:

- **Paths from git or the OS are not your paths.** `t.TempDir()` is `/var/...` on macOS while git
  answers `/private/var/...`; on Windows it is `C:\Users\RUNNER~1\...` (an 8.3 short name) while
  git answers `C:/Users/runneradmin/...`. Never compare them as strings; ask git for relative
  paths (`rev-parse --show-prefix`, `--show-cdup`) or compare what one source gave.
- **Shell commands differ.** `tmux.RunShell`, `tmux.ShJoin` and `UICommand` build POSIX sh on Unix
  and PowerShell on Windows (`& /opt/imux ... *>&1 | Out-Null; exit 0`). Build expected values
  with the same helpers, and do not assert that a command lacks `;`, `&` or quotes; check for the
  data itself instead.
- **Programs may be missing.** `sh`, `git`, `tmux` or `xdg-open` may not be on `PATH`: look them
  up and `t.Skip` when they are not.
- **Files.** Windows has no Unix permission bits (skip `0o600` checks there), file names are
  case-insensitive on macOS and Windows, and Windows tools print CRLF.
- **Homes.** Tests call `testutil.Isolate(t)`; on Windows the user profile is `USERPROFILE`,
  `APPDATA` and `LOCALAPPDATA`, which a test that writes there must set too.
