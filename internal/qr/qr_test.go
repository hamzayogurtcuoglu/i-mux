package qr

import (
	"errors"
	"strings"
	"testing"
)

// helloM is "HELLO" as Encode makes it (version 1, level M), decoded back
// to "HELLO" by an independent decoder (OpenCV's) when it was recorded.
var helloM = []string{
	"111111101101001111111",
	"100000100110101000001",
	"101110100111101011101",
	"101110101001001011101",
	"101110101000101011101",
	"100000101011001000001",
	"111111101010101111111",
	"000000001111100000000",
	"100010111111011111001",
	"000111001011100101111",
	"101100101011001110010",
	"111001000100011010000",
	"001011100100111000110",
	"000000001110111001011",
	"111111101100110001010",
	"100000100001100100010",
	"101110101001001110101",
	"101110100001100001011",
	"101110100111001111000",
	"100000100100011000000",
	"111111101000111110101",
}

func rows(c *Code) []string {
	var out []string
	for y := range c.Size {
		var b strings.Builder
		for x := range c.Size {
			if c.Dark(x, y) {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
		out = append(out, b.String())
	}
	return out
}

func TestHello(t *testing.T) {
	c, err := Encode("HELLO")
	if err != nil {
		t.Fatal(err)
	}
	if got := rows(c); strings.Join(got, "\n") != strings.Join(helloM, "\n") {
		t.Fatalf("HELLO:\n%s", strings.Join(got, "\n"))
	}
}

// TestVersions: the smallest version that holds the text, the standard's
// patterns in place, and the format information readable in both copies.
func TestVersions(t *testing.T) {
	for _, c := range []struct {
		n, version int
	}{{1, 1}, {14, 1}, {15, 2}, {84, 5}, {100, 6}, {122, 7}, {180, 9}, {181, 10}, {213, 10}} {
		code, err := Encode(strings.Repeat("a", c.n))
		if err != nil {
			t.Fatalf("%d bytes: %v", c.n, err)
		}
		if code.Version != c.version || code.Size != 4*c.version+17 {
			t.Errorf("%d bytes: version %d size %d, want version %d", c.n, code.Version, code.Size, c.version)
		}
		n := code.Size
		// Finders: a dark ring, a light ring, a dark 3×3 center.
		for _, o := range [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}} {
			for dy := range 7 {
				for dx := range 7 {
					d := max(abs(dx-3), abs(dy-3))
					if code.Dark(o[0]+dx, o[1]+dy) != (d != 2) {
						t.Fatalf("version %d: finder at %v broken at %d,%d", code.Version, o, dx, dy)
					}
				}
			}
		}
		for i := 8; i < n-8; i++ {
			if code.Dark(i, 6) != (i%2 == 0) || code.Dark(6, i) != (i%2 == 0) {
				t.Fatalf("version %d: timing broken at %d", code.Version, i)
			}
		}
		if !code.Dark(8, n-8) {
			t.Fatalf("version %d: no dark module", code.Version)
		}
		// The format's two copies say level M and the chosen mask.
		var a, b int
		for i := 0; i <= 5; i++ {
			a |= bit(code.Dark(8, i)) << i
		}
		a |= bit(code.Dark(8, 7))<<6 | bit(code.Dark(8, 8))<<7 | bit(code.Dark(7, 8))<<8
		for i := 9; i < 15; i++ {
			a |= bit(code.Dark(14-i, 8)) << i
		}
		for i := range 8 {
			b |= bit(code.Dark(n-1-i, 8)) << i
		}
		for i := 8; i < 15; i++ {
			b |= bit(code.Dark(8, n-15+i)) << i
		}
		if a != b || (a^0x5412)>>10 != code.Mask {
			t.Fatalf("version %d: format %015b / %015b, mask %d", code.Version, a, b, code.Mask)
		}
	}
	if _, err := Encode(strings.Repeat("a", 214)); !errors.Is(err, ErrTooLong) {
		t.Fatalf("214 bytes: %v", err)
	}
}

func bit(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestTerminal: two modules a line, the quiet zone around, black on white.
func TestTerminal(t *testing.T) {
	c, _ := Encode("HELLO")
	lines := strings.Split(strings.TrimSuffix(c.Terminal(), "\n"), "\n")
	if want := (21 + 2*Quiet + 1) / 2; len(lines) != want {
		t.Fatalf("%d lines, want %d", len(lines), want)
	}
	if got := strings.Count(lines[0], "▀"); got != 21+2*Quiet {
		t.Fatalf("%d columns", got)
	}
	if !strings.HasPrefix(lines[0], "\x1b[38;5;231;48;5;231m▀") {
		t.Fatalf("quiet zone: %q", lines[0][:30])
	}
}
