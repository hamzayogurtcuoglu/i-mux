//go:build windows

package sysx

import (
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	conOutOnce sync.Once
	conOut     = windows.InvalidHandle
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
func ConsoleSize() (cols, rows int) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(consoleOut(), &info); err != nil {
		return 80, 24
	}
	cols = int(info.Window.Right-info.Window.Left) + 1
	rows = int(info.Window.Bottom-info.Window.Top) + 1
	if cols <= 0 || rows <= 0 {
		return 80, 24
	}
	return cols, rows
}

// cpUTF8 is the UTF-8 code page.
const cpUTF8 = 65001

// RawMode puts the console into the mode a TUI needs, as raw mode does a
// Unix terminal: input arrives as VT sequences (keys, the mouse) without
// echo, line editing or Ctrl+C turning into a signal; output interprets VT
// sequences; both code pages are UTF-8. It returns a func that restores the
// old modes.
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

	if err := windows.SetConsoleMode(in, windows.ENABLE_VIRTUAL_TERMINAL_INPUT|windows.ENABLE_EXTENDED_FLAGS); err != nil {
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

var (
	readMu  sync.Mutex
	pending []uint16 // a high surrogate whose pair has not come yet
)

// ReadInput reads what the console has, waiting at most d: nil when nothing
// came. Records that carry no text (focus, size, key releases) are dropped,
// so the read that follows never blocks.
func ReadInput(fd int, d time.Duration) ([]byte, error) {
	h := windows.Handle(fd)
	deadline := time.Now().Add(d)
	for {
		left := max(time.Until(deadline), 0)
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

// hasText reports whether the next input record gives a character,
// discarding the ones before it that do not.
func hasText(h windows.Handle) (bool, error) {
	var rec inputRecord
	for {
		var n uint32
		if err := consoleInput(procPeekConsoleInput, h, &rec, &n); err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
		if rec.isTextKey() {
			return true, nil
		}
		if err := consoleInput(procReadConsoleInput, h, &rec, &n); err != nil {
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

// keyEvent is INPUT_RECORD's KEY_EVENT.
const keyEvent = 0x0001

func (r *inputRecord) isTextKey() bool {
	return r.EventType == keyEvent && r.KeyDown != 0 && r.Char != 0
}

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procPeekConsoleInput        = kernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInput        = kernel32.NewProc("ReadConsoleInputW")
	procGetConsoleWindow        = kernel32.NewProc("GetConsoleWindow")
	procGetCurrentConsoleFontEx = kernel32.NewProc("GetCurrentConsoleFontEx")
	procSetCurrentConsoleFontEx = kernel32.NewProc("SetCurrentConsoleFontEx")
	procIsWindowVisible         = user32.NewProc("IsWindowVisible")
)

// consoleFontInfo is CONSOLE_FONT_INFOEX.
type consoleFontInfo struct {
	Size       uint32
	Font       uint32
	FontWidth  int16
	FontHeight int16
	Family     uint32
	Weight     uint32
	Face       [32]uint16
}

// tmpfTrueType is TMPF_TRUETYPE in a font's family: not a raster font.
const tmpfTrueType = 0x04

// visibleConsole reports whether this process draws in a console window of
// its own (conhost) and not in a pseudo console, whose window is hidden: in
// Windows Terminal, VS Code and i-mux's own panes the font is the
// terminal's, and the console's says nothing about it.
func visibleConsole() bool {
	if procGetConsoleWindow.Find() != nil || procIsWindowVisible.Find() != nil {
		return false
	}
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return false
	}
	v, _, _ := procIsWindowVisible.Call(hwnd)
	return v != 0
}

func currentFont(out windows.Handle) (consoleFontInfo, bool) {
	info := consoleFontInfo{Size: uint32(unsafe.Sizeof(consoleFontInfo{}))}
	if out == windows.InvalidHandle || procGetCurrentConsoleFontEx.Find() != nil {
		return info, false
	}
	r, _, _ := procGetCurrentConsoleFontEx.Call(uintptr(out), 0, uintptr(unsafe.Pointer(&info)))
	return info, r != 0
}

// ConsoleFont is the font of the console window this process draws in
// (conhost's, the window's properties → Font); false when it draws in a
// terminal instead (Windows Terminal, VS Code, a pane), whose font the
// console does not know.
func ConsoleFont() (string, bool) {
	if !visibleConsole() {
		return "", false
	}
	info, ok := currentFont(consoleOut())
	if !ok {
		return "", false
	}
	return windows.UTF16ToString(info.Face[:]), true
}

// SetConsoleFont switches the console window's font to face, keeping its
// height (a raster font's cells become a 16 px TrueType one). The window
// redraws with it at once; it reports whether the console took it.
func SetConsoleFont(face string) bool {
	if !visibleConsole() || procSetCurrentConsoleFontEx.Find() != nil {
		return false
	}
	out := consoleOut()
	info, ok := currentFont(out)
	if !ok {
		return false
	}
	name, err := windows.UTF16FromString(face)
	if err != nil || len(name) > len(info.Face) {
		return false
	}
	if info.Family&tmpfTrueType == 0 || info.FontHeight <= 0 {
		info.FontHeight = 16
	}
	info.Font, info.FontWidth = 0, 0
	info.Family, info.Weight = 0x36, 400 // FF_MODERN | TMPF_TRUETYPE | TMPF_VECTOR, FW_NORMAL
	info.Face = [32]uint16{}
	copy(info.Face[:], name)
	r, _, _ := procSetCurrentConsoleFontEx.Call(uintptr(out), 0, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return false
	}
	// The console falls back to another font when it lacks face.
	now, ok := currentFont(out)
	return ok && strings.EqualFold(windows.UTF16ToString(now.Face[:]), face)
}

// consoleInput calls PeekConsoleInputW or ReadConsoleInputW for one record.
func consoleInput(proc *windows.LazyProc, h windows.Handle, rec *inputRecord, n *uint32) error {
	r, _, err := proc.Call(uintptr(h), uintptr(unsafe.Pointer(rec)), 1, uintptr(unsafe.Pointer(n)))
	if r == 0 {
		return err
	}
	return nil
}
