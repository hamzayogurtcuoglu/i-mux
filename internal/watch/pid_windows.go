//go:build windows

package watch

// pidOffset is where watch.pid holds the pid: after the first byte, which
// sysx.Lock locks. A Windows lock is mandatory, so another process could not
// read a pid written there (Status would see the watcher with pid 0).
const pidOffset = 1
