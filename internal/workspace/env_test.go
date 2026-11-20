package workspace

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// secret is a fake value: no tmux command may ever carry it (D84).
const secret = "fake-secret-value-123"

// envPaneRow is a pane row with the command it runs and its environment.
func envPaneRow(p *Pane) string {
	dead := "0"
	if p.Dead {
		dead = "1"
	}
	return strings.Join([]string{p.ID, string(p.Agent), "1762300000000", p.CWD, p.CWD, "", dead, "proj", "@1", p.Command, "", "", p.Trail.String(), p.Role, "", p.Env, p.StartCmd}, "\t") + "\n"
}

func writeEnv(t *testing.T, name, text string) {
	t.Helper()
	testutil.WriteFile(t, envfile.Path(name), []byte(text))
}

// noSecret fails when a tmux command carries a value.
func noSecret(t *testing.T, f *fakeTmux) {
	t.Helper()
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), secret) {
			t.Fatalf("a value on a tmux command line: %q", c)
		}
	}
}

// specOf reads the launch spec of the last respawn-pane.
func specOf(t *testing.T, f *fakeTmux) *launch.Spec {
	t.Helper()
	var respawn []string
	for _, c := range f.calls {
		if slices.Contains(c, "respawn-pane") {
			respawn = c
		}
	}
	for i, a := range respawn {
		if a == "__exec" {
			s, err := launch.Read(respawn[i+1], false)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
	}
	t.Fatalf("no spec in %q", respawn)
	return nil
}

// A run gets its environment's variables in its spec; reserved keys and a
// name whose file is gone add nothing.
func TestSpecEnv(t *testing.T) {
	testutil.SkipOnWindows(t, "POSIX agent paths (/opt/bin); Windows spawns agents in the e2e suite")
	w, _, h := testWS(t)
	writeEnv(t, "glm", "ANTHROPIC_BASE_URL=https://api.example.test\nANTHROPIC_AUTH_TOKEN="+secret+"\nPATH=/evil\n")
	s, _, err := w.Spec(Launch{Agent: "claude", CWD: h.Project, Env: "glm"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Env["ANTHROPIC_AUTH_TOKEN"] != secret || s.Env["ANTHROPIC_BASE_URL"] != "https://api.example.test" || s.Env["PATH"] != "/opt/bin:/usr/bin" {
		t.Fatalf("env %v", s.Env)
	}
	s, _, _ = w.Spec(Launch{Agent: "claude", CWD: h.Project, Env: "gone"})
	if len(s.Env) != 1 {
		t.Fatalf("a missing environment adds nothing: %v", s.Env)
	}
}

// respawn takes the pane's environment, whatever the caller's launch says.
func TestRespawnTakesPaneEnv(t *testing.T) {
	testutil.SkipOnWindows(t, "POSIX agent paths (/opt/bin); Windows spawns agents in the e2e suite")
	w, f, h := testWS(t)
	writeEnv(t, "glm", "TOKEN="+secret+"\n")
	p := &Pane{ID: "%1", CWD: h.Project, Env: "glm"}
	if err := w.respawn(context.Background(), p, Launch{Agent: "claude", CWD: h.Project, Pane: "%1"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if s := specOf(t, f); s.Env["TOKEN"] != secret {
		t.Fatalf("spec env %v", s.Env)
	}
	noSecret(t, f)
}

// What applying does depends on what runs in the pane.
func TestEnvKindOf(t *testing.T) {
	w, f, h := testWS(t)
	f.replies["show-options"] = "/usr/bin/zsh\n"
	ctx := context.Background()
	for _, c := range []struct {
		p    Pane
		want EnvKind
	}{
		{Pane{Agent: "claude", Command: "node"}, EnvAgent},
		{Pane{Command: "bash"}, EnvShell},
		{Pane{Command: "-zsh"}, EnvShell},
		{Pane{Command: "fish"}, EnvShell},
		{Pane{Command: "vim"}, EnvBusy},
		{Pane{Command: "sh", StartCmd: `"sleep 600"`}, EnvBusy},
		{Pane{Command: "zsh", StartCmd: "/usr/bin/zsh -l"}, EnvShell},
		{Pane{Command: "sh", StartCmd: "/opt/imux/bin/imux __exec /h/launch/x.json"}, EnvShell},
		{Pane{Agent: "claude", Dead: true}, EnvBusy},
		{Pane{Role: "empty", Command: "imux"}, EnvEmpty},
		{Pane{Role: "sidebar", Command: "imux"}, EnvBusy},
	} {
		c.p.CWD = h.Project
		if got := w.EnvKindOf(ctx, &c.p); got != c.want {
			t.Errorf("%+v: %v, want %v", c.p, got, c.want)
		}
	}
}

// A shell at its prompt sources an apply file: only its path is typed,
// after C-u and a space; the file exports the new variables and unsets the
// previous environment's others.
func TestApplyEnvShell(t *testing.T) {
	w, f, h := testWS(t)
	i18n.SetLocale("en")
	writeEnv(t, "old", "OLD_ONLY=1\nSHARED=1\n")
	writeEnv(t, "glm", "SHARED=2\nTOKEN="+secret+"\n")
	f.replies["display-message"] = envPaneRow(&Pane{ID: "%2", CWD: h.Project, Command: "bash", Env: "old"})
	res, err := w.ApplyEnv(context.Background(), sources.New(config.Default()), "%2", "glm", "")
	if err != nil || res.Kind != EnvShell {
		t.Fatalf("%+v %v", res, err)
	}
	all := ""
	for _, c := range f.calls {
		all += strings.Join(c, " ") + "\n"
	}
	typed := typedLine(f)
	if !strings.Contains(all, "send-keys -t %2 C-u ; send-keys -t %2 -l") || !strings.Contains(all, "send-keys -t %2 Enter") ||
		!strings.Contains(all, "set-option -p -t %2 "+OptEnv+" glm") || !strings.HasPrefix(typed, " . '") {
		t.Fatalf("calls %q", f.calls)
	}
	path := strings.TrimSuffix(strings.TrimPrefix(typed, " . '"), "'")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "unset OLD_ONLY\nexport SHARED='2'\nexport TOKEN='" + secret + "'\ncommand rm -f -- '" + path + "'\n"; string(b) != want {
		t.Fatalf("apply file\n got %q\nwant %q", b, want)
	}
	noSecret(t, f)
}

// fish gets `source` and its own syntax.
func TestApplyEnvFish(t *testing.T) {
	w, f, h := testWS(t)
	writeEnv(t, "glm", "TOKEN=x\n")
	f.replies["display-message"] = envPaneRow(&Pane{ID: "%2", CWD: h.Project, Command: "fish"})
	if _, err := w.ApplyEnv(context.Background(), sources.New(config.Default()), "%2", "glm", ""); err != nil {
		t.Fatal(err)
	}
	typed := typedLine(f)
	path := strings.TrimSuffix(strings.TrimPrefix(typed, " source '"), "'")
	if b, err := os.ReadFile(path); err != nil || !strings.HasPrefix(string(b), "set -gx TOKEN 'x'\n") {
		t.Fatalf("fish file %q %v (typed %q)", b, err, typed)
	}
}

// typedLine is the text typed into a pane (send-keys -l).
func typedLine(f *fakeTmux) string {
	for _, c := range f.calls {
		for i := 1; i+1 < len(c); i++ {
			if c[i] == "-l" && slices.Contains(c[:i], "send-keys") {
				return c[i+1]
			}
		}
	}
	return ""
}

// A pane that runs another program is left alone.
func TestApplyEnvBusy(t *testing.T) {
	w, f, h := testWS(t)
	writeEnv(t, "glm", "TOKEN=x\n")
	f.replies["display-message"] = envPaneRow(&Pane{ID: "%2", CWD: h.Project, Command: "vim"})
	_, err := w.ApplyEnv(context.Background(), sources.New(config.Default()), "%2", "glm", "")
	if !errors.Is(err, ErrEnvBusy) {
		t.Fatalf("err %v", err)
	}
	for _, c := range f.calls {
		if slices.Contains(c, "send-keys") || slices.Contains(c, OptEnv) {
			t.Fatalf("a busy pane got %q", c)
		}
	}
}

// An agent restarts with the environment and resumes its conversation; the
// full handoff is the fallback.
func TestApplyEnvAgentRestarts(t *testing.T) {
	testutil.SkipOnWindows(t, "POSIX agent paths (/opt/bin); Windows spawns agents in the e2e suite")
	w, f, h := testWS(t)
	i18n.SetLocale("en")
	w.SetBins(map[session.AgentID]agent.Resolved{
		"claude": {Agent: "claude", Installed: true, Path: "/opt/bin/claude", PathEnv: "/opt/bin"},
	})
	writeEnv(t, "glm", "ANTHROPIC_AUTH_TOKEN="+secret+"\n")
	tr, _ := multiHop(t, h)
	cur := Trail{tr[0]}
	cur[0].End = 0 // Claude's run is the current one
	p := &Pane{ID: "%1", Agent: "claude", CWD: h.Project, Trail: cur, Command: "node"}
	f.replies["display-message"] = envPaneRow(p)
	f.replies["list-panes"] = envPaneRow(p)
	res, err := w.ApplyEnv(context.Background(), sources.New(config.Default()), "%1", "glm", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != EnvAgent || res.Restart == nil || !res.Restart.Resumed || !res.Restart.Restarted || res.Restart.Env != "glm" {
		t.Fatalf("result %+v %+v", res, res.Restart)
	}
	s := specOf(t, f)
	if !slices.Contains(s.Args, "--resume") || !slices.Contains(s.Args, claudeA) || s.Env["ANTHROPIC_AUTH_TOKEN"] != secret {
		t.Fatalf("spec %q %v", s.Args, s.Env)
	}
	var fallback string
	for _, c := range f.calls {
		if i := slices.Index(c, OptFallback); i >= 0 && i+1 < len(c) && slices.Contains(c, "respawn-pane") {
			fallback = c[i+1]
		}
	}
	fb, err := launch.Read(fallback, false)
	if err != nil || fb.Env["ANTHROPIC_AUTH_TOKEN"] != secret || slices.Contains(fb.Args, "--resume") {
		t.Fatalf("fallback %+v %v", fb, err)
	}
	var msg bool
	for _, c := range f.calls {
		if slices.Contains(c, "display-message") && strings.Contains(strings.Join(c, " "), "Claude restarted with 🔑 glm") {
			msg = true
		}
	}
	if !msg {
		t.Fatalf("no message: %q", f.calls)
	}
	noSecret(t, f)
}

// An agent pane asks first; while the agent works, the question says its
// turn stops.
func TestEnvMenu(t *testing.T) {
	w, f, h := testWS(t)
	i18n.SetLocale("en")
	p := &Pane{ID: "%1", Agent: "claude", CWD: h.Project, Env: "old"}
	m := w.EnvMenu(context.Background(), p, "glm", "/dev/pts/1")
	if !strings.Contains(m.Title, "Restart Claude with glm?") || strings.Contains(m.Title, "working") {
		t.Fatalf("title %q", m.Title)
	}
	if c := cmdOf(m, "y"); !strings.Contains(c, "env apply glm --yes --pane %1 --client /dev/pts/1") {
		t.Fatalf("yes %q", c)
	}
	f.replies["show-options"] = "working\n"
	m = w.EnvMenu(context.Background(), p, "", "")
	if !strings.Contains(m.Title, "Restart Claude without old?") || !strings.Contains(m.Title, "is working") {
		t.Fatalf("clear title %q", m.Title)
	}
	if c := cmdOf(m, "y"); !strings.Contains(c, "env clear --yes --pane %1") {
		t.Fatalf("clear yes %q", c)
	}
}

// The environment's menu: apply here (or remove here when it is the
// pane's), edit, delete; skipped lines are listed.
func TestEnvRowMenu(t *testing.T) {
	w, f, h := testWS(t)
	i18n.SetLocale("en")
	writeEnv(t, "glm", "A=1\nB=2\nnot a line\n")
	f.replies["display-message"] = envPaneRow(&Pane{ID: "%1", CWD: h.Project, Command: "bash"})
	m := w.EnvRowMenu(context.Background(), "glm", "%1", "")
	if m.Title != "🔑 glm · 2 variables" || !strings.Contains(m.Items[0].Name, "line 3 skipped") {
		t.Fatalf("menu %+v", m)
	}
	if !strings.Contains(cmdOf(m, "a"), "env apply glm --pane %1") || !strings.Contains(cmdOf(m, "e"), "env edit glm") ||
		!strings.Contains(cmdOf(m, "x"), "env confirm-rm glm") || cmdOf(m, "r") != "" {
		t.Fatalf("items %+v", m.Items)
	}
	f.replies["display-message"] = envPaneRow(&Pane{ID: "%1", CWD: h.Project, Command: "bash", Env: "glm"})
	m = w.EnvRowMenu(context.Background(), "glm", "%1", "")
	if !strings.Contains(cmdOf(m, "r"), "env clear --pane %1") || !strings.Contains(cmdOf(m, "a"), "env apply glm --pane %1") {
		t.Fatalf("on its pane %+v", m.Items)
	}
}

// Deleting an environment unsets it from the panes that have it.
func TestDeleteEnv(t *testing.T) {
	w, f, h := testWS(t)
	writeEnv(t, "glm", "A=1\n")
	f.replies["list-panes"] = envPaneRow(&Pane{ID: "%1", CWD: h.Project, Env: "glm"}) + envPaneRow(&Pane{ID: "%2", CWD: h.Project, Env: "other"})
	if err := w.DeleteEnv(context.Background(), "glm"); err != nil {
		t.Fatal(err)
	}
	if envfile.Exists("glm") {
		t.Fatal("file left")
	}
	var unset []string
	for _, c := range f.calls {
		if slices.Contains(c, "-u") && slices.Contains(c, OptEnv) {
			unset = append(unset, c[slices.Index(c, "-t")+1])
		}
	}
	if strings.Join(unset, ",") != "%1" {
		t.Fatalf("unset %v", unset)
	}
}

// The shell an agent exits to keeps the pane's environment: it starts
// through __exec with it.
func TestPaneDiedShellKeepsEnv(t *testing.T) {
	testutil.SkipOnWindows(t, "the POSIX login shell; Windows' shell is PowerShell")
	w, f, h := testWS(t)
	i18n.SetLocale("en")
	writeEnv(t, "glm", "TOKEN="+secret+"\n")
	f.replies["display-message"] = envPaneRow(&Pane{ID: "%1", Agent: "claude", CWD: h.Project, Dead: true, Env: "glm", Trail: Trail{{Agent: "claude", Start: 1}}})
	f.replies["show-options"] = "/bin/sh\n"
	if err := w.PaneDied(context.Background(), "%1", 0); err != nil {
		t.Fatal(err)
	}
	s := specOf(t, f)
	if s.Bin != "/bin/sh" || !slices.Equal(s.Args, []string{"-l"}) || s.Env["TOKEN"] != secret || s.Agent != "" {
		t.Fatalf("shell spec %+v", s)
	}
	noSecret(t, f)
}
