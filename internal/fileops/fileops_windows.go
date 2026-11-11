//go:build windows

package fileops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/hamzayogurtcuoglu/i-mux/internal/winx"
)

// command is the command run starts: a console helper without a window;
// explorer.exe as it is (a hidden start would hide its window) and with its
// own command line, as it reads /select,"C:\a b\c" but not Go's quoting of
// the whole argument.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if !strings.EqualFold(filepath.Base(name), "explorer.exe") {
		return winx.Hidden(cmd)
	}
	line := syscall.EscapeArg(name)
	for _, a := range args {
		if p, ok := strings.CutPrefix(a, "/select,"); ok {
			a = `/select,"` + p + `"` // a Windows path has no "
		} else {
			a = syscall.EscapeArg(a)
		}
		line += " " + a
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	return cmd
}

var shFileOperation = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// SHFileOperationW's operation and flags.
const (
	foDelete           = 0x0003
	fofSilent          = 0x0004 // no progress window
	fofNoConfirmation  = 0x0010 // the menu already asked
	fofAllowUndo       = 0x0040 // to the Recycle Bin
	fofNoErrorUI       = 0x0400 // errors come back to imux's menu
	fofWantNukeWarning = 0x4000 // ask before deleting what cannot be recycled
)

// shFileOpStruct is SHFILEOPSTRUCTW in its 64-bit layout (imux ships for
// amd64 and arm64).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// errNoBin is a drive without a Recycle Bin (network shares, USB sticks).
var errNoBin = errors.New("this drive has no Recycle Bin")

// recycle moves path to the Recycle Bin with the shell (Explorer's Restore
// works). A drive without a Recycle Bin is an error, as a Mac volume without
// a Trash is, so the menu can offer to delete it for good instead.
func recycle(path string) error {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return errors.New("the Recycle Bin needs a 64-bit imux")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(abs); err != nil {
		return err
	}
	if !hasBin(abs) {
		return errNoBin
	}
	from, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	from = append(from, 0) // a list: ends with two NULs
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofSilent | fofNoConfirmation | fofAllowUndo | fofNoErrorUI | fofWantNukeWarning,
	}
	r, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	switch {
	case r != 0:
		return fmt.Errorf("could not move to the Recycle Bin (error %#x)", r)
	case op.fAnyOperationsAborted != 0:
		return errors.New("moving to the Recycle Bin was cancelled")
	}
	if _, err := os.Lstat(abs); err == nil {
		return errors.New("could not move to the Recycle Bin")
	}
	return nil
}

// hasBin reports whether abs is on a fixed drive, the drives Windows keeps
// a Recycle Bin on.
func hasBin(abs string) bool {
	root, err := windows.UTF16PtrFromString(filepath.VolumeName(abs) + `\`)
	if err != nil {
		return false
	}
	return windows.GetDriveType(root) == windows.DRIVE_FIXED
}
