package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/usage"
)

// __statusline claude records the percentages and prints a short line, or
// the user's own status line run with the same input.
func TestStatusline(t *testing.T) {
	h := testutil.Isolate(t)
	i18n.SetLocale("en")
	// A fresh script's first run can wait for the OS's malware scan.
	old := StatuslineTimeout
	StatuslineTimeout = 30 * time.Second
	t.Cleanup(func() { StatuslineTimeout = old })
	in := `{"workspace":{"current_dir":"` + h.Project + `"},"rate_limits":{"five_hour":{"used_percentage":72.4,"resets_at":1999999999},"seven_day":{"used_percentage":18}}}`
	run := func() string {
		var o, e bytes.Buffer
		if code := Run(&Env{Stdin: strings.NewReader(in), Stdout: &o, Stderr: &e}, []string{"__statusline", "claude"}); code != ExitOK {
			t.Fatalf("exit %d: %s", code, e.String())
		}
		return o.String()
	}
	if out := run(); out != "5h 72% · week 18%\n" {
		t.Fatalf("own line %q", out)
	}
	u := usage.Load(paths.ClaudeUsage())
	if u == nil || u.FiveHour.UsedPercent != 72.4 || u.Weekly.UsedPercent != 18 {
		t.Fatalf("saved %+v", u)
	}
	// The user's status line gets the same input.
	script := filepath.Join(h.Home, "line.sh")
	testutil.WriteFile(t, script, []byte("#!/bin/sh\nprintf 'mine:'; head -c 13\n"))
	_ = os.Chmod(script, 0o755)
	testutil.WriteFile(t, filepath.Join(h.Home, ".claude", "settings.json"), []byte(`{"statusLine":{"type":"command","command":"`+script+`"}}`))
	if out := run(); out != `mine:{"workspace":` {
		t.Fatalf("user line %q", out)
	}
}
