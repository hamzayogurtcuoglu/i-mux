// Package codex parses OpenAI Codex CLI rollouts:
// ~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl (§7.4).
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
)

func init() { session.Register(session.Codex, New) }

// Provider reads Codex's rollout store.
type Provider struct {
	root string // $CODEX_HOME or ~/.codex
}

// New returns a provider for env.
func New(env session.Env) session.Provider {
	return &Provider{root: env.Resolve(session.Codex)}
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.Codex }

func (p *Provider) sessions() string { return filepath.Join(p.root, "sessions") }

type meta struct {
	ID        string `json:"id"`
	CWD       string `json:"cwd"`
	Timestamp string `json:"timestamp"`
}

// readMeta returns the session_meta payload from the first line.
func readMeta(path string) (meta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return meta{}, false
	}
	defer f.Close()
	r := bufio.NewReaderSize(io.LimitReader(f, 4<<20), 64<<10)
	line, err := r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return meta{}, false
	}
	var l struct {
		Type    string `json:"type"`
		Payload meta   `json:"payload"`
	}
	if json.Unmarshal(line, &l) != nil || l.Type != "session_meta" {
		return meta{}, false
	}
	return l.Payload, true
}

func sortedDirs(dir string, desc bool) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	if desc {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

// uuidFromName extracts the trailing UUID of a rollout file name.
func uuidFromName(name string) string {
	n := strings.TrimSuffix(name, ".jsonl")
	if len(n) >= 36 {
		return n[len(n)-36:]
	}
	return ""
}

// Find implements session.Provider: a date-directory walk, newest first,
// stopping early before day directories older than q.Since.
func (p *Provider) Find(ctx context.Context, q session.Query) ([]session.Ref, error) {
	var refs []session.Ref
	var sinceDay string
	if !q.Since.IsZero() {
		// A day earlier than Since covers time-zone differences between
		// the directory date and UTC timestamps.
		sinceDay = q.Since.Add(-24 * time.Hour).Format("2006/01/02")
	}
	for _, y := range sortedDirs(p.sessions(), true) {
		for _, m := range sortedDirs(filepath.Join(p.sessions(), y), true) {
			for _, d := range sortedDirs(filepath.Join(p.sessions(), y, m), true) {
				if sinceDay != "" && y+"/"+m+"/"+d < sinceDay {
					return finish(refs, q), nil
				}
				if ctx.Err() != nil {
					return refs, ctx.Err()
				}
				dir := filepath.Join(p.sessions(), y, m, d)
				files := session.ListFiles(dir, func(n string) bool {
					return strings.HasPrefix(n, "rollout-") && strings.HasSuffix(n, ".jsonl")
				})
				sort.Slice(files, func(i, j int) bool { return files[i].Path > files[j].Path })
				for _, f := range files {
					if !q.Since.IsZero() && f.ModTime.Before(q.Since) {
						continue
					}
					if q.ID != "" && uuidFromName(filepath.Base(f.Path)) != q.ID {
						continue
					}
					md, ok := readMeta(f.Path)
					if !ok {
						continue
					}
					if q.ID != "" && md.ID != "" && md.ID != q.ID {
						continue
					}
					if q.CWD != "" && !session.SameDir(md.CWD, q.CWD) {
						continue
					}
					id := md.ID
					if id == "" {
						id = uuidFromName(filepath.Base(f.Path))
					}
					refs = append(refs, session.Ref{
						Agent: session.Codex, ID: id, Path: f.Path, CWD: md.CWD,
						StartedAt: session.ParseTime(md.Timestamp), UpdatedAt: f.ModTime, Size: f.Size,
					})
					if q.ID != "" {
						return finish(refs, q), nil
					}
				}
			}
		}
	}
	return finish(refs, q), nil
}

func finish(refs []session.Ref, q session.Query) []session.Ref {
	session.SortRefs(refs)
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs
}

// --- parsing -------------------------------------------------------------

type line struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type part struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type item struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   []part          `json:"content"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Action    *struct {
		Type    string   `json:"type"`
		Command []string `json:"command"`
	} `json:"action"`
}

type rateWindow struct {
	UsedPercent     float64         `json:"used_percent"`
	WindowMinutes   int             `json:"window_minutes"`
	ResetsInSeconds *float64        `json:"resets_in_seconds"`
	ResetsAt        json.RawMessage `json:"resets_at"`
}

type eventMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Info    *struct {
		TotalTokenUsage *struct {
			InputTokens       int64 `json:"input_tokens"`
			CachedInputTokens int64 `json:"cached_input_tokens"`
			OutputTokens      int64 `json:"output_tokens"`
		} `json:"total_token_usage"`
	} `json:"info"`
	RateLimits *struct {
		Primary   *rateWindow `json:"primary"`
		Secondary *rateWindow `json:"secondary"`
	} `json:"rate_limits"`
}

func resetTime(w *rateWindow, at time.Time) time.Time {
	if w == nil {
		return time.Time{}
	}
	if w.ResetsInSeconds != nil {
		return at.Add(time.Duration(*w.ResetsInSeconds * float64(time.Second)))
	}
	raw := strings.Trim(string(w.ResetsAt), `"`)
	if raw == "" || raw == "null" {
		return time.Time{}
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n).UTC()
		}
		return time.Unix(n, 0).UTC()
	}
	return session.ParseTime(raw)
}

func isContextMessage(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "<environment_context>") || strings.HasPrefix(t, "<user_instructions>") ||
		strings.HasPrefix(t, "# AGENTS.md instructions")
}

type parser struct {
	s       *session.Session
	callIdx map[string][2]int
	first   bool
	warn    session.Counter
}

// Load implements session.Provider. The transcript is built only from
// response_item lines; event_msg user/agent messages duplicate them.
func (p *Provider) Load(ctx context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	opt = opt.WithDefaults()
	s := &session.Session{Ref: ref}
	s.Agent = session.Codex
	ps := &parser{s: s, callIdx: map[string][2]int{}, first: true, warn: session.Counter{}}
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
		s.ID = uuidFromName(filepath.Base(ref.Path))
	}
	if !opt.Since.IsZero() {
		s.FilterSince(opt.Since)
	}
	return s, nil
}

func (ps *parser) line(b []byte) {
	var l line
	if err := json.Unmarshal(b, &l); err != nil {
		ps.warn.Add("malformed line(s) skipped")
		return
	}
	s := ps.s
	at := session.ParseTime(l.Timestamp)
	switch l.Type {
	case "session_meta":
		var m meta
		if json.Unmarshal(l.Payload, &m) == nil {
			if m.ID != "" {
				s.ID = m.ID
			}
			if s.CWD == "" {
				s.CWD = m.CWD
			}
			if t := session.ParseTime(m.Timestamp); !t.IsZero() {
				s.StartedAt = t
			}
		}
	case "turn_context":
		var tc struct {
			CWD   string `json:"cwd"`
			Model string `json:"model"`
		}
		if json.Unmarshal(l.Payload, &tc) == nil {
			if tc.Model != "" {
				s.Model = tc.Model
			}
			if s.CWD == "" {
				s.CWD = tc.CWD
			}
		}
	case "response_item":
		var it item
		if json.Unmarshal(l.Payload, &it) != nil {
			ps.warn.Add("malformed item(s) skipped")
			return
		}
		ps.item(&it, at)
	case "event_msg":
		var ev eventMsg
		if json.Unmarshal(l.Payload, &ev) != nil {
			return
		}
		switch ev.Type {
		case "token_count":
			if ev.Info != nil && ev.Info.TotalTokenUsage != nil {
				u := ev.Info.TotalTokenUsage
				s.Usage.InputTokens = u.InputTokens
				s.Usage.CacheReadTokens = u.CachedInputTokens
				s.Usage.OutputTokens = u.OutputTokens
			}
			if rl := ev.RateLimits; rl != nil && (rl.Primary != nil || rl.Secondary != nil) {
				snap := &session.RateLimitSnapshot{At: at}
				if rl.Primary != nil {
					snap.PrimaryUsedPercent = rl.Primary.UsedPercent
					snap.PrimaryWindowMinutes = rl.Primary.WindowMinutes
					snap.PrimaryResetsAt = resetTime(rl.Primary, at)
				}
				if rl.Secondary != nil {
					snap.SecondaryUsedPercent = rl.Secondary.UsedPercent
					snap.SecondaryWindowMins = rl.Secondary.WindowMinutes
					snap.SecondaryResetsAt = resetTime(rl.Secondary, at)
				}
				s.Usage.RateLimits = snap
			}
		case "error", "stream_error":
			if ev.Message != "" {
				s.Errors = append(s.Errors, session.APIError{At: at, Code: ev.Type, Text: ev.Message})
			}
		}
	case "compacted":
		var c struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(l.Payload, &c) == nil && c.Message != "" {
			s.Compaction = &session.Compaction{At: at, Trigger: "auto", Summary: c.Message}
			s.Omitted += len(s.Entries)
			s.Entries = nil
			ps.callIdx = map[string][2]int{}
		}
	default:
		ps.warn.Add("unknown line type(s) skipped")
	}
}

func (ps *parser) item(it *item, at time.Time) {
	s := ps.s
	switch it.Type {
	case "message":
		var texts []string
		for _, p := range it.Content {
			switch p.Type {
			case "input_text", "output_text", "text":
				texts = append(texts, p.Text)
			case "input_image":
				texts = append(texts, "[image]")
			}
		}
		text := strings.TrimSpace(strings.Join(texts, "\n"))
		switch it.Role {
		case "user":
			if text == "" || isContextMessage(text) {
				return
			}
			text = clean.Text(text, clean.GenericRules)
			if text == "" {
				return
			}
			e := session.Entry{Role: session.RoleUser, At: at, Text: text}
			if ps.first {
				ps.first = false
				if session.IsHandoff(text) {
					e.Kind = session.KindHandoff
					s.Chained = session.ParseChained(text)
				}
			}
			s.Entries = append(s.Entries, e)
		case "assistant":
			s.Usage.Replies++
			if text == "" {
				return
			}
			s.Entries = append(s.Entries, session.Entry{Role: session.RoleAssistant, At: at, Text: text})
		}
	case "function_call", "custom_tool_call", "local_shell_call":
		tc := session.ToolCall{Name: it.Name}
		switch it.Type {
		case "function_call":
			tc.Args = decodeArgs(it.Arguments)
		case "custom_tool_call":
			tc.Args = map[string]any{"input": it.Input}
		case "local_shell_call":
			tc.Name = "shell"
			if it.Action != nil {
				argv := make([]any, len(it.Action.Command))
				for i, a := range it.Action.Command {
					argv[i] = a
				}
				tc.Args = map[string]any{"command": argv}
			}
		}
		if tc.Name == "update_plan" {
			if todos := session.TodosFromArgs(tc.Args); todos != nil {
				s.Todos = todos
			}
		}
		n := len(s.Entries)
		if n == 0 || s.Entries[n-1].Role != session.RoleAssistant {
			s.Entries = append(s.Entries, session.Entry{Role: session.RoleAssistant, At: at})
			n++
		}
		s.Entries[n-1].Tools = append(s.Entries[n-1].Tools, tc)
		if it.CallID != "" {
			ps.callIdx[it.CallID] = [2]int{n - 1, len(s.Entries[n-1].Tools) - 1}
		}
	case "function_call_output", "custom_tool_call_output":
		if idx, ok := ps.callIdx[it.CallID]; ok && failedOutput(it.Output) &&
			idx[0] < len(s.Entries) && idx[1] < len(s.Entries[idx[0]].Tools) {
			s.Entries[idx[0]].Tools[idx[1]].Failed = true
		}
	case "reasoning":
	default:
		ps.warn.Add("unknown item type(s) skipped")
	}
}

// decodeArgs decodes Codex's JSON-string arguments (or an object).
func decodeArgs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var str string
	if json.Unmarshal(raw, &str) == nil {
		var m map[string]any
		if json.Unmarshal([]byte(str), &m) == nil {
			return m
		}
		return map[string]any{"arguments": str}
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		return m
	}
	return nil
}

// failedOutput reports whether a tool output signals failure: a non-zero
// exit code or success=false, possibly inside a JSON string.
func failedOutput(raw json.RawMessage) bool {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return false
	}
	if s, ok := v.(string); ok {
		if json.Unmarshal([]byte(s), &v) != nil {
			return false
		}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if b, ok := m["success"].(bool); ok && !b {
		return true
	}
	if md, ok := m["metadata"].(map[string]any); ok {
		if code, ok := md["exit_code"].(float64); ok && code != 0 {
			return true
		}
	}
	return false
}

// Preview implements session.Previewer.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	ps := &parser{s: &session.Session{}, callIdx: map[string][2]int{}, first: true, warn: session.Counter{}}
	_, err := jsonl.Read(ref.Path, jsonl.Options{HeadBytes: 512 << 10, TailBytes: 1}, func(b []byte, off int64) error {
		if off > 512<<10 {
			return jsonl.ErrStop
		}
		ps.line(b)
		if ps.s.FirstUserText() != "" {
			return jsonl.ErrStop
		}
		return nil
	})
	return ps.s.FirstUserText(), err
}
