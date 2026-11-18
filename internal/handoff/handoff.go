// Package handoff renders the markdown handoff document that carries a
// conversation from one agent CLI to another (§8). The document is
// model-facing and therefore always English.
//
// Build fills sections within a character budget: fixed sections are capped,
// and everything left goes to the Conversation, filled newest-first. The
// result also respects a byte limit for passing it as one argv argument.
package handoff

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/redact"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/toolsum"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Defaults.
const (
	DefaultBudget      = 30000
	DefaultMaxArgBytes = 102400
	referenceBudget    = 30000 // caps below are sized for this budget
)

// Section caps at the reference budget (runes).
const (
	capTasks         = 2000
	capFiles         = 1500
	capRepo          = 2500
	capLastUser      = 3000
	capLastAssistant = 4000
	capEntryUser     = 1500
	capEntryAsst     = 1200
	capMeanwhile     = 2500
	maxToolLines     = 12
	origFraction     = 0.12
	historyFraction  = 0.25
)

// Reason says why the switch happens.
type Reason struct {
	Limit   bool      // the source agent hit its usage limit
	ResetAt time.Time // when the limit resets, if known
}

// Options control rendering.
type Options struct {
	From, To          session.AgentID
	Budget            int            // runes; default 30000
	MaxArgBytes       int            // bytes; default 102400
	Now               time.Time      // marker time; default time.Now()
	Location          *time.Location // for rendered times; default time.Local
	Reason            Reason
	Git               *gitinfo.Info // nil: repository state not collected
	CWD               string        // project dir; default: the session's cwd
	IncludeSourcePath bool
	Redact            bool
	Delta             bool // "welcome back" mode (target resumed natively)
	// Continue makes the new agent carry on with an unfinished request at
	// once; otherwise it confirms in one sentence and waits (§8.2a).
	Continue bool
}

// Doc is a rendered handoff.
type Doc struct {
	Text       string
	Runes      int
	Bytes      int
	Included   int // conversation blocks included
	Omitted    int // conversation blocks omitted (plus entries the parser skipped)
	Redactions int
}

func (o *Options) defaults() {
	if o.Budget <= 0 {
		o.Budget = DefaultBudget
	}
	if o.MaxArgBytes <= 0 {
		o.MaxArgBytes = DefaultMaxArgBytes
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Location == nil {
		o.Location = time.Local
	}
}

// scale returns a cap scaled to the budget (never above the reference cap).
func (o *Options) scale(c int) int {
	if o.Budget >= referenceBudget {
		return c
	}
	v := c * o.Budget / referenceBudget
	if v < 80 {
		v = 80
	}
	return v
}

// Build renders a handoff from sources (one session for a full handoff; one
// or more, oldest first, for a delta). Sources must not be empty.
func Build(sources []*session.Session, opt Options) (*Doc, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("handoff: no source session")
	}
	opt.defaults()
	if opt.From == "" {
		opt.From = sources[len(sources)-1].Agent
	}
	if opt.CWD == "" {
		for _, s := range sources {
			if s.CWD != "" {
				opt.CWD = s.CWD
			}
		}
	}
	r := newRenderer(sources, &opt)
	convBudget := -1 // computed on the first pass
	for attempt := 0; attempt < 8; attempt++ {
		text, inc, omit := r.render(convBudget)
		red := 0
		if opt.Redact {
			text, red = redact.String(text)
		}
		runes := utf8.RuneCountInString(text)
		over := runes - opt.Budget
		if b := len(text) - opt.MaxArgBytes; b > over {
			over = b
		}
		if over <= 0 {
			return &Doc{Text: text, Runes: runes, Bytes: len(text), Included: inc, Omitted: omit, Redactions: red}, nil
		}
		if convBudget < 0 {
			convBudget = r.lastConvBudget
		}
		if convBudget == 0 {
			break
		}
		convBudget -= over + 16
		if convBudget < 0 {
			convBudget = 0
		}
	}
	// Last resort: middle-truncate everything below the marker line.
	text, _, omit := r.render(0)
	red := 0
	if opt.Redact {
		text, red = redact.String(text)
	}
	marker, rest, _ := strings.Cut(text, "\n")
	room := opt.Budget - utf8.RuneCountInString(marker) - 1
	if lim := opt.MaxArgBytes - len(marker) - 1; lim/4 < room {
		room = lim / 4
	}
	if room < 0 {
		room = 0
	}
	text = marker + "\n" + clean.Truncate(rest, room)
	return &Doc{Text: text, Runes: utf8.RuneCountInString(text), Bytes: len(text), Omitted: omit, Redactions: red}, nil
}

// --- rendering -----------------------------------------------------------

type renderer struct {
	src            []*session.Session
	opt            *Options
	main           *session.Session // the newest source
	blocks         []string         // conversation blocks, oldest first
	extraOmitted   int
	gap            bool
	lastConvBudget int
}

func newRenderer(src []*session.Session, opt *Options) *renderer {
	r := &renderer{src: src, opt: opt, main: src[len(src)-1]}
	for _, s := range src {
		if len(src) > 1 {
			r.blocks = append(r.blocks, fmt.Sprintf("*(in %s)*", session.DisplayName(s.Agent)))
		}
		r.blocks = append(r.blocks, conversationBlocks(s, opt.CWD)...)
		r.extraOmitted += s.Omitted
		if s.Gap {
			r.gap = true
		}
	}
	return r
}

// conversationBlocks renders entries into blocks: a message followed by the
// tool lines of the run that follows it.
func conversationBlocks(s *session.Session, cwd string) []string {
	type blk struct {
		head  string
		tools []string
	}
	var bs []*blk
	for _, e := range s.Entries {
		if e.Kind == session.KindHandoff {
			continue // the chained handoff: "Earlier history"
		}
		text := strings.TrimSpace(e.Text)
		if laterHandoff(e) {
			// The "Welcome back" the agent got when it resumed: what
			// happened in the other agent meanwhile, without the document's
			// instructions.
			if c := session.ParseChained(text); c != nil {
				head := "**Meanwhile the user continued in " + session.DisplayName(c.From) + ":**"
				if h := strings.TrimSpace(c.History); h != "" {
					head += "\n" + indent(clean.Truncate(h, capMeanwhile), "  ")
				}
				bs = append(bs, &blk{head: head})
			}
			continue
		}
		if text != "" || len(bs) == 0 && len(e.Tools) > 0 {
			var head string
			switch e.Role {
			case session.RoleUser:
				head = "**User:** " + clean.Truncate(text, capEntryUser)
			case session.RoleAssistant:
				if text == "" {
					text = "(tool calls)"
				}
				head = "**Assistant:** " + clean.Truncate(text, capEntryAsst)
			default:
				head = "*Notice: " + clean.OneLine(text, 300) + "*"
			}
			bs = append(bs, &blk{head: head})
		}
		for _, tc := range e.Tools {
			sum := toolsum.Summarize(tc, cwd)
			if sum.Line == "" {
				continue
			}
			b := bs[len(bs)-1]
			b.tools = append(b.tools, sum.Line)
		}
	}
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		var sb strings.Builder
		sb.WriteString(b.head)
		n := len(b.tools)
		for i, t := range b.tools {
			if i == maxToolLines {
				fmt.Fprintf(&sb, "\n  · (+%d more tool calls)", n-maxToolLines)
				break
			}
			sb.WriteString("\n  · " + t)
		}
		out = append(out, sb.String())
	}
	return out
}

// laterHandoff reports whether e is a handoff an agent got after the start
// of its session (a resume's "Welcome back"), not a request of the user.
func laterHandoff(e session.Entry) bool {
	return e.Role == session.RoleUser && e.Kind == session.KindMessage && session.IsHandoff(e.Text)
}

// indent prefixes every line of s.
func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

// render produces the document. convBudget < 0 means "everything left".
func (r *renderer) render(convBudget int) (string, int, int) {
	o := r.opt
	var top, bottom strings.Builder
	top.WriteString(session.Marker(o.From, o.To, o.Now.In(o.Location)))
	top.WriteString("\n\n")
	if o.Delta {
		r.deltaPreamble(&top)
	} else {
		r.preamble(&top)
		if s := r.originalRequest(); s != "" {
			section(&top, "Original request", s)
		}
		if title, s := r.earlierHistory(); s != "" {
			section(&top, title, s)
		}
		if s := r.taskList(); s != "" {
			section(&top, "Current task list", s)
		}
	}
	if s := r.filesTouched(); s != "" {
		section(&top, "Files touched", s)
	}
	section(&top, "Repository state", r.repoState())

	section(&bottom, "Where it stopped", r.whereStopped())
	section(&bottom, "What to do now", r.whatNow())
	if o.IncludeSourcePath && !o.Delta && r.main.Path != "" && !r.main.Reconstructed {
		if session.SharedStore(r.main.Agent) {
			// One database for every session: point to the CLI's export.
			bottom.WriteString("Full transcript of the previous session (read only if needed): run `opencode session export " + r.main.ID + "`\n")
		} else {
			bottom.WriteString("Full transcript of the previous session (read only if needed): " + r.main.Path + "\n")
		}
	}

	fixed := utf8.RuneCountInString(top.String()) + utf8.RuneCountInString(bottom.String())
	headingRoom := 120
	if convBudget < 0 {
		convBudget = o.Budget - fixed - headingRoom
		if convBudget < 0 {
			convBudget = 0
		}
	}
	r.lastConvBudget = convBudget

	// Fill newest-first; stop at the first block that does not fit.
	used := 0
	start := len(r.blocks)
	for i := len(r.blocks) - 1; i >= 0; i-- {
		n := utf8.RuneCountInString(r.blocks[i]) + 1
		if used+n > convBudget {
			break
		}
		used += n
		start = i
	}
	included := len(r.blocks) - start
	omitted := start + r.extraOmitted
	var title string
	switch {
	case o.Delta:
		title = "Conversation (while you were away, oldest first"
	default:
		title = "Conversation (condensed, oldest first"
	}
	switch {
	case r.gap:
		title += fmt.Sprintf("; %d+ earlier entries omitted)", omitted)
	case omitted > 0:
		title += fmt.Sprintf("; %d earlier entries omitted)", omitted)
	default:
		title += ")"
	}
	body := strings.Join(r.blocks[start:], "\n")
	if len(r.blocks) == 0 {
		body = "(no messages recorded)"
	} else if included == 0 {
		body = "(omitted for space; see \"Where it stopped\")"
	}
	var doc strings.Builder
	doc.WriteString(top.String())
	section(&doc, title, body)
	doc.WriteString(bottom.String())
	return doc.String(), included, omitted
}

func section(b *strings.Builder, title, body string) {
	b.WriteString("## " + title + "\n")
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n\n")
}

func (r *renderer) preamble(b *strings.Builder) {
	o := r.opt
	from := session.DisplayName(o.From)
	model := ""
	if r.main.Model != "" {
		model = " (model: " + r.main.Model + ")"
	}
	fmt.Fprintf(b, "You are continuing a coding session that the user was running in %s%s.\n", from, model)
	if o.Reason.Limit {
		reset := ""
		if !o.Reason.ResetAt.IsZero() {
			reset = " (resets " + r.clock(o.Reason.ResetAt) + ")"
		}
		fmt.Fprintf(b, "They switched to you because %s hit its usage limit%s.\n", from, reset)
	} else {
		b.WriteString("The user switched to you manually.\n")
	}
	if r.main.Reconstructed {
		b.WriteString("The record below was reconstructed from terminal scrollback and may be incomplete.\n")
	}
	b.WriteString("- Treat the record below as your own memory of this session.\n")
	b.WriteString("- Do not redo finished work. Files may have changed since; re-read a file before editing it.\n")
	if names := r.toolNames(); names != "" {
		fmt.Fprintf(b, "- Tool names in the record (%s, ...) belong to the previous agent. Use your own tools.\n", names)
	} else {
		b.WriteString("- Tool names in the record belong to the previous agent. Use your own tools.\n")
	}
	b.WriteString("- Reply in the language the user writes in.\n\n")
}

func (r *renderer) deltaPreamble(b *strings.Builder) {
	var names []string
	seen := map[session.AgentID]bool{}
	for _, s := range r.src {
		if !seen[s.Agent] {
			seen[s.Agent] = true
			names = append(names, session.DisplayName(s.Agent))
		}
	}
	fmt.Fprintf(b, "Welcome back. While you were away the user continued in %s; here is what happened there.\n", strings.Join(names, ", then "))
	b.WriteString("- Files may have changed since; re-read a file before editing it.\n")
	b.WriteString("- Tool names in the record belong to the other agent. Use your own tools.\n")
	b.WriteString("- Reply in the language the user writes in.\n\n")
}

func (r *renderer) toolNames() string {
	var names []string
	seen := map[string]bool{}
	for _, s := range r.src {
		for _, e := range s.Entries {
			for _, tc := range e.Tools {
				if tc.Name == "" || seen[tc.Name] || len(names) == 3 || toolsum.Summarize(tc, r.opt.CWD).Line == "" {
					continue // not in the record
				}
				seen[tc.Name] = true
				names = append(names, tc.Name)
			}
		}
	}
	return strings.Join(names, ", ")
}

func (r *renderer) clock(t time.Time) string {
	lt := t.In(r.opt.Location)
	now := r.opt.Now.In(r.opt.Location)
	if lt.Year() == now.Year() && lt.YearDay() == now.YearDay() {
		return lt.Format("15:04")
	}
	return lt.Format("Jan 2 15:04")
}

func (r *renderer) originalRequest() string {
	first := r.src[0]
	text := first.FirstUserText()
	if text == "" {
		return ""
	}
	return clean.Truncate(text, int(float64(r.opt.Budget)*origFraction))
}

func (r *renderer) earlierHistory() (string, string) {
	s := r.main
	var parts []string
	title := "Earlier history"
	if s.Chained != nil && strings.TrimSpace(s.Chained.History) != "" {
		parts = append(parts, strings.TrimSpace(s.Chained.History))
		title = "Earlier history (carried over from a previous handoff)"
	}
	if s.Compaction != nil && strings.TrimSpace(s.Compaction.Summary) != "" {
		parts = append(parts, "Summary of the conversation before it was compacted:\n"+strings.TrimSpace(s.Compaction.Summary))
		if s.Chained == nil {
			title = "Earlier history (summary from context compaction)"
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	return title, clean.TruncateHead(strings.Join(parts, "\n\n"), int(float64(r.opt.Budget)*historyFraction))
}

func (r *renderer) todos() []session.Todo {
	if len(r.main.Todos) > 0 {
		return r.main.Todos
	}
	return toolsum.LatestTodos(r.main.Entries)
}

func (r *renderer) taskList() string {
	todos := r.todos()
	if len(todos) == 0 {
		return ""
	}
	limit := r.opt.scale(capTasks)
	var b strings.Builder
	for i, t := range todos {
		var line string
		text := clean.OneLine(t.Text, 200)
		switch t.Status {
		case session.TodoDone:
			line = "- [x] " + text
		case session.TodoInProgress:
			line = "- [~] " + text + " (in progress)"
		case session.TodoBlocked:
			line = "- [!] " + text + " (blocked)"
		default:
			line = "- [ ] " + text
		}
		if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(line)+40 > limit {
			fmt.Fprintf(&b, "(+%d more)\n", len(todos)-i)
			break
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (r *renderer) activity() session.FileActivity {
	var fa session.FileActivity
	for _, s := range r.src {
		a := s.Files
		if len(a.Edited)+len(a.Created)+a.ReadCount == 0 {
			a = toolsum.Activity(s.Entries, r.opt.CWD)
		}
		fa.Edited = appendUnique(fa.Edited, a.Edited...)
		fa.Created = appendUnique(fa.Created, a.Created...)
		fa.Read = appendUnique(fa.Read, a.Read...)
		fa.ReadCount += a.ReadCount
	}
	return fa
}

func appendUnique(list []string, items ...string) []string {
	for _, it := range items {
		dup := false
		for _, x := range list {
			if x == it {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, it)
		}
	}
	return list
}

func (r *renderer) filesTouched() string {
	fa := r.activity()
	limit := r.opt.scale(capFiles)
	var parts []string
	if len(fa.Edited) > 0 {
		parts = append(parts, "Edited: "+joinCapped(fa.Edited, limit/2))
	}
	if len(fa.Created) > 0 {
		parts = append(parts, "Created: "+joinCapped(fa.Created, limit/4))
	}
	if fa.ReadCount > 0 {
		noun := "files"
		if fa.ReadCount == 1 {
			noun = "file"
		}
		sample := joinCapped(fa.Read, limit/4)
		if fa.ReadCount > len(fa.Read) {
			sample += ", ..."
		}
		parts = append(parts, fmt.Sprintf("Read: %d %s (%s)", fa.ReadCount, noun, sample))
	}
	return clean.OneLine(strings.Join(parts, " · "), limit)
}

func joinCapped(items []string, limit int) string {
	var b strings.Builder
	for i, it := range items {
		add := it
		if i > 0 {
			add = ", " + it
		}
		if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(add) > limit && i > 0 {
			fmt.Fprintf(&b, " (+%d more)", len(items)-i)
			break
		}
		b.WriteString(add)
	}
	return b.String()
}

func (r *renderer) repoState() string {
	o := r.opt
	var b strings.Builder
	cwd := o.CWD
	if cwd == "" {
		cwd = "(unknown)"
	}
	g := o.Git
	branch := ""
	if g != nil && g.Branch != "" {
		branch = " · branch: " + g.Branch
	} else if r.main.GitBranch != "" {
		branch = " · branch: " + r.main.GitBranch + " (as recorded by the agent)"
	}
	switch {
	case g == nil:
		b.WriteString("cwd: " + cwd + branch + "\n(git state not collected)\n")
		return b.String()
	case g.State == gitinfo.NotRepo:
		b.WriteString("cwd: " + cwd + " · not a git repository\n")
		return b.String()
	case g.State == gitinfo.NoGit:
		b.WriteString("cwd: " + cwd + branch + " · git is not installed\n")
		return b.String()
	case g.State == gitinfo.TimedOut:
		b.WriteString("cwd: " + cwd + branch + "\ngit status timed out\n")
		return b.String()
	}
	b.WriteString("cwd: " + cwd + branch + "\n")
	limit := o.scale(capRepo)
	if len(g.Status) == 0 {
		b.WriteString("git status: clean\n")
	} else {
		fmt.Fprintf(&b, "git status --short (%d lines):\n", g.StatusTotal)
		for i, l := range g.Status {
			if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(l) > limit*6/10 {
				fmt.Fprintf(&b, "(+%d more)\n", g.StatusTotal-i)
				break
			}
			b.WriteString(l + "\n")
			if i == len(g.Status)-1 && g.StatusMore() > 0 {
				fmt.Fprintf(&b, "(+%d more)\n", g.StatusMore())
			}
		}
	}
	if len(g.DiffStat) > 0 {
		b.WriteString("git diff --stat:\n")
		for i, l := range g.DiffStat {
			if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(l) > limit {
				fmt.Fprintf(&b, "(+%d more)\n", g.DiffTotal-i)
				break
			}
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func (r *renderer) whereStopped() string {
	var lastUser, lastAsst string
	for _, s := range r.src {
		for _, e := range s.Entries {
			t := strings.TrimSpace(e.Text)
			if laterHandoff(e) {
				// The last turns of the agent the user continued in meanwhile.
				if c := session.ParseChained(t); c != nil {
					for _, l := range strings.Split(c.History, "\n") {
						if u, ok := strings.CutPrefix(l, "**User:** "); ok {
							lastUser = strings.TrimSpace(u)
						} else if a, ok := strings.CutPrefix(l, "**Assistant:** "); ok {
							lastAsst = strings.TrimSpace(a)
						}
					}
				}
				continue
			}
			if t == "" || e.Kind == session.KindHandoff {
				continue
			}
			switch e.Role {
			case session.RoleUser:
				lastUser = t
			case session.RoleAssistant:
				lastAsst = t
			}
		}
	}
	if lastUser == "" && lastAsst == "" {
		return "(nothing recorded)"
	}
	var b strings.Builder
	if lastUser != "" {
		b.WriteString("Last user request: " + clean.Truncate(lastUser, r.opt.scale(capLastUser)) + "\n")
	}
	if lastAsst != "" {
		b.WriteString("Last assistant message: " + clean.Truncate(lastAsst, r.opt.scale(capLastAssistant)) + "\n")
	}
	return b.String()
}

func (r *renderer) whatNow() string {
	if !r.opt.Continue {
		return "Do not start working yet. Reply with one short sentence in the user's language saying you have\n" +
			"the context and what was last being worked on, then wait for the user's next message."
	}
	if r.opt.Delta {
		return "Pick up from here. If the last request is not finished, continue it. Otherwise give a 2–3 sentence status\nand wait for the user."
	}
	return "If the last request is not finished, continue it. Otherwise give a 2–3 sentence status\nand wait for the user."
}
