//go:build windows

package envfile

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
)

// The apply files run in Windows PowerShell and cmd.exe as they are typed into a pane: the
// values arrive as they are, the old key goes, and the file deletes itself.
func TestScriptRunsOnWindows(t *testing.T) {
	t.Setenv("IMUX_HOME", t.TempDir())
	value := `it's "quoted" 100% & <done> ^| $HOME`
	e := &Env{Vars: []Var{{"ENVTEST_A", value}, {"ENVTEST_B", "b"}}}
	for _, c := range []struct {
		sh    Shell
		argv  func(line string) []string
		print string
	}{
		{ShellPowerShell, func(line string) []string {
			return []string{
				"powershell.exe", "-NoLogo", "-NoProfile", "-Command",
				line + `; [Console]::Write($env:ENVTEST_A + '|' + $env:ENVTEST_B + '|' + [string]($null -eq $env:ENVTEST_OLD))`,
			}
		}, value + "|b|True"},
		{ShellCmd, func(line string) []string {
			// The line as typed: the command line given as it is, which Go would otherwise quote.
			return []string{"cmd.exe", `cmd.exe /d /v:off /s /c "` + line + ` & set ENVTEST"`}
		}, "ENVTEST_A=" + value + "\r\nENVTEST_B=b\r\n"},
	} {
		p, err := WriteScript(e, []string{"ENVTEST_OLD"}, c.sh)
		if err != nil {
			t.Fatal(err)
		}
		argv := c.argv(c.sh.Line(p))
		cmd := exec.Command(argv[0], argv[1:]...)
		if c.sh == ShellCmd {
			cmd = exec.Command(argv[0])
			cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: argv[1]}
		}
		cmd.Env = append(os.Environ(), "ENVTEST_OLD=old")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%v: %v\n%s", argv, err, out)
		}
		if string(out) != c.print {
			t.Errorf("shell %d: got %q, want %q", c.sh, out, c.print)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("shell %d: the apply file is still there", c.sh)
		}
	}
}
