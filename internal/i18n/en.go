package i18n

// en is the reference catalog. Keys are grouped by area.
var en = map[string]string{
	// CLI
	"cli.unknown_command": "unknown command %q",
	"cli.help.header":     "ajan: continue an AI coding conversation in another agent CLI, inside tmux.",
	"cli.help.usage":      "Usage:",
	"cli.help.commands":   "Commands:",
	"cli.help.footer":     "Run 'ajan help <command>' for details.",
	"cli.warning":         "warning: %s",
	"cli.unknown_agent":   "%s: unknown agent %q (known: %s)",
	"cli.bad_reason":      "--reason must be manual or limit, not %q",
	"cli.usage":           "usage: ajan %s",
	"cli.no_pane":         "not inside an ajan pane: pass --pane %N",

	// Command summaries
	"cmd.version":  "Print version, commit and build date.",
	"cmd.config":   "Write a commented default config, print its path, or print the effective config.",
	"cmd.handoff":  "Build a handoff from an agent session; start the target agent here, or --print it.",
	"cmd.sessions": "List agent sessions found for this directory.",
	"cmd.internal": "Internal command (used by tmux bindings and hooks).",
	"cmd.attach":   "Attach to an ajan session (default: the most recently used one).",
	"cmd.ls":       "List ajan sessions and their panes.",
	"cmd.switch":   "Continue this pane's conversation in another agent (with context).",
	"cmd.split":    "Open a new pane with an agent in the same directory.",
	"cmd.pick":     "Open the session picker on the attached client.",

	// workspace: start / attach / ls
	"start.no_agent":      "no agent CLI is installed. Install one of:\n%s",
	"start.window":        "opened a new window (%s)",
	"start.created":       "created session %s (pane %s)",
	"start.exists":        "session %s already exists",
	"attach.none":         "no ajan session is running; start one with 'ajan'",
	"attach.no_such":      "no ajan session named %q",
	"ls.none":             "no ajan sessions",
	"ls.header":           "SESSION\tPANE\tAGENT\tSTATE\tCWD",
	"ls.attached":         "attached",
	"ls.detached":         "detached",
	"ls.dead":             "exited",
	"switch.done":         "Switched to %s · %s chars of context",
	"switch.fresh":        "Started %s (no earlier conversation in this pane)",
	"switch.resumed":      "Resumed the %s conversation",
	"switch.warnings":     "%d parser warnings",
	"switch.busy":         "A switch of this pane is already in progress",
	"switch.failed":       "Could not switch to %s: %s",
	"menu.title":          "ajan · %s",
	"menu.shell":          "shell",
	"menu.limited_title":  "%s: limited until %s",
	"menu.switch":         "Switch to %s (with context)",
	"menu.start":          "Start %s here",
	"menu.not_installed":  "(not installed)",
	"menu.limited_suffix": "(limited until %s)",
	"menu.sessions":       "Sessions…",
	"menu.new_pane":       "New pane → %s",
	"menu.preview":        "Preview handoff",
	"menu.limits":         "Limits",
	"menu.limits_soon":    "The limits panel is coming soon",
	"menu.close":          "Close pane",
	"bar.sessions":        "Sessions",

	// session picker
	"picker.title":             "Sessions · %s",
	"picker.no_preview":        "(no text)",
	"picker.more":              "More…",
	"picker.none":              "No conversations found for %s",
	"picker.session_title":     "%s · %s",
	"picker.resume":            "Continue in %s (resume)",
	"picker.continue":          "Continue in %s (with context)",
	"picker.resume_new_pane":   "New pane: %s (resume)",
	"picker.continue_new_pane": "New pane: %s (with context)",
	"click.limited_title":      "%s is limited until %s. Switch anyway?",
	"click.switch_anyway":      "Switch to %s",
	"click.cancel":             "Cancel",
	"died.exited":              "%s exited (status %s)",
	"died.handoff":             "handoff: %s",
	"died.doctor":              "run 'ajan doctor'",
	"died.fallback":            "Resume failed; started a new %s session with the full handoff",
	"pick.no_client":           "no client is attached to this ajan session",
	"tmux.missing":             "tmux is not installed (brew install tmux / apt install tmux)",
	"tmux.too_old":             "tmux %s is too old; ajan needs %s or newer",

	// config command
	"config.exists":      "%s already exists (use --force to overwrite)",
	"config.wrote":       "wrote %s",
	"config.unknown_sub": "config: unknown subcommand %q (init, path, show)",

	// handoff / sessions
	"handoff.no_session":     "no agent session found for %s",
	"handoff.saved":          "handoff saved to %s (%s chars)",
	"handoff.no_prompt_flag": "this agent has no initial-prompt flag; paste the handoff from %s",
	"sessions.none":          "no sessions found for %s",
	"sessions.header":        "AGENT\tID\tUPDATED\tSIZE\tFIRST REQUEST",

	// agents
	"agent.not_installed": "%s is not installed: %s",

	// notices
	"notice.cross_provider": "Note: switching sends excerpts of your %s conversation and repository state to %s. Secrets are redacted on a best-effort basis. (shown once)",

	// relative times
	"time.just_now":  "just now",
	"time.min_ago":   "%d min ago",
	"time.hours_ago": "%d h ago",
	"time.yesterday": "yesterday",
	"time.days_ago":  "%d days ago",
}
