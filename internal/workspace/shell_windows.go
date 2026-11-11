//go:build windows

package workspace

import (
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// pagedCmd is the popup command that shows what the imux command argv
// prints, errors too, in less: imux __page runs it and pages it itself, as
// a PowerShell pipe would re-encode the UTF-8 document (argv[0] is imux).
func pagedCmd(argv []string) string {
	return tmux.ShJoin(append([]string{argv[0], "__page"}, argv[1:]...)...)
}

// statuslineCmd is the status line command Claude Code runs: on Windows it
// runs it with Git Bash, so the path has forward slashes and bash's double
// quotes.
func statuslineCmd(self string) string {
	p := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace(filepath.ToSlash(self))
	return `"` + p + `" __statusline claude`
}
