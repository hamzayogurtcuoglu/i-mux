package toolsum

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// cwd is the fixtures' session directory: absolute on this OS.
var cwd = abs("/home/test/proj")

// abs makes a Unix absolute fixture path absolute on this OS: on Windows it
// gets a drive and backslashes (C:\home\test\proj), elsewhere it is p.
func abs(p string) string {
	if runtime.GOOS == "windows" {
		return "C:" + filepath.FromSlash(p)
	}
	return p
}

// in is the path rel (slash-separated) inside cwd.
func in(rel string) string { return filepath.Join(cwd, rel) }

// osRel is the relative path rel (slash-separated) as display shows it on
// this OS.
func osRel(rel string) string { return filepath.FromSlash(rel) }

type A = map[string]any

func TestRules(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: src/a.ts\n@@\n-x\n+y\n*** Add File: src/new.ts\n+z\n*** Delete File: old.ts\n*** End Patch"
	cases := []struct {
		agent, name string
		args        A
		failed      bool
		want        string
	}{
		// Claude
		{"claude", "Bash", A{"command": "npm test\n--watch", "description": "run tests"}, false, "$ npm test …"},
		{"claude", "Read", A{"file_path": in("src/auth.ts")}, false, "read " + osRel("src/auth.ts")},
		{"claude", "Edit", A{"file_path": in("src/auth.ts"), "old_string": "a", "new_string": "b"}, false, "edited " + osRel("src/auth.ts")},
		{"claude", "MultiEdit", A{"file_path": "/etc/hosts", "edits": []any{}}, false, "edited /etc/hosts"},
		{"claude", "Write", A{"file_path": in("README.md"), "content": "x"}, false, "wrote README.md"},
		{"claude", "Grep", A{"pattern": "TODO", "path": in("src")}, false, `searched "TODO" in src`},
		{"claude", "Glob", A{"pattern": "**/*.go"}, false, `searched "**/*.go"`},
		{"claude", "WebFetch", A{"url": "https://example.com/docs", "prompt": "summarize"}, false, "fetched https://example.com/docs"},
		{"claude", "WebSearch", A{"query": "tmux status-format range"}, false, `web search "tmux status-format range"`},
		{"claude", "TodoWrite", A{"todos": []any{A{"content": "x", "status": "pending"}}}, false, ""},
		{"claude", "mcp__github__get_issue", A{"owner": "x", "issue_number": 12.0}, false, "mcp github.get_issue(12)"},
		{"claude", "Task", A{"description": "explore repo", "prompt": "long..."}, false, "Task(explore repo)"},
		{"claude", "Bash", A{"command": "false"}, true, "$ false (failed)"},
		// Gemini
		{"gemini", "read_file", A{"absolute_path": in("go.mod")}, false, "read go.mod"},
		{"gemini", "write_file", A{"file_path": in("main.go"), "content": "x"}, false, "wrote main.go"},
		{"gemini", "replace", A{"file_path": in("main.go"), "old_string": "a"}, false, "edited main.go"},
		{"gemini", "run_shell_command", A{"command": "go test ./..."}, false, "$ go test ./..."},
		{"gemini", "glob", A{"pattern": "*.md"}, false, `searched "*.md"`},
		{"gemini", "search_file_content", A{"pattern": "func main"}, false, `searched "func main"`},
		{"gemini", "list_directory", A{"path": in("internal")}, false, "listed internal"},
		{"gemini", "web_fetch", A{"prompt": "read https://example.com"}, false, "web_fetch(read https://example.com)"},
		{"gemini", "google_web_search", A{"query": "go 1.27 release"}, false, `web search "go 1.27 release"`},
		{"gemini", "write_todos", A{"todos": []any{}}, false, ""},
		// Copilot
		{"copilot", "view", A{"path": in("src/router.ts")}, false, "read " + osRel("src/router.ts")},
		{"copilot", "rg", A{"pattern": "health"}, false, `searched "health"`},
		{"copilot", "bash", A{"command": "npm run build", "description": "build"}, false, "$ npm run build"},
		{"copilot", "create", A{"path": in("src/health.ts"), "file_text": "x"}, false, "wrote " + osRel("src/health.ts")},
		{"copilot", "edit", A{"path": in("src/router.ts")}, false, "edited " + osRel("src/router.ts")},
		// Codex
		{"codex", "shell", A{"command": []any{"bash", "-lc", "rg limiter"}}, false, "$ rg limiter"},
		{"codex", "shell", A{"command": []any{"ls", "-la"}}, false, "$ ls -la"},
		{"codex", "shell", A{"command": []any{"apply_patch", patch}}, false, "edited src/a.ts, created src/new.ts, deleted old.ts"},
		{"codex", "apply_patch", A{"input": patch}, false, "edited src/a.ts, created src/new.ts, deleted old.ts"},
		{"codex", "exec_command", A{"cmd": "make test"}, false, "$ make test"},
		{"codex", "update_plan", A{"plan": []any{A{"step": "x", "status": "completed"}}}, false, ""},
	}
	for _, c := range cases {
		got := Summarize(session.ToolCall{Name: c.name, Args: c.args, Failed: c.failed}, cwd)
		if got.Line != c.want {
			t.Errorf("%s %s: got %q want %q", c.agent, c.name, got.Line, c.want)
		}
	}
	if s := Summarize(session.ToolCall{Name: "report_intent", Args: A{"intent": "x"}}, cwd); !s.Skip || s.Line != "" {
		t.Fatalf("report_intent %+v", s)
	}
	if s := Summarize(session.ToolCall{Name: "TodoWrite", Args: A{"todos": []any{}}}, cwd); !s.Todos {
		t.Fatalf("todos %+v", s)
	}
}

func TestLineCap(t *testing.T) {
	s := Summarize(session.ToolCall{Name: "Bash", Args: A{"command": strings.Repeat("x", 500)}, Failed: true}, cwd)
	if n := len([]rune(s.Line)); n > MaxLine || !strings.HasSuffix(s.Line, " (failed)") {
		t.Fatalf("%d %q", n, s.Line)
	}
}

func TestActivity(t *testing.T) {
	entries := []session.Entry{
		{Tools: []session.ToolCall{
			{Name: "Read", Args: A{"file_path": in("a.go")}},
			{Name: "Read", Args: A{"file_path": in("a.go")}},
			{Name: "Read", Args: A{"file_path": in("b.go")}},
			{Name: "Edit", Args: A{"file_path": in("a.go")}},
			{Name: "Edit", Args: A{"file_path": in("b.go")}},
			{Name: "Write", Args: A{"file_path": in("c.go")}},
		}},
		{Tools: []session.ToolCall{
			{Name: "Edit", Args: A{"file_path": in("a.go")}},               // moves a.go to the end
			{Name: "Edit", Args: A{"file_path": in("c.go")}},               // created: stays in Created
			{Name: "Edit", Args: A{"file_path": in("x.go")}, Failed: true}, // failed: ignored
		}},
	}
	fa := Activity(entries, cwd)
	if !reflect.DeepEqual(fa.Edited, []string{"b.go", "a.go"}) || !reflect.DeepEqual(fa.Created, []string{"c.go"}) ||
		fa.ReadCount != 2 || !reflect.DeepEqual(fa.Read, []string{"a.go", "b.go"}) {
		t.Fatalf("%+v", fa)
	}
}

func TestLatestTodos(t *testing.T) {
	entries := []session.Entry{
		{Tools: []session.ToolCall{{Name: "update_plan", Args: A{"plan": []any{A{"step": "old"}}}}}},
		{Tools: []session.ToolCall{{Name: "write_todos", Args: A{"todos": []any{A{"description": "new", "status": "in_progress"}}}}, {Name: "Bash", Args: A{"command": "x"}}}},
	}
	todos := LatestTodos(entries)
	if len(todos) != 1 || todos[0].Text != "new" || todos[0].Status != session.TodoInProgress {
		t.Fatalf("%+v", todos)
	}
}

func FuzzSummarize(f *testing.F) {
	f.Add("Bash", "command", "ls")
	f.Add("apply_patch", "input", "*** Begin Patch\n*** Add File: x\n")
	f.Add("mcp____", "x", "y")
	f.Fuzz(func(t *testing.T, name, key, val string) {
		s := Summarize(session.ToolCall{Name: name, Args: A{key: val, "list": []any{val, 1.0}}}, cwd)
		if len([]rune(s.Line)) > MaxLine {
			t.Fatalf("line too long: %d", len([]rune(s.Line)))
		}
	})
}

// TestHandoffFileReads: reading imux's own handoff file is how the agent got
// its context; it is neither a conversation line nor a file read.
func TestHandoffFileReads(t *testing.T) {
	hf := abs("/home/test/.imux/handoffs/20261108-093139-claude-to-opencode.md")
	for _, tc := range []session.ToolCall{
		{Name: "read", Args: A{"file_path": hf}},
		{Name: "Read", Args: A{"file_path": hf}},
		{Name: "shell", Args: A{"command": []any{"bash", "-lc", "cat " + hf}}},
		{Name: "run_shell_command", Args: A{"command": "sed -n '1,200p' '" + hf + "'"}},
	} {
		if s := Summarize(tc, cwd); !s.Skip || s.Line != "" || len(s.Files) != 0 {
			t.Errorf("%s %v: %+v", tc.Name, tc.Args, s)
		}
	}
	// A file merely named like one, or not in a handoffs folder, is work.
	for _, p := range []string{in("handoffs/notes.md"), in("20261108-093139-claude-to-opencode.md")} {
		if s := Summarize(session.ToolCall{Name: "read", Args: A{"file_path": p}}, cwd); s.Skip || s.Line == "" {
			t.Errorf("%s skipped: %+v", p, s)
		}
	}
	fa := Activity([]session.Entry{{Tools: []session.ToolCall{{Name: "read", Args: A{"file_path": hf}}, {Name: "read", Args: A{"file_path": in("a.go")}}}}}, cwd)
	if fa.ReadCount != 1 || len(fa.Read) != 1 || fa.Read[0] != "a.go" {
		t.Errorf("activity %+v", fa)
	}
}
