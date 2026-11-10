package sidebar

import (
	"bytes"
	"strconv"
	"time"
	"unicode/utf8"
)

// EventKind is the kind of an input event.
type EventKind int

// Input events.
const (
	EvKey        EventKind = iota // Key holds the key
	EvClick                       // left button press at X, Y (1-based)
	EvRightClick                  // right button press at X, Y
	EvScroll                      // mouse wheel at X, Y: Key "up" or "down"
	EvMove                        // the pointer moved to X, Y (X 0: it left the sidebar)
	EvPaste                       // a bracketed paste: Text holds it, Key is "paste"
)

// Event is one key press or mouse click.
type Event struct {
	Kind EventKind
	Key  string // "up", "down", "left", "right", "home", "end", "delete", "pgup", "pgdown", "enter", "esc", "tab", "backtab", "backspace", "ctrl-a", "ctrl-e", "ctrl-u", "ctrl-w", "ctrl-c", or the character
	X, Y int
	Text string // EvPaste: the pasted text
}

// Paste markers of bracketed paste mode (only a program that turns the mode
// on with ESC [ ? 2004 h gets them).
const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
	// maxPaste is how much of a paste with no end marker is kept waiting.
	maxPaste = 1 << 20
)

// Parse decodes terminal input: printable keys, Enter, Tab, Esc, arrows,
// Home, End, Delete, PgUp and PgDn (CSI and SS3 forms), bracketed pastes and SGR mouse
// reports (ESC [ < b ; x ; y M|m), of which only left-button presses become
// events. It returns the events and the bytes of an incomplete sequence (or
// paste) to keep for the next read. A lone ESC at the end of the input is
// Esc when final is true.
func Parse(in []byte, final bool) ([]Event, []byte) {
	var evs []Event
	key := func(k string) { evs = append(evs, Event{Kind: EvKey, Key: k}) }
	i := 0
	for i < len(in) {
		c := in[i]
		switch {
		case c == 0x1b:
			if i+1 >= len(in) {
				if final {
					key("esc")
					return evs, nil
				}
				return evs, in[i:]
			}
			switch in[i+1] {
			case '[':
				if i+2 < len(in) && in[i+2] == '<' {
					end := bytes.IndexAny(in[i+3:], "Mm")
					if end < 0 {
						return evs, in[i:]
					}
					body := string(in[i+3 : i+3+end])
					fb := in[i+3+end]
					if ev, ok := mouse(body, fb); ok {
						evs = append(evs, ev)
					}
					i += 3 + end + 1
					continue
				}
				if i+2 >= len(in) {
					return evs, in[i:]
				}
				if bytes.HasPrefix(in[i:], []byte(pasteStart)) {
					body := in[i+len(pasteStart):]
					end := bytes.Index(body, []byte(pasteEnd))
					if end < 0 && len(body) < maxPaste {
						return evs, in[i:] // the rest of the paste is still coming
					}
					if end < 0 {
						end = len(body)
					}
					evs = append(evs, Event{Kind: EvPaste, Key: "paste", Text: string(body[:end])})
					i += len(pasteStart) + min(end+len(pasteEnd), len(body))
					continue
				}
				// CSI: parameters then a final byte.
				j := i + 2
				for j < len(in) && in[j] >= 0x30 && in[j] <= 0x3f {
					j++
				}
				if j >= len(in) {
					return evs, in[i:]
				}
				switch in[j] {
				case 'A':
					key("up")
				case 'B':
					key("down")
				case 'C':
					key("right")
				case 'D':
					key("left")
				case 'H':
					key("home")
				case 'F':
					key("end")
				case 'Z':
					key("backtab")
				case '~':
					switch string(in[i+2 : j]) {
					case "1", "7":
						key("home")
					case "4", "8":
						key("end")
					case "3":
						key("delete")
					case "5":
						key("pgup")
					case "6":
						key("pgdown")
					}
				}
				i = j + 1
				continue
			case 'O':
				if i+2 >= len(in) {
					return evs, in[i:]
				}
				switch in[i+2] {
				case 'A':
					key("up")
				case 'B':
					key("down")
				case 'C':
					key("right")
				case 'D':
					key("left")
				case 'H':
					key("home")
				case 'F':
					key("end")
				}
				i += 3
				continue
			default:
				key("esc")
				i++
				continue
			}
		case c == '\r' || c == '\n':
			key("enter")
		case c == '\t':
			key("tab")
		case c == 0x7f || c == 0x08:
			key("backspace")
		case c == 0x15:
			key("ctrl-u")
		case c == 0x01:
			key("ctrl-a")
		case c == 0x05:
			key("ctrl-e")
		case c == 0x17:
			key("ctrl-w")
		case c == 0x03:
			key("ctrl-c")
		case c == 0x0e:
			key("ctrl-n")
		case c == 0x10:
			key("ctrl-p")
		case c >= 0x20 && c < 0x7f:
			key(string(c))
		case c >= 0xc0:
			// A UTF-8 character (folder names: "Masaüstü").
			if !utf8.FullRune(in[i:]) {
				if final {
					return evs, nil
				}
				return evs, in[i:]
			}
			r, size := utf8.DecodeRune(in[i:])
			if r != utf8.RuneError {
				key(string(r))
			}
			i += size
			continue
		}
		i++
	}
	return evs, nil
}

// mouse decodes the body "b;x;y" of an SGR report: a left-button press
// (b == 0, final 'M') is a click, a right one a right-click; a motion (32 +
// the button, 35 with none) moves the hover.
func mouse(body string, final byte) (Event, bool) {
	var f [3]int
	n := 0
	start := 0
	for k := 0; k <= len(body); k++ {
		if k == len(body) || body[k] == ';' {
			if n >= 3 {
				return Event{}, false
			}
			v, err := strconv.Atoi(body[start:k])
			if err != nil {
				return Event{}, false
			}
			f[n] = v
			n++
			start = k + 1
		}
	}
	if n != 3 || final != 'M' {
		return Event{}, false
	}
	if f[0]&32 != 0 && f[0]&64 == 0 {
		return Event{Kind: EvMove, X: f[1], Y: f[2]}, true
	}
	switch f[0] {
	case 0:
		return Event{Kind: EvClick, X: f[1], Y: f[2]}, true
	case 2:
		return Event{Kind: EvRightClick, X: f[1], Y: f[2]}, true
	case 64:
		return Event{Kind: EvScroll, Key: "up", X: f[1], Y: f[2]}, true
	case 65:
		return Event{Kind: EvScroll, Key: "down", X: f[1], Y: f[2]}, true
	}
	return Event{}, false
}

// PopupGuard is how long a popup's program ignores clicks after it starts:
// the click that opened it acts at once, and a double click's second press
// lands on the popup (§9.3 "Clicks at once").
const PopupGuard = 400 * time.Millisecond

var started = time.Now()

// EarlyClick reports whether a click comes within PopupGuard of this
// process's start, so a popup drops it.
func EarlyClick() bool { return time.Since(started) < PopupGuard }
