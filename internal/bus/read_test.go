package bus

import (
	"context"
	"strings"
	"testing"
)

func TestLastLines(t *testing.T) {
	in := "a\nb  \n\x1b[31mc\x1b[0m\nd\n\n   \n\n"
	if got := LastLines(StripANSI(in), 2); got != "c\nd\n" {
		t.Fatalf("got %q", got)
	}
	if got := LastLines("\n\n", 5); got != "" {
		t.Fatalf("blank: %q", got)
	}
	if got := LastLines("x\r\ny", 10); got != "x\ny\n" {
		t.Fatalf("crlf: %q", got)
	}
}

func TestRead(t *testing.T) {
	f := &fakeTmux{screens: map[string]string{"%2": "one\ntwo\nthree\n\n\n"}}
	b, _ := newBus(t, f)
	got, err := b.Read(context.Background(), "%2", 2)
	if err != nil || got != "two\nthree\n" {
		t.Fatalf("Read: %q %v", got, err)
	}
	if args := strings.Join(f.calls[0], " "); !strings.Contains(args, "capture-pane -p -J -t %2 -S -2") {
		t.Fatalf("capture args %q", args)
	}
}
