package watch

import "github.com/hamzayogurtcuoglu/i-mux/internal/agent"

// Asking reports whether the bottom of a screen asks the user something
// (agent.Asking: the paste of a handoff waits on it too).
func Asking(screen string) bool { return agent.Asking(screen) }
