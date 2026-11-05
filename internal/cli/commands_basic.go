package cli

import (
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

func init() {
	register(&command{
		Name:    "version",
		Summary: "cmd.version",
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
	i18n.Init("")
}

// isAgentArg reports whether s names an agent (`ajan claude`).
func isAgentArg(s string) bool { return false }

// defaultAction handles `ajan` and `ajan <agent>`.
func defaultAction(e *Env, args []string) error {
	printHelp(e.Stdout)
	return nil
}
