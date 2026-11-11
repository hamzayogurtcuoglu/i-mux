//go:build windows

package pwshshim

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Run runs Windows PowerShell with args[1:] when this process was started
// as pwsh.exe, and returns its exit code and true; otherwise false.
func Run(args []string) (int, bool) {
	if len(args) == 0 || !isShimName(args[0]) {
		return 0, false
	}
	cmd := exec.Command(windowsPowerShell(), args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, true
	case errors.As(err, &ee):
		return ee.ExitCode(), true
	}
	return 1, true
}

func isShimName(arg0 string) bool {
	b := strings.ToLower(filepath.Base(arg0))
	return b == "pwsh.exe" || b == "pwsh"
}

func windowsPowerShell() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

var dir = sync.OnceValue(func() string {
	if _, err := exec.LookPath("pwsh"); err == nil {
		return "" // the real one
	}
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return ""
	}
	d := filepath.Join(base, "imux", "pwsh-shim")
	if err := install(self, filepath.Join(d, "pwsh.exe")); err != nil {
		return ""
	}
	return d
})

// Dir returns the folder holding the pwsh.exe shim, creating or refreshing
// it, or "" when pwsh is installed (or the shim cannot be written).
func Dir() string { return dir() }

// install copies self to dst unless a shim is there already: any imux build
// forwards to PowerShell the same way, and replacing a shim psmux may be
// starting at that moment would make that start fail.
func install(self, dst string) error {
	if di, err := os.Stat(dst); err == nil && di.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(self)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// A running shim cannot be replaced: move it aside first.
	_ = os.Remove(dst + ".old")
	_ = os.Rename(dst, dst+".old")
	return os.Rename(tmp, dst)
}
