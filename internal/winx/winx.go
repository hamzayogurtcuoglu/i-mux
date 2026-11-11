// Package winx holds the Windows helpers the desktop features share
// (clipboard, notifications, the file manager, editors and pagers): console
// helpers started without a window, Windows PowerShell, and the tools of Git
// for Windows (its bash, less, vim and nano), which are not on the PATH.
//
// On Unix every function does nothing or finds nothing.
package winx

import "os/exec"

// Hidden returns cmd set to run without a console window of its own (on
// Windows), so none flashes when a hidden imux process starts PowerShell or
// another console helper. GUI programs (explorer.exe) are started as they
// are: a hidden start would hide their window too.
func Hidden(cmd *exec.Cmd) *exec.Cmd {
	hide(cmd)
	return cmd
}
