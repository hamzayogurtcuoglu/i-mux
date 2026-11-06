# `imux limits`

`imux limits` shows, for each agent, the usage limit i-mux last saw on its screen and the usage it
can count from local session files. It reads local files only and makes no network calls.
Inside the workspace the pane menu's **Limits** item opens it in a popup (`imux limits --pager`).

```
AGENT    STATUS                 USAGE (local data)
Claude   ⛔ limited until 15:00  last 5 h: 42 replies · 310.5K in / 28.1K out · 12.4M cache read
                                "5-hour limit reached ∙ resets 3pm" (seen 12 min ago)
Gemini   ● ok                   today: 8 requests · 41.2K in / 3.9K out
Copilot  not installed          this month: 0 premium requests
Codex    ◐ approaching a limit  5 h window 81% used (resets 15:40) · weekly 23% used (resets Nov 8 09:00)

Usage is counted from local session files. For exact plan percentages, see CodexBar (brew install --cask codexbar).
```

| Agent | Usage shown | Source |
|---|---|---|
| Claude | replies and tokens in the last 5 hours | `message.usage` of recently modified session files, read backwards, deduplicated by `message.id` |
| Gemini | requests (Gemini replies) and tokens today, local time | chats in every project modified today |
| Copilot | premium requests this calendar month | the latest `session.shutdown` / `session.usage_checkpoint` total of each session |
| Codex | the newest rate-limit percentages and reset times | `token_count` events with `rate_limits` (newer Codex versions only; otherwise "n/a") |

## JSON (`imux limits --json`)

Field names are stable; new fields may be added.

```json
{
  "generated_at": "2026-11-05T14:32:00Z",
  "agents": [
    {
      "agent": "claude",
      "name": "Claude Code",
      "installed": true,
      "status": "limited",
      "limit": {
        "at": "2026-11-05T12:01:07Z",
        "text": "5-hour limit reached ∙ resets 3pm",
        "reset_at": "2026-11-05T15:00:00+03:00",
        "expires_at": "2026-11-05T15:00:00+03:00",
        "source": "screen",
        "pane": "%1",
        "severity": "limit"
      },
      "usage": {
        "window": "5h",
        "from": "2026-11-05T09:32:00Z",
        "replies": 42,
        "input_tokens": 310500,
        "output_tokens": 28100,
        "cache_read_tokens": 12400000,
        "cache_write_tokens": 81000,
        "sessions": 3
      }
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `agents[]` | one entry per agent, in the configured `order` |
| `agent`, `name`, `installed` | id (`claude`, `gemini`, `copilot`, `codex`), product name, whether its CLI was found |
| `status` | `limited` (a limit was seen and has not expired), `warn` (an "approaching" message was seen) or `ok` |
| `limit` | present when `status` is not `ok`: the record from `~/.imux/state.json` (`reset_at` is omitted when the message had no reset time; `expires_at` is then `at` + `limits.default_expiry`) |
| `usage.window` | `5h` (Claude), `today` (Gemini), `month` (Copilot), `latest` (Codex) |
| `usage.from` | start of the window (local midnight for `today`, the first of the month for `month`) |
| `usage.replies` | assistant replies (Claude: unique `message.id`s; Gemini: requests) |
| `usage.*_tokens` | token counts; zero values are omitted |
| `usage.premium_requests` | Copilot premium requests |
| `usage.sessions` | session files that contributed |
| `usage.rate_limits` | Codex only: `{"At", "PrimaryUsedPercent", "PrimaryResetsAt", "SecondaryUsedPercent", "SecondaryResetsAt"}`; absent when Codex does not write rate limits |
