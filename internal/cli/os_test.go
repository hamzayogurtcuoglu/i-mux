package cli

import (
	"encoding/json"
	"runtime"
)

// jsonPath is p escaped for the inside of a JSON string (Windows paths have
// backslashes).
func jsonPath(p string) string {
	b, _ := json.Marshal(p)
	return string(b[1 : len(b)-1])
}

// unixModes reports whether files keep Unix permission bits (Windows keeps
// none: the profile is private).
var unixModes = runtime.GOOS != "windows"
