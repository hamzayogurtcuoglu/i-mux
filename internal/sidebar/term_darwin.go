package sidebar

import "syscall"

const (
	ioctlGet = syscall.TIOCGETA
	ioctlSet = syscall.TIOCSETA
)

// waitReadable waits up to tv for fd to have input.
func waitReadable(fd int, tv *syscall.Timeval) (bool, error) {
	var set syscall.FdSet
	set.Bits[fd/32] |= 1 << (uint(fd) % 32)
	if err := syscall.Select(fd+1, &set, nil, nil, tv); err != nil {
		return false, err
	}
	return set.Bits[fd/32]&(1<<(uint(fd)%32)) != 0, nil
}
