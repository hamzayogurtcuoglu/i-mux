// Package claude parses Claude Code sessions:
// ~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl (§7.1).
package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/jsonl"
)

func init() { session.Register(session.Claude, New) }

// Provider reads Claude Code's session store.
type Provider struct {
	root string // $CLAUDE_CONFIG_DIR or ~/.claude
}

// New returns a provider for env.
func New(env session.Env) session.Provider {
	return &Provider{root: env.Resolve(session.Claude)}
}

// Agent implements session.Provider.
func (p *Provider) Agent() session.AgentID { return session.Claude }

func (p *Provider) projects() string { return filepath.Join(p.root, "projects") }

// EncodeCWD maps an absolute cwd to its project directory name: every
// non-alphanumeric character becomes '-'.
func EncodeCWD(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alnum {
			b[i] = '-'
		}
	}
	return string(b)
}

// SessionPath returns where Claude writes session id for cwd.
func (p *Provider) SessionPath(cwd, id string) string {
	return filepath.Join(p.projects(), EncodeCWD(cwd), id+".jsonl")
}

// fallbackScanLimit bounds the "cwd" scan across project folders.
const fallbackScanLimit = 200

// Find implements session.Provider.
func (p *Provider) Find(ctx context.Context, q session.Query) ([]session.Ref, error) {
	var refs []session.Ref
	add := func(f session.FileInfo, cwd string) {
		if !q.Since.IsZero() && f.ModTime.Before(q.Since) {
			return
		}
		refs = append(refs, session.Ref{
			Agent: session.Claude, ID: strings.TrimSuffix(filepath.Base(f.Path), ".jsonl"),
			Path: f.Path, CWD: cwd, UpdatedAt: f.ModTime, Size: f.Size,
		})
	}
	if q.ID != "" {
		if q.CWD != "" {
			if f, ok := stat(p.SessionPath(q.CWD, q.ID)); ok {
				add(f, q.CWD)
				return refs, nil
			}
		}
		for _, dir := range p.projectDirs() {
			if f, ok := stat(filepath.Join(dir, q.ID+".jsonl")); ok {
				add(f, q.CWD)
				return refs, nil
			}
		}
		return nil, nil
	}
	if q.CWD == "" {
		for _, dir := range p.projectDirs() {
			for _, f := range session.ListFiles(dir, session.HasSuffix(".jsonl")) {
				add(f, "")
			}
		}
	} else {
		for _, f := range session.ListFiles(filepath.Join(p.projects(), EncodeCWD(q.CWD)), session.HasSuffix(".jsonl")) {
			add(f, q.CWD)
		}
		if len(refs) == 0 {
			p.fallbackScan(ctx, q, add)
		}
	}
	session.SortRefs(refs)
	if q.Limit > 0 && len(refs) > q.Limit {
		refs = refs[:q.Limit]
	}
	return refs, nil
}

// fallbackScan looks for recent files whose first lines carry a matching
// "cwd" (long or unusual paths whose folder encoding is unknown, Q7).
func (p *Provider) fallbackScan(ctx context.Context, q session.Query, add func(session.FileInfo, string)) {
	var all []session.FileInfo
	for _, dir := range p.projectDirs() {
		all = append(all, session.ListFiles(dir, session.HasSuffix(".jsonl"))...)
	}
	session.SortNewest(all)
	if len(all) > fallbackScanLimit {
		all = all[:fallbackScanLimit]
	}
	for _, f := range all {
		if ctx.Err() != nil {
			return
		}
		if !q.Since.IsZero() && f.ModTime.Before(q.Since) {
			continue
		}
		if cwd := headCWD(f.Path); cwd != "" && session.SameDir(cwd, q.CWD) {
			add(f, q.CWD)
		}
	}
}

func (p *Provider) projectDirs() []string {
	ents, err := os.ReadDir(p.projects())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, filepath.Join(p.projects(), e.Name()))
		}
	}
	return out
}

func stat(path string) (session.FileInfo, bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return session.FileInfo{}, false
	}
	return session.FileInfo{Path: path, ModTime: fi.ModTime(), Size: fi.Size()}, true
}

// headCWD returns the first "cwd" found in the first 64 KiB of a file.
func headCWD(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, 64<<10))
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	for sc.Scan() {
		var l struct {
			CWD string `json:"cwd"`
		}
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.CWD != "" {
			return l.CWD
		}
	}
	return ""
}

// --- parsing -------------------------------------------------------------

type usage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

type message struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *usage          `json:"usage"`
}

type rawLine struct {
	Type              string          `json:"type"`
	Subtype           string          `json:"subtype"`
	UUID              string          `json:"uuid"`
	SessionID         string          `json:"sessionId"`
	Timestamp         string          `json:"timestamp"`
	CWD               string          `json:"cwd"`
	GitBranch         string          `json:"gitBranch"`
	IsSidechain       bool            `json:"isSidechain"`
	IsMeta            bool            `json:"isMeta"`
	IsAPIErrorMessage bool            `json:"isApiErrorMessage"`
	IsCompactSummary  bool            `json:"isCompactSummary"`
	Error             json.RawMessage `json:"error"`
	Content           json.RawMessage `json:"content"`
	CompactMetadata   *struct {
		Trigger           string `json:"trigger"`
		PreservedMessages *struct {
			UUIDs []string `json:"uuids"`
		} `json:"preservedMessages"`
	} `json:"compactMetadata"`
	Message *message `json:"message"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     map[string]any  `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

// knownTypes are line types we recognize (most are ignored on purpose).
var knownTypes = map[string]bool{
	"user": true, "assistant": true, "system": true, "summary": true,
	"attachment": true, "last-prompt": true, "mode": true, "queue-operation": true,
	"file-history-snapshot": true, "file-history-delta": true, "api-request": true,
	"api-request-blob": true, "custom-title": true, "ai-title": true, "agent-name": true,
	"cost-state": true, "progress": true, "tag": true,
}

type rec struct {
	session.Entry
	uuids       []string
	msgID       string
	preBoundary bool
}

type parser struct {
	s           *session.Session
	recs        []rec
	toolIndex   map[string][2]int // tool_use id → rec index, tool index
	seenUsage   map[string]bool
	preserved   map[string]bool
	sawBoundary bool
	trigger     string
	firstUser   bool // the first user message has been seen
	warn        session.Counter
}

// Load implements session.Provider.
func (p *Provider) Load(ctx context.Context, ref session.Ref, opt session.LoadOptions) (*session.Session, error) {
	opt = opt.WithDefaults()
	s := &session.Session{Ref: ref}
	s.Agent = session.Claude
	ps := &parser{s: s, toolIndex: map[string][2]int{}, seenUsage: map[string]bool{}, warn: session.Counter{}}
	lines := 0
	st, err := jsonl.Read(ref.Path, jsonl.Options{HeadBytes: opt.HeadBytes, TailBytes: opt.TailBytes}, func(line []byte, _ int64) error {
		lines++
		if lines%4096 == 0 && ctx.Err() != nil {
			return ctx.Err()
		}
		ps.line(line)
		return nil
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if lines == 0 {
			return nil, err
		}
		s.Warnings = append(s.Warnings, "read error: "+err.Error())
	}
	s.Size = st.Size
	s.Gap = st.Gap
	ps.finish()
	s.Warnings = append(s.Warnings, st.Warnings()...)
	s.Warnings = append(s.Warnings, ps.warn.Warnings()...)
	if fi, err := os.Stat(ref.Path); err == nil && s.UpdatedAt.IsZero() {
		s.UpdatedAt = fi.ModTime()
	}
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(ref.Path), ".jsonl")
	}
	if !opt.Since.IsZero() {
		s.FilterSince(opt.Since)
	}
	return s, nil
}

func (ps *parser) line(b []byte) {
	var l rawLine
	if err := json.Unmarshal(b, &l); err != nil {
		ps.warn.Add("malformed line(s) skipped")
		return
	}
	if !knownTypes[l.Type] {
		ps.warn.Add("unknown line type(s) skipped")
		return
	}
	s := ps.s
	if s.ID == "" && l.SessionID != "" {
		s.ID = l.SessionID
	}
	if s.CWD == "" && l.CWD != "" {
		s.CWD = l.CWD
	}
	if l.GitBranch != "" && l.GitBranch != "HEAD" {
		s.GitBranch = l.GitBranch
	}
	at := session.ParseTime(l.Timestamp)
	if s.StartedAt.IsZero() && !at.IsZero() {
		s.StartedAt = at
	}
	if l.IsSidechain {
		return
	}
	switch l.Type {
	case "system":
		ps.system(&l, at)
	case "user":
		if l.IsMeta {
			return
		}
		ps.user(&l, at)
	case "assistant":
		ps.assistant(&l, at)
	}
}

func (ps *parser) system(l *rawLine, at time.Time) {
	switch l.Subtype {
	case "compact_boundary":
		for i := range ps.recs {
			ps.recs[i].preBoundary = true
		}
		ps.sawBoundary = true
		ps.preserved = map[string]bool{}
		ps.trigger = ""
		if m := l.CompactMetadata; m != nil {
			ps.trigger = m.Trigger
			if m.PreservedMessages != nil {
				for _, u := range m.PreservedMessages.UUIDs {
					ps.preserved[u] = true
				}
			}
		}
	case "api_error":
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(l.Error, &e) == nil && e.Message != "" {
			ae := session.APIError{At: at, Code: "api_error", Text: e.Message}
			if ae.LimitLike() {
				ps.s.Errors = append(ps.s.Errors, ae)
			}
		}
	}
}

func (ps *parser) user(l *rawLine, at time.Time) {
	if l.Message == nil {
		return
	}
	text, toolResults := userContent(l.Message.Content)
	for _, tr := range toolResults {
		if idx, ok := ps.toolIndex[tr.ToolUseID]; ok && tr.IsError {
			ps.recs[idx[0]].Tools[idx[1]].Failed = true
		}
	}
	if l.IsCompactSummary {
		ps.s.Compaction = &session.Compaction{At: at, Trigger: ps.trigger, Summary: strings.TrimSpace(text)}
		return
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	e := session.Entry{Role: session.RoleUser, At: at}
	if cmd, ok := clean.SlashCommand(text); ok {
		e.Kind = session.KindSlashCommand
		e.Text = cmd
	} else {
		e.Text = clean.Text(text, clean.ClaudeRules)
	}
	if e.Text == "" {
		return
	}
	if !ps.firstUser {
		ps.firstUser = true
		if session.IsHandoff(e.Text) {
			e.Kind = session.KindHandoff
			ps.s.Chained = session.ParseChained(e.Text)
		}
	}
	ps.recs = append(ps.recs, rec{Entry: e, uuids: []string{l.UUID}})
}

// userContent returns the text of a user message and its tool results.
func userContent(raw json.RawMessage) (string, []block) {
	if len(raw) == 0 {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s, nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var texts []string
	var results []block
	for _, b := range blocks {
		switch b.Type {
		case "text":
			texts = append(texts, b.Text)
		case "image":
			texts = append(texts, "[image]")
		case "document":
			texts = append(texts, "[document]")
		case "tool_result":
			results = append(results, b)
		}
	}
	return strings.Join(texts, "\n\n"), results
}

func (ps *parser) assistant(l *rawLine, at time.Time) {
	m := l.Message
	if m == nil {
		return
	}
	if l.IsAPIErrorMessage {
		code := ""
		_ = json.Unmarshal(l.Error, &code)
		text, _ := userContent(m.Content)
		ps.s.Errors = append(ps.s.Errors, session.APIError{At: at, Code: code, Text: strings.TrimSpace(text)})
		return
	}
	if m.Model == "<synthetic>" {
		return
	}
	if m.Model != "" {
		ps.s.Model = m.Model
	}
	if m.Usage != nil && (m.ID == "" || !ps.seenUsage[m.ID]) {
		if m.ID != "" {
			ps.seenUsage[m.ID] = true
		}
		u := &ps.s.Usage
		u.InputTokens += m.Usage.InputTokens
		u.OutputTokens += m.Usage.OutputTokens
		u.CacheReadTokens += m.Usage.CacheReadInputTokens
		u.CacheWriteTokens += m.Usage.CacheCreationInputTokens
		u.Replies++
	}
	var blocks []block
	if len(m.Content) > 0 && m.Content[0] == '"' {
		var s string
		_ = json.Unmarshal(m.Content, &s)
		blocks = []block{{Type: "text", Text: s}}
	} else if json.Unmarshal(m.Content, &blocks) != nil {
		ps.warn.Add("malformed assistant content(s) skipped")
		return
	}
	// Merge consecutive one-block lines of the same API message.
	var r *rec
	if n := len(ps.recs); n > 0 && m.ID != "" && ps.recs[n-1].msgID == m.ID && ps.recs[n-1].Role == session.RoleAssistant {
		r = &ps.recs[n-1]
		r.uuids = append(r.uuids, l.UUID)
	} else {
		ps.recs = append(ps.recs, rec{Entry: session.Entry{Role: session.RoleAssistant, At: at}, uuids: []string{l.UUID}, msgID: m.ID})
		r = &ps.recs[len(ps.recs)-1]
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				if r.Text != "" {
					r.Text += "\n\n"
				}
				r.Text += t
			}
		case "tool_use":
			r.Tools = append(r.Tools, session.ToolCall{Name: b.Name, Args: b.Input})
			if b.ID != "" {
				ps.toolIndex[b.ID] = [2]int{len(ps.recs) - 1, len(r.Tools) - 1}
			}
			if b.Name == "TodoWrite" {
				if todos := parseTodos(b.Input); todos != nil {
					ps.s.Todos = todos
				}
			}
		}
	}
}

func parseTodos(in map[string]any) []session.Todo {
	list, ok := in["todos"].([]any)
	if !ok {
		return nil
	}
	out := []session.Todo{}
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		text, _ := m["content"].(string)
		status, _ := m["status"].(string)
		if text == "" {
			continue
		}
		out = append(out, session.Todo{Text: text, Status: session.ParseTodoStatus(status)})
	}
	return out
}

func (ps *parser) finish() {
	s := ps.s
	removed := 0
	for _, r := range ps.recs {
		if r.preBoundary && !anyPreserved(r.uuids, ps.preserved) {
			removed++
			continue
		}
		s.Entries = append(s.Entries, r.Entry)
	}
	s.Omitted += removed
	if s.Compaction != nil && s.Compaction.Trigger == "" {
		s.Compaction.Trigger = ps.trigger
	}
}

func (ps *parser) hasUserMessage() bool {
	for _, r := range ps.recs {
		if r.Role == session.RoleUser && r.Kind == session.KindMessage {
			return true
		}
	}
	return false
}

func anyPreserved(uuids []string, preserved map[string]bool) bool {
	for _, u := range uuids {
		if preserved[u] {
			return true
		}
	}
	return false
}

// Preview implements session.Previewer: the first user request, cleaned.
func (p *Provider) Preview(ctx context.Context, ref session.Ref) (string, error) {
	ps := &parser{s: &session.Session{}, toolIndex: map[string][2]int{}, seenUsage: map[string]bool{}, warn: session.Counter{}}
	_, err := jsonl.Read(ref.Path, jsonl.Options{HeadBytes: 256 << 10, TailBytes: 1}, func(line []byte, off int64) error {
		if off >= 256<<10 {
			return jsonl.ErrStop
		}
		ps.line(line)
		if ps.s.Chained != nil || ps.hasUserMessage() {
			return jsonl.ErrStop
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	ps.finish()
	return ps.s.FirstUserText(), nil
}
