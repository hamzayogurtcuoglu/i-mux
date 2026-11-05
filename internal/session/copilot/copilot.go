// Package copilot parses GitHub Copilot CLI sessions:
// ~/.copilot/session-state/<id>/{workspace.yaml,events.jsonl,session.db} (§7.3).
package copilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
)

func init() { session.Register(session.Copilot, New) }

// Provider reads Copilot's session-state store.
type Provider struct {
	root string // $COPILOT_HOME or ~/.copilot
}

// New returns a provider for env.
func New(env session.Env) session.Provider {
	return &Provider{root: env.Resolve(session.Copilot)}
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.Copilot }

func (p *Provider) stateDir() string { return filepath.Join(p.root, "session-state") }

// ReadWorkspace reads the top-level scalar keys of a workspace.yaml. Nested
// values, lists and block scalars are ignored.
func ReadWorkspace(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' || line[0] == '-' {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if v == "" || v == "|" || v == ">" || strings.HasPrefix(v, "|") || strings.HasPrefix(v, ">") {
			continue
		}
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"') {
			var s string
			if json.Unmarshal([]byte(v), &s) == nil {
				v = s
			} else {
				v = v[1 : len(v)-1]
			}
		} else if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
			v = strings.ReplaceAll(v[1:len(v)-1], "''", "'")
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		out[k] = v
	}
	return out
}

// Find implements session.Provider.
func (p *Provider) Find(ctx context.Context, q session.Query) ([]session.Ref, error) {
	ents, err := os.ReadDir(p.stateDir())
	if err != nil {
		return nil, nil
	}
	var refs []session.Ref
	for _, e := range ents {
		if ctx.Err() != nil {
			return refs, ctx.Err()
		}
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(p.stateDir(), e.Name())
		ev, err := os.Stat(filepath.Join(dir, "events.jsonl"))
		if err != nil || !ev.Mode().IsRegular() {
			continue // no events.jsonl: skipped
		}
		if !q.Since.IsZero() && ev.ModTime().Before(q.Since) {
			continue
		}
		ws := ReadWorkspace(filepath.Join(dir, "workspace.yaml"))
		ref := session.Ref{
			Agent: session.Copilot, ID: e.Name(), Path: dir, CWD: ws["cwd"],
			UpdatedAt: ev.ModTime(), Size: ev.Size(), StartedAt: session.ParseTime(ws["created_at"]),
		}
		if ws["id"] != "" {
			ref.ID = ws["id"]
		}
		if q.ID != "" && ref.ID != q.ID && e.Name() != q.ID {
			continue
		}
		if ref.CWD == "" {
			ref.CWD = startCWD(filepath.Join(dir, "events.jsonl"))
		}
		if q.CWD != "" && !session.SameDir(ref.CWD, q.CWD) {
			continue
		}
		refs = append(refs, ref)
	}
	session.SortRefs(refs)
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// startCWD reads session.start's context.cwd from the head of events.jsonl.
func startCWD(path string) string {
	cwd := ""
	_, _ = jsonl.Read(path, jsonl.Options{HeadBytes: 64 << 10, TailBytes: 1}, func(line []byte, off int64) error {
		if off > 64<<10 {
			return jsonl.ErrStop
		}
		var ev event
		if json.Unmarshal(line, &ev) == nil && ev.Type == "session.start" {
			var d startData
			if json.Unmarshal(ev.Data, &d) == nil {
				cwd = d.Context.CWD
			}
			return jsonl.ErrStop
		}
		return nil
	})
	return cwd
}

// --- parsing -------------------------------------------------------------

type event struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
}

type startData struct {
	Context struct {
		CWD    string `json:"cwd"`
		Branch string `json:"branch"`
	} `json:"context"`
	SelectedModel string `json:"selectedModel"`
}

type toolRequest struct {
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
	ToolCallID string         `json:"toolCallId"`
}

type assistantData struct {
	Content      string        `json:"content"`
	Model        string        `json:"model"`
	ToolRequests []toolRequest `json:"toolRequests"`
}

type completeData struct {
	ToolCallID string `json:"toolCallId"`
	Success    *bool  `json:"success"`
}

type usageData struct {
	TotalPremiumRequests *float64 `json:"totalPremiumRequests"`
}

var ignoredPrefixes = []string{
	"abort", "permission.", "hook.", "assistant.turn_", "subagent.", "skill.", "system.",
	"tool.execution_start", "tool.execution_partial", "session.info", "session.model_change",
	"session.compaction", "session.truncation", "session.error",
}

// SQLiteLookPath finds the sqlite3 binary; tests replace it.
var SQLiteLookPath = func() (string, error) { return exec.LookPath("sqlite3") }

// Load implements session.Provider. ref.Path is the session directory.
func (p *Provider) Load(ctx context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	opt = opt.WithDefaults()
	s := &session.Session{Ref: ref}
	s.Agent = session.Copilot
	warn := session.Counter{}
	ws := ReadWorkspace(filepath.Join(ref.Path, "workspace.yaml"))
	if s.CWD == "" {
		s.CWD = ws["cwd"]
	}
	if ws["branch"] != "" {
		s.GitBranch = ws["branch"]
	}
	if s.ID == "" {
		s.ID = ws["id"]
	}
	if s.ID == "" {
		s.ID = filepath.Base(ref.Path)
	}
	toolIdx := map[string][2]int{}
	first := true
	evPath := filepath.Join(ref.Path, "events.jsonl")
	st, err := jsonl.Read(evPath, jsonl.Options{HeadBytes: opt.HeadBytes, TailBytes: opt.TailBytes}, func(line []byte, _ int64) error {
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			warn.Add("malformed line(s) skipped")
			return nil
		}
		at := session.ParseTime(ev.Timestamp)
		if s.StartedAt.IsZero() && !at.IsZero() {
			s.StartedAt = at
		}
		switch ev.Type {
		case "session.start", "session.resume":
			var d startData
			if json.Unmarshal(ev.Data, &d) == nil {
				if s.CWD == "" {
					s.CWD = d.Context.CWD
				}
				if d.Context.Branch != "" && ws["branch"] == "" {
					s.GitBranch = d.Context.Branch // workspace.yaml (current) wins
				}
				if d.SelectedModel != "" {
					s.Model = d.SelectedModel
				}
			}
		case "user.message":
			var d struct {
				Content string `json:"content"`
			}
			if json.Unmarshal(ev.Data, &d) != nil {
				warn.Add("malformed event(s) skipped")
				return nil
			}
			text := clean.Text(d.Content, clean.GenericRules)
			if text == "" {
				return nil
			}
			e := session.Entry{Role: session.RoleUser, At: at, Text: text}
			if strings.HasPrefix(text, "/") && !strings.Contains(text, "\n") {
				e.Kind = session.KindSlashCommand
			}
			if first {
				first = false
				if session.IsHandoff(text) {
					e.Kind = session.KindHandoff
					s.Chained = session.ParseChained(text)
				}
			}
			s.Entries = append(s.Entries, e)
		case "assistant.message":
			var d assistantData
			if json.Unmarshal(ev.Data, &d) != nil {
				warn.Add("malformed event(s) skipped")
				return nil
			}
			if d.Model != "" {
				s.Model = d.Model
			}
			s.Usage.Replies++
			e := session.Entry{Role: session.RoleAssistant, At: at, Text: strings.TrimSpace(d.Content)}
			for _, tr := range d.ToolRequests {
				if tr.Name == "report_intent" {
					continue
				}
				e.Tools = append(e.Tools, session.ToolCall{Name: tr.Name, Args: tr.Arguments})
				if tr.ToolCallID != "" {
					toolIdx[tr.ToolCallID] = [2]int{len(s.Entries), len(e.Tools) - 1}
				}
			}
			if e.Text == "" && len(e.Tools) == 0 {
				return nil
			}
			s.Entries = append(s.Entries, e)
		case "tool.execution_complete":
			var d completeData
			if json.Unmarshal(ev.Data, &d) == nil && d.Success != nil && !*d.Success {
				if idx, ok := toolIdx[d.ToolCallID]; ok && idx[0] < len(s.Entries) && idx[1] < len(s.Entries[idx[0]].Tools) {
					s.Entries[idx[0]].Tools[idx[1]].Failed = true
				}
			}
		case "session.usage_checkpoint", "session.shutdown":
			var d usageData
			if json.Unmarshal(ev.Data, &d) == nil && d.TotalPremiumRequests != nil {
				s.Usage.PremiumRequests = *d.TotalPremiumRequests
			}
		default:
			for _, p := range ignoredPrefixes {
				if strings.HasPrefix(ev.Type, p) {
					return nil
				}
			}
			warn.Add("unknown event type(s) skipped")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.Gap = st.Gap
	if s.Size == 0 {
		s.Size = st.Size
	}
	s.Warnings = append(s.Warnings, st.Warnings()...)
	s.Todos, s.Warnings = loadTodos(ctx, filepath.Join(ref.Path, "session.db"), s.Warnings)
	s.Warnings = append(s.Warnings, warn.Warnings()...)
	if !opt.Since.IsZero() {
		s.FilterSince(opt.Since)
	}
	return s, nil
}

// loadTodos reads the task list from session.db with the sqlite3 CLI, if
// present (Q5: no SQLite driver in the binary).
func loadTodos(ctx context.Context, db string, warnings []string) ([]session.Todo, []string) {
	if _, err := os.Stat(db); err != nil {
		return nil, warnings
	}
	bin, err := SQLiteLookPath()
	if err != nil {
		return nil, append(warnings, "sqlite3 not found: Copilot task list omitted")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-readonly", "-json", db, "select title, status from todos order by rowid").Output()
	if err != nil {
		return nil, append(warnings, "could not read Copilot task list")
	}
	var rows []struct {
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, warnings
	}
	if json.Unmarshal(out, &rows) != nil {
		return nil, append(warnings, "could not parse Copilot task list")
	}
	var todos []session.Todo
	for _, r := range rows {
		if r.Title != "" {
			todos = append(todos, session.Todo{Text: r.Title, Status: session.ParseTodoStatus(r.Status)})
		}
	}
	return todos, warnings
}

// Preview implements session.Previewer.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	text := ""
	_, err := jsonl.Read(filepath.Join(ref.Path, "events.jsonl"), jsonl.Options{HeadBytes: 256 << 10, TailBytes: 1}, func(line []byte, off int64) error {
		if off > 256<<10 {
			return jsonl.ErrStop
		}
		var ev event
		if json.Unmarshal(line, &ev) != nil || ev.Type != "user.message" {
			return nil
		}
		var d struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return nil
		}
		t := clean.Text(d.Content, clean.GenericRules)
		if t == "" || (strings.HasPrefix(t, "/") && !strings.Contains(t, "\n")) {
			return nil
		}
		if c := session.ParseChained(t); c != nil {
			t = c.OriginalRequest
		}
		text = t
		return jsonl.ErrStop
	})
	return text, err
}
