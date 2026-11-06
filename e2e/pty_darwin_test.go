//go:build e2e && darwin

package e2e

import (
	"bytes"
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
	if err := ioctl(m.Fd(), syscall.TIOCPTYGRANT, 0); err != nil {
		m.Close()
		return nil, "", fmt.Errorf("TIOCPTYGRANT: %w", err)
	}
	if err := ioctl(m.Fd(), syscall.TIOCPTYUNLK, 0); err != nil {
		m.Close()
		return nil, "", fmt.Errorf("TIOCPTYUNLK: %w", err)
	}
	var name [128]byte
	if err := ioctl(m.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); err != nil {
		m.Close()
		return nil, "", fmt.Errorf("TIOCPTYGNAME: %w", err)
	}
	if i := bytes.IndexByte(name[:], 0); i >= 0 {
		return m, string(name[:i]), nil
	}
	return m, string(name[:]), nil
}
