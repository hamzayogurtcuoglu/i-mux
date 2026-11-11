//go:build windows

package sidebar

import (
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// rawMode puts the console on fd into the mode a TUI needs and returns a
// func that restores it.
func rawMode(fd int) (func(), error) { return sysx.RawMode(fd) }

// signalRedraw asks the sidebar process pid to redraw.
func signalRedraw(pid int) { _ = sysx.Wake(pid) }

// RawMode puts the terminal on fd into raw mode and returns a func that
// restores it (the folder picker uses it too).
func RawMode(fd int) (func(), error) { return rawMode(fd) }

// Size returns the terminal size.
func Size(fd int) (w, h int) { return sysx.TermSize(fd) }

// ReadInput reads what the terminal fd has, waiting at most d: nil when
// nothing came.
func ReadInput(fd int, d time.Duration) ([]byte, error) { return sysx.ReadInput(fd, d) }
