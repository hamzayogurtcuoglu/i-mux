package bus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func (f *fakeTmux) Spawn(bin string, args []string) error {
	_, _, err := f.Run(context.Background(), bin, args)
	return err
}

func (f *fakeTmux) has(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

func TestSendGuards(t *testing.T) {
	now := testutil.FrozenTime
	f := &fakeTmux{panes: row("%1", "claude", "proj", false, now) + row("%2", "antigravity", "proj", false, now)}
	b, _ := newBus(t, f)
	b.W.Cfg.Bus.RatePerMinute = 2
	ctx := context.Background()
	req := SendReq{From: "%1", FromAgent: "claude", Target: "antigravity", Msg: "please review\nsrc/a.ts"}
	res, err := b.Send(ctx, req)
	if err != nil || res.Pane != "%2" {
		t.Fatalf("send: %+v %v", res, err)
	}
	if !f.has("paste-buffer -p -d -b imux-") || !f.has("send-keys -t %2 Enter") {
		t.Fatalf("delivery commands: %q", f.calls)
	}
	// The pasted file carried the sender tag (it is deleted afterwards;
	// check the prefix itself).
	if Prefix("claude", "%1") != "[from claude %1] " || Prefix("", "") != "[from user] " {
		t.Fatal("prefix")
	}
	if _, err := b.Send(ctx, req); err != nil {
		t.Fatalf("second send: %v", err)
	}
	if _, err := b.Send(ctx, req); !errors.Is(err, ErrRate) {
		t.Fatalf("third send in a minute: %v", err)
	}
	// Another sender has its own budget.
	if _, err := b.Send(ctx, SendReq{From: "%2", Target: "claude", Msg: "hi"}); err != nil {
		t.Fatalf("other sender: %v", err)
	}
	if _, err := b.Send(ctx, SendReq{From: "%2", Target: "%2", Msg: "hi"}); !errors.Is(err, ErrSelf) {
		t.Fatalf("self send: %v", err)
	}
	b.W.Cfg.Bus.MaxMessageBytes = 4
	if _, err := b.Send(ctx, SendReq{From: "%2", Target: "claude", Msg: "too long"}); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("size: %v", err)
	}
	b.W.Cfg.Bus.Enabled = false
	if _, err := b.Send(ctx, SendReq{From: "%2", Target: "claude", Msg: "x"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled: %v", err)
	}
}

func TestSendConfirmAndDeliver(t *testing.T) {
	now := testutil.FrozenTime
	f := &fakeTmux{panes: row("%1", "claude", "proj", false, now) + row("%2", "antigravity", "proj", false, now)}
	b, _ := newBus(t, f)
	b.W.Cfg.Bus.Confirm = true
	ctx := context.Background()
	res, err := b.Send(ctx, SendReq{From: "%1", FromAgent: "claude", Target: "%2", Msg: "run the tests"})
	if err != nil || res.Pending == "" {
		t.Fatalf("confirm: %+v %v", res, err)
	}
	if f.has("paste-buffer") || !f.has("display-menu") || !f.has("From claude %1 to antigravity %2:") {
		t.Fatalf("confirm should ask, not paste: %q", f.calls)
	}
	b2, err := os.ReadFile(res.Pending)
	if err != nil || string(b2) != "[from claude %1] run the tests" {
		t.Fatalf("parked: %q %v", b2, err)
	}
	if fi, _ := os.Stat(res.Pending); fi.Mode().Perm() != 0o600 {
		t.Fatal("parked file not private")
	}
	if err := b.Deliver(ctx, res.Pending, "%2", true, false); err != nil {
		t.Fatal(err)
	}
	if !f.has("paste-buffer -p -d") {
		t.Fatal("not delivered")
	}
	if _, err := os.Stat(res.Pending); !os.IsNotExist(err) {
		t.Fatal("parked file kept")
	}
	// Only files in ~/.imux/bus can be delivered.
	other := filepath.Join(paths.Home(), "state.json")
	if err := b.Deliver(ctx, other, "%2", true, false); err == nil {
		t.Fatal("delivered a file outside the bus dir")
	}
}
