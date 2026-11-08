package sidebar

import "syscall"

const (
	ioctlGet = syscall.TCGETS
	ioctlSet = syscall.TCSETS
)

// waitReadable waits up to tv for fd to have input.
func waitReadable(fd int, tv *syscall.Timeval) (bool, error) {
	var set syscall.FdSet
	set.Bits[fd/64] |= 1 << (uint(fd) % 64)
	n, err := syscall.Select(fd+1, &set, nil, nil, tv)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
