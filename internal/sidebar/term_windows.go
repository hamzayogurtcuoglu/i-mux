//go:build windows

package sidebar

import (
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// The terminal on Windows is the console (D85): its modes, size and input
// come from sysx, and a redraw is asked for with the sidebar's wake event.

// rawMode puts the console on fd into raw mode (VT input, no echo, no line
// editing, no signals from keys) and returns a func that restores it.
func rawMode(fd int) (func(), error) { return sysx.RawMode(fd) }

// signalRedraw asks the sidebar process pid to redraw.
func signalRedraw(pid int) { _ = sysx.Wake(pid) }

// RawMode puts the terminal on fd into raw mode and returns a func that
// restores it (the folder picker uses it too).
func RawMode(fd int) (func(), error) { return rawMode(fd) }

// Size returns the console's size (every handle of a console has the same).
func Size(int) (w, h int) { return sysx.ConsoleSize() }

// ReadInput reads what the console has, waiting at most d: nil when nothing
// came. Popups read this way rather than from a goroutine: a read left
// waiting when the popup hands the console on (to the editor) would swallow
// the editor's first key.
func ReadInput(fd int, d time.Duration) ([]byte, error) { return sysx.ReadInput(fd, d) }
