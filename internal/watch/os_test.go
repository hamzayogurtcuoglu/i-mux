package watch

import (
	"runtime"
	"strings"
)

// plat turns an expected shell command written for sh into what imux writes
// on this OS: on Windows menu items run PowerShell, which calls imux with &
// and silences it with *>&1 | Out-Null; exit 0 (see tmux.Silent).
func plat(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}
	return strings.NewReplacer(
		"'/opt/imux ", "'& /opt/imux ",
		" >/dev/null 2>&1 || true", " *>&1 | Out-Null; exit 0",
	).Replace(s)
}

// unixModes reports whether files keep Unix permission bits (Windows keeps
// none: the profile is private).
var unixModes = runtime.GOOS != "windows"
