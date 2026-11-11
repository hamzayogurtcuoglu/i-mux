package versions

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// PsmuxID is psmux's winget package.
const PsmuxID = "marlocarlo.psmux"

// PsmuxURL is where psmux is when no package manager has it.
const PsmuxURL = "https://github.com/marlocarlo/psmux"

// psmuxRow is the multiplexer's row on Windows: psmux, from winget (or
// scoop, cargo). Windows cannot replace a running psmux.exe, so its update
// is shown to run outside i-mux (after its server quits), not run from the
// popup, which itself runs inside psmux.
func psmuxRow(ctx context.Context, e Env) Row {
	r := Row{Key: "t", Name: "psmux", Tmux: true}
	bin := e.Mux
	if bin == "" {
		bin = "psmux"
	}
	path, err := e.LookPath(bin)
	if err != nil {
		r.Status, r.Note = Missing, i18n.T("versions.tmux_missing")
		if _, err := e.LookPath("winget"); err == nil {
			r.Fix = []string{"winget", "install", "--id", PsmuxID, "-e"}
			r.Show = shellText(r.Fix)
		} else {
			r.Note += "\n" + i18n.T("versions.psmux_manual", PsmuxURL)
		}
		return r
	}
	v, ok := tmux.ParseVersion(e.Output(ctx, path, "-V"))
	switch {
	case !ok:
		r.Status, r.Note = Unknown, i18n.T("versions.unknown")
	case !v.AtLeast(tmux.MinVersion[0], tmux.MinVersion[1]):
		r.Version, r.Status, r.Note = v.String(), Old, i18n.T("versions.tmux_too_old")
	default:
		r.Version = v.String()
	}
	up := psmuxUpdate(e, path)
	if up == "" {
		r.Note = lines(r.Note, i18n.T("versions.psmux_manual", PsmuxURL))
		return r
	}
	r.Show = "psmux -L " + tmux.Socket() + " kill-server; " + up
	r.Note = lines(r.Note, i18n.T("versions.psmux_closed"))
	return r
}

// psmuxUpdate is the command that updates psmux at path, by where it was
// installed from ("" when none is known).
func psmuxUpdate(e Env, path string) string {
	p := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	switch {
	case strings.Contains(p, `\scoop\`):
		return "scoop update psmux"
	case strings.Contains(p, `\.cargo\`):
		return "cargo install psmux"
	}
	if _, err := e.LookPath("winget"); err == nil {
		return shellText([]string{"winget", "upgrade", "--id", PsmuxID, "-e"})
	}
	return ""
}

// lines joins non-empty note lines.
func lines(a, b string) string {
	if a == "" {
		return b
	}
	return a + "\n" + b
}
