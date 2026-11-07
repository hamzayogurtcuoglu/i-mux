//go:build darwin || linux

package sidebar

import (
	"syscall"
	"unsafe"
)

// rawMode puts the terminal on fd into raw mode (no echo, no line
// buffering, no signals from keys) and returns a func that restores it.
func rawMode(fd int) (func(), error) {
	var old syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlGet, uintptr(unsafe.Pointer(&old))); e != 0 {
		return nil, e
	}
	t := old
	t.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.BRKINT | syscall.INPCK | syscall.ISTRIP
	t.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag |= syscall.CS8
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlSet, uintptr(unsafe.Pointer(&t))); e != 0 {
		return nil, e
	}
	return func() {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), ioctlSet, uintptr(unsafe.Pointer(&old)))
	}, nil
}

// signalRedraw asks the sidebar process pid to redraw.
func signalRedraw(pid int) { _ = syscall.Kill(pid, syscall.SIGUSR1) }

// RawMode puts the terminal on fd into raw mode and returns a func that
// restores it (the folder picker uses it too).
func RawMode(fd int) (func(), error) { return rawMode(fd) }

// Size returns the terminal size of fd.
func Size(fd int) (w, h int) {
	var ws struct{ Row, Col, X, Y uint16 }
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws))); e != 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}
