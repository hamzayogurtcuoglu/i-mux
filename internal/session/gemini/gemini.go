// Package gemini parses Gemini CLI chats:
// ~/.gemini/tmp/<project-id>/chats/session-<ts>-<shortid>.json[l] (§7.2).
package gemini

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
)

func init() { session.Register(session.Gemini, New) }

// MaxJSONBytes caps whole-file reads of .json chats.
const MaxJSONBytes = 64 << 20

// Provider reads Gemini CLI's chat store.
type Provider struct {
	root string // ~/.gemini
}

// New returns a provider for env.
func New(env session.Env) session.Provider {
	return &Provider{root: env.Resolve(session.Gemini)}
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.Gemini }

func (p *Provider) tmp() string { return filepath.Join(p.root, "tmp") }

// ProjectHash is sha256(cwd) in hex, Gemini's legacy project id.
func ProjectHash(cwd string) string {
	h := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(h[:])
}

// ProjectDirs returns the project directories for cwd, in lookup order:
// projects.json registry, the sha256 directory, then a .project_root scan.
func (p *Provider) ProjectDirs(cwd string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(d string) {
		if d == "" || seen[d] {
			return
		}
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			seen[d] = true
			out = append(out, d)
		}
	}
	if b, err := os.ReadFile(filepath.Join(p.root, "projects.json")); err == nil {
		var reg struct {
			Projects map[string]string `json:"projects"`
		}
		if json.Unmarshal(b, &reg) == nil {
			for k, v := range reg.Projects {
				if session.SameDir(k, cwd) && v != "" && !strings.ContainsAny(v, `/\`) && v != ".." {
					add(filepath.Join(p.tmp(), v))
				}
			}
		}
	}
	add(filepath.Join(p.tmp(), ProjectHash(cwd)))
	for _, d := range p.allProjectDirs() {
		if seen[d] {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(d, ".project_root")); err == nil && session.SameDir(strings.TrimSpace(string(b)), cwd) {
			add(d)
		}
	}
	return out
}

func (p *Provider) allProjectDirs() []string {
	ents, err := os.ReadDir(p.tmp())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, filepath.Join(p.tmp(), e.Name()))
		}
	}
	return out
}

func isChat(name string) bool {
	return strings.HasPrefix(name, "session-") && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonl"))
}

var (
	sessionIDRe = regexp.MustCompile(`"sessionId"\s*:\s*"([^"]+)"`)
	startTimeRe = regexp.MustCompile(`"startTime"\s*:\s*"([^"]+)"`)
)

// Find implements session.Provider.
func (p *Provider) Find(ctx context.Context, q session.Query) ([]session.Ref, error) {
	dirs := p.allProjectDirs()
	if q.CWD != "" {
		dirs = p.ProjectDirs(q.CWD)
	}
	var refs []session.Ref
	for _, d := range dirs {
		cwd := q.CWD
		if cwd == "" {
			if b, err := os.ReadFile(filepath.Join(d, ".project_root")); err == nil {
				cwd = strings.TrimSpace(string(b))
			}
		}
		for _, f := range session.ListFiles(filepath.Join(d, "chats"), isChat) {
			if ctx.Err() != nil {
				return refs, ctx.Err()
			}
			if !q.Since.IsZero() && f.ModTime.Before(q.Since) {
				continue
			}
			head := readHead(f.Path, 4096)
			ref := session.Ref{Agent: session.Gemini, Path: f.Path, CWD: cwd, UpdatedAt: f.ModTime, Size: f.Size}
			if m := sessionIDRe.FindSubmatch(head); m != nil {
				ref.ID = string(m[1])
			}
			if m := startTimeRe.FindSubmatch(head); m != nil {
				ref.StartedAt = session.ParseTime(string(m[1]))
			}
			if ref.ID == "" {
				ref.ID = shortID(f.Path)
			}
			if q.ID != "" && ref.ID != q.ID && !strings.HasPrefix(q.ID, shortID(f.Path)) {
				continue
			}
			refs = append(refs, ref)
		}
	}
	session.SortRefs(refs)
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// shortID returns the short id at the end of a chat file name.
func shortID(path string) string {
	n := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".jsonl"), ".json")
	if i := strings.LastIndexByte(n, '-'); i >= 0 {
		return n[i+1:]
	}
	return n
}

func readHead(path string, n int) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, n)
	k, _ := io.ReadFull(f, b)
	return b[:k]
}

// --- parsing -------------------------------------------------------------

type toolCall struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	Status string         `json:"status"`
}

type tokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Cached int64 `json:"cached"`
}

type message struct {
	ID             string          `json:"id"`
	Timestamp      string          `json:"timestamp"`
	Type           string          `json:"type"`
	Content        json.RawMessage `json:"content"`
	DisplayContent json.RawMessage `json:"displayContent"`
	Model          string          `json:"model"`
	Tokens         *tokens         `json:"tokens"`
	ToolCalls      []toolCall      `json:"toolCalls"`
}

type record struct {
	SessionID   string    `json:"sessionId"`
	StartTime   string    `json:"startTime"`
	LastUpdated string    `json:"lastUpdated"`
	Messages    []message `json:"messages"`
}

// messageSet keeps messages keyed by id, last write wins, first-seen order.
type messageSet struct {
	order []string
	byID  map[string]message
	anon  int
}

func newMessageSet() *messageSet { return &messageSet{byID: map[string]message{}} }

func (ms *messageSet) put(m message) {
	key := m.ID
	if key == "" {
		ms.anon++
		key = fmt.Sprintf("\x00anon-%d", ms.anon)
	}
	if _, ok := ms.byID[key]; !ok {
		ms.order = append(ms.order, key)
	}
	ms.byID[key] = m
}

func (ms *messageSet) list() []message {
	out := make([]message, 0, len(ms.order))
	for _, k := range ms.order {
		out = append(out, ms.byID[k])
	}
	return out
}

// Load implements session.Provider.
func (p *Provider) Load(ctx context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	opt = opt.WithDefaults()
	s := &session.Session{Ref: ref}
	s.Agent = session.Gemini
	warn := session.Counter{}
	ms := newMessageSet()
	var rec record
	if strings.HasSuffix(ref.Path, ".jsonl") {
		st, err := jsonl.Read(ref.Path, jsonl.Options{HeadBytes: opt.HeadBytes, TailBytes: opt.TailBytes}, func(line []byte, _ int64) error {
			parseJSONLLine(line, &rec, ms, warn)
			return nil
		})
		if err != nil {
			return nil, err
		}
		s.Gap = st.Gap
		s.Size = st.Size
		s.Warnings = append(s.Warnings, st.Warnings()...)
	} else {
		fi, err := os.Stat(ref.Path)
		if err != nil {
			return nil, err
		}
		s.Size = fi.Size()
		if fi.Size() > MaxJSONBytes {
			s.Warnings = append(s.Warnings, fmt.Sprintf("chat file larger than %d MiB not parsed", MaxJSONBytes>>20))
			return s, nil
		}
		b, err := os.ReadFile(ref.Path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &rec); err != nil {
			return nil, fmt.Errorf("gemini chat %s: %w", filepath.Base(ref.Path), err)
		}
		for _, m := range rec.Messages {
			ms.put(m)
		}
	}
	if rec.SessionID != "" {
		s.ID = rec.SessionID
	}
	if t := session.ParseTime(rec.StartTime); !t.IsZero() {
		s.StartedAt = t
	}
	if t := session.ParseTime(rec.LastUpdated); !t.IsZero() && s.UpdatedAt.IsZero() {
		s.UpdatedAt = t
	}
	build(s, ms.list(), warn)
	s.Warnings = append(s.Warnings, warn.Warnings()...)
	if !opt.Since.IsZero() {
		s.FilterSince(opt.Since)
	}
	return s, nil
}

func parseJSONLLine(line []byte, rec *record, ms *messageSet, warn session.Counter) {
	var probe struct {
		Kind      string          `json:"kind"`
		Set       json.RawMessage `json:"$set"`
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		SessionID string          `json:"sessionId"`
		StartTime string          `json:"startTime"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		warn.Add("malformed line(s) skipped")
		return
	}
	switch {
	case probe.Kind != "" || probe.SessionID != "":
		if probe.SessionID != "" {
			rec.SessionID = probe.SessionID
		}
		if probe.StartTime != "" {
			rec.StartTime = probe.StartTime
		}
	case len(probe.Set) > 0:
		var set struct {
			LastUpdated string    `json:"lastUpdated"`
			Messages    []message `json:"messages"`
		}
		if err := json.Unmarshal(probe.Set, &set); err != nil {
			warn.Add("malformed patch(es) skipped")
			return
		}
		if set.LastUpdated != "" {
			rec.LastUpdated = set.LastUpdated
		}
		for _, m := range set.Messages {
			ms.put(m)
		}
	case probe.Type != "":
		var m message
		if json.Unmarshal(line, &m) == nil {
			ms.put(m)
		}
	default:
		warn.Add("unknown line(s) skipped")
	}
}

// contentText renders Gemini content: a string, a part list, or one part.
func contentText(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	case '[':
		var parts []json.RawMessage
		if json.Unmarshal(raw, &parts) != nil {
			return ""
		}
		var out []string
		for _, p := range parts {
			if t := partText(p); t != "" {
				out = append(out, t)
			}
		}
		return strings.Join(out, "\n")
	case '{':
		return partText(raw)
	}
	return ""
}

func partText(raw json.RawMessage) string {
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	var p map[string]json.RawMessage
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	if t, ok := p["text"]; ok {
		var s string
		_ = json.Unmarshal(t, &s)
		return s
	}
	if _, ok := p["thought"]; ok {
		return ""
	}
	if _, ok := p["functionCall"]; ok {
		return ""
	}
	if _, ok := p["functionResponse"]; ok {
		return ""
	}
	return "[attachment]"
}

func build(s *session.Session, msgs []message, warn session.Counter) {
	first := true
	for _, m := range msgs {
		at := session.ParseTime(m.Timestamp)
		switch m.Type {
		case "user":
			raw := m.DisplayContent
			if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
				raw = m.Content
			}
			text := clean.Text(contentText(raw), clean.GenericRules)
			if text == "" {
				continue
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
		case "gemini", "model", "assistant":
			e := session.Entry{Role: session.RoleAssistant, At: at, Text: strings.TrimSpace(contentText(m.Content))}
			for _, tc := range m.ToolCalls {
				e.Tools = append(e.Tools, session.ToolCall{Name: tc.Name, Args: tc.Args, Failed: tc.Status == "error" || tc.Status == "cancelled"})
				if tc.Name == "write_todos" {
					if todos := session.TodosFromArgs(tc.Args); todos != nil {
						s.Todos = todos
					}
				}
			}
			if m.Model != "" {
				s.Model = m.Model
			}
			if m.Tokens != nil {
				s.Usage.InputTokens += m.Tokens.Input
				s.Usage.OutputTokens += m.Tokens.Output
				s.Usage.CacheReadTokens += m.Tokens.Cached
			}
			s.Usage.Replies++
			if e.Text == "" && len(e.Tools) == 0 {
				continue
			}
			s.Entries = append(s.Entries, e)
		case "info", "warning":
			if t := strings.TrimSpace(contentText(m.Content)); t != "" {
				s.Entries = append(s.Entries, session.Entry{Role: session.RoleNotice, At: at, Text: t})
			}
		case "error":
			s.Errors = append(s.Errors, session.APIError{At: at, Code: "error", Text: strings.TrimSpace(contentText(m.Content))})
		default:
			warn.Add("unknown message type(s) skipped")
		}
	}
}

// Preview implements session.Previewer: the first user request without
// reading the whole file.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	f, err := os.Open(ref.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := bufio.NewReaderSize(io.LimitReader(f, 4<<20), 64<<10)
	var msgs []message
	if strings.HasSuffix(ref.Path, ".jsonl") {
		ms := newMessageSet()
		var rec record
		warn := session.Counter{}
		_, err := jsonl.Scan(r, 0, func(line []byte, _ int64) error {
			parseJSONLLine(line, &rec, ms, warn)
			for _, m := range ms.list() {
				if m.Type == "user" {
					return jsonl.ErrStop
				}
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		msgs = ms.list()
	} else {
		m, err := firstUserInJSON(r)
		if err != nil {
			return "", err
		}
		if m != nil {
			msgs = []message{*m}
		}
	}
	s := &session.Session{}
	build(s, msgs, session.Counter{})
	return s.FirstUserText(), nil
}

// firstUserInJSON streams a chat record up to its first user message.
func firstUserInJSON(r io.Reader) (*message, error) {
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, err
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := t.(string)
		if key != "messages" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, err
			}
			continue
		}
		if t, err := dec.Token(); err != nil || t != json.Delim('[') {
			return nil, err
		}
		for dec.More() {
			var m message
			if err := dec.Decode(&m); err != nil {
				return nil, err
			}
			if m.Type == "user" {
				return &m, nil
			}
		}
		return nil, nil
	}
	return nil, nil
}
