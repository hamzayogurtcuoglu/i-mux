//go:build e2e && windows

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// exeSuffix is a program file's suffix.
const exeSuffix = ".exe"

// noTerminal: a Windows program asks no terminal it was not given.
func noTerminal() *syscall.SysProcAttr { return nil }

// maxRSS: Windows reports no peak memory of a finished program here.
func maxRSS(*os.ProcessState) int64 { return 0 }

// killPid kills process pid at once (TerminateProcess).
func killPid(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// executable reports whether st is a program file: Windows has no execute
// bit, a program is an .exe.
func executable(st os.FileInfo) bool { return strings.EqualFold(filepath.Ext(st.Name()), ".exe") }

// posixShell is the sh of the panes and of the fake agents' scripts: Git for
// Windows' (IMUX_E2E_SH picks another), as in i-mux-core's suite.
func posixShell() string {
	if sh := os.Getenv("IMUX_E2E_SH"); sh != "" {
		return sh
	}
	return `C:\Program Files\Git\usr\bin\sh.exe`
}

// basePath is the suite's PATH: the engine, sh and its tools, and Windows'
// own programs; never a real agent CLI.
func basePath(engine string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return strings.Join([]string{
		filepath.Dir(engine), filepath.Dir(posixShell()), gitDir(),
		filepath.Join(root, "System32"), root, filepath.Join(root, "System32", "WindowsPowerShell", "v1.0"),
		filepath.Join(os.Getenv("ProgramFiles"), "PowerShell", "7"), // pwsh, the panes' shell when there
	}, ";")
}

// stageFakes builds the Go fake agent (fake.sh's port) into dir once per
// agent name, and as nvim.exe: a script cannot be an agent's program here.
func stageFakes(dir string) error {
	first := filepath.Join(dir, "fake-claude.exe")
	if out, err := exec.Command("go", "build", "-o", first, "./fakeagents/fakeagent").CombinedOutput(); err != nil {
		return fmt.Errorf("build the fake agent: %w\n%s", err, out)
	}
	for _, name := range []string{"fake-antigravity", "fake-copilot", "fake-codex", "fake-opencode", "nvim"} {
		if err := copyFile(first, filepath.Join(dir, name+".exe"), 0o755); err != nil {
			return err
		}
	}
	return nil
}

// skipOnSystem skips a test listed in windowsSkips, with its reason.
func skipOnSystem(t *testing.T) {
	t.Helper()
	name, _, _ := strings.Cut(t.Name(), "/")
	if why, ok := windowsSkips[name]; ok {
		t.Skip("not on Windows: " + why)
	}
}

// gitDir is Git for Windows' cmd folder, where git.exe is: two up from its sh
// (Git\usr\bin\sh.exe), as the installer lays it out.
func gitDir() string {
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(posixShell()))), "cmd")
}

// binRunning reports whether a process runs the program at path: Windows
// has no pgrep, so a snapshot of the processes and their images.
func binRunning(path string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err := windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		ok := windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil
		_ = windows.CloseHandle(h)
		if ok && strings.EqualFold(windows.UTF16ToString(buf[:n]), path) {
			return true
		}
	}
	return false
}
