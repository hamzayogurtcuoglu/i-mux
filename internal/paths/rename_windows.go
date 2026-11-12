//go:build windows

package paths

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// renameFor is how long Rename tries again.
const renameFor = 2 * time.Second

// Rename is os.Rename, tried again for up to two seconds while Windows
// refuses it for a moment: it does not replace a file another process has
// open (a virus scanner reading a file just written, a reader), and says so
// as "access denied", a sharing violation or, briefly, a missing file.
// cmd/go's robustio does the same.
func Rename(oldpath, newpath string) error {
	deadline := time.Now().Add(renameFor)
	for wait := time.Millisecond; ; wait = min(2*wait, 100*time.Millisecond) {
		err := os.Rename(oldpath, newpath)
		if err == nil || !inUse(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(wait)
	}
}

func inUse(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_FILE_NOT_FOUND)
}
