//go:build !windows

package paths

import "os"

// Rename is os.Rename (on Windows it waits for a file in use).
func Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
