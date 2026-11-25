package phone

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// RoleSidebar is the @imux_role of a sidebar pane, which the phone never
// lists (sidebar.RoleSidebar).
const RoleSidebar = "sidebar"

// TmuxEngine is the workspace on i-mux's engine (D87).
type TmuxEngine struct {
	T *tmux.Client

	mu       sync.Mutex
	branches map[string]branch // project folder → its branch, read at most every 30 s
}

type branch struct {
	name string
	at   time.Time
}

// Projects lists the sessions with their live agent panes, as the remote
// machines' report does (D73).
func (e *TmuxEngine) Projects(ctx context.Context) ([]Project, error) {
	out, err := e.T.Run(ctx, tmux.Cmd{
		"list-panes", "-a", "-F",
		"#{session_name}\t#{session_path}\t#{pane_id}\t#{@imux_agent}\t#{@imux_name}\t#{@imux_state}\t#{@imux_state_at}\t#{@imux_role}\t#{pane_dead}",
	})
	if err != nil {
		return nil, err
	}
	byName := map[string]*Project{}
	var order []string
	paths := map[string]string{}
	for _, f := range tmux.Rows(out, 9) {
		p := byName[f[0]]
		if p == nil {
			p = &Project{Name: filepath.Base(f[1]), Agents: []Agent{}}
			byName[f[0]] = p
			order = append(order, f[0])
			paths[f[0]] = f[1]
		}
		if f[7] == RoleSidebar || f[3] == "" || f[8] == "1" {
			continue // the UI, a plain shell, or an agent that ended
		}
		since, _ := strconv.ParseInt(f[6], 10, 64)
		p.Agents = append(p.Agents, Agent{Pane: f[2], CLI: f[3], Name: f[4], State: f[5], Since: since})
	}
	out2 := make([]Project, 0, len(order))
	for _, s := range order {
		p := byName[s]
		p.Branch = e.branch(ctx, paths[s])
		out2 = append(out2, *p)
	}
	return out2, nil
}

func (e *TmuxEngine) branch(ctx context.Context, dir string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.branches == nil {
		e.branches = map[string]branch{}
	}
	if b, ok := e.branches[dir]; ok && time.Since(b.at) < 30*time.Second {
		return b.name
	}
	name := gitops.CurrentBranch(ctx, dir)
	e.branches[dir] = branch{name: name, at: time.Now()}
	return name
}

// Screen is pane's visible screen with its colors, and its size.
func (e *TmuxEngine) Screen(ctx context.Context, pane string) (Screen, error) {
	out, err := e.T.Run(ctx,
		tmux.DelimCmd("size"), tmux.Cmd{"display-message", "-p", "-t", pane, "#{pane_width}\t#{pane_height}"},
		tmux.DelimCmd("screen"), tmux.Cmd{"capture-pane", "-p", "-e", "-t", pane})
	if err != nil {
		return Screen{}, err
	}
	sec := tmux.Sections(out)
	var sc Screen
	if f := strings.Split(strings.TrimSpace(sec["size"]), "\t"); len(f) == 2 {
		sc.Width, _ = strconv.Atoi(f[0])
		sc.Height, _ = strconv.Atoi(f[1])
	}
	sc.Text = strings.TrimRight(sec["screen"], "\n")
	return sc, nil
}

// Send types text literally, then the keys, into pane.
func (e *TmuxEngine) Send(ctx context.Context, pane, text string, keys []string) error {
	var cmds []tmux.Cmd
	if text != "" {
		cmds = append(cmds, tmux.Cmd{"send-keys", "-t", pane, "-l", "--", text})
	}
	if len(keys) > 0 {
		cmds = append(cmds, append(tmux.Cmd{"send-keys", "-t", pane}, keys...))
	}
	if len(cmds) == 0 {
		return nil
	}
	_, err := e.T.Run(ctx, cmds...)
	return err
}
