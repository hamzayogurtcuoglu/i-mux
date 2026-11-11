//go:build !windows

package watch

// pidOffset is where watch.pid holds the pid: at the start (flock locks the
// whole file without keeping others from reading it).
const pidOffset = 0
