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

	// Command summaries
	"cmd.version": "Print version, commit and build date.",
	"cmd.config":  "Write a commented default config, print its path, or print the effective config.",

	// config command
	"config.exists":      "%s already exists (use --force to overwrite)",
	"config.wrote":       "wrote %s",
	"config.unknown_sub": "config: unknown subcommand %q (init, path, show)",
}
