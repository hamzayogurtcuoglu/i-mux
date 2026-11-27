package watch

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/approve"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Approver answers the agents' prompts when the config says so
// (D112, D113, `on_permission = "approve"`): one key per matching prompt,
// and a count on the pane for the row's "✓ N". It is the watcher's, like
// the states: no process of its own, one engine query per tick that answers.
type Approver struct {
	T   *tmux.Client
	On  bool
	Log *log.Logger
	// Words are the words that make a choice an approval (D113): the first
	// choice holding one, case-insensitive, is answered.
	Words []string
	// GiveUp is how long a prompt may stay after its key was sent before
	// i-mux leaves it to the user; 0 is DefaultGiveUp. IMUX_APPROVE_GIVEUP
	// shortens it for tests.
	GiveUp time.Duration

	answered map[string]*answer // pane instance → the prompt it answers
}

// answer is what became of one prompt, and how many the pane's agent has
// been answered.
type answer struct {
	hash   uint64 // the prompt on screen now (0 when none is), by its block
	at     time.Time
	failed bool // its key did not take: the user's, never tried again
	count  int
}

// DefaultGiveUp is how long a prompt may stay on screen after its key was
// sent before i-mux leaves it to the user: the key may not have taken, and
// nothing is ever typed twice into a pane (D112).
const DefaultGiveUp = 10 * time.Second

// giveUp is the Approver's GiveUp, set.
func (a *Approver) giveUp() time.Duration {
	if a.GiveUp > 0 {
		return a.GiveUp
	}
	return DefaultGiveUp
}

// Tick answers the prompts at the bottom of the agent panes' screens, each
// pane's last capture: once per prompt, one exec sends every key and bumps
// every count.
func (a *Approver) Tick(ctx context.Context, panes []PaneRow, screens map[string]string, now time.Time) {
	if !a.On || len(panes) == 0 {
		return
	}
	type pending struct {
		row   PaneRow
		inst  string
		hash  uint64
		key   string
		label string // the choice the words picked (OpenCode's highlight)
	}
	live := map[string]bool{}
	var out, colored []pending
	for _, p := range panes {
		inst := p.ID + "@" + strconv.FormatInt(p.Start.UnixMilli(), 10)
		live[inst] = true
		screen := screens[p.ID]
		if screen == "" {
			continue
		}
		st := a.answered[inst]
		if st == nil {
			// A watcher that just started takes each pane's count from what
			// its option says.
			st = &answer{count: p.Approved}
			if a.answered == nil {
				a.answered = map[string]*answer{}
			}
			a.answered[inst] = st
		} else if p.Approved < st.count {
			// The pane was looked at: its count starts over (D112).
			st.count = p.Approved
		}
		q := approve.Find(screen)
		if q == nil {
			// No prompt: whatever was answered is over, and a prompt that
			// looks the same later is a new one.
			if st.hash != 0 {
				st.hash, st.failed, st.at = 0, false, time.Time{}
			}
			continue
		}
		h := q.Hash()
		if h == 0 {
			h = 1 // 0 is "no prompt on screen"
		}
		if st.hash == h {
			if !st.failed && now.Sub(st.at) >= a.giveUp() {
				st.failed = true
				a.logf("pane %s %s: the answer did not take; the prompt stays with the user", p.ID, p.Agent)
			}
			continue
		}
		key, label, ok := approve.Answer(string(p.Agent), q, a.Words)
		if !ok || key == "" {
			continue
		}
		pd := pending{row: p, inst: inst, hash: h, key: key, label: label}
		if p.Agent == session.OpenCode {
			colored = append(colored, pd) // Enter takes the highlighted button
		} else {
			out = append(out, pd)
		}
	}
	a.prune(live)
	// OpenCode's buttons have no key of their own: Enter takes the
	// highlighted one, so the screens are read once more, with their colors,
	// to see which that is (capture-pane -e, one engine query for them all).
	if len(colored) > 0 {
		cmds := make([]tmux.Cmd, 0, 2*len(colored))
		for _, pd := range colored {
			cmds = append(cmds, tmux.DelimCmd("ap"+pd.row.ID), tmux.Cmd{"capture-pane", "-p", "-e", "-t", pd.row.ID})
		}
		res, err := a.T.Run(ctx, cmds...)
		if err != nil {
			a.logf("colors: %v", err)
		} else {
			sec := tmux.Sections(res)
			for _, pd := range colored {
				if approve.Highlighted(sec["ap"+pd.row.ID], pd.label) {
					out = append(out, pd)
				}
			}
		}
	}
	if len(out) == 0 {
		return
	}
	// One exec sends the keys and writes the counts; what each prompt became
	// is kept only once it ran.
	cmds := make([]tmux.Cmd, 0, 2*len(out))
	type sent struct {
		pd   pending
		st   *answer
		next int
	}
	sents := make([]sent, 0, len(out))
	for _, pd := range out {
		st := a.answered[pd.inst]
		cmds = append(cmds,
			tmux.Cmd{"send-keys", "-t", pd.row.ID, pd.key},
			tmux.Cmd{"set-option", "-p", "-q", "-t", pd.row.ID, sidebar.OptApproved, strconv.Itoa(st.count + 1)},
		)
		sents = append(sents, sent{pd, st, st.count + 1})
	}
	if _, err := a.T.Run(ctx, cmds...); err != nil {
		a.logf("answer: %v", err)
		return
	}
	for _, s := range sents {
		s.st.hash, s.st.at, s.st.count = s.pd.hash, now, s.next
		a.logf("pane %s %s: answered a permission prompt", s.pd.row.ID, s.pd.row.Agent)
	}
}

// prune forgets panes that are gone.
func (a *Approver) prune(live map[string]bool) {
	for k := range a.answered {
		if !live[k] {
			delete(a.answered, k)
		}
	}
}

func (a *Approver) logf(format string, args ...any) {
	if a.Log != nil {
		a.Log.Printf(format, args...)
	}
}
