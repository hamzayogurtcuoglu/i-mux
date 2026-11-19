package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Environments in panes (D84, §9.3 "Environments").

// EnvKind is what applying an environment to a pane does.
type EnvKind int

// What runs in a pane, for an environment.
const (
	EnvBusy  EnvKind = iota // another program: nothing is applied
	EnvAgent                // an agent: it restarts with it (asked first)
	EnvShell                // a shell at its prompt: it sources it now
	EnvEmpty                // the "No agents" pane: the option only
)

// shells are the programs at a prompt that i-mux types an apply file
// into: POSIX shells and fish.
var shells = map[string]bool{"bash": true, "zsh": true, "fish": true, "sh": true, "dash": true, "ksh": true, "mksh": true}

// EnvKindOf says what applying an environment to pane p does.
func (w *WS) EnvKindOf(ctx context.Context, p *Pane) EnvKind {
	switch {
	case p.Role == sidebar.RoleSidebar || p.Dead:
		return EnvBusy
	case p.IsEmpty():
		return EnvEmpty
	case p.Agent != "":
		return EnvAgent
	}
	login := filepath.Base(w.loginShell(ctx))
	isShell := func(name string) bool { return shells[name] || name == login }
	if !isShell(strings.TrimPrefix(p.Command, "-")) { // a login shell's "-bash"
		return EnvBusy
	}
	// A pane started with a command (`sh -c 'sleep 600'`) shows the shell
	// that runs it: only a pane started as a shell, or through i-mux (the
	// shell an agent exits to), is at a prompt.
	if f := strings.Fields(strings.Trim(strings.TrimSpace(p.StartCmd), `"'`)); len(f) > 0 {
		if first := filepath.Base(f[0]); !isShell(first) && first != filepath.Base(w.Self) {
			return EnvBusy
		}
	}
	return EnvShell
}

// ErrEnvBusy means the pane runs a program that is neither an agent nor a
// shell at its prompt.
var ErrEnvBusy = errors.New("pane busy")

// EnvApplied says what ApplyEnv did.
type EnvApplied struct {
	Kind    EnvKind
	Restart *SwitchResult // the agent's restart (EnvAgent)
}

// ApplyEnv gives pane the environment name ("" removes the pane's): an
// agent restarts with it, resuming its conversation; a shell at its prompt
// sources it at once; the "No agents" pane only keeps it for the agent
// started there. Another program is left alone (ErrEnvBusy). Asking before
// an agent restarts is the caller's (EnvMenu).
func (w *WS) ApplyEnv(ctx context.Context, f *sources.Finder, pane, name, client string) (*EnvApplied, error) {
	var env *envfile.Env
	if name != "" {
		e, err := envfile.Read(name)
		if err != nil {
			return nil, err
		}
		env = e
	}
	p, err := w.Pane(ctx, pane)
	if err != nil {
		return nil, err
	}
	kind := w.EnvKindOf(ctx, p)
	if kind == EnvBusy {
		return &EnvApplied{Kind: kind}, ErrEnvBusy
	}
	prev := p.Env
	if err := w.setEnvOpt(ctx, p.ID, name); err != nil {
		return nil, err
	}
	p.Env = name
	res := &EnvApplied{Kind: kind}
	switch kind {
	case EnvAgent:
		r, err := w.Restart(ctx, f, p, client)
		if err != nil {
			_ = w.setEnvOpt(ctx, p.ID, prev) // the old one stays as it is
			return nil, err
		}
		res.Restart = r
	case EnvShell:
		var keys []string
		if prev != "" {
			if old, err := envfile.Read(prev); err == nil {
				keys = old.Keys()
			}
		}
		if err := w.sourceEnv(ctx, p, env, keys); err != nil {
			_ = w.setEnvOpt(ctx, p.ID, prev)
			return nil, err
		}
		w.Message(ctx, client, p.ID, EnvMessage(name, ""))
	case EnvEmpty:
		w.Message(ctx, client, p.ID, EnvMessage(name, ""))
	}
	w.RefreshSidebars(ctx)
	return res, nil
}

// EnvMessage is what a pane says after its environment changed; agent is
// the short name of the agent that restarted ("" for a shell).
func EnvMessage(name, agent string) string {
	switch {
	case agent != "" && name != "":
		return i18n.T("env.restarted", agent, name)
	case agent != "":
		return i18n.T("env.restarted_none", agent)
	case name != "":
		return i18n.T("env.applied", name)
	}
	return i18n.T("env.cleared")
}

// setEnvOpt sets (or, with "", unsets) the pane's @imux_env.
func (w *WS) setEnvOpt(ctx context.Context, pane, name string) error {
	c := tmux.Cmd{"set-option", "-p", "-t", pane, OptEnv, name}
	if name == "" {
		c = tmux.Cmd{"set-option", "-p", "-u", "-t", pane, OptEnv}
	}
	_, err := w.T.Run(ctx, c)
	return err
}

// sourceEnv makes the shell at p's prompt take env (nil: none) instead of
// the keys in prev: an apply file it sources, which deletes itself. Only
// the file's path is typed, after a space (out of the history where the
// shell skips such lines) and C-u (a half-typed line is cleared).
func (w *WS) sourceEnv(ctx context.Context, p *Pane, env *envfile.Env, prev []string) error {
	fish := envfile.Fish(strings.TrimPrefix(p.Command, "-"))
	path, err := envfile.WriteScript(env, prev, fish)
	if err != nil {
		return err
	}
	line := " . " + envfile.ShQuote(path)
	if fish {
		line = " source " + envfile.ShQuote(path)
	}
	_, err = w.T.Run(ctx,
		tmux.Cmd{"send-keys", "-t", p.ID, "C-u"},
		tmux.Cmd{"send-keys", "-t", p.ID, "-l", line},
		tmux.Cmd{"send-keys", "-t", p.ID, "Enter"})
	if err != nil {
		os.Remove(path)
	}
	return err
}

// Restart starts pane p's agent again in place (D84), with the pane's
// environment as it is now: its conversation resumes natively, with the
// full handoff as the fallback for a resume that fails fast (§9.5); an
// agent with no conversation yet starts fresh.
func (w *WS) Restart(ctx context.Context, f *sources.Finder, p *Pane, client string) (*SwitchResult, error) {
	if p.Agent == "" || p.Dead {
		return nil, errors.New(i18n.T("env.no_agent"))
	}
	lock, err := w.lockPane(ctx, p)
	if err != nil {
		return nil, err
	}
	defer lock.release(ctx)
	res := &SwitchResult{Pane: p.ID, From: p.Agent, To: p.Agent, Restarted: true, Env: p.Env}
	l := Launch{Agent: p.Agent, CWD: p.CWD, Pane: p.ID}
	trail := p.Trail
	fallback := ""
	ac := w.Cfg.Agent(string(p.Agent))
	all, err := w.Panes(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := w.Discover(ctx, f, p, all)
	if err == nil && ref.ID != "" && ac.Resume && len(ac.ResumeArgs) > 0 {
		l.ResumeID, res.Resumed = ref.ID, true
		trail = recordSource(trail, ref)
		if built, err := w.BuildHandoff(ctx, f, HandoffReq{Pane: p, All: all, To: p.Agent, From: &ref}); err == nil {
			if path, err := handoff.Save(built.Doc.Text, built.From, p.Agent, w.Now()); err == nil {
				spec, _, err := w.Spec(Launch{Agent: p.Agent, CWD: p.CWD, Prompt: w.firstMessage(built.Doc.Text, path, built.From, p.Agent, false, false), Handoff: path, Pane: p.ID, Env: p.Env})
				if err == nil {
					fallback, _ = writeSpec(spec)
				}
			}
		}
	} else {
		res.Fresh = true
	}
	if err := w.respawn(ctx, p, l, trail, fallback); err != nil {
		if fallback != "" {
			os.Remove(fallback)
		}
		return nil, err
	}
	w.RefreshSidebars(ctx)
	w.Message(ctx, client, p.ID, EnvMessage(p.Env, session.ShortName(p.Agent)))
	return res, nil
}

// EnvMenu asks before an agent restarts with environment name ("" to
// remove the pane's): yes runs `imux env apply|clear --yes`.
func (w *WS) EnvMenu(ctx context.Context, p *Pane, name, client string) *Menu {
	agent := session.ShortName(p.Agent)
	title := i18n.T("env.ask", agent, name)
	yes := i18n.T("env.ask_yes", agent)
	args := []string{"env", "apply", name, "--yes"}
	if name == "" {
		title = i18n.T("env.ask_clear", agent, p.Env)
		args = []string{"env", "clear", "--yes"}
	}
	if out, err := w.T.Run(ctx, tmux.Cmd{"show-options", "-pqv", "-t", p.ID, sidebar.OptState}); err == nil && strings.TrimSpace(out) == sidebar.StateWorking {
		title += " " + i18n.T("env.ask_working", agent)
	}
	return &Menu{Title: title, Items: []MenuItem{
		{Name: yes, Key: "y", Cmd: w.ui(append(args, uiTarget(p.ID, client)...)...)},
		{Name: i18n.T("click.cancel"), Key: "n"},
	}}
}

// EnvRowMenu is an environment's menu in the Env view: apply it to the
// target pane (or remove it there when it is the pane's), edit, delete.
func (w *WS) EnvRowMenu(ctx context.Context, name, pane, client string) *Menu {
	tgt := uiTarget(pane, client)
	title := "🔑 " + name
	var items []MenuItem
	if e, err := envfile.Read(name); err == nil {
		title += " · " + i18n.T("env.vars", len(e.Vars))
		for _, pr := range e.Problems {
			items = append(items, MenuItem{Name: "⚠ " + i18n.T("env.problem", pr.Line, reasonText(pr.Reason)), Disabled: true})
		}
		if len(items) > 0 {
			items = append(items, Separator())
		}
	}
	cur := ""
	if pane != "" {
		if p, err := w.Pane(ctx, pane); err == nil {
			cur = p.Env
		}
	}
	if pane != "" {
		if cur == name {
			items = append(items,
				MenuItem{Name: i18n.T("env.reapply"), Key: "a", Cmd: w.ui(append([]string{"env", "apply", name}, tgt...)...)},
				MenuItem{Name: i18n.T("env.remove_here"), Key: "r", Cmd: w.ui(append([]string{"env", "clear"}, tgt...)...)})
		} else {
			items = append(items, MenuItem{Name: i18n.T("env.apply_here"), Key: "a", Cmd: w.ui(append([]string{"env", "apply", name}, tgt...)...)})
		}
	}
	items = append(items,
		MenuItem{Name: i18n.T("env.edit"), Key: "e", Cmd: w.popupCmd("90%", "90%", "env", "edit", name)},
		Separator(),
		MenuItem{Name: i18n.T("env.delete"), Key: "x", Cmd: w.ui(append([]string{"env", "confirm-rm", name}, tgt...)...)},
	)
	return &Menu{Title: title, Items: items}
}

// reasonText is why a line was skipped, in words.
func reasonText(r string) string {
	switch r {
	case envfile.BadKey:
		return i18n.T("env.reason.key")
	case envfile.ReservedKey:
		return i18n.T("env.reason.reserved")
	case envfile.BadQuote:
		return i18n.T("env.reason.quote")
	case envfile.BadValue:
		return i18n.T("env.reason.value")
	}
	return i18n.T("env.reason.line")
}

// EnvDeleteMenu asks before deleting environment name.
func (w *WS) EnvDeleteMenu(name, pane, client string) *Menu {
	return &Menu{Title: i18n.T("env.delete_title", name), Items: []MenuItem{
		{Name: i18n.T("env.delete_yes"), Key: "y", Cmd: w.ui(append([]string{"env", "rm", name}, uiTarget(pane, client)...)...)},
		{Name: i18n.T("click.cancel"), Key: "n"},
	}}
}

// DeleteEnv removes environment name and unsets it from the panes that
// have it; their programs keep what they got until they restart.
func (w *WS) DeleteEnv(ctx context.Context, name string) error {
	if err := envfile.Delete(name); err != nil {
		return err
	}
	if panes, err := w.Panes(ctx); err == nil {
		for _, p := range panes {
			if p.Env == name {
				_ = w.setEnvOpt(ctx, p.ID, "")
			}
		}
	}
	w.RefreshSidebars(ctx)
	return nil
}
