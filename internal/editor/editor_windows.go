//go:build windows

package editor

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// candidates are the editors Choose tries, in order: as on Unix, with Neovim
// and Microsoft Edit (in Windows 11) before Git for Windows' nano and vim.
func candidates(configured string) []string {
	return []string{configured, "micro", os.Getenv("VISUAL"), os.Getenv("EDITOR"), "nvim", "edit", "nano", "vim"}
}

// progName is the editor's name as Command knows it: without the folder,
// the extension (.exe, .cmd) and case.
func progName(path string) string {
	b := strings.ToLower(filepath.Base(path))
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// platformCommand opens path in Microsoft Edit, which takes the line as
// FILE:LINE.
func platformCommand(name string, argv []string, path string, line int) ([]string, bool) {
	if name != "edit" {
		return nil, false
	}
	if line > 0 {
		path += ":" + strconv.Itoa(line)
	}
	return append(argv, path), true
}

// vscodePaths are where VS Code's command line tool is when it is not on the
// PATH: the user's and the machine's install folders.
func vscodePaths() []string {
	var ps []string
	for _, base := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"), os.Getenv("ProgramFiles")} {
		if base != "" && base != "Programs" {
			ps = append(ps, filepath.Join(base, "Microsoft VS Code", "bin", "code.cmd"))
		}
	}
	return ps
}
