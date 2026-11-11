//go:build windows

package sysx

import (
	"io"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

var (
	conOutOnce sync.Once
	conOut     windows.Handle = windows.InvalidHandle
)

// consoleOut is the console's screen buffer (CONOUT$), which answers size
// questions even when stdout is redirected.
func consoleOut() windows.Handle {
	conOutOnce.Do(func() {
		name, _ := windows.UTF16PtrFromString("CONOUT$")
		h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			conOut = h
		}
	})
	return conOut
}

// ConsoleSize returns the console window's columns and rows (80x24 without
// a console).
func ConsoleSize() (w, h int) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(consoleOut(), &info); err != nil {
		return 80, 24
	}
	w = int(info.Window.Right-info.Window.Left) + 1
	h = int(info.Window.Bottom-info.Window.Top) + 1
	if w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

// RawMode puts the console into the mode a TUI needs: input arrives as VT
// sequences (keys, the mouse) without echo, line editing or Ctrl+C turning
// into a signal; output interprets VT sequences; both code pages are UTF-8.
// It returns a func that restores the old modes.
func RawMode(fd int) (func(), error) {
	in := windows.Handle(fd)
	var oldIn, oldOut uint32
	if err := windows.GetConsoleMode(in, &oldIn); err != nil {
		return nil, err
	}
	out := consoleOut()
	haveOut := windows.GetConsoleMode(out, &oldOut) == nil
	oldCP, _ := windows.GetConsoleCP()
	oldOutCP, _ := windows.GetConsoleOutputCP()

	mode := uint32(windows.ENABLE_VIRTUAL_TERMINAL_INPUT | windows.ENABLE_EXTENDED_FLAGS)
	if err := windows.SetConsoleMode(in, mode); err != nil {
		return nil, err
	}
	if haveOut {
		_ = windows.SetConsoleMode(out, oldOut|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
	_ = windows.SetConsoleCP(cpUTF8)
	_ = windows.SetConsoleOutputCP(cpUTF8)
	return func() {
		_ = windows.SetConsoleMode(in, oldIn)
		if haveOut {
			_ = windows.SetConsoleMode(out, oldOut)
		}
		if oldCP != 0 {
			_ = windows.SetConsoleCP(oldCP)
		}
		if oldOutCP != 0 {
			_ = windows.SetConsoleOutputCP(oldOutCP)
		}
	}, nil
}

// EnableVT turns on VT processing for the console's output, so the escape
// sequences a TUI writes are drawn rather than printed.
func EnableVT() {
	out := consoleOut()
	var m uint32
	if windows.GetConsoleMode(out, &m) == nil {
		_ = windows.SetConsoleMode(out, m|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
	_ = windows.SetConsoleOutputCP(cpUTF8)
}

// TermSize returns the console size; fd is not needed on Windows (any of the
// console's handles has the same size).
func TermSize(int) (w, h int) { return ConsoleSize() }

var (
	readMu  sync.Mutex
	pending []uint16 // a high surrogate whose pair has not come yet
)

// ReadInput reads what the console has, waiting at most d: nil when nothing
// came. Records that carry no text (focus, size, key releases) are dropped
// so that the read that follows never blocks.
func ReadInput(fd int, d time.Duration) ([]byte, error) {
	h := windows.Handle(fd)
	deadline := time.Now().Add(d)
	for {
		left := time.Until(deadline)
		if left < 0 {
			left = 0
		}
		r, err := windows.WaitForSingleObject(h, uint32(left/time.Millisecond))
		if err != nil {
			return nil, err
		}
		if r != windows.WAIT_OBJECT_0 {
			return nil, nil
		}
		text, err := hasText(h)
		if err != nil {
			return nil, err
		}
		if text {
			return readConsole(h)
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
	}
}

// hasText reports whether the next input record produces a character,
// discarding the ones before it that do not.
func hasText(h windows.Handle) (bool, error) {
	var rec [1]inputRecord
	for {
		var n uint32
		if err := peekConsoleInput(h, &rec[0], 1, &n); err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
		if rec[0].isTextKey() {
			return true, nil
		}
		if err := readConsoleInput(h, &rec[0], 1, &n); err != nil {
			return false, err
		}
	}
}

// readConsole reads the characters waiting and returns them as UTF-8.
func readConsole(h windows.Handle) ([]byte, error) {
	readMu.Lock()
	defer readMu.Unlock()
	buf := make([]uint16, 1024)
	var n uint32
	if err := windows.ReadConsole(h, &buf[0], uint32(len(buf)), &n, nil); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, io.EOF
	}
	u := append(pending, buf[:n]...)
	pending = nil
	if last := u[len(u)-1]; utf16.IsSurrogate(rune(last)) && last < 0xdc00 {
		pending = []uint16{last}
		u = u[:len(u)-1]
	}
	out := make([]byte, 0, len(u))
	for _, r := range utf16.Decode(u) {
		out = utf8.AppendRune(out, r)
	}
	return out, nil
}

// inputRecord is INPUT_RECORD with the KEY_EVENT_RECORD fields imux reads.
type inputRecord struct {
	EventType uint16
	_         uint16
	KeyDown   int32
	Repeat    uint16
	VKey      uint16
	VScan     uint16
	Char      uint16
	Control   uint32
}

const (
	keyEvent = 0x0001
	cpUTF8   = 65001
)

func (r *inputRecord) isTextKey() bool {
	return r.EventType == keyEvent && r.KeyDown != 0 && r.Char != 0
}

var (
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInput  = kernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInputW = kernel32.NewProc("ReadConsoleInputW")
)

func peekConsoleInput(h windows.Handle, rec *inputRecord, n uint32, read *uint32) error {
	r, _, err := procPeekConsoleInput.Call(uintptr(h), uintptrOf(rec), uintptr(n), uintptrOfU32(read))
	if r == 0 {
		return err
	}
	return nil
}

func readConsoleInput(h windows.Handle, rec *inputRecord, n uint32, read *uint32) error {
	r, _, err := procReadConsoleInputW.Call(uintptr(h), uintptrOf(rec), uintptr(n), uintptrOfU32(read))
	if r == 0 {
		return err
	}
	return nil
}
