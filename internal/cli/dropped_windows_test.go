//go:build windows

package cli

import "testing"

func TestImageTextWindows(t *testing.T) {
	got := ImageText("claude", []string{`C:\Users\me\.imux\images\a.png`, `C:\Users\Ada Lovelace\b.png`})
	if want := `C:\Users\me\.imux\images\a.png "C:\Users\Ada Lovelace\b.png" `; got != want {
		t.Fatalf("claude %q, want %q", got, want)
	}
	if got := ImageText("antigravity", []string{`C:\a b\c.png`}); got != `@"C:\a b\c.png" ` {
		t.Fatalf("antigravity %q", got)
	}
}
