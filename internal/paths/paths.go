// Package paths resolves ajan's home directory (AJAN_HOME or ~/.ajan) and the
// files inside it, and creates them with private permissions.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// DirMode is used for every directory ajan creates.
	DirMode os.FileMode = 0o700
	// FileMode is used for every file ajan creates.
	FileMode os.FileMode = 0o600
)

// SetUmask makes every file and directory created by this process private.
// It is called once at startup.
func SetUmask() { syscall.Umask(0o077) }

// UserHome returns $HOME, falling back to os.UserHomeDir.
func UserHome() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

// Home returns ajan's state directory: $AJAN_HOME, or ~/.ajan.
func Home() string {
	if h := os.Getenv("AJAN_HOME"); h != "" {
		return filepath.Clean(h)
	}
	return filepath.Join(UserHome(), ".ajan")
}

// Files inside Home.
func Config() string     { return filepath.Join(Home(), "config.toml") }
func State() string      { return filepath.Join(Home(), "state.json") }
func StateLock() string  { return filepath.Join(Home(), "state.lock") }
func TmuxConf() string   { return filepath.Join(Home(), "tmux.conf") }
func TmuxLocal() string  { return filepath.Join(Home(), "tmux.local.conf") }
func WatchPid() string   { return filepath.Join(Home(), "watch.pid") }
func WatchLog() string   { return filepath.Join(Home(), "watch.log") }
func Handoffs() string   { return filepath.Join(Home(), "handoffs") }
func Launch() string     { return filepath.Join(Home(), "launch") }
func BusDir() string     { return filepath.Join(Home(), "bus") }
func Activity() string   { return filepath.Join(Home(), "activity.json") }
func PruneStamp() string { return filepath.Join(Home(), ".pruned") }

// Ensure creates Home and its subdirectories with DirMode. Existing
// directories that are too permissive are tightened.
func Ensure() error {
	for _, d := range []string{Home(), Handoffs(), Launch()} {
		if err := EnsureDir(d); err != nil {
			return err
		}
	}
	return nil
}

// EnsureDir creates dir (and parents) with DirMode and makes sure dir itself
// is not group/world accessible.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, DirMode); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return os.Chmod(dir, DirMode)
	}
	return nil
}

// WriteFile atomically writes data to path with FileMode: temp file in the
// same directory, fsync, rename.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if err := f.Chmod(FileMode); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// Shorten replaces the user's home directory prefix with "~" for display.
func Shorten(p string) string {
	h := UserHome()
	if h == "" || h == "/" {
		return p
	}
	if p == h {
		return "~"
	}
	if strings.HasPrefix(p, h+string(filepath.Separator)) {
		return "~" + p[len(h):]
	}
	return p
}

// IsNotExist reports whether err means a missing file.
func IsNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }
