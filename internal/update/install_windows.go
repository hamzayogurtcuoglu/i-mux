//go:build windows

package update

import (
	"context"
	"io"
)

const archiveExt = ".zip"

// Install downloads, verifies and unpacks release version's zip and replaces
// imux-core.exe and imux.exe in dir as one step (T90.7).
func Install(ctx context.Context, version, dir string, _ io.Reader, stdout, _ io.Writer) error {
	return InstallArchive(ctx, version, dir, stdout)
}
