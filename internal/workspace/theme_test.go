package workspace

import (
	"context"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"
)

// TestPaintTerminals (T82.1, D77): a theme applied gives the attached
// terminals its background at once, before tmux redraws, and the override
// for the clients to come; a control-mode client and anything that is not
// a terminal device are left alone. Following the terminal's light or dark,
// and quitting, give the terminals their own background back.
func TestPaintTerminals(t *testing.T) {
	w, f, _ := testWS(t)
	ctx := context.Background()
	f.replies[""] = "tmux 3.4\n"
	f.replies["list-clients"] = "0\t/dev/pts/7\n1\t/dev/pts/8\n0\t/tmp/not-a-terminal\n"
	type write struct {
		tty, seq string
		calls    int // tmux calls before it
	}
	var wrote []write
	old := writeTerminal
	writeTerminal = func(tty, s string) error {
		wrote = append(wrote, write{tty, s, len(f.calls)})
		return nil
	}
	t.Cleanup(func() { writeTerminal = old })
	callAt := func(cmd string) int {
		for i, c := range f.calls {
			if strings.Contains(strings.Join(c, " "), cmd) {
				return i
			}
		}
		t.Fatalf("no %s in %q", cmd, f.calls)
		return -1
	}

	if err := w.ApplyTheme(ctx, theme.Builtin("nord")); err != nil {
		t.Fatal(err)
	}
	if len(wrote) != 1 || wrote[0].tty != "/dev/pts/7" || wrote[0].seq != "\x1b]11;rgb:27/2c/36\x07" {
		t.Fatalf("written %+v", wrote)
	}
	if styles := callAt("window-style"); wrote[0].calls > styles {
		t.Error("the terminal painted after tmux's styles")
	}
	if c := strings.Join(f.calls[callAt("window-style")], " "); !strings.Contains(c, `terminal-overrides[90] *:Enfcs=\E[?1004h\E]11;rgb::27/2c/36\007:`) {
		t.Errorf("no override for the clients to come: %s", c)
	}

	w.Cfg.Theme.AutoSwitch = true
	wrote = nil
	if err := w.ApplyTheme(ctx, theme.Builtin("nord")); err != nil {
		t.Fatal(err)
	}
	if len(wrote) != 1 || wrote[0].seq != "\x1b]111\x07" {
		t.Fatalf("following the terminal: written %+v", wrote)
	}

	wrote, f.calls = nil, nil
	if err := w.Quit(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(wrote) != 1 || wrote[0].seq != "\x1b]111\x07" || wrote[0].calls > callAt("kill-server") {
		t.Fatalf("quit: written %+v", wrote)
	}
}
