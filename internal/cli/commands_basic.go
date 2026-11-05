package cli

import (
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

func init() {
	register(&command{
		Name:    "version",
		Summary: "Print version, commit and build date.",
		Run: func(e *Env, args []string) error {
			v, c, d := version.Info()
			if c == "" {
				c = "unknown"
			}
			if d == "" {
				d = "unknown"
			}
			fmt.Fprintf(e.Stdout, "ajan %s (commit %s, built %s)\n", v, c, d)
			return nil
		},
	})
}

// setup runs once per process before dispatch.
func setup() {
	paths.SetUmask()
}

// isAgentArg reports whether s names an agent (`ajan claude`).
func isAgentArg(s string) bool { return false }

// defaultAction handles `ajan` and `ajan <agent>`.
func defaultAction(e *Env, args []string) error {
	printHelp(e.Stdout)
	return nil
}

var messages = map[string]string{
	"cli.unknown_command": "unknown command %q",
	"cli.help.header":     "ajan: continue an AI coding conversation in another agent CLI, inside tmux.",
	"cli.help.usage":      "Usage:",
	"cli.help.commands":   "Commands:",
	"cli.help.footer":     "Run 'ajan help <command>' for details.",
}

func tr(key string, args ...any) string {
	f, ok := messages[key]
	if !ok {
		return key
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}
