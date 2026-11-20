//go:build e2e && windows

package e2e

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// conTerm is a program on a pseudo console (T90.9), as i-mux-core starts a
// pane's: the test holds the console's input and output pipes, and the
// program and what it starts run in a job that ends with it.
type conTerm struct {
	hpc     windows.Handle
	in, out *os.File
	proc    windows.Handle
	job     windows.Handle
	done    chan struct{}
	hpcOnce sync.Once
}

func startTerminal(cols, rows int, dir string, env, argv []string) (terminal, error) {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, err
	}
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		_ = windows.CloseHandle(inR)
		_ = windows.CloseHandle(inW)
		return nil, err
	}
	var hpc windows.Handle
	err = windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inR, outW, 0, &hpc)
	// The pseudo console holds its own references to its ends of the pipes.
	_ = windows.CloseHandle(inR)
	_ = windows.CloseHandle(outW)
	if err != nil {
		_ = windows.CloseHandle(inW)
		_ = windows.CloseHandle(outR)
		return nil, err
	}
	t := &conTerm{
		hpc:  hpc,
		in:   os.NewFile(uintptr(inW), "conpty-in"),
		out:  os.NewFile(uintptr(outR), "conpty-out"),
		done: make(chan struct{}),
	}
	if err := t.spawn(path, dir, env, argv); err != nil {
		t.closeTerm()
		return nil, err
	}
	go func() {
		_, _ = windows.WaitForSingleObject(t.proc, windows.INFINITE)
		_ = windows.CloseHandle(t.proc)
		close(t.done)
		// Then the console closes, so reading its output ends once what is
		// left has been read.
		go t.closeConsole()
	}()
	return t, nil
}

func (t *conTerm) spawn(path, dir string, env, argv []string) error {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	// The attribute's value is the HPCON itself, not a pointer to it.
	hpc := *(*unsafe.Pointer)(unsafe.Pointer(&t.hpc))
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, hpc, unsafe.Sizeof(t.hpc)); err != nil {
		return err
	}
	si := &windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(*si))
	// No standard handles of ours: the program gets the pseudo console's.
	si.Flags = windows.STARTF_USESTDHANDLES
	si.ProcThreadAttributeList = attrs.List()

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	t.job = job
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	// imux's detached children (the watcher) break away, as in a pane.
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return err
	}
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return err
	}
	wdir, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	var block []uint16
	for _, kv := range env {
		u, _ := windows.UTF16FromString(kv)
		block = append(block, u...)
	}
	block = append(block, 0)
	pi := &windows.ProcessInformation{}
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(app, cmdline, nil, nil, false, flags, &block[0], wdir, &si.StartupInfo, pi); err != nil {
		return &os.PathError{Op: "CreateProcess", Path: path, Err: err}
	}
	defer func() { _ = windows.CloseHandle(pi.Thread) }()
	t.proc = pi.Process
	// Suspended until it is in the job, so nothing it starts escapes the job.
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		return err
	}
	_, err = windows.ResumeThread(pi.Thread)
	return err
}

// closeConsole closes the pseudo console once. It can wait until its output
// has been read, so it is bounded.
func (t *conTerm) closeConsole() {
	t.hpcOnce.Do(func() {
		closed := make(chan struct{})
		go func() {
			windows.ClosePseudoConsole(t.hpc)
			close(closed)
		}()
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
		}
	})
}

func (t *conTerm) Read(b []byte) (int, error) {
	n, err := t.out.Read(b)
	if err != nil && (errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, os.ErrClosed)) {
		err = io.EOF
	}
	return n, err
}

func (t *conTerm) Write(b []byte) (int, error) { return t.in.Write(b) }

func (t *conTerm) kill() {
	if t.job != 0 {
		_ = windows.TerminateJobObject(t.job, 1)
	}
}

func (t *conTerm) closeTerm() {
	t.closeConsole()
	_ = t.in.Close()
	_ = t.out.Close()
	if t.job != 0 {
		_ = windows.CloseHandle(t.job)
		t.job = 0
	}
}

func (t *conTerm) wait() {
	if t.proc == 0 {
		return
	}
	<-t.done
}
