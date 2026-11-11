//go:build windows

package fileops

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

// Windows names: a drive or a leading backslash is not relative, and ..\
// leaves the folder as ../ does.
func TestCleanNameWindows(t *testing.T) {
	d := t.TempDir()
	for _, bad := range []string{`C:x`, `C:\x`, `\x`, `..\x`, `a\..\..\x`, `\\srv\share\x`, `/x`} {
		if _, err := Create(d, bad, false); !errors.Is(err, ErrBadName) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if p, err := Create(d, `sub\a.txt`, false); err != nil || p == "" {
		t.Fatalf("backslash folders: %q %v", p, err)
	}
	if _, err := Create(d, `dir\`, true); err != nil {
		t.Fatalf("trailing backslash: %v", err)
	}
}

// explorer.exe gets /select,"path" as it reads it, and its window; console
// helpers start without one.
func TestCommandWindows(t *testing.T) {
	ctx := context.Background()
	c := command(ctx, "explorer.exe", `/select,C:\Users\Ada Lovelace\a b.txt`)
	if c.SysProcAttr == nil || c.SysProcAttr.CmdLine != `explorer.exe /select,"C:\Users\Ada Lovelace\a b.txt"` || c.SysProcAttr.HideWindow {
		t.Fatalf("explorer %+v", c.SysProcAttr)
	}
	c = command(ctx, "powershell.exe", "-Command", "Set-Clipboard x")
	if c.SysProcAttr == nil || !c.SysProcAttr.HideWindow || c.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 || c.SysProcAttr.CmdLine != "" {
		t.Fatalf("powershell %+v", c.SysProcAttr)
	}
}
