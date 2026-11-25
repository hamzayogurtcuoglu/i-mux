package qr

import (
	"os"
	"strings"
	"testing"
)

// TestDump writes codes as rows of 0 and 1 for an independent decoder when
// QR_DUMP names a file (T109.1): every one was decoded by OpenCV's
// QRCodeDetector when this package was written, versions 1, 6, 8 and 10.
func TestDump(t *testing.T) {
	out := os.Getenv("QR_DUMP")
	if out == "" {
		t.Skip("QR_DUMP not set")
	}
	texts := []string{"HELLO", "http://192.168.1.23:7655/#p=abcdefghijklmnopqrstuv.ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq", strings.Repeat("x", 150), strings.Repeat("0123456789", 21), "i-mux"}
	var b strings.Builder
	for _, s := range texts {
		c, err := Encode(s)
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString(s + "\n")
		for y := range c.Size {
			for x := range c.Size {
				if c.Dark(x, y) {
					b.WriteByte('1')
				} else {
					b.WriteByte('0')
				}
			}
			b.WriteByte('\n')
		}
		b.WriteString("--\n")
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
