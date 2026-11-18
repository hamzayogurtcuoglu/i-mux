// Package sources finds and loads agent sessions with the user's
// configuration, and turns them into saved handoff documents. It is shared by
// `imux handoff`, `imux switch`, the session picker and the watcher.
package sources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	_ "github.com/hamzayogurtcuoglu/i-mux/internal/session/all" // register providers
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
)

// ErrNotFound means no session matched.
var ErrNotFound = errors.New("no session found")

// Env returns the provider environment with per-agent home overrides from cfg.
func Env(cfg *config.Config) session.Env {
	env := session.Env{
		ClaudeHome:      cfg.Agent("claude").Home,
		AntigravityHome: cfg.Agent("antigravity").Home,
		CopilotHome:     cfg.Agent("copilot").Home,
		CodexHome:       cfg.Agent("codex").Home,
		OpenCodeHome:    cfg.Agent("opencode").Home,
	}
	// OpenCode's sessions are read through its own CLI: find it the way
	// launches do (config, PATH, then the bin cache: menus run with tmux's
	// PATH).
	st, _ := state.Read()
	if r := agent.Resolve(cfg, session.OpenCode, st); r.Installed {
		env.OpenCodeBin = r.Path
	}
	return env
}

// Finder finds and loads sessions.
type Finder struct {
	Cfg *config.Config
	Env session.Env
}

// New returns a Finder for cfg.
func New(cfg *config.Config) *Finder { return &Finder{Cfg: cfg, Env: Env(cfg)} }

// Provider returns the provider for agent id.
func (f *Finder) Provider(id session.AgentID) session.Provider {
	return session.ProviderFor(id, f.Env)
}

// Find lists sessions of the given agents (all when empty) for q, newest first.
func (f *Finder) Find(ctx context.Context, agents []session.AgentID, q session.Query) []session.Ref {
	if len(agents) == 0 {
		agents = session.AllAgents
	}
	var all []session.Ref
	for _, id := range agents {
		p := f.Provider(id)
		if p == nil {
			continue
		}
		refs, err := p.Find(ctx, q)
		if err != nil {
			continue
		}
		all = append(all, refs...)
	}
	session.SortRefs(all)
	if q.Limit > 0 && len(all) > q.Limit {
		all = all[:q.Limit]
	}
	return all
}

// Newest returns the newest session for cwd among agents.
func (f *Finder) Newest(ctx context.Context, agents []session.AgentID, cwd string, since time.Time) (session.Ref, error) {
	refs := f.Find(ctx, agents, session.Query{CWD: cwd, Since: since, Limit: 1})
	if len(refs) == 0 {
		return session.Ref{}, ErrNotFound
	}
	return refs[0], nil
}

// Resolve turns "agent:id", a bare id (with agent) or a file path into a Ref.
func (f *Finder) Resolve(ctx context.Context, agent session.AgentID, spec, cwd string) (session.Ref, error) {
	if a, id, ok := strings.Cut(spec, ":"); ok && session.Known(a) {
		agent, spec = session.AgentID(a), id
	}
	if strings.ContainsRune(spec, os.PathSeparator) {
		abs, err := filepath.Abs(spec)
		if err != nil {
			return session.Ref{}, err
		}
		fi, err := os.Stat(abs)
		if err != nil {
			return session.Ref{}, err
		}
		if agent == "" {
			agent = guessAgent(abs)
		}
		if agent == "" {
			return session.Ref{}, fmt.Errorf("cannot tell which agent wrote %s (use --from)", spec)
		}
		ref := session.Ref{Agent: agent, Path: abs, CWD: cwd, UpdatedAt: fi.ModTime(), Size: fi.Size()}
		if agent == session.Copilot && !fi.IsDir() {
			ref.Path = filepath.Dir(abs)
		}
		return ref, nil
	}
	agents := session.AllAgents
	if agent != "" {
		agents = []session.AgentID{agent}
	}
	for _, id := range agents {
		p := f.Provider(id)
		if p == nil {
			continue
		}
		refs, _ := p.Find(ctx, session.Query{ID: spec, CWD: cwd})
		if len(refs) == 0 && cwd != "" {
			refs, _ = p.Find(ctx, session.Query{ID: spec})
		}
		if len(refs) > 0 {
			return refs[0], nil
		}
	}
	return session.Ref{}, fmt.Errorf("%w: %s", ErrNotFound, spec)
}

func guessAgent(path string) session.AgentID {
	p := filepath.ToSlash(path)
	switch {
	case strings.Contains(p, "/.claude/") || strings.Contains(p, "/projects/") && strings.HasSuffix(p, ".jsonl"):
		return session.Claude
	case strings.Contains(p, "/.gemini/antigravity") || strings.Contains(p, "/brain/") && strings.Contains(p, "/.system_generated/"):
		return session.Antigravity
	case strings.Contains(p, "/session-state/"):
		return session.Copilot
	case strings.Contains(p, "/.codex/") || strings.HasPrefix(filepath.Base(p), "rollout-"):
		return session.Codex
	}
	return ""
}

// Load parses ref with the configured windows. since > 0 selects delta mode.
func (f *Finder) Load(ctx context.Context, ref session.Ref, since time.Time) (*session.Session, error) {
	p := f.Provider(ref.Agent)
	if p == nil {
		return nil, fmt.Errorf("no parser for agent %q", ref.Agent)
	}
	return p.Load(ctx, ref, session.LoadOptions{
		HeadBytes: f.Cfg.Handoff.HeadBytes,
		TailBytes: f.Cfg.Handoff.TailBytes,
		Since:     since,
	})
}

// Preview returns the first user request of ref (cheap).
func (f *Finder) Preview(ctx context.Context, ref session.Ref) string {
	if pv, ok := f.Provider(ref.Agent).(session.Previewer); ok {
		if s, err := pv.Preview(ctx, ref); err == nil {
			return s
		}
	}
	return ""
}

// BuildRequest describes a handoff to build.
type BuildRequest struct {
	Sources []*session.Session // oldest first
	From    session.AgentID
	To      session.AgentID
	CWD     string
	Reason  handoff.Reason
	Delta   bool
	Budget  int // 0 = config
	Now     time.Time
}

// Build renders the handoff document with config options and git state.
func (f *Finder) Build(ctx context.Context, req BuildRequest) (*handoff.Doc, error) {
	cfg := f.Cfg
	opt := handoff.Options{
		From: req.From, To: req.To,
		Budget:            cfg.Handoff.BudgetChars,
		MaxArgBytes:       cfg.Handoff.MaxArgBytes,
		Now:               req.Now,
		Reason:            req.Reason,
		CWD:               req.CWD,
		IncludeSourcePath: cfg.Handoff.IncludeSourcePath,
		Redact:            cfg.Handoff.RedactSecrets,
		Delta:             req.Delta,
		Continue:          cfg.Handoff.OnArrival == "continue",
	}
	if req.Budget > 0 {
		opt.Budget = req.Budget
	}
	if cfg.Handoff.Inject == "paste" || cfg.Handoff.Deliver != "inline" {
		opt.MaxArgBytes = 1 << 30 // no argv limit when pasting or pointing to the file
	}
	if cfg.Handoff.IncludeGit && req.CWD != "" {
		g := gitinfo.Collect(ctx, req.CWD)
		opt.Git = &g
	}
	return handoff.Build(req.Sources, opt)
}

// LimitReason derives the reason from the source's last API error when it is
// limit-like (a manual switch away from an agent that just failed with a
// limit), with the reset time when the error text has one.
func LimitReason(s *session.Session) handoff.Reason {
	if s == nil {
		return handoff.Reason{}
	}
	if e := s.LastError(); e != nil && e.LimitLike() {
		at := e.At
		if at.IsZero() {
			at = time.Now()
		}
		return handoff.Reason{Limit: true, ResetAt: limits.ParseReset(e.Text, at, time.Local)}
	}
	return handoff.Reason{}
}
