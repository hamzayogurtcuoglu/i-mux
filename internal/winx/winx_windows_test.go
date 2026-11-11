//go:build windows

package winx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHidden(t *testing.T) {
	c := Hidden(exec.Command("powershell.exe"))
	if c.SysProcAttr == nil || !c.SysProcAttr.HideWindow || c.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("%+v", c.SysProcAttr)
	}
}

func TestWindowsPowerShell(t *testing.T) {
	t.Setenv("SystemRoot", `D:\Win`)
	if p := WindowsPowerShell(); p != `D:\Win\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Fatal(p)
	}
}

// Git's tools are found where Git for Windows is installed; names with a
// folder are not looked up.
func TestGitTools(t *testing.T) {
	if GitTool(`..\x`) != "" || GitTool("a/b") != "" || GitTool("") != "" {
		t.Fatal("a name with a folder")
	}
	root := GitRoot()
	if root == "" {
		t.Skip("Git for Windows is not installed here")
	}
	if p := GitTool("less"); !strings.EqualFold(p, filepath.Join(root, "usr", "bin", "less.exe")) {
		t.Fatalf("less %q (root %q)", p, root)
	}
	if GitTool("no-such-tool") != "" {
		t.Fatal("missing tool found")
	}
	if b := GitBash(); b == "" {
		t.Fatal("no bash with Git")
	}
	// Claude Code's own setting wins.
	f := filepath.Join(t.TempDir(), "bash.exe")
	if err := os.WriteFile(f, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CODE_GIT_BASH_PATH", f)
	if b := GitBash(); b != f {
		t.Fatalf("CLAUDE_CODE_GIT_BASH_PATH: %q", b)
	}
}
