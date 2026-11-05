// Package version holds build information injected with -ldflags.
package version

import "runtime/debug"

// Set at build time:
//
//	-ldflags "-X github.com/hamzayogurtcuoglu/i-mux/internal/version.Version=v0.1.0
//	          -X github.com/hamzayogurtcuoglu/i-mux/internal/version.Commit=abc123
//	          -X github.com/hamzayogurtcuoglu/i-mux/internal/version.Date=2026-11-05T12:00:00Z"
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Info returns version, commit and date, falling back to the module build info
// when the binary was built without ldflags (e.g. `go install`).
func Info() (ver, commit, date string) {
	ver, commit, date = Version, Commit, Date
	if commit != "" && date != "" {
		return
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if ver == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		ver = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "" {
				commit = s.Value
				if len(commit) > 12 {
					commit = commit[:12]
				}
			}
		case "vcs.time":
			if date == "" {
				date = s.Value
			}
		}
	}
	return
}
