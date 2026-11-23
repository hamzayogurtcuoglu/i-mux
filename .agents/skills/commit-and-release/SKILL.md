---
name: commit-and-release
description: Commits changes in English without AI co-author trailers, updates CHANGELOG.md, tags a vX.Y.Z release, and pushes to trigger GoReleaser. Use when asked to commit, tag, cut a release, or publish a new version of i-mux.
---

# Commit and release

## Hard rules

- **English only:** Every commit subject, commit body, tag message, `CHANGELOG.md` entry, `ARCHITECTURE.md` section, and `TASKS.md` item must be written in English.
- **No AI co-authors:** **Never** add `Co-Authored-By:` lines (or any trailer/mention) for Claude, Antigravity, Gemini, Copilot, Codex, OpenCode, or any other AI agent. Commit strictly as the repository's configured Git author.
- **Safe local binary install:** Never run `cp bin/imux ~/.local/bin/imux` or `go build -o ~/.local/bin/imux`. Always use `make install-local` so macOS does not kill running `imux` processes (`SIGKILL: Code Signature Invalid`).

## Commit format

Follow the repository's commit history style:

1. **Docs commit first** (when adding or updating a design/milestone in `ARCHITECTURE.md` and `TASKS.md`):
   ```text
   docs: <Sentence-case summary> (M<N>)
   ```
   Example: `docs: Sidebars come back after imux is killed (M41)`

2. **Code and release commit:**
   ```text
   <type>: <Sentence-case summary under 72 chars>

   <Concise English explanation of why the change was made and what it does,
   wrapped at ~72 columns. No Co-Authored-By trailers.>
   ```
   - Allowed `<type>` prefixes: `feat`, `fix`, `ci`, `docs`, `refactor`, `perf`, `test`, `e2e`.
   - Capitalize the first word after `<type>: ` (e.g., `feat: Project skills in the sidebar`, `fix: Sidebars come back after imux was killed`).

## Pre-commit checks

Run every check CI runs, here, before creating the code commit and again before tagging (the
`check-before-push` skill, `.agents/skills/check-before-push/SKILL.md`):

```sh
make ci
```

It covers Linux and macOS (a symlinked `TMPDIR`), lint, generated files (`docs/CONFIG.md`),
fixtures and e2e. **Do not commit, tag or push while it
fails**, and tell the user which checks could not run on this machine.

## Release steps

0. **Start from the newest `main` (D95).** Releases are cut from more than one machine; a
   checkout that missed one would tag an old tree or reuse a version number:
   ```sh
   git fetch origin --tags
   git status -sb                           # must not say "behind"
   git merge-base --is-ancestor origin/main HEAD && echo ok   # HEAD contains origin/main
   ```
   If HEAD is behind, `git pull --ff-only` (rebase local commits on it) and run `make ci` again.

1. **Choose the next version (`v0.Y.Z`):**
   - Check the latest tag, on origin too (the fetch above brought its tags):
     ```sh
     git tag --sort=-version:refname | head -n 5
     git ls-remote --tags origin | sed 's|.*refs/tags/||; s|\^{}||' | sort -uV | tail -n 3
     ```
   - The new tag must be above every one of them.
   - Bump minor (`v0.Y.0`) for `feat:` releases; bump patch (`v0.Y.Z`) for `fix:` or `ci:` releases.

2. **Update `CHANGELOG.md` (and `TASKS.md` / `README*.md` if applicable):**
   - Under `## [Unreleased]`, add `## [<0.Y.Z>] - YYYY-MM-DD` (no `v` prefix in the heading, e.g., `## [0.36.2] - 2026-11-09`) with `### Added`, `### Changed`, or `### Fixed` bullets in English.
   - Mark completed tasks `[x]` in `TASKS.md`.

3. **Commit (without `Co-Authored-By`):**
   - Stage only the intended files (`git status`, `git diff --staged`).
   - Commit in English and verify with `git show -s --format=fuller HEAD` that no `Co-Authored-By` trailer is present.

4. **Tag in `v0.Y.Z` format and push:**
   ```sh
   git tag v0.Y.Z
   git push origin main v0.Y.Z
   ```
   Pushing `v0.Y.Z` triggers `.github/workflows/release.yml` (GoReleaser archives, Homebrew tap update, and `release-smoke.yml`).

5. **Update local binary (optional):**
   ```sh
   make install-local
   ```
   It refuses to install from a checkout behind its upstream (`scripts/fresh-check.sh`). On a
   machine that only uses i-mux, `imux update` installs the release itself: a development build
   (`v0.65.0-3-g…`) is offered every release above its tag.
