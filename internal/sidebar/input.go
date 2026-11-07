package sidebar

import (
	"bytes"
	"strconv"
)

// EventKind is the kind of an input event.
type EventKind int

// Input events.
const (
	EvKey   EventKind = iota // Key holds the key
	EvClick                  // left button press at X, Y (1-based)
)

// Event is one key press or mouse click.
type Event struct {
	Kind EventKind
	Key  string // "up", "down", "enter", "esc", "tab", or the character
	X, Y int
}

// Parse decodes terminal input: printable keys, Enter, Tab, Esc, arrows
// (CSI and SS3 forms) and SGR mouse reports (ESC [ < b ; x ; y M|m), of
// which only left-button presses become events. It returns the events and
// the bytes of an incomplete sequence to keep for the next read. A lone ESC
// at the end of the input is Esc when final is true.
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
				case 'Z':
					key("backtab")
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
		case c >= 0x20 && c < 0x7f:
			key(string(c))
		}
		i++
	}
	return evs, nil
}

// mouse decodes the body "b;x;y" of an SGR report; only a left-button press
// (b == 0, final 'M') is a click.
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
	if n != 3 || final != 'M' || f[0] != 0 {
		return Event{}, false
	}
	return Event{Kind: EvClick, X: f[1], Y: f[2]}, true
}
