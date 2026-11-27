package tmux

import "os"

// PaneEnv is what i-mux adds to the environment its processes start with, so
// the engine's server, every pane in it and every agent i-mux starts have it
// (D114). The engine has no set-environment; tmux and i-mux-core both give
// their panes the environment the server started with.
//
// COPILOT_MULTIPLEXER=none: GitHub Copilot CLI copies the text selected in it
// with plain OSC 52, which set-clipboard on passes on to the terminal. Its
// tmux path asks the `tmux` on PATH, which cannot talk to i-mux-core, and
// falls back to a DCS passthrough, which i-mux-core drops.
var PaneEnv = [][2]string{{"COPILOT_MULTIPLEXER", "none"}}

// SetPaneEnv puts PaneEnv into this process's environment, keeping any value
// the user set.
func SetPaneEnv() {
	for _, kv := range PaneEnv {
		if _, ok := os.LookupEnv(kv[0]); !ok {
			_ = os.Setenv(kv[0], kv[1])
		}
	}
}
