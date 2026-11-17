package aider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// history is a synthetic chat history in Aider's format: two runs.
const history = `
# aider chat started at 2026-11-17 10:00:00

> /home/u/.local/bin/aider
> Aider v0.86.1
> Main model: openai/gpt-test with diff edit format
> Git repo: .git with 3 files

#### add a greet function to hello.py
#### and keep it short

I will add the function.

hello.py
` + "```python" + `
<<<<<<< SEARCH
=======
def greet():
    return "hi"
>>>>>>> REPLACE
` + "```" + `

> Tokens: 2.3k sent, 512 received. Cost: $0.01 message, $0.01 session.
> Applied edit to hello.py
> Commit 1a2b3c4d5e feat: Add greet function

#### /add notes.md
> Added notes.md to the chat

#### run the tests

Run them with:

` + "```bash" + `
python -m pytest
` + "```" + `

> Running python -m pytest
> litellm.RateLimitError: RateLimitError: OpenAIException - Rate limit reached for requests
> The API provider has rate limited you. Try again later or check your quotas.

# aider chat started at 2026-11-17 11:30:00

> Aider v0.86.1
> Model: anthropic/claude-test with diff edit format

#### what changed?

Only hello.py.

> Tokens: 1k sent, 20 received.
`

func writeRepo(t *testing.T) (repo, file string) {
	t.Helper()
	repo = t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	file = filepath.Join(repo, HistoryName)
	if err := os.WriteFile(file, []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(file, mt, mt); err != nil {
		t.Fatal(err)
	}
	return repo, file
}

func TestFind(t *testing.T) {
	repo, file := writeRepo(t)
	sub := filepath.Join(repo, "pkg", "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Provider{}
	refs, err := p.Find(context.Background(), session.Query{CWD: sub})
	if err != nil || len(refs) != 2 {
		t.Fatalf("%v %+v", err, refs)
	}
	if refs[0].ID != "20261117T113000" || refs[1].ID != "20261117T100000" || refs[0].Path != file {
		t.Fatalf("ids %+v", refs)
	}
	if !refs[1].UpdatedAt.Equal(refs[0].StartedAt) || refs[0].UpdatedAt.Hour() != 12 {
		t.Fatalf("times %+v", refs)
	}
	// Since and ID.
	refs, _ = p.Find(context.Background(), session.Query{CWD: repo, Since: time.Date(2026, 11, 17, 11, 45, 0, 0, time.Local)})
	if len(refs) != 1 || refs[0].ID != "20261117T113000" {
		t.Fatalf("since %+v", refs)
	}
	refs, _ = p.Find(context.Background(), session.Query{CWD: repo, ID: "20261117T100000"})
	if len(refs) != 1 {
		t.Fatalf("id %+v", refs)
	}
	// No folder, or a repository without a history: nothing.
	if refs, _ := p.Find(context.Background(), session.Query{}); refs != nil {
		t.Fatal("no cwd")
	}
	if refs, _ := p.Find(context.Background(), session.Query{CWD: t.TempDir()}); refs != nil {
		t.Fatal("no history")
	}
}

func TestLoad(t *testing.T) {
	repo, _ := writeRepo(t)
	p := &Provider{}
	refs, _ := p.Find(context.Background(), session.Query{CWD: repo})
	s, err := p.Load(context.Background(), refs[1], session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "openai/gpt-test" {
		t.Errorf("model %q", s.Model)
	}
	if s.Usage.InputTokens != 2300 || s.Usage.OutputTokens != 512 || s.Usage.Replies != 1 {
		t.Errorf("usage %+v", s.Usage)
	}
	if got := s.FirstUserText(); got != "add a greet function to hello.py\nand keep it short" {
		t.Errorf("first %q", got)
	}
	var kinds []string
	var tools []string
	for _, e := range s.Entries {
		kinds = append(kinds, map[session.Role]string{session.RoleUser: "u", session.RoleAssistant: "a"}[e.Role])
		for _, tc := range e.Tools {
			tools = append(tools, tc.Name)
		}
	}
	if strings.Join(kinds, "") != "uauaua" {
		t.Errorf("entries %s: %+v", strings.Join(kinds, ""), s.Entries)
	}
	if strings.Join(tools, " ") != "edit commit read bash" {
		t.Errorf("tools %v", tools)
	}
	if s.Entries[1].Text == "" || !strings.Contains(s.Entries[1].Text, "def greet") {
		t.Errorf("reply %q", s.Entries[1].Text)
	}
	if s.Entries[2].Kind != session.KindSlashCommand {
		t.Errorf("slash %+v", s.Entries[2])
	}
	if e := s.LastError(); e == nil || !e.LimitLike() {
		t.Errorf("errors %+v", s.Errors)
	}
	// The newest run, and the newest when no id is given.
	s, err = p.Load(context.Background(), session.Ref{Agent: session.Aider, CWD: repo}, session.LoadOptions{})
	if err != nil || s.ID != "20261117T113000" || s.Model != "anthropic/claude-test" || s.FirstUserText() != "what changed?" {
		t.Fatalf("%v %+v", err, s)
	}
	if _, err := p.Load(context.Background(), session.Ref{Agent: session.Aider, CWD: repo, ID: "nope"}, session.LoadOptions{}); err == nil {
		t.Fatal("unknown run")
	}
}

// A run that starts with an i-mux handoff is a chained handoff.
func TestChained(t *testing.T) {
	doc := session.Marker(session.Claude, session.Aider, time.Date(2026, 11, 17, 9, 0, 0, 0, time.Local)) +
		"\n\n## Original request\nfix the parser\n\n## What to do now\ncontinue\n"
	var b strings.Builder
	b.WriteString("\n# aider chat started at 2026-11-17 09:00:05\n\n")
	for _, l := range strings.Split(doc, "\n") {
		b.WriteString("#### " + l + "  \n")
	}
	b.WriteString("\nOn it.\n\n")
	s := Parse([]byte(b.String()), session.Ref{Agent: session.Aider}, session.LoadOptions{})
	if s.Chained == nil || s.Chained.OriginalRequest != "fix the parser" || s.Chained.From != session.Claude {
		t.Fatalf("%+v", s.Chained)
	}
	if s.Entries[0].Kind != session.KindHandoff || s.FirstUserText() != "fix the parser" {
		t.Fatalf("%+v", s.Entries)
	}
}

// Malformed input never panics; a run over the windows keeps both ends.
func TestParseRobust(t *testing.T) {
	for _, in := range []string{"", ">", "####", "> \n\n#### \n", "# aider chat started at nonsense\nhello\n"} {
		_ = Parse([]byte(in), session.Ref{}, session.LoadOptions{})
	}
	big := "# aider chat started at 2026-11-17 10:00:00\n#### first  \n\n" + strings.Repeat("reply line\n", 2000) + "#### last  \n"
	s := Parse([]byte(big), session.Ref{}, session.LoadOptions{HeadBytes: 100, TailBytes: 100})
	if !s.Gap || s.FirstUserText() != "first" || s.Entries[len(s.Entries)-1].Text != "last" {
		t.Fatalf("%+v", s.Entries)
	}
	if tokens("1.1M") != 1100000 || tokens("12k") != 12000 || tokens("x") != 0 {
		t.Fatal("tokens")
	}
}
