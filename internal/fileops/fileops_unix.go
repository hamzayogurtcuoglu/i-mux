//go:build !windows

package fileops

import (
	"context"
	"errors"
	"os/exec"
)

// command is the command run starts.
func command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// recycle moves path to Windows' Recycle Bin, which Unix has not.
func recycle(string) error { return errors.New("the Recycle Bin is on Windows") }
