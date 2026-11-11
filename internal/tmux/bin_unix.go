//go:build !windows

package tmux

// DefaultBin is the multiplexer imux runs when IMUX_TMUX_BIN is not set.
func DefaultBin() string { return "tmux" }

// Name is how messages and `imux versions` call the multiplexer.
const Name = "tmux"
