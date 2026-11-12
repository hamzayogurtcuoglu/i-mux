package agent

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestExpand(t *testing.T) {
	tricky := "say \"hi\" 'there'\nline2 $(rm -rf ~) `id` ; | & {id} ğüşıöç 🎉"
	cases := []struct {
		name string
		tpl  []string
		v    Vars
		want []string
	}{
		{"claude new", []string{"--session-id", "{uuid}", "{prompt}"}, Vars{Prompt: tricky, UUID: "u-1"}, []string{"--session-id", "u-1", tricky}},
		{"claude resume no delta", []string{"--resume", "{id}", "{prompt}"}, Vars{ID: "abc"}, []string{"--resume", "abc"}},
		{"gemini", []string{"--resume", "{id}", "-i", "{prompt}"}, Vars{ID: "g", Prompt: "p"}, []string{"--resume", "g", "-i", "p"}},
		{"embedded", []string{"--prompt={prompt}", "--x"}, Vars{Prompt: "a b"}, []string{"--prompt=a b", "--x"}},
		{"prompt has placeholders", []string{"{prompt}"}, Vars{Prompt: "{uuid} {id}", UUID: "U", ID: "I"}, []string{"{uuid} {id}"}},
		{"no prompt drops its flag", []string{"-i", "{prompt}"}, Vars{}, []string{}},
		{"gemini resume, no delta", []string{"--resume", "{id}", "-i", "{prompt}"}, Vars{ID: "g"}, []string{"--resume", "g"}},
		{"claude new, no prompt", []string{"--session-id", "{uuid}", "{prompt}"}, Vars{UUID: "u"}, []string{"--session-id", "u"}},
		{"no id drops its flag", []string{"--resume", "{id}", "{prompt}"}, Vars{Prompt: "p"}, []string{"p"}},
		{"embedded empty", []string{"--prompt={prompt}", "--x"}, Vars{}, []string{"--x"}},
		{"codex resume", []string{"resume", "{id}", "{prompt}"}, Vars{ID: "c"}, []string{"resume", "c"}},
	}
	for _, c := range cases {
		if got := Expand(c.tpl, c.v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if !HasPrompt([]string{"a", "x{prompt}"}) || HasPrompt([]string{"--resume", "{id}"}) {
		t.Fatal("HasPrompt")
	}
}

func TestNewUUID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a, b := NewUUID(), NewUUID()
	if !re.MatchString(a) || a == b {
		t.Fatalf("%s %s", a, b)
	}
}

func TestRegistry(t *testing.T) {
	keys := map[string]bool{}
	for _, a := range All() {
		if a.Name == "" || a.Short == "" || a.Bin == "" || a.InstallHint == "" || a.Key == "" {
			t.Errorf("incomplete %+v", a)
		}
		if keys[a.Key] {
			t.Errorf("duplicate mnemonic %s", a.Key)
		}
		keys[a.Key] = true
	}
	cfg := config.Default()
	cfg.Order = []string{"codex", "claude"}
	var ids []session.AgentID
	for _, a := range Ordered(cfg) {
		ids = append(ids, a.ID)
	}
	if !reflect.DeepEqual(ids, []session.AgentID{"codex", "claude", "antigravity", "copilot", "opencode"}) {
		t.Fatalf("ordered %v", ids)
	}
}

// TestMain lifts the version-probe timeout. Fake agents are freshly written
// shell scripts, and macOS scans a new executable on its first run: about
// 0.2 s when idle, but over 2 s when every package's tests run in parallel.
// The probe logic is what these tests check, not machine speed.
func TestMain(m *testing.M) {
	SetProbeTimeout(30 * time.Second)
	os.Exit(m.Run())
}

func writeExe(t *testing.T, path, script string) {
	t.Helper()
	testutil.WriteFile(t, path, []byte("#!/bin/sh\n"+script+"\n"))
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveSymlinkKeepsUnresolvedDir(t *testing.T) {
	h := testutil.Isolate(t)
	nvmBin := filepath.Join(h.Home, "nvm", "bin")
	libDir := filepath.Join(h.Home, "nvm", "lib", "claude-code")
	writeExe(t, filepath.Join(libDir, "cli.js"), "echo 2.1.0 '(Claude Code)'")
	if err := os.MkdirAll(nvmBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(libDir, "cli.js"), filepath.Join(nvmBin, "claude")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", nvmBin+":/usr/bin:/bin")
	r := Resolve(config.Default(), session.Claude, nil)
	if !r.Installed || r.Path != filepath.Join(nvmBin, "claude") {
		t.Fatalf("%+v", r)
	}
	if !strings.HasPrefix(r.PathEnv, nvmBin+":") {
		t.Fatalf("PathEnv %q should start with the unresolved dir", r.PathEnv)
	}
	if v := ProbeVersion(r.Path, r.PathEnv); v != "2.1.0" {
		t.Fatalf("version %q", v)
	}
}

func TestResolveConfigBinWins(t *testing.T) {
	h := testutil.Isolate(t)
	onPath := filepath.Join(h.Home, "bin", "agy")
	custom := filepath.Join(h.Home, "custom", "my-agy")
	writeExe(t, onPath, "true")
	writeExe(t, custom, "true")
	t.Setenv("PATH", filepath.Dir(onPath))
	cfg := config.Default()
	cfg.Agent("antigravity").Bin = custom
	r := Resolve(cfg, session.Antigravity, nil)
	if r.Path != custom {
		t.Fatalf("config bin should win: %+v", r)
	}
	cfg.Agent("antigravity").Bin = filepath.Join(h.Home, "nope")
	if r := Resolve(cfg, session.Antigravity, nil); r.Installed {
		t.Fatal("missing configured bin must not fall back to PATH")
	}
}

func TestResolveFromCache(t *testing.T) {
	h := testutil.Isolate(t)
	bin := filepath.Join(h.Home, "nvm", "bin", "codex")
	writeExe(t, bin, "echo codex-cli 0.38.0")
	t.Setenv("PATH", "/usr/bin:/bin") // server env without nvm
	st := &state.State{}
	st.SetBin("codex", &state.Bin{Path: bin, PathEnv: "/nvm/bin:/usr/bin", Version: "0.38.0"})
	r := Resolve(config.Default(), session.Codex, st)
	if !r.Installed || !r.FromCache || r.Version != "0.38.0" || !strings.HasPrefix(r.PathEnv, filepath.Dir(bin)+":") {
		t.Fatalf("%+v", r)
	}
	if r := Resolve(config.Default(), session.Copilot, st); r.Installed {
		t.Fatal("copilot should not be installed")
	}
}

func TestRefreshCache(t *testing.T) {
	h := testutil.Isolate(t)
	bin := filepath.Join(h.Home, "bin", "claude")
	counter := filepath.Join(h.Home, "count")
	writeExe(t, bin, `echo x >> "`+counter+`"; [ "$1" = "--version" ] && echo 1.0.110 || exit 3`)
	t.Setenv("PATH", filepath.Dir(bin)+":/usr/bin:/bin")
	now := testutil.FrozenTime
	res, err := RefreshCache(config.Default(), now)
	if err != nil {
		t.Fatal(err)
	}
	if res[session.Claude].Version != "1.0.110" {
		t.Fatalf("%+v", res[session.Claude])
	}
	st, _ := state.Read()
	if st.Bins["claude"] == nil || st.Bins["claude"].Version != "1.0.110" || st.Bins["claude"].PathEnv == "" {
		t.Fatalf("cache %+v", st.Bins["claude"])
	}
	// Second refresh within a day does not probe again.
	if _, err := RefreshCache(config.Default(), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(counter)
	if strings.Count(string(b), "x") != 1 {
		t.Fatalf("probed %d times", strings.Count(string(b), "x"))
	}
}

func TestProbeVersionTimesOutOnHangingBinary(t *testing.T) {
	h := testutil.Isolate(t)
	bin := filepath.Join(h.Home, "bin", "hang")
	// The shell forks sleep, which inherits stdout; killing only the shell
	// must not leave the probe waiting for the child to close the pipe.
	writeExe(t, bin, "sleep 20; echo 9.9.9")
	defer SetProbeTimeout(300 * time.Millisecond)()
	start := time.Now()
	if v := ProbeVersion(bin, "/usr/bin:/bin"); v != "" {
		t.Fatalf("version %q from a hanging binary", v)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("probe took %v; the timeout did not stop it", d)
	}
}

func TestReadDirArgs(t *testing.T) {
	for id, want := range map[session.AgentID]string{
		session.Claude: "--add-dir=/h", session.Copilot: "--add-dir=/h", session.Antigravity: "--add-dir=/h", session.Codex: "",
	} {
		got := strings.Join(ReadDirArgs(id, "/h"), " ")
		if got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	if ReadDirArgs(session.Claude, "") != nil || ReadDirArgs("nope", "/h") != nil {
		t.Fatal("empty dir or unknown agent")
	}
}

// The config's install command wins over i-mux's; OpenCode's hint is prose,
// its command is not.
func TestInstallCommand(t *testing.T) {
	cfg := config.Default()
	if got := Install(cfg, session.Codex); got != "npm i -g @openai/codex" {
		t.Errorf("codex: %q", got)
	}
	if got := Install(cfg, session.OpenCode); got != "curl -fsSL https://opencode.ai/install | bash" {
		t.Errorf("opencode: %q", got)
	}
	cfg.Agents = map[string]*config.Agent{"codex": {Install: "brew install codex"}}
	if got := Install(cfg, session.Codex); got != "brew install codex" {
		t.Errorf("configured: %q", got)
	}
}
