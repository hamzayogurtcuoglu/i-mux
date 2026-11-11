package workspace

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
)

// Fake agent binaries. launch takes only absolute paths, and on Windows an
// absolute path has a drive.
var (
	optClaude = absPath("/opt/bin/claude")
	optCodex  = absPath("/opt/bin/codex")
	optAgy    = absPath("/opt/bin/agy")
)

// absPath is the Unix absolute path p as an absolute path on this OS.
func absPath(p string) string {
	if runtime.GOOS == "windows" {
		return `C:` + filepath.FromSlash(p)
	}
	return p
}

// plat turns an expected shell command written for sh into what imux writes
// on this OS: on Windows menu items run PowerShell, which calls imux with &
// and silences it with *>&1 | Out-Null; exit 0 (see tmux.Silent).
func plat(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}
	return strings.NewReplacer(
		"'/opt/imux/bin/imux ", "'& /opt/imux/bin/imux ",
		" >/dev/null 2>&1 || true", " *>&1 | Out-Null; exit 0",
	).Replace(s)
}

// jsonPath is p escaped for the inside of a JSON string (Windows paths have
// backslashes).
func jsonPath(p string) string {
	b, _ := json.Marshal(p)
	return string(b[1 : len(b)-1])
}
