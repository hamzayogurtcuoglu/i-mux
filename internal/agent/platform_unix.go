//go:build !windows

package agent

import (
	"os"
	"path/filepath"
)

// platformInfo leaves the registry as it is: its commands and folders are the Unix ones.
func platformInfo(*Info) {}

// installDirs are the folders the agent's own install puts it in: Dirs, under the home folder.
func installDirs(info Info) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(info.Dirs))
	for _, d := range info.Dirs {
		dirs = append(dirs, filepath.Join(home, d))
	}
	return dirs
}

// programFiles are the files name may be in dir: name itself on Unix.
func programFiles(dir, name string) []string { return []string{filepath.Join(dir, name)} }
