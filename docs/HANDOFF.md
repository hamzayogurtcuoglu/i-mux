# The handoff document

When a pane switches from one agent to another, i-mux condenses the conversation into a markdown
**handoff** and gives it to the new agent as its first prompt. It is model-facing, always in
English, and built only from local files: the source agent's session file and `git` output.

- Saved to `~/.imux/handoffs/<YYYYMMDD-HHMMSS>-<from>-to-<to>.md` (mode 0600); files older than
  `handoff.keep_days` (30) are deleted.
- Passed in argv (`handoff.inject = "argv"`, the default) or pasted into the agent after it
  started (`"paste"`, which keeps it out of `ps`).
- Preview without switching: `imux handoff --print` (newest session in this directory), or
  **Preview handoff** in the pane menu.

## Layout

| Part | Content | Size |
|---|---|---|
| Marker (line 1) | `[i-mux handoff · <From CLI> → <To CLI> · <YYYY-MM-DD HH:MM>]` | fixed |
| Preamble | who the user was working with, why they switched ("hit its usage limit (resets 15:00)" or "switched manually"), how to treat the record | fixed |
| `## Original request` | the conversation's first user request | ≤ 12 % of the budget, middle-truncated |
| `## Earlier history` | only for a chained handoff: the previous handoff's history and conversation | ≤ 25 %, newest part kept |
| `## Current task list` | the latest todo / plan snapshot: `[x]` done, `[~]` in progress, `[ ]` open | ≤ 2K |
| `## Files touched` | edited, created and read files (relative to the project) | ≤ 1.5K |
| `## Repository state` | cwd, branch, `git status --short` (≤ 40 lines), `git diff --stat` (≤ 25 lines) | ≤ 2.5K |
| `## Conversation` | user and assistant messages, oldest first, with one-line tool summaries (`$ npm test`, `edited src/a.ts`); filled newest-first until the budget is used | everything left |
| `## Where it stopped` | the last user request and the last assistant message | ≤ 3K + 4K |
| `## What to do now` | continue the unfinished request, or give a short status and wait | fixed |
| Source path | the previous session's transcript file (`include_source_path`) | fixed |

Rules:

- The budget is `handoff.budget_chars` (30,000 characters). Long messages are middle-truncated
  (60 % head, 40 % tail, cut on character boundaries) with a `[… N chars omitted …]` marker.
- In argv mode the document also stays under `handoff.max_arg_bytes` (100 KiB; Linux limits a
  single argument to 128 KiB).
- Left out: thinking blocks, API error messages, the limit message itself, sidechains, meta lines.
- Secrets are redacted on a best-effort basis (`handoff.redact_secrets`): private keys, AWS key
  ids, GitHub, OpenAI, Anthropic, Google and Slack tokens, `Authorization: Bearer` headers become
  `[REDACTED:<kind>]`.
- **Chained handoffs.** When the source conversation itself started from a handoff, its original
  request is carried over and its history goes into `## Earlier history`; the old handoff is not
  shown as a user message. When reading a handoff back, only the section headings above split it:
  a `## ` line in the user's own text (a pasted brief) stays inside its section, so the original
  request survives every hop whole.
- **No session file.** If the source agent's session cannot be found, the pane's terminal
  scrollback is used instead and the record says it was reconstructed.

## Example (synthetic)

```markdown
[i-mux handoff · Claude Code → Antigravity CLI · 2026-11-05 14:32]

You are continuing a coding session that the user was running in Claude Code (model: claude-opus-4-1).
They switched to you because Claude Code hit its usage limit (resets 15:00).
- Treat the record below as your own memory of this session.
- Do not redo finished work. Files may have changed since; re-read a file before editing it.
- Tool names in the record (Read, Bash, Edit, ...) belong to the previous agent. Use your own tools.
- Reply in the language the user writes in.

## Original request
fix the login redirect bug

## Current task list
- [x] find the bug
- [~] add a test (in progress)
- [ ] update docs

## Files touched
Edited: src/auth.ts · Read: 1 file (src/auth.ts)

## Repository state
cwd: /home/test/proj · branch: feature/login
git status --short (2 lines):
 M src/auth.ts
?? src/new.ts
git diff --stat:
 src/auth.ts | 4 ++--
 1 file changed, 2 insertions(+), 2 deletions(-)

## Conversation (condensed, oldest first)
**User:** fix the login redirect bug
**Assistant:** Looking at the auth handler.
  · read src/auth.ts
  · $ npm test
  · edited src/auth.ts
**Assistant:** The redirect now keeps the `next` parameter. Tests pass.
**User:** great, also add a test for it. Token: [REDACTED:github-token]

## Where it stopped
Last user request: great, also add a test for it. Token: [REDACTED:github-token]
Last assistant message: The redirect now keeps the `next` parameter. Tests pass.

## What to do now
If the last request is not finished, continue it. Otherwise give a 2–3 sentence status
and wait for the user.

Full transcript of the previous session (read only if needed): /home/test/.claude/projects/-home-test-proj/s-short.jsonl
```

## Delta handoff (switching back)

Switching a pane back to an agent that already ran there resumes that agent's own session
natively (for example `claude --resume <id> "<delta>"`) instead of starting over. The **delta**
only covers what happened in the other agents since, so nothing is repeated:

- the preamble becomes "Welcome back. While you were away the user continued in Antigravity CLI, then
  Codex CLI; here is what happened there.";
- it keeps `## Files touched`, `## Repository state`, `## Conversation (while you were away)` with
  `*(in Antigravity CLI)*` markers, `## Where it stopped` and `## What to do now`;
- with nothing new, the agent resumes without a prompt.

If the resume fails within 10 seconds (for example the session was deleted), i-mux starts a new
session with the full handoff instead. `imux switch --no-resume` or `resume = false` for an agent
always start a new session.
