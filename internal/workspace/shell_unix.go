//go:build !windows

package workspace

import "github.com/hamzayogurtcuoglu/i-mux/internal/tmux"

// pagedCmd is the popup command that shows what the imux command argv
// prints, errors too, in less. LESSCHARSET: the document is UTF-8 whatever
// the popup's locale (under a C locale less shows "·" as <C2><B7>).
func pagedCmd(argv []string) string {
	return tmux.ShJoin(argv...) + " 2>&1 | LESSCHARSET=utf-8 less -R"
}

// statuslineCmd is the status line command Claude Code runs with sh.
func statuslineCmd(self string) string { return tmux.ShQuote(self) + " __statusline claude" }
