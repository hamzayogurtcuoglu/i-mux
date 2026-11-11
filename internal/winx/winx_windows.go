//go:build windows

package winx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

func hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// WindowsPowerShell is powershell.exe (5.1) in System32: it is on every
// Windows, and unlike pwsh it reaches the Windows Runtime (toasts).
func WindowsPowerShell() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

// GitRoot is the folder Git for Windows is installed in (the one with
// usr\bin), found from git on the PATH, else the usual places; "" when it
// is not installed.
func GitRoot() string { return gitRoot() }

var gitRoot = sync.OnceValue(func() string {
	var try []string
	if git, err := exec.LookPath("git"); err == nil {
		// <root>\cmd\git.exe, <root>\bin\git.exe or <root>\mingw64\bin\git.exe
		d := filepath.Dir(git)
		try = append(try, filepath.Dir(d), filepath.Dir(filepath.Dir(d)))
	}
	for _, env := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)"} {
		if p := os.Getenv(env); p != "" {
			try = append(try, filepath.Join(p, "Git"))
		}
	}
	if p := os.Getenv("LOCALAPPDATA"); p != "" {
		try = append(try, filepath.Join(p, "Programs", "Git"))
	}
	for _, root := range try {
		if isFile(filepath.Join(root, "usr", "bin", "bash.exe")) {
			return root
		}
	}
	return ""
})

// GitTool is name (less, vim, nano ...) in Git for Windows' usr\bin, ""
// when Git or the program is not there.
func GitTool(name string) string {
	root := GitRoot()
	if root == "" || name == "" || strings.ContainsAny(name, `/\`) {
		return ""
	}
	if !strings.EqualFold(filepath.Ext(name), ".exe") {
		name += ".exe"
	}
	if p := filepath.Join(root, "usr", "bin", name); isFile(p) {
		return p
	}
	return ""
}

// GitBash is the bash Claude Code runs its commands with on Windows:
// CLAUDE_CODE_GIT_BASH_PATH, else Git for Windows' bin\bash.exe; "" when
// there is none.
func GitBash() string {
	if p := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); p != "" && isFile(p) {
		return p
	}
	root := GitRoot()
	if root == "" {
		return ""
	}
	for _, p := range []string{filepath.Join(root, "bin", "bash.exe"), filepath.Join(root, "usr", "bin", "bash.exe")} {
		if isFile(p) {
			return p
		}
	}
	return ""
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
