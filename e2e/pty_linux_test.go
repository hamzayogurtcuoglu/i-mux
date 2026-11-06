//go:build e2e && linux

package e2e

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

func openPty() (*os.File, string, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	var unlock int32
	if err := ioctl(m.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); err != nil {
		m.Close()
		return nil, "", fmt.Errorf("TIOCSPTLCK: %w", err)
	}
	var n uint32
	if err := ioctl(m.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		m.Close()
		return nil, "", fmt.Errorf("TIOCGPTN: %w", err)
	}
	return m, fmt.Sprintf("/dev/pts/%d", n), nil
}
