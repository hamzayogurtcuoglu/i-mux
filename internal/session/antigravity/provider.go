package antigravity

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
)

// Antigravity CLI keeps every conversation in brain/<conversation-id>/ under
// ~/.gemini/antigravity-cli (the Antigravity app uses ~/.gemini/antigravity):
//
//	.system_generated/logs/transcript_full.jsonl  one step per line (also transcript.jsonl)
//	task.md                                       the agent's plan as a checklist
//
// A step is {step_index, source, type, status, created_at, content, ...}:
//
//	USER_EXPLICIT / USER_INPUT       content wraps the request in <USER_REQUEST>
//	MODEL / PLANNER_RESPONSE         content (text), thinking, tool_calls [{name, args}], token counts
//	MODEL / GENERIC, LIST_DIRECTORY  a tool's result, in tool-call order; status ERROR with error
//	SYSTEM / SYSTEM_MESSAGE, ...     skipped
//
// The transcript has no working directory; history.jsonl next to brain/ has
// {display, timestamp (ms), workspace} per typed prompt, matched by text and
// time. Verified against Antigravity CLI 1.5.2 on 2026-11-06.

func init() { session.Register(session.Antigravity, New) }

// Provider reads Antigravity CLI's brain directory.
type Provider struct {
	root string // ~/.gemini/antigravity-cli
	app  string // ~/.gemini/antigravity (the desktop app; no history.jsonl)
}

// New returns a provider for env.
func New(env session.Env) session.Provider {
	root := env.Resolve(session.Antigravity)
	return &Provider{root: root, app: filepath.Join(filepath.Dir(root), "antigravity")}
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.Antigravity }

const logsDir = ".system_generated/logs"

// transcriptOf returns the transcript file of a conversation directory, or "".
func transcriptOf(dir string) string {
	for _, name := range []string{"transcript_full.jsonl", "transcript.jsonl"} {
		p := filepath.Join(dir, logsDir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

// convDir is the brain/<id> directory of a transcript path.
func convDir(transcript string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(transcript)))
}

type step struct {
	StepIndex int             `json:"step_index"`
	Source    string          `json:"source"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	CreatedAt string          `json:"created_at"`
	Content   json.RawMessage `json:"content"`
	Error     json.RawMessage `json:"error"`
	Input     int64           `json:"input_tokens"`
	CacheRead int64           `json:"cache_read_tokens"`
	Output    int64           `json:"output_tokens"`
	ToolCalls []struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"tool_calls"`
}

// text returns a raw JSON value as text: strings as-is, anything else as JSON.
func text(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

var (
	userRequestRe = regexp.MustCompile(`(?s)<USER_REQUEST>\s*(.*?)\s*</USER_REQUEST>`)
	// Context Antigravity adds around a request: not the user's words.
	wrapperRe = regexp.MustCompile(`(?s)<(ADDITIONAL_METADATA|USER_SETTINGS_CHANGE|EPHEMERAL_MESSAGE|USER_RULES|MEMORIES)>.*?</(ADDITIONAL_METADATA|USER_SETTINGS_CHANGE|EPHEMERAL_MESSAGE|USER_RULES|MEMORIES)>`)
)

// userText extracts what the user typed from a USER_INPUT step.
func userText(content string) string {
	if m := userRequestRe.FindStringSubmatch(content); m != nil {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(wrapperRe.ReplaceAllString(content, ""))
}

// argAliases maps Antigravity's argument names to the ones the tool
// summarizer understands, so "$ npm test" and "read src/a.ts" render as for
// the other agents.
var argAliases = map[string]string{
	"CommandLine":   "command",
	"Path":          "file_path",
	"AbsolutePath":  "file_path",
	"TargetFile":    "file_path",
	"FilePath":      "file_path",
	"DirectoryPath": "path",
	"SearchPath":    "path",
	"SearchDir":     "path",
	"Query":         "query",
	"Pattern":       "pattern",
	"Url":           "url",
	"URL":           "url",
}

func decodeArgs(raw json.RawMessage) map[string]any {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		var s string
		if json.Unmarshal(raw, &s) != nil || json.Unmarshal([]byte(s), &m) != nil {
			return map[string]any{}
		}
	}
	if m == nil {
		return map[string]any{}
	}
	for k, alias := range argAliases {
		if v, ok := m[k]; ok {
			if _, taken := m[alias]; !taken {
				m[alias] = v
			}
		}
	}
	return m
}

// histEntry is one line of history.jsonl.
type histEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Workspace string `json:"workspace"`
}

// history reads the newest part of history.jsonl.
func (p *Provider) history() []histEntry {
	var out []histEntry
	_, _ = jsonl.Read(filepath.Join(p.root, "history.jsonl"), jsonl.Options{HeadBytes: 0, TailBytes: 4 << 20}, func(b []byte, _ int64) error {
		var h histEntry
		if json.Unmarshal(b, &h) == nil && h.Workspace != "" {
			out = append(out, h)
		}
		return nil
	})
	return out
}

func normSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// workspaceFor finds the directory of a conversation: the history entry
// with the same text close in time, else the closest entry within a minute.
func workspaceFor(hist []histEntry, req string, at time.Time) string {
	if at.IsZero() {
		return ""
	}
	want := normSpace(req)
	best, bestD := "", time.Duration(-1)
	for _, h := range hist {
		d := time.UnixMilli(h.Timestamp).Sub(at)
		if d < 0 {
			d = -d
		}
		if want != "" && normSpace(h.Display) == want && d <= 10*time.Minute {
			return h.Workspace
		}
		if d <= time.Minute && (bestD < 0 || d < bestD) {
			best, bestD = h.Workspace, d
		}
	}
	return best
}

// head reads the first steps of a transcript: when it started and the first
// request.
func head(path string) (start time.Time, req string, reqAt time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	r := bufio.NewReaderSize(io.LimitReader(f, 4<<20), 64<<10)
	for i := 0; i < 50; i++ {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var s step
			if json.Unmarshal(line, &s) == nil {
				at := session.ParseTime(s.CreatedAt)
				if start.IsZero() {
					start = at
				}
				if s.Type == "USER_INPUT" {
					return start, userText(text(s.Content)), at
				}
			}
		}
		if err != nil {
			return
		}
	}
	return
}

// Find implements session.Provider. Conversations whose directory is
// unknown (the app's, or a prompt missing from history.jsonl) match a query
// for a directory only when it also bounds the time (a pane's run).
func (p *Provider) Find(ctx context.Context, q session.Query) ([]session.Ref, error) {
	hist := p.history()
	var refs []session.Ref
	for _, brain := range []string{filepath.Join(p.root, "brain"), filepath.Join(p.app, "brain")} {
		ents, err := os.ReadDir(brain)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if ctx.Err() != nil {
				return refs, ctx.Err()
			}
			if !e.IsDir() || (q.ID != "" && e.Name() != q.ID) {
				continue
			}
			tp := transcriptOf(filepath.Join(brain, e.Name()))
			if tp == "" {
				continue
			}
			fi, err := os.Stat(tp)
			if err != nil || (!q.Since.IsZero() && fi.ModTime().Before(q.Since)) {
				continue
			}
			start, req, reqAt := head(tp)
			cwd := workspaceFor(hist, req, reqAt)
			if q.CWD != "" && q.ID == "" {
				if cwd != "" && !session.SameDir(cwd, q.CWD) {
					continue
				}
				if cwd == "" && q.Since.IsZero() {
					continue
				}
			}
			if start.IsZero() {
				start = fi.ModTime()
			}
			refs = append(refs, session.Ref{
				Agent: session.Antigravity, ID: e.Name(), Path: tp, CWD: cwd,
				StartedAt: start, UpdatedAt: fi.ModTime(), Size: fi.Size(),
			})
		}
	}
	session.SortRefs(refs)
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

type parser struct {
	s       *session.Session
	pending [][2]int // tool calls awaiting their result step: entry, call
	first   bool
	warn    session.Counter
}

// Load implements session.Provider.
func (p *Provider) Load(ctx context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	opt = opt.WithDefaults()
	s := &session.Session{Ref: ref}
	s.Agent = session.Antigravity
	ps := &parser{s: s, first: true, warn: session.Counter{}}
	st, err := jsonl.Read(ref.Path, jsonl.Options{HeadBytes: opt.HeadBytes, TailBytes: opt.TailBytes}, func(b []byte, _ int64) error {
		ps.line(b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.Gap = st.Gap
	s.Size = st.Size
	s.Warnings = append(s.Warnings, st.Warnings()...)
	s.Warnings = append(s.Warnings, ps.warn.Warnings()...)
	if s.ID == "" {
		s.ID = filepath.Base(convDir(ref.Path))
	}
	s.Todos = ParseTaskList(filepath.Join(convDir(ref.Path), "task.md"))
	if !opt.Since.IsZero() {
		s.FilterSince(opt.Since)
	}
	return s, nil
}

func (ps *parser) line(b []byte) {
	var st step
	if err := json.Unmarshal(b, &st); err != nil {
		ps.warn.Add("malformed line(s) skipped")
		return
	}
	s := ps.s
	at := session.ParseTime(st.CreatedAt)
	if s.StartedAt.IsZero() {
		s.StartedAt = at
	}
	switch {
	case st.Type == "USER_INPUT":
		ps.pending = nil
		t := clean.Text(userText(text(st.Content)), clean.GenericRules)
		if t == "" {
			return
		}
		e := session.Entry{Role: session.RoleUser, At: at, Text: t}
		if ps.first {
			ps.first = false
			if session.IsHandoff(t) {
				e.Kind = session.KindHandoff
				s.Chained = session.ParseChained(t)
			}
		}
		s.Entries = append(s.Entries, e)
	case st.Type == "PLANNER_RESPONSE":
		s.Usage.Replies++
		s.Usage.InputTokens += st.Input
		s.Usage.CacheReadTokens += st.CacheRead
		s.Usage.OutputTokens += st.Output
		e := session.Entry{Role: session.RoleAssistant, At: at, Text: strings.TrimSpace(text(st.Content))}
		for _, tc := range st.ToolCalls {
			if tc.Name == "" {
				continue
			}
			e.Tools = append(e.Tools, session.ToolCall{Name: tc.Name, Args: decodeArgs(tc.Args)})
		}
		if e.Text == "" && len(e.Tools) == 0 {
			return
		}
		s.Entries = append(s.Entries, e)
		ps.pending = nil
		for i := range e.Tools {
			ps.pending = append(ps.pending, [2]int{len(s.Entries) - 1, i})
		}
	case st.Source == "MODEL":
		// A tool's result, in the order the calls were made.
		var idx [2]int
		if len(ps.pending) > 0 {
			idx, ps.pending = ps.pending[0], ps.pending[1:]
		} else {
			idx = [2]int{-1, -1}
		}
		if st.Status != "ERROR" {
			return
		}
		if idx[0] >= 0 {
			s.Entries[idx[0]].Tools[idx[1]].Failed = true
		}
		if msg := text(st.Error); msg != "" {
			if e := (session.APIError{At: at, Code: "error", Text: msg}); e.LimitLike() {
				s.Errors = append(s.Errors, e)
			}
		}
	default:
		ps.warn.Add("step(s) of other types skipped")
	}
}

// Preview implements session.Previewer: the first request.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	_, req, _ := head(ref.Path)
	if session.IsHandoff(req) {
		if c := session.ParseChained(req); c != nil && c.OriginalRequest != "" {
			return c.OriginalRequest, nil
		}
	}
	return req, nil
}
