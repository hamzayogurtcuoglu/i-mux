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
