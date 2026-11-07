// Package session defines the agent-neutral conversation model that every
// agent's session parser (provider) produces, and the Provider interface.
//
// Contract for every provider: parsing never panics. Unknown fields and line
// types are ignored; malformed input is skipped and counted into
// Session.Warnings. Load returns an error only when nothing usable could be
// read. Providers are pure: they never talk to tmux.
package session

import (
	"context"
	"strings"
	"time"
)

// AgentID identifies an agent CLI.
type AgentID string

// Known agents.
const (
	Claude      AgentID = "claude"
	Antigravity AgentID = "antigravity" // Antigravity CLI (agy), Google's successor to Gemini CLI for personal accounts
	Copilot     AgentID = "copilot"
	Codex       AgentID = "codex"
	OpenCode    AgentID = "opencode" // any model endpoint (OpenAI-compatible, Anthropic, Ollama ...)
)

// AllAgents lists the known agents in default order.
var AllAgents = []AgentID{Claude, Antigravity, Copilot, Codex, OpenCode}

// SharedStore reports whether an agent keeps every session in one file (a
// database: OpenCode). Its sessions are told apart by id, never by Ref.Path.
func SharedStore(id AgentID) bool { return id == OpenCode }

// Known reports whether id is a known agent.
func Known(id string) bool {
	for _, a := range AllAgents {
		if string(a) == id {
			return true
		}
	}
	return false
}

// DisplayName returns the full product name ("Claude Code").
func DisplayName(id AgentID) string {
	switch id {
	case Claude:
		return "Claude Code"
	case Antigravity:
		return "Antigravity CLI"
	case Copilot:
		return "GitHub Copilot CLI"
	case Codex:
		return "Codex CLI"
	case OpenCode:
		return "OpenCode"
	}
	return string(id)
}

// ShortName returns the short name used in the UI ("Claude").
func ShortName(id AgentID) string {
	switch id {
	case Claude:
		return "Claude"
	case Antigravity:
		return "Antigravity"
	case Copilot:
		return "Copilot"
	case Codex:
		return "Codex"
	case OpenCode:
		return "OpenCode"
	}
	return string(id)
}

// Ref identifies a session on disk without parsing it.
type Ref struct {
	Agent     AgentID
	ID        string    // native session id (a UUID for all four agents)
	Path      string    // file (Claude, Antigravity, Codex) or directory (Copilot)
	CWD       string    // may be "" until parsed
	StartedAt time.Time // zero if unknown
	UpdatedAt time.Time // file mtime
	Size      int64
}

// Key returns "agent:id", the form used by --from-session.
func (r Ref) Key() string { return string(r.Agent) + ":" + r.ID }

// Session is the agent-neutral result of parsing.
type Session struct {
	Ref
	Model      string          // last real model seen (Claude "<synthetic>" ignored)
	GitBranch  string          // as recorded by the agent, if any
	Entries    []Entry         // chronological; superseded and excluded lines removed
	Omitted    int             // entries known to exist but not included (outside window, compacted)
	Gap        bool            // the middle of the file was not read (head + tail windows)
	Compaction *Compaction     // latest compaction, if any
	Todos      []Todo          // latest task-list snapshot
	Files      FileActivity    // filled by the handoff builder when empty
	Usage      Usage           // token and request counts
	Errors     []APIError      // never in Entries; used for the switch reason and limits
	Chained    *ChainedHandoff // set when the first user message is an imux handoff
	Warnings   []string        // non-fatal parse problems ("312 unknown lines skipped", ...)
	// Reconstructed is set when the transcript comes from terminal
	// scrollback instead of a session file.
	Reconstructed bool
}

// Role is who produced an entry.
type Role uint8

// Roles.
const (
	RoleUser Role = iota
	RoleAssistant
	RoleNotice // CLI info/warning lines, compaction summaries
)

// EntryKind refines an entry's meaning.
type EntryKind uint8

// Entry kinds.
const (
	KindMessage        EntryKind = iota
	KindSlashCommand             // "/name args"
	KindCompactSummary           // text of a compaction summary
	KindHandoff                  // an imux handoff document (excluded from Conversation, parsed into Chained)
)

// Entry is one turn in the conversation.
type Entry struct {
	Role  Role
	Kind  EntryKind
	At    time.Time
	Text  string     // cleaned text, may be empty for tool-only turns
	Tools []ToolCall // tool calls issued in this assistant turn, in order
}

// ToolCall is one tool invocation by the assistant.
type ToolCall struct {
	Name   string
	Args   map[string]any // decoded; JSON-string arguments (Codex) are decoded too
	Failed bool           // when the result says so
}

// TodoStatus is the state of a task-list item.
type TodoStatus uint8

// Todo states.
const (
	TodoPending TodoStatus = iota
	TodoInProgress
	TodoDone
	TodoBlocked
)

// ParseTodoStatus maps the agents' status strings to TodoStatus.
func ParseTodoStatus(s string) TodoStatus {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "in_progress", "in-progress", "inprogress", "active", "doing", "running":
		return TodoInProgress
	case "completed", "complete", "done", "finished", "cancelled", "canceled":
		return TodoDone
	case "blocked":
		return TodoBlocked
	}
	return TodoPending
}

// Todo is one task-list item.
type Todo struct {
	Text   string
	Status TodoStatus
}

// FileActivity summarizes files the agent touched.
type FileActivity struct {
	Edited    []string // deduped, ordered by last touch
	Created   []string
	Read      []string // capped sample
	ReadCount int
}

// Usage counts tokens and requests.
type Usage struct {
	InputTokens, OutputTokens         int64
	CacheReadTokens, CacheWriteTokens int64
	Replies                           int     // assistant API messages (Claude: deduped by message.id)
	PremiumRequests                   float64 // Copilot
	RateLimits                        *RateLimitSnapshot
}

// RateLimitSnapshot is a provider-reported rate-limit state (Codex).
type RateLimitSnapshot struct {
	At                   time.Time
	PrimaryUsedPercent   float64
	PrimaryWindowMinutes int
	PrimaryResetsAt      time.Time
	SecondaryUsedPercent float64
	SecondaryWindowMins  int
	SecondaryResetsAt    time.Time
}

// APIError is an error the agent recorded (rate limit, auth, ...).
type APIError struct {
	At   time.Time
	Code string // e.g. "rate_limit", "authentication_failed"
	Text string
}

// LimitLike reports whether the error looks like a usage or rate limit.
func (e APIError) LimitLike() bool {
	s := strings.ToLower(e.Code + " " + e.Text)
	for _, k := range []string{"rate_limit", "rate limit", "usage limit", "limit reached", "quota", "resource_exhausted", "429", "too many requests", "hit your"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// Compaction is the latest context compaction.
type Compaction struct {
	At      time.Time
	Trigger string // "auto" | "manual"
	Summary string
}

// Query selects sessions in Find.
type Query struct {
	CWD   string
	ID    string    // exact id when known (trail, --session)
	Since time.Time // ignore sessions not updated since (pane start time)
	Limit int       // 0 = no limit
}

// LoadOptions bounds parsing.
type LoadOptions struct {
	HeadBytes int64     // default 1 MiB: original request, metadata
	TailBytes int64     // default 8 MiB: recent history, latest todos
	Since     time.Time // delta mode: only entries after this time
}

// Defaults for LoadOptions.
const (
	DefaultHeadBytes = 1 << 20
	DefaultTailBytes = 8 << 20
)

// WithDefaults fills zero fields.
func (o LoadOptions) WithDefaults() LoadOptions {
	if o.HeadBytes <= 0 {
		o.HeadBytes = DefaultHeadBytes
	}
	if o.TailBytes <= 0 {
		o.TailBytes = DefaultTailBytes
	}
	return o
}

// Provider finds and parses one agent's sessions.
type Provider interface {
	Agent() AgentID
	// Find lists candidate sessions, newest first. Never reads whole files.
	Find(ctx context.Context, q Query) ([]Ref, error)
	// Load parses one session. Malformed input never panics; problems go to
	// Session.Warnings. An error is returned only when nothing usable could
	// be read.
	Load(ctx context.Context, ref Ref, opt LoadOptions) (*Session, error)
}

// Previewer is implemented by providers that can cheaply return the first
// user request of a session (session picker).
type Previewer interface {
	Preview(ctx context.Context, ref Ref) (string, error)
}

// FirstUserText returns the first user message of s that is not a handoff,
// or the chained handoff's original request.
func (s *Session) FirstUserText() string {
	if s.Chained != nil && s.Chained.OriginalRequest != "" {
		return s.Chained.OriginalRequest
	}
	for _, e := range s.Entries {
		if e.Role == RoleUser && e.Kind == KindMessage && strings.TrimSpace(e.Text) != "" {
			return e.Text
		}
	}
	return ""
}

// LastError returns the newest recorded API error, or nil.
func (s *Session) LastError() *APIError {
	if len(s.Errors) == 0 {
		return nil
	}
	return &s.Errors[len(s.Errors)-1]
}

// FilterSince drops entries before t (delta mode).
func (s *Session) FilterSince(t time.Time) {
	if t.IsZero() {
		return
	}
	// Entries before the delta window are not "omitted": the target agent
	// saw them already.
	out := s.Entries[:0]
	for _, e := range s.Entries {
		if !e.At.IsZero() && e.At.Before(t) {
			continue
		}
		out = append(out, e)
	}
	s.Entries = out
	s.Omitted = 0
	s.Gap = false
}
