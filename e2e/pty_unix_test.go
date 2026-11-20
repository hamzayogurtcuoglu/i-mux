//go:build e2e && !windows

package e2e

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

type winsize struct{ Row, Col, X, Y uint16 }

func ioctl(fd, req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}

// ptyTerm is a program on a pty: the test holds the master.
type ptyTerm struct {
	master *os.File
	cmd    *exec.Cmd
}

func startTerminal(cols, rows int, dir string, env, argv []string) (terminal, error) {
	master, slaveName, err := openPty()
	if err != nil {
		return nil, err
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, err
	}
	defer slave.Close()
	// Set the size on the slave: macOS rejects TIOCSWINSZ on a master whose
	// slave is not open yet (ENOTTY).
	ws := winsize{Row: uint16(rows), Col: uint16(cols)}
	if err := ioctl(slave.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws))); err != nil {
		master.Close()
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		master.Close()
		return nil, err
	}
	return &ptyTerm{master: master, cmd: cmd}, nil
}

func (p *ptyTerm) Read(b []byte) (int, error)  { return p.master.Read(b) }
func (p *ptyTerm) Write(b []byte) (int, error) { return p.master.Write(b) }
func (p *ptyTerm) kill()                       { _ = p.cmd.Process.Kill() }
func (p *ptyTerm) closeTerm()                  { p.master.Close() }
func (p *ptyTerm) wait()                       { _, _ = p.cmd.Process.Wait() }
