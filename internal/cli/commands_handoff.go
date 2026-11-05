package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

func init() {
	register(&command{
		Name:    "handoff",
		Args:    "[--from A] [--to B] [--cwd DIR] [--session ID|PATH] [--pane %N] [--print] [--budget N] [--delta-since T] [--reason manual|limit]",
		Summary: "cmd.handoff",
		Run:     runHandoff,
	})
	register(&command{
		Name:    "sessions",
		Args:    "[--agent A] [--cwd DIR] [--all] [--limit N]",
		Summary: "cmd.sessions",
		Run:     runSessions,
	})
}

// now is the clock used by commands; tests replace it.
var now = time.Now

// housekeeping prunes old handoffs (at most once a day) and stale launch specs.
func housekeeping(cfg *config.Config) {
	handoff.Prune(cfg.Handoff.KeepDays, now())
	launch.Sweep(time.Hour)
}

func parseAgentFlag(name, v string) (session.AgentID, error) {
	if v == "" {
		return "", nil
	}
	if !session.Known(v) {
		return "", &UsageError{Msg: i18n.T("cli.unknown_agent", name, v, "claude, gemini, copilot, codex")}
	}
	return session.AgentID(v), nil
}

func parseTimeFlag(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.UnixMilli(ms), nil
	}
	return time.Parse(time.RFC3339, v)
}

func getwd(flagVal string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	return os.Getwd()
}

// paneHandoff builds a handoff for a tmux pane (set by the workspace layer).
var paneHandoff func(e *Env, cfg *config.Config, pane string, to session.AgentID, budget int) (string, error)

func runHandoff(e *Env, args []string) error {
	fs := newFlags(e, "handoff")
	fromF := fs.String("from", "", "source agent (default: newest session of any agent)")
	toF := fs.String("to", "", "target agent")
	cwdF := fs.String("cwd", "", "project directory (default: current directory)")
	sessF := fs.String("session", "", "session id, agent:id or file path")
	paneF := fs.String("pane", "", "build from a tmux pane (%N)")
	printF := fs.Bool("print", false, "print the document instead of starting the target agent")
	budgetF := fs.Int("budget", 0, "character budget (default: config handoff.budget_chars)")
	deltaF := fs.String("delta-since", "", "delta mode: only entries after this time (RFC 3339 or unix ms)")
	reasonF := fs.String("reason", "", "manual or limit")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	housekeeping(cfg)
	from, err := parseAgentFlag("--from", *fromF)
	if err != nil {
		return err
	}
	to, err := parseAgentFlag("--to", *toF)
	if err != nil {
		return err
	}
	if *paneF != "" {
		if paneHandoff == nil {
			return errors.New("--pane needs the tmux workspace")
		}
		text, err := paneHandoff(e, cfg, *paneF, to, *budgetF)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(e.Stdout, text)
		return err
	}
	if *reasonF != "" && *reasonF != "manual" && *reasonF != "limit" {
		return &UsageError{Msg: i18n.T("cli.bad_reason", *reasonF)}
	}
	since, err := parseTimeFlag(*deltaF)
	if err != nil {
		return &UsageError{Msg: "--delta-since: " + err.Error()}
	}
	cwd, err := getwd(*cwdF)
	if err != nil {
		return err
	}
	ctx := context.Background()
	f := sources.New(cfg)
	var ref session.Ref
	if *sessF != "" {
		ref, err = f.Resolve(ctx, from, *sessF, cwd)
	} else {
		var agents []session.AgentID
		if from != "" {
			agents = []session.AgentID{from}
		}
		ref, err = f.Newest(ctx, agents, cwd, time.Time{})
	}
	if errors.Is(err, sources.ErrNotFound) {
		return errors.New(i18n.T("handoff.no_session", cwd))
	}
	if err != nil {
		return err
	}
	from = ref.Agent
	if to == "" {
		to = defaultTarget(cfg, from)
	}
	s, err := f.Load(ctx, ref, since)
	if err != nil {
		return err
	}
	for _, w := range s.Warnings {
		fmt.Fprintf(e.Stderr, "ajan: %s\n", i18n.T("cli.warning", w))
	}
	reason := sources.LimitReason(s)
	switch *reasonF {
	case "limit":
		reason.Limit = true
	case "manual":
		reason.Limit = false
	}
	doc, err := f.Build(ctx, sources.BuildRequest{
		Sources: []*session.Session{s}, From: from, To: to, CWD: firstNonEmpty(s.CWD, cwd),
		Reason: reason, Delta: !since.IsZero(), Budget: *budgetF, Now: now(),
	})
	if err != nil {
		return err
	}
	if *printF {
		_, err = fmt.Fprint(e.Stdout, doc.Text)
		return err
	}
	path, err := handoff.Save(doc.Text, from, to, now())
	if err != nil {
		return err
	}
	fmt.Fprintln(e.Stderr, i18n.T("handoff.saved", path, clean.Thousands(doc.Runes)))
	crossProviderNotice(e, from, to)
	st, _ := state.Read()
	r := agent.Resolve(cfg, to, st)
	if !r.Installed {
		return errors.New(i18n.T("agent.not_installed", agent.MustGet(to).Name, agent.MustGet(to).InstallHint))
	}
	ac := cfg.Agent(string(to))
	argv := append(append([]string{}, ac.Args...), agent.Expand(ac.NewArgs, agent.Vars{Prompt: doc.Text, UUID: agent.NewUUID()})...)
	if !agent.HasPrompt(ac.NewArgs) {
		fmt.Fprintln(e.Stderr, i18n.T("handoff.no_prompt_flag", path))
	}
	return launchExec(&launch.Spec{
		Agent: to, Bin: r.Path, Args: argv, Dir: cwd,
		Env: map[string]string{"PATH": r.PathEnv}, MaxArgBytes: cfg.Handoff.MaxArgBytes,
	})
}

// launchExec replaces the process with the agent; tests replace it.
var launchExec = launch.Exec

// defaultTarget picks the first agent in config order other than from,
// preferring installed ones.
func defaultTarget(cfg *config.Config, from session.AgentID) session.AgentID {
	st, _ := state.Read()
	var fallback session.AgentID
	for _, a := range agent.Ordered(cfg) {
		if a.ID == from {
			continue
		}
		if fallback == "" {
			fallback = a.ID
		}
		if agent.Resolve(cfg, a.ID, st).Installed {
			return a.ID
		}
	}
	return fallback
}

// crossProviderNotice prints the one-time cross-provider disclosure.
func crossProviderNotice(e *Env, from, to session.AgentID) {
	st, err := state.Read()
	if err != nil || st.Notices["cross_provider_ack"] {
		return
	}
	fmt.Fprintln(e.Stderr, i18n.T("notice.cross_provider", session.DisplayName(from), session.DisplayName(to)))
	_ = state.Update(func(s *state.State) error { s.SetNotice("cross_provider_ack"); return nil })
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func runSessions(e *Env, args []string) error {
	fs := newFlags(e, "sessions")
	agentF := fs.String("agent", "", "only this agent")
	cwdF := fs.String("cwd", "", "project directory (default: current directory)")
	allF := fs.Bool("all", false, "sessions of every project")
	limitF := fs.Int("limit", 30, "maximum number of sessions")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	a, err := parseAgentFlag("--agent", *agentF)
	if err != nil {
		return err
	}
	cwd := ""
	if !*allF {
		if cwd, err = getwd(*cwdF); err != nil {
			return err
		}
	}
	var agents []session.AgentID
	if a != "" {
		agents = []session.AgentID{a}
	}
	ctx := context.Background()
	f := sources.New(cfg)
	refs := f.Find(ctx, agents, session.Query{CWD: cwd, Limit: *limitF})
	if len(refs) == 0 {
		fmt.Fprintln(e.Stderr, i18n.T("sessions.none", firstNonEmpty(cwd, "*")))
		return nil
	}
	tw := tabwriter.NewWriter(e.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T("sessions.header"))
	t := now()
	for _, r := range refs {
		preview := clean.OneLine(f.Preview(ctx, r), 40)
		if *allF {
			preview = firstNonEmpty(r.CWD, "?") + "  " + preview
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.Agent, r.ID, textfmt.Ago(t, r.UpdatedAt), textfmt.Size(r.Size), strings.TrimSpace(preview))
	}
	return tw.Flush()
}
