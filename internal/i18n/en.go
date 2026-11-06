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

	// Command summaries
	"cmd.version":  "Print version, commit and build date.",
	"cmd.config":   "Write a commented default config, print its path, or print the effective config.",
	"cmd.handoff":  "Build a handoff from an agent session; start the target agent here, or --print it.",
	"cmd.sessions": "List agent sessions found for this directory.",
	"cmd.internal": "Internal command (used by tmux bindings and hooks).",

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
