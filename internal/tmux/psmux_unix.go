//go:build !windows

package tmux

import (
	"context"
	"errors"
	"os"
)

// IDSep joins a psmux id to its session in imux's ids (psmux only).
const IDSep = "~"

// psmuxMode is false: on Unix imux runs on tmux.
func (c *Client) psmuxMode() bool { return false }

func (c *Client) runPsmux(context.Context, []Cmd) (string, error) {
	return "", errors.New("tmux: psmux is for Windows")
}

func (c *Client) spawnPsmux([]string, []Cmd) error { return errors.New("tmux: psmux is for Windows") }

// GlobalizeFormat returns s: tmux ids are unique on its server.
func GlobalizeFormat(s string) string { return s }

// spaceBlockSeps returns conf: tmux reads "a; b" in a { } block.
func spaceBlockSeps(conf string) string { return conf }

// EnvPane is the pane imux runs in ($TMUX_PANE).
func EnvPane() string { return os.Getenv("TMUX_PANE") }

// SplitID returns id and no session: tmux ids need none.
func SplitID(id string) (local, session string) { return id, "" }
