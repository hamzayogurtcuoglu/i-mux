//go:build unix

package update

import (
	"context"
	"io"
	"os"
	"os/exec"
)

const archiveExt = ".tar.gz"

// Install installs release version into dir with that release's install.sh
// (or directly from BaseURL's archive when IMUX_UPDATE_URL is set in tests).
func Install(ctx context.Context, version, dir string, stdin io.Reader, stdout, stderr io.Writer) error {
	if os.Getenv("IMUX_UPDATE_URL") != "" {
		return InstallArchive(ctx, version, dir, stdout)
	}
	argv := InstallCmd(version, dir)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}
