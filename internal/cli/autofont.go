package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/nerdfont"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

// The first start's Nerd Font step (D90): the installers put the font in
// place; for the ways that run none of them (go install, Homebrew, a copied
// binary, an update) the first interactive imux does it once.

// autoFontRetry is how long a failed attempt waits before the next.
const autoFontRetry = 24 * time.Hour

// autoFontTimeout bounds the download and the setup.
const autoFontTimeout = 2 * time.Minute

// fontStamp records the step: "done", or "failed <unix time>".
func fontStamp() string { return filepath.Join(paths.Home(), "font.auto") }

// autoFontWanted reports whether this start looks at the Nerd Font: not over
// SSH (the terminal is on another machine), not with icons = "emoji" or
// IMUX_NO_FONT=1, and not within a day of a failure. Set up once, it is
// looked at again with every new version (D94), so an install and an
// update both leave it in place; a terminal the user set back is left alone
// until then.
func autoFontWanted(cfg *config.Config, getenv func(string) string, stamp []byte, now time.Time, ver string) bool {
	if getenv("IMUX_NO_FONT") == "1" || getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return false
	}
	if cfg != nil && cfg.Tmux.Icons == sidebar.IconsEmoji {
		return false
	}
	f := strings.Fields(string(stamp))
	switch {
	case len(f) == 0:
		return true
	case f[0] == "failed" && len(f) >= 2:
		at, err := strconv.ParseInt(f[1], 10, 64)
		return err != nil || now.Sub(time.Unix(at, 0)) >= autoFontRetry
	case f[0] == "done":
		return len(f) < 3 || f[2] != ver
	}
	return true
}

// autoFont runs the step when autoFontWanted: `imux font install --yes`,
// saying so first. It never fails the start.
func autoFont(e *Env, cfg *config.Config) {
	stamp, _ := os.ReadFile(fontStamp())
	if !autoFontWanted(cfg, os.Getenv, stamp, now(), version.Version) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), autoFontTimeout)
	defer cancel()
	record := func(s string) { _ = paths.WriteFile(fontStamp(), []byte(s+"\n")) }
	done := "done " + strconv.FormatInt(now().Unix(), 10) + " " + version.Version
	if sys, err := nerdfont.Detect(ctx); err == nil && sys.Ready() {
		record(done) // in place: nothing to say
		return
	}
	fmt.Fprintln(e.Stdout, i18n.T("font.auto"))
	if err := runFontCtx(ctx, e, []string{"install", "--yes"}); err != nil {
		record("failed " + strconv.FormatInt(now().Unix(), 10))
		fmt.Fprintln(e.Stderr, i18n.T("font.auto_failed", err.Error()))
		return
	}
	record(done)
	fmt.Fprintln(e.Stdout)
}
