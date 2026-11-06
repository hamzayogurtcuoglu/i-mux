package bus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// Errors of the bus guards (§12).
var (
	ErrDisabled = errors.New("agent bus disabled")
	ErrTooLarge = errors.New("message too large")
	ErrRate     = errors.New("rate limit")
	ErrSelf     = errors.New("cannot send to yourself")
)

// SendReq is one `ajan send`.
type SendReq struct {
	From      string          // sender pane ($TMUX_PANE); "" outside tmux
	FromAgent session.AgentID // sender agent ($AJAN_AGENT)
	Target    string          // "%N" or agent name
	Msg       string
	WaitIdle  bool
	NoEnter   bool
	Timeout   time.Duration // for WaitIdle; 0 = 10 min
}

// SendResult says what happened.
type SendResult struct {
	Pane    string
	Pending string // confirm mode: the parked message file
}

// Prefix is the sender tag put in front of every message.
func Prefix(agent session.AgentID, pane string) string {
	who := string(agent)
	if who == "" {
		who = "user"
	}
	if pane == "" {
		return "[from " + who + "] "
	}
	return "[from " + who + " " + pane + "] "
}

// Send delivers a message to another pane after the §12 guards: the bus is
// enabled, the message fits bus.max_message_bytes, the sender is within
// bus.rate_per_minute, and the target is another pane on ajan's socket.
// With bus.confirm the user confirms it in a menu first.
func (b *Bus) Send(ctx context.Context, req SendReq) (*SendResult, error) {
	cfg := b.W.Cfg.Bus
	if !cfg.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrDisabled, i18n.T("bus.disabled"))
	}
	if n := len(req.Msg); n > cfg.MaxMessageBytes {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, i18n.T("bus.too_large", n, cfg.MaxMessageBytes))
	}
	if strings.TrimSpace(req.Msg) == "" {
		return nil, errors.New(i18n.T("bus.empty"))
	}
	p, err := b.Resolve(ctx, req.Target, req.From)
	if err != nil {
		return nil, err
	}
	if p.ID == req.From {
		return nil, fmt.Errorf("%w: %s", ErrSelf, i18n.T("bus.self"))
	}
	if err := b.takeRate(req.From); err != nil {
		return nil, err
	}
	text := Prefix(req.FromAgent, req.From) + req.Msg
	if cfg.Confirm {
		file, err := b.confirm(ctx, p, req, text)
		return &SendResult{Pane: p.ID, Pending: file}, err
	}
	if req.WaitIdle {
		to := req.Timeout
		if to <= 0 {
			to = 10 * time.Minute
		}
		if _, err := b.Wait(ctx, p.ID, b.W.Cfg.Bus.IdleAfter.Duration, to); err != nil {
			return nil, err
		}
	}
	return &SendResult{Pane: p.ID}, b.W.Paste(ctx, p.ID, text, !req.NoEnter)
}

// takeRate records a send by sender, refusing it when the last minute
// already had bus.rate_per_minute sends (this stops agent ping-pong).
func (b *Bus) takeRate(sender string) error {
	if sender == "" {
		sender = "cli"
	}
	now := b.W.Now()
	limit := b.W.Cfg.Bus.RatePerMinute
	return state.Update(func(s *state.State) error {
		if s.BusSends == nil {
			s.BusSends = map[string][]int64{}
		}
		cutoff := now.Add(-time.Minute).UnixMilli()
		var recent []int64
		for _, t := range s.BusSends[sender] {
			if t > cutoff {
				recent = append(recent, t)
			}
		}
		if len(recent) >= limit {
			return fmt.Errorf("%w: %s", ErrRate, i18n.T("bus.rate", limit))
		}
		s.BusSends[sender] = append(recent, now.UnixMilli())
		for k, v := range s.BusSends { // forget idle senders
			if k != sender && (len(v) == 0 || v[len(v)-1] <= cutoff) {
				delete(s.BusSends, k)
			}
		}
		return nil
	})
}

// confirm parks the message in a 0600 file and asks the user, on the client
// showing the target, whether to deliver it.
func (b *Bus) confirm(ctx context.Context, p *workspace.Pane, req SendReq, text string) (string, error) {
	if err := paths.EnsureDir(paths.BusDir()); err != nil {
		return "", err
	}
	var r [6]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	file := filepath.Join(paths.BusDir(), hex.EncodeToString(r[:])+".txt")
	if err := paths.WriteFile(file, []byte(text)); err != nil {
		return "", err
	}
	deliver := []string{"__deliver", file, "--pane", p.ID}
	if req.NoEnter {
		deliver = append(deliver, "--no-enter")
	}
	from := string(req.FromAgent)
	if from == "" {
		from = "user"
	}
	m := &workspace.Menu{
		// tmux cuts a menu title to the width of the items, so the details
		// are item lines.
		Title: i18n.T("bus.confirm_menu"),
		Items: []workspace.MenuItem{
			{Name: i18n.T("bus.confirm_title", from, req.From, string(p.Agent), p.ID), Disabled: true},
			{Name: clean.OneLine(req.Msg, 60), Disabled: true},
			workspace.Separator(),
			{Name: i18n.T("bus.deliver"), Key: "y", Cmd: b.W.UICommand(deliver...)},
			{Name: i18n.T("bus.discard"), Key: "n", Cmd: b.W.UICommand("__deliver", file, "--discard")},
		},
	}
	client := b.W.ClientFor(ctx, p)
	if err := b.W.Show(ctx, m, client, p.ID); err != nil {
		os.Remove(file)
		return "", err
	}
	return file, nil
}

// Deliver sends a message parked by confirm mode (or discards it). The file
// must be inside ~/.ajan/bus.
func (b *Bus) Deliver(ctx context.Context, file, pane string, enter, discard bool) error {
	if filepath.Dir(filepath.Clean(file)) != filepath.Clean(paths.BusDir()) {
		return errors.New(i18n.T("bus.bad_file"))
	}
	text, err := os.ReadFile(file)
	os.Remove(file)
	if err != nil || discard {
		return err
	}
	return b.W.Paste(ctx, pane, string(text), enter)
}
