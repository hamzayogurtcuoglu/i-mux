package bus

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWait(t *testing.T) {
	f := &fakeTmux{screens: map[string]string{"%2": "working 0\n"}}
	b, clk := newBus(t, f)
	n := 0
	f.onCap = func() {
		n++
		if n < 3 {
			f.screens["%2"] = "working " + itoa(int64(n)) + "\n"
		}
	}
	b.Sleep = func(d time.Duration) { clk.Set(clk.Now().Add(d)) }
	start := clk.Now()
	screen, err := b.Wait(context.Background(), "%2", 5*time.Second, time.Minute)
	if err != nil || screen != "working 2\n" {
		t.Fatalf("Wait: %q %v", screen, err)
	}
	if d := clk.Now().Sub(start); d < 5*time.Second || d > 9*time.Second {
		t.Fatalf("idle after %v", d)
	}
	// A screen that keeps changing times out.
	f.onCap = func() { n++; f.screens["%2"] = "tick " + itoa(int64(n)) + "\n" }
	if _, err := b.Wait(context.Background(), "%2", 5*time.Second, 4*time.Second); !errors.Is(err, ErrTimeout) {
		t.Fatalf("want timeout, got %v", err)
	}
}
