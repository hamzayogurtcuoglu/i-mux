package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// fakeTmux scripts tmux replies by the first command of each invocation.
type fakeTmux struct {
	calls   [][]string
	replies map[string]string // first command → stdout
	errs    map[string]string // first command → stderr (failure)
}

func (f *fakeTmux) Spawn(bin string, args []string) error {
	_, _, err := f.Run(context.Background(), bin, args)
	return err
}

func (f *fakeTmux) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	cmd := ""
	for i, a := range args {
		if !strings.HasPrefix(a, "-") && (i == 0 || args[i-1] != "-L" && args[i-1] != "-f") {
			cmd = a
			break
		}
	}
	if e, ok := f.errs[cmd]; ok {
		return nil, []byte(e), errors.New("exit status 1")
	}
	return []byte(f.replies[cmd]), nil, nil
}

func testWS(t *testing.T) (*WS, *fakeTmux, *testutil.Homes) {
	t.Helper()
	h := testutil.Isolate(t)
	f := &fakeTmux{replies: map[string]string{}, errs: map[string]string{}}
	w := &WS{
		Cfg:  config.Default(),
		T:    &tmux.Client{Bin: "tmux", Socket: "test", Runner: f},
		Self: "/opt/imux/bin/imux",
		Now:  testutil.Clock().Now,
	}
	w.SetBins(map[session.AgentID]agent.Resolved{
		"claude":      {Agent: "claude", Installed: true, Path: "/opt/bin/claude", PathEnv: "/opt/bin:/usr/bin"},
		"antigravity": {Agent: "antigravity", Installed: true, Path: "/opt/bin/agy", PathEnv: "/opt/bin:/usr/bin"},
		"codex":       {Agent: "codex"},
	})
	return w, f, h
}

func TestSessionName(t *testing.T) {
	cases := map[string]string{
		"/home/test/proj":      "proj-",
		"/home/test/my.app":    "my_app-",
		"/home/test/a:b c":     "a_b_c-",
		"/":                    "root-",
		"/home/test/Türkçe-ış": "T_rk_e--",
	}
	for cwd, prefix := range cases {
		got := SessionName(cwd)
		if !strings.HasPrefix(got, prefix) || len(got) != len(prefix)+4 || strings.ContainsAny(got, ".:") {
			t.Errorf("SessionName(%q) = %q, want %s<4 hex>", cwd, got, prefix)
		}
	}
	if SessionName("/a/proj") == SessionName("/b/proj") {
		t.Fatal("same basename in different dirs must differ")
	}
}

func TestTrail(t *testing.T) {
	now := testutil.FrozenTime
	var tr Trail
	for i := 0; i < 12; i++ {
		tr = tr.Push(TrailEntry{Agent: "claude", Start: int64(i)}, now)
	}
	if len(tr) != MaxTrail || tr[0].Start != 2 {
		t.Fatalf("cap: len %d first %d", len(tr), tr[0].Start)
	}
	for _, e := range tr[:len(tr)-1] {
		if e.End != now.UnixMilli() {
			t.Fatalf("earlier run not closed: %+v", e)
		}
	}
	if tr.Last().End != 0 {
		t.Fatal("current run must stay open")
	}
	back := ParseTrail(tr.String())
	if !reflect.DeepEqual(back, tr) {
		t.Fatalf("round trip:\n%v\n%v", back, tr)
	}
	if ParseTrail("garbage") != nil || ParseTrail("") != nil || (Trail{}).String() != "[]" {
		t.Fatal("garbage handling")
	}
	closed := tr.Close(now.Add(time.Hour))
	if closed.Last().End != now.Add(time.Hour).UnixMilli() || tr.Last().End != 0 {
		t.Fatal("Close must copy")
	}
}

func TestSpecNewResumeAndMissing(t *testing.T) {
	w, _, h := testWS(t)
	s, te, err := w.Spec(Launch{Agent: "claude", CWD: h.Project})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Args) != 2 || s.Args[0] != "--session-id" || s.Args[1] != te.SessionID || te.SessionID == "" {
		t.Fatalf("claude new: %q %+v", s.Args, te)
	}
	if s.Bin != "/opt/bin/claude" || s.Env["PATH"] != "/opt/bin:/usr/bin" || s.Dir != h.Project {
		t.Fatalf("spec %+v", s)
	}
	if te.Start != testutil.FrozenTime.UnixMilli() || te.Resumed {
		t.Fatalf("trail entry %+v", te)
	}
	s, te, _ = w.Spec(Launch{Agent: "claude", CWD: h.Project, ResumeID: "abc", Prompt: "delta"})
	if !reflect.DeepEqual(s.Args, []string{"--resume", "abc", "delta"}) || te.SessionID != "abc" || !te.Resumed {
		t.Fatalf("resume: %q %+v", s.Args, te)
	}
	s, te, _ = w.Spec(Launch{Agent: "antigravity", CWD: h.Project, Prompt: "hand\noff", Handoff: "/h.md"})
	if !reflect.DeepEqual(s.Args, []string{"-i", "hand\noff"}) || te.SessionID != "" || te.Handoff != "/h.md" {
		t.Fatalf("antigravity: %q %+v", s.Args, te)
	}
	var ni *NotInstalledError
	if _, _, err := w.Spec(Launch{Agent: "codex"}); !errors.As(err, &ni) || !strings.Contains(err.Error(), "npm i -g @openai/codex") {
		t.Fatalf("missing: %v", err)
	}
	w.Cfg.Handoff.MaxArgBytes = 10
	if _, _, err := w.Spec(Launch{Agent: "antigravity", Prompt: strings.Repeat("x", 11)}); !errors.Is(err, launch.ErrTooLarge) {
		t.Fatalf("argv guard: %v", err)
	}
}

func TestLabelAndDefaultAgent(t *testing.T) {
	w, _, _ := testWS(t)
	if w.Label("claude") != "Claude" {
		t.Fatal(w.Label("claude"))
	}
	w.Cfg.Agents["claude"].Args = []string{"--model", "opus"}
	if w.Label("claude") != "Claude · opus" {
		t.Fatal(w.Label("claude"))
	}
	if a, err := w.DefaultAgent(); err != nil || a != "claude" {
		t.Fatalf("default %v %v", a, err)
	}
	w.Cfg.Order = []string{"codex", "antigravity", "claude", "copilot"}
	if a, _ := w.DefaultAgent(); a != "antigravity" {
		t.Fatalf("first installed in order: %v", a)
	}
	w.Cfg.DefaultAgent = "codex"
	if a, _ := w.DefaultAgent(); a != "codex" {
		t.Fatalf("config default: %v", a)
	}
	w.SetBins(map[session.AgentID]agent.Resolved{})
	w.Cfg.DefaultAgent = ""
	if _, err := w.DefaultAgent(); err == nil || !strings.Contains(err.Error(), "npm i -g") {
		t.Fatalf("none installed: %v", err)
	}
}

// TestStartBatchesOptions: the new session and its pane options are set in
// one tmux exec, targeting the pane the first command created.
func TestStartBatchesOptions(t *testing.T) {
	w, f, h := testWS(t)
	f.errs["has-session"] = "can't find session: x"
	f.replies["new-session"] = "%5\n"
	res, err := w.Start(context.Background(), "", h.Project)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.Pane != "%5" || res.Session != SessionName(h.Project) {
		t.Fatalf("result %+v", res)
	}
	last := f.calls[len(f.calls)-1]
	joined := strings.Join(last, " ")
	for _, want := range []string{
		"new-session -d -s " + res.Session + " -c " + h.Project, "/opt/imux/bin/imux __exec ",
		"; set-option -p remain-on-exit on", "; set-option -p @imux_agent claude", "; set-option -p @imux_cwd " + h.Project,
		"; set-option -p @imux_label Claude", `; set-option -p @imux_trail [{"a":"claude"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("batch lacks %q:\n%s", want, joined)
		}
	}
	// The spec file referenced in the command exists and is private.
	var spec string
	for i, a := range last {
		if a == "__exec" {
			spec = last[i+1]
		}
	}
	fi, err := os.Stat(spec)
	if err != nil || fi.Mode().Perm() != 0o600 || filepath.Dir(spec) != filepath.Join(h.Imux, "launch") {
		t.Fatalf("spec %s: %v %v", spec, fi, err)
	}
	// An existing session is found, not recreated.
	delete(f.errs, "has-session")
	n := len(f.calls)
	res, err = w.Start(context.Background(), "", h.Project)
	if err != nil || res.Created || len(f.calls) != n+1 {
		t.Fatalf("existing: %+v %v (%d calls)", res, err, len(f.calls)-n)
	}
}

func TestPaneParsing(t *testing.T) {
	w, f, _ := testWS(t)
	f.replies["display-message"] = "%3\tantigravity\t1762352000123\t/home/test/proj\t/elsewhere\tAntigravity\t0\tproj-1a2b\t@1\tnode\t\t\t[{\"a\":\"claude\",\"s\":1,\"e\":2},{\"a\":\"antigravity\",\"s\":2}]\n"
	p, err := w.Pane(context.Background(), "%3")
	if err != nil {
		t.Fatal(err)
	}
	if p.Agent != "antigravity" || p.CWD != "/home/test/proj" || p.Start.UnixMilli() != 1762352000123 || len(p.Trail) != 2 || p.Dead {
		t.Fatalf("pane %+v", p)
	}
	f.replies["display-message"] = "%4\t\t\t\t/elsewhere\t\t1\ts\t@1\tzsh\t\t\t\n"
	p, _ = w.Pane(context.Background(), "%4")
	if p.Agent != "" || p.CWD != "/elsewhere" || !p.Dead || p.Trail != nil {
		t.Fatalf("plain pane %+v", p)
	}
	if _, err := w.Pane(context.Background(), "%9"); !errors.Is(err, tmux.ErrNoPane) {
		t.Fatalf("mismatched id: %v", err)
	}
}

func TestPasteModeKeepsPromptOutOfArgv(t *testing.T) {
	w, _, h := testWS(t)
	w.Cfg.Handoff.Inject = "paste"
	s, _, err := w.Spec(Launch{Agent: "antigravity", CWD: h.Project, Prompt: "the handoff"})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Args) != 0 {
		t.Fatalf("paste mode argv %q", s.Args)
	}
	if !w.pasteMode(Launch{Agent: "antigravity", Prompt: "x"}) || w.pasteMode(Launch{Agent: "antigravity"}) {
		t.Fatal("pasteMode")
	}
	// An agent whose template has no {prompt} slot always gets a paste.
	w.Cfg.Handoff.Inject = "argv"
	w.Cfg.Agents["antigravity"].NewArgs = []string{"--interactive"}
	if !w.pasteMode(Launch{Agent: "antigravity", Prompt: "x"}) {
		t.Fatal("no prompt slot should paste")
	}
}
