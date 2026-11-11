//go:build !windows

package editor

import (
	"os"
	"path/filepath"
)

// candidates are the editors Choose tries, in order.
func candidates(configured string) []string {
	return []string{configured, "micro", os.Getenv("VISUAL"), os.Getenv("EDITOR"), "nano", "vim", "vi"}
}

// progName is the editor's name as Command knows it.
func progName(path string) string { return filepath.Base(path) }

// platformCommand is an editor only this platform has (none on Unix).
func platformCommand(string, []string, string, int) ([]string, bool) { return nil, false }

// vscodePaths are where VS Code's command line tool is when it is not on the
// PATH: inside the macOS app.
func vscodePaths() []string {
	return []string{
		"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		filepath.Join(os.Getenv("HOME"), "Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"),
	}
}
