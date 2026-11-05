// Package clock abstracts time.Now so that tests can freeze it.
package clock

import (
	"sync"
	"time"
)

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// Real is the wall clock.
var Real Clock = realClock{}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Fixed is a settable clock for tests. The zero value reports the zero time.
type Fixed struct {
	mu sync.Mutex
	t  time.Time
}

// NewFixed returns a clock frozen at t.
func NewFixed(t time.Time) *Fixed { return &Fixed{t: t} }

// Now returns the frozen time.
func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Set moves the clock to t.
func (f *Fixed) Set(t time.Time) {
	f.mu.Lock()
	f.t = t
	f.mu.Unlock()
}

// Advance moves the clock forward by d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}
