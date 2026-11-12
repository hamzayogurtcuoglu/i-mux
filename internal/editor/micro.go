package editor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fetch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// MicroVersion is the micro release i-mux gets (D45); MicroMB its download.
const (
	MicroVersion = "2.0.15"
	MicroMB      = 5
)

// MicroAsset is one of the release's archives: its name, size in bytes and
// SHA-256 (the one GitHub lists, checked on every download).
type MicroAsset struct {
	Name string
	Size int64
	Sum  string
}

var microAssets = map[string]MicroAsset{
	"linux/amd64":  {"micro-2.0.15-linux64-static.tar.gz", 5087804, "267d238eac1e26ed053d13d4d48bd421b87f9eb538b604f0b2f74a85598b6cc2"},
	"linux/arm64":  {"micro-2.0.15-linux-arm64.tar.gz", 4680364, "5ca127857bf5500be3879f1a70b27556e737a49da04a1be5334de9e8e8781ad9"},
	"darwin/amd64": {"micro-2.0.15-osx.tar.gz", 5195727, "d765b9bdd6478c4f5a3cbd5a93ebe7bab2b9cec30b5350016975cf1045855cee"},
	"darwin/arm64": {"micro-2.0.15-macos-arm64.tar.gz", 4879518, "2ff285cc49f2ac3f7a6e9110e2dabd3c848b73eed56a8396d627c0af80a34624"},
}

// MicroFor is the archive for goos and goarch.
func MicroFor(goos, goarch string) (MicroAsset, bool) {
	a, ok := microAssets[goos+"/"+goarch]
	return a, ok
}

// CanGetMicro reports whether i-mux has a micro for this machine.
func CanGetMicro() bool {
	_, ok := MicroFor(runtime.GOOS, runtime.GOARCH)
	return ok
}

// MicroURL is where a is downloaded from; IMUX_MICRO_URL replaces it (a
// mirror; tests).
func MicroURL(a MicroAsset) string {
	if u := os.Getenv("IMUX_MICRO_URL"); u != "" {
		return u
	}
	return "https://github.com/zyedidia/micro/releases/download/v" + MicroVersion + "/" + a.Name
}

// MicroDir is where i-mux keeps the micro it got.
func MicroDir() string { return filepath.Join(paths.Tools(), "micro-"+MicroVersion) }

// OurMicro is the micro i-mux got, "" when it has none.
var OurMicro = func() string {
	p := filepath.Join(MicroDir(), "micro")
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
		return p
	}
	return ""
}

// HasMicro reports a micro on the PATH or i-mux's.
func HasMicro() bool {
	if _, err := LookPath("micro"); err == nil {
		return true
	}
	return OurMicro() != ""
}

// microSum is the checksum InstallMicro wants for a (tests replace it).
var microSum = func(a MicroAsset) string { return a.Sum }

// InstallMicro gets micro into MicroDir: the release's archive, checked,
// unpacked into a temporary folder that is renamed when complete, then
// run once. out shows the steps; bar redraws the progress on one line.
func InstallMicro(ctx context.Context, out io.Writer, bar bool) error {
	a, ok := MicroFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return fmt.Errorf("no micro build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err := paths.EnsureDir(paths.Tools()); err != nil {
		return err
	}
	part := filepath.Join(paths.Tools(), a.Name+".part")
	defer os.Remove(part)
	if err := fetch.Download(ctx, MicroURL(a), part, a.Size, out, bar); err != nil {
		return err
	}
	say := func(s string) {
		if out != nil {
			fmt.Fprintln(out, "  "+s)
		}
	}
	say(i18n.T("fetch.checking"))
	if err := fetch.Check(part, microSum(a)); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(paths.Tools(), ".micro-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := fetch.Untar(part, tmp); err != nil {
		return err
	}
	src := filepath.Join(tmp, "micro-"+MicroVersion)
	bin := filepath.Join(src, "micro")
	if fi, err := os.Stat(bin); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s has no micro-%s/micro", a.Name, MicroVersion)
	}
	vctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if v, err := exec.CommandContext(vctx, bin, "-version").CombinedOutput(); err != nil {
		return fmt.Errorf("micro -version: %w %s", err, strings.TrimSpace(string(v)))
	}
	if err := os.RemoveAll(MicroDir()); err != nil {
		return err
	}
	return os.Rename(src, MicroDir())
}
