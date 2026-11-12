package auto

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	testutil.WriteFile(t, path, []byte(s))
}

// TestList (T66.1): the agents of every place (not a README without front
// matter), the personal ones after them, then the skills.
func TestList(t *testing.T) {
	h := testutil.Isolate(t)
	root := h.Project
	write(t, filepath.Join(root, ".claude/agents/sim-tag.md"), "---\nname: sim-tag\ndescription: |\n  Watches the simulator's tags\n  and starts the pipeline.\nmodel: haiku\ntools:\n  - Bash\n---\nCheck the tags.\n")
	write(t, filepath.Join(root, ".claude/agents/README.md"), "# Our agents\n")
	write(t, filepath.Join(root, ".github/agents/docs.agent.md"), "---\ndescription: 'Keeps the docs right'\n---\nRead the docs.\n")
	write(t, filepath.Join(root, ".agents/agents/any.md"), "---\nname: Any One\ndescription: \"runs with \\\"any\\\" CLI\"\n---\n")
	write(t, filepath.Join(h.Home, ".claude/agents/mine.md"), "---\nname: mine\ndescription: personal\n---\n")
	write(t, filepath.Join(root, ".agents/skills/release-notes/SKILL.md"), "---\nname: release-notes\ndescription: Writes release notes\n---\nSteps.\n")
	defs := List(root, h.Home)
	var got []string
	for _, d := range defs {
		got = append(got, string(d.Kind)+":"+d.Name+":"+string(d.Owner)+":"+d.Place)
		if !ValidID(d.ID) || d.ID != ID(root, d.File) {
			t.Errorf("%s: id %q", d.Name, d.ID)
		}
	}
	want := []string{
		"agent:Any One::.agents/agents",
		"agent:docs:copilot:.github/agents",
		"agent:sim-tag:claude:.claude/agents",
		"agent:mine:claude:~/.claude/agents",
		"skill:release-notes::.agents/skills",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("defs\n%q\nwant\n%q", got, want)
	}
	sim, _ := Find(defs, ID(root, filepath.Join(root, ".claude/agents/sim-tag.md")))
	if sim.Description != "Watches the simulator's tags and starts the pipeline." || sim.Model != "haiku" || sim.Personal {
		t.Errorf("sim-tag %+v", sim)
	}
	if any, _ := Find(defs, defs[0].ID); any.Description != `runs with "any" CLI` {
		t.Errorf("quoted description %q", any.Description)
	}
	if !defs[3].Personal || List(root, "")[3].Kind != KindSkill {
		t.Error("personal agents")
	}
	// The same file in another project is another definition.
	if ID(root, sim.File) == ID(root+"x", sim.File) {
		t.Error("ids do not depend on the project")
	}
}

func TestFrontMatter(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	write(t, p, "---\nname: a\ndescription: >\n  folded\n  text\n# a comment\nlist:\n  - x\n---\nBody line 1\n\nBody line 2\n")
	fm, body, ok := FrontMatter(p)
	if !ok || fm["name"] != "a" || fm["description"] != "folded text" || fm["list"] != "" || body != "Body line 1\n\nBody line 2" {
		t.Fatalf("%v %q %q", ok, fm, body)
	}
	for _, s := range []string{"no front matter\n", "---\nname: a\n", ""} {
		write(t, p, s)
		if _, _, ok := FrontMatter(p); ok {
			t.Errorf("front matter in %q", s)
		}
	}
}

// TestWhen (T66.1): what can be typed, how it is stored and said.
func TestWhen(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "manual": "", "5m": "5m", "5 min": "5m", "every 10 minutes": "10m", "her 5 dk": "5m", "15": "15m",
		"2h": "2h", "2 saat": "2h", "1h30m": "1h30m", "90m": "1h30m", "1d": "1d", "1 gün": "1d",
		"hourly": "hourly", "saat başı": "hourly", "Her saat başı": "hourly",
		"09:00": "09:00", "18:30 9:05, 18:30": "09:05 18:30",
	} {
		w, err := ParseWhen(in)
		if err != nil || w.String() != want {
			t.Errorf("ParseWhen(%q) = %q %v, want %q", in, w.String(), err, want)
		}
	}
	for _, in := range []string{"30s", "soon", "25:00", "9:5", "9:00 x"} {
		if _, err := ParseWhen(in); err == nil {
			t.Errorf("ParseWhen(%q) accepted", in)
		}
	}
	if _, err := ParseWhen("10 saniye"); err != ErrTooOften {
		t.Errorf("10 seconds: %v", err)
	}
}

func TestNextAndDue(t *testing.T) {
	loc := time.FixedZone("TRT", 3*3600)
	at := func(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, loc) }
	every, _ := ParseWhen("5m")
	hourly, _ := ParseWhen("hourly")
	daily, _ := ParseWhen("09:00 18:30")
	if got := hourly.Next(at(12, 10, 59)); !got.Equal(at(12, 11, 0)) {
		t.Errorf("hourly next %v", got)
	}
	if got := daily.Next(at(12, 9, 0)); !got.Equal(at(12, 18, 30)) {
		t.Errorf("daily next %v", got)
	}
	if got := daily.Next(at(12, 20, 0)); !got.Equal(at(13, 9, 0)) {
		t.Errorf("daily next, tomorrow %v", got)
	}
	created := at(12, 10, 30)
	for _, c := range []struct {
		name      string
		w         When
		last, now time.Time
		due       bool
	}{
		{"interval, never ran: at once", every, time.Time{}, created, true},
		{"interval, ran 4 min ago", every, at(12, 10, 31), at(12, 10, 35), false},
		{"interval, ran 5 min ago", every, at(12, 10, 31), at(12, 10, 36), true},
		{"daily, made after today's 09:00", daily, time.Time{}, at(12, 11, 0), false},
		{"daily, its next time came", daily, time.Time{}, at(12, 18, 30), true},
		{"daily, missed while closed: once", daily, at(12, 18, 30), at(13, 12, 0), true},
		{"daily, ran after it", daily, at(13, 12, 0), at(13, 13, 0), false},
		{"hourly, on the hour", hourly, at(12, 10, 0), at(12, 11, 0), true},
		{"by hand", When{}, time.Time{}, at(13, 0, 0), false},
	} {
		if due, _ := c.w.Due(created, c.last, c.now); due != c.due {
			t.Errorf("%s: due %v", c.name, due)
		}
	}
}

// TestCommand (T66.1): each CLI's headless command line, the headless flag
// first; --agent when the CLI runs its own kind of definition; the model;
// read-only access.
func TestCommand(t *testing.T) {
	claudeAgent := Def{Kind: KindAgent, Name: "sim-tag", Owner: session.Claude}
	copilotAgent := Def{Kind: KindAgent, Name: "docs", Owner: session.Copilot}
	skill := Def{Kind: KindSkill, Name: "notes"}
	for _, c := range []struct {
		cli      session.AgentID
		d        Def
		model    string
		readOnly bool
		want     []string
	}{
		{session.Claude, claudeAgent, "haiku", false, []string{"claude", "-p", "--session-id", "U", "--agent", "sim-tag", "--model", "haiku", "--dangerously-skip-permissions", "P"}},
		{session.Claude, copilotAgent, "", true, []string{"claude", "-p", "--session-id", "U", "--permission-mode", "dontAsk", "P"}},
		{session.Copilot, copilotAgent, "auto", false, []string{"copilot", "-p", "P", "--agent", "docs", "--model", "auto", "--yolo", "-s", "--no-color"}},
		{session.Copilot, skill, "", true, []string{"copilot", "-p", "P", "-s", "--no-color"}},
		{session.Antigravity, claudeAgent, "gemini-x", false, []string{"agy", "-p", "P", "--model", "gemini-x", "--dangerously-skip-permissions"}},
		{session.Antigravity, skill, "", true, []string{"agy", "-p", "P", "--mode", "plan"}},
		{session.Codex, skill, "o9", false, []string{"codex", "exec", "--skip-git-repo-check", "--color", "never", "--output-last-message", "OUT", "-m", "o9", "--dangerously-bypass-approvals-and-sandbox", "P"}},
		{session.OpenCode, Def{Kind: KindAgent, Name: "rev", Owner: session.OpenCode}, "a/b", false, []string{"opencode", "run", "--agent", "rev", "-m", "a/b", "P"}},
	} {
		inv, err := Command(c.cli, string(c.cli[:min(len(c.cli), 8)]), c.d, c.model, "P", c.readOnly, "OUT", "U")
		want := append([]string{}, c.want...)
		want[0] = string(c.cli[:min(len(c.cli), 8)])
		if err != nil || !reflect.DeepEqual(inv.Argv, want) {
			t.Errorf("%s %s: %q %v\nwant %q", c.cli, c.d.Name, inv.Argv, err, want)
		}
	}
	if inv, _ := Command(session.Claude, "claude", claudeAgent, "", "P", false, "", "U"); inv.Session != "U" {
		t.Error("Claude's session id")
	}
	if _, err := Command("nope", "x", skill, "", "P", false, "", ""); err == nil {
		t.Error("an unknown CLI")
	}
}

func TestPrompt(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sim-tag.md")
	write(t, f, "---\nname: sim-tag\ndescription: x\n---\nCompare the newest tag of org/sim.\n")
	d := Def{Kind: KindAgent, Name: "sim-tag", File: f, Owner: session.Claude}
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	native := Prompt(PromptInput{Def: d, CLI: session.Claude, When: "every 5 min", Dir: "/p", Lang: "Turkish", Now: now})
	if strings.Contains(native, "Compare the newest tag") || !strings.Contains(native, "on a schedule (every 5 min) in /p") ||
		!strings.Contains(native, "This is the first run.") || !strings.Contains(native, "in Turkish") || !strings.Contains(native, NothingNew) {
		t.Errorf("native prompt:\n%s", native)
	}
	prev := &Run{Start: now.Add(-5 * time.Minute), Report: "v2.2 is the newest."}
	other := Prompt(PromptInput{Def: d, CLI: session.Copilot, Dir: "/p", Prev: prev, Now: now, Manual: true})
	if !strings.Contains(other, "Compare the newest tag of org/sim.") || !strings.Contains(other, "started by hand") ||
		!strings.Contains(other, "(2026-11-12 11:55):\nv2.2 is the newest.") || !strings.Contains(other, "in English") {
		t.Errorf("prompt with the instructions:\n%s", other)
	}
	sk := Prompt(PromptInput{Def: Def{Kind: KindSkill, Name: "notes", File: "/p/.agents/skills/notes/SKILL.md"}, CLI: session.Claude, Dir: "/p", Now: now})
	if !strings.HasPrefix(sk, `Use the Agent Skill "notes" at /p/.agents/skills/notes/SKILL.md`) {
		t.Errorf("skill prompt:\n%s", sk)
	}
}

func TestSummarize(t *testing.T) {
	for in, want := range map[string]string{
		"## **v2.3 is out**: pipeline started\nmore": "v2.3 is out: pipeline started",
		"\x1b[1m- `go test` failed\x1b[0m":           "go test failed",
		"1. Done.":                                   "Done.",
	} {
		if s, q := Summarize(in); s != want || q {
			t.Errorf("Summarize(%q) = %q %v", in, s, q)
		}
	}
	for _, in := range []string{"", "  \n", NothingNew, "`" + NothingNew + "`", "Checked the tags.\n\n" + NothingNew + "\n"} {
		if _, q := Summarize(in); !q {
			t.Errorf("%q is not quiet", in)
		}
	}
	if s, _ := Summarize(strings.Repeat("x", 300)); len([]rune(s)) != 200 {
		t.Errorf("not cut: %d", len([]rune(s)))
	}
}

// TestStore (T66.1): a schedule is made from its definition, changed,
// read back, and removed with its runs.
func TestStore(t *testing.T) {
	testutil.Isolate(t)
	now := time.Date(2026, 11, 12, 9, 30, 15, 0, time.UTC)
	d := Def{ID: "0123456789ab", Kind: KindAgent, Name: "sim-tag", File: "/p/.claude/agents/sim-tag.md"}
	s, err := Change(d.ID, &d, "/p", now, func(s *Schedule) { s.When, s.CLI = "5m", "claude" })
	if err != nil || s.Access != AccessFull || !s.Created.Equal(now) || s.Project != "/p" {
		t.Fatalf("made %+v %v", s, err)
	}
	if _, err := Change(d.ID, nil, "", now, func(s *Schedule) { s.Model, s.Paused = "haiku", true }); err != nil {
		t.Fatal(err)
	}
	if _, err := Change("ffffffffffff", nil, "", now, func(*Schedule) {}); err == nil {
		t.Error("changed a missing schedule")
	}
	ss, err := Load()
	got, _ := Get(ss, d.ID)
	if err != nil || len(ss) != 1 || got.Model != "haiku" || !got.Paused || got.When != "5m" || !got.Created.Equal(now) {
		t.Fatalf("loaded %+v %v", ss, err)
	}
	if b, _ := os.ReadFile(Path()); !strings.HasPrefix(string(b), "# i-mux's Scheduler") || !strings.Contains(string(b), "[[schedule]]") {
		t.Errorf("schedules.toml:\n%s", b)
	}
	if err := os.MkdirAll(Dir(d.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Remove(d.ID); err != nil {
		t.Fatal(err)
	}
	if ss, _ := Load(); len(ss) != 0 {
		t.Errorf("left %+v", ss)
	}
	if _, err := os.Stat(Dir(d.ID)); !os.IsNotExist(err) {
		t.Error("runs left")
	}
}

// TestRuns (T66.1): a run holds its lock (Running sees it, a second one
// cannot start); the runs are kept newest first, MaxRuns of them.
func TestRuns(t *testing.T) {
	testutil.Isolate(t)
	const id = "0123456789ab"
	e, err := Begin(id)
	if err != nil {
		t.Fatal(err)
	}
	e.Started(Current{Start: time.Unix(100, 0), PGID: 42, CLI: "claude"})
	if c, ok := Running(id); !ok || c.PGID != 42 || c.CLI != "claude" {
		t.Fatalf("running %+v %v", c, ok)
	}
	if _, err := Begin(id); err != ErrRunning {
		t.Fatalf("second run: %v", err)
	}
	for i := range MaxRuns + 2 {
		log := LogName(time.Unix(int64(1000+i), 0))
		write(t, LogPath(id, log), "x")
		if i < MaxRuns+1 {
			if err := addRun(id, Run{Start: time.Unix(int64(1000+i), 0), Log: log}); err != nil {
				t.Fatal(err)
			}
		}
	}
	last := LogName(time.Unix(int64(1000+MaxRuns+1), 0))
	if err := e.Finish(Run{Start: time.Unix(int64(1000+MaxRuns+1), 0), Status: StatusOK, Log: last}); err != nil {
		t.Fatal(err)
	}
	if _, ok := Running(id); ok {
		t.Fatal("still running")
	}
	rs := Runs(id)
	if len(rs) != MaxRuns || rs[0].Log != last || !LastStart(id).Equal(time.Unix(int64(1000+MaxRuns+1), 0)) {
		t.Fatalf("%d runs, first %+v", len(rs), rs[0])
	}
	if _, err := os.Stat(LogPath(id, LogName(time.Unix(1000, 0)))); !os.IsNotExist(err) {
		t.Error("the oldest log was kept")
	}
	if _, err := os.Stat(LogPath(id, LogName(time.Unix(1002, 0)))); err != nil {
		t.Error("a kept run's log went")
	}
}

// TestExecute (T66.1): the output goes to the log and the result; an error
// exit and a timeout are told apart; the timeout kills the whole group.
func TestExecute(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir()) // the shell's $PWD is the real path
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "run.log")
	var pgid int
	o := Execute(context.Background(), []string{sh, "-c", `echo "report $PWD"; echo warn >&2`}, os.Environ(), dir, log, 5*time.Second, func(p int) { pgid = p })
	if o.Err != nil || o.Exit != 0 || o.TimedOut || strings.TrimSpace(o.Stdout) != "report "+dir || strings.TrimSpace(o.Stderr) != "warn" || pgid == 0 {
		t.Fatalf("%+v pgid %d", o, pgid)
	}
	if b, _ := os.ReadFile(log); !strings.Contains(string(b), "$ "+sh+" -c ") || !strings.Contains(string(b), "report") || !strings.Contains(string(b), "# exit 0") {
		t.Errorf("log:\n%s", b)
	}
	if o := Execute(context.Background(), []string{sh, "-c", "echo boom >&2; exit 3"}, nil, dir, log, 5*time.Second, nil); o.Exit != 3 || o.TimedOut {
		t.Errorf("exit 3: %+v", o)
	}
	marker := filepath.Join(dir, "child-lived")
	start := time.Now()
	o = Execute(context.Background(), []string{sh, "-c", "(sleep 2; touch " + marker + ") & sleep 5"}, nil, dir, log, 300*time.Millisecond, nil)
	if !o.TimedOut || time.Since(start) > 4*time.Second {
		t.Fatalf("timeout: %+v after %v", o, time.Since(start))
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a child of the run outlived the timeout")
	}
	if o := Execute(context.Background(), []string{filepath.Join(dir, "no-such-cli")}, nil, dir, log, time.Second, nil); o.Err == nil {
		t.Error("a missing binary started")
	}
}

// TestDueNow (T66.2): only what is due, not paused, not running and still
// defined starts; a schedule just started waits for its lock.
func TestDueNow(t *testing.T) {
	h := testutil.Isolate(t)
	def := filepath.Join(h.Project, "a.md")
	write(t, def, "---\nname: a\n---\n")
	now := time.Now()
	mk := func(id, when string, paused bool, file string) Schedule {
		return Schedule{ID: id, When: when, Paused: paused, File: file, Created: now.Add(-time.Hour)}
	}
	ss := []Schedule{
		mk("aaaaaaaaaaaa", "5m", false, def),         // never ran: due
		mk("bbbbbbbbbbbb", "5m", true, def),          // paused
		mk("cccccccccccc", "", false, def),           // by hand
		mk("dddddddddddd", "5m", false, def+".gone"), // definition gone
		mk("eeeeeeeeeeee", "5m", false, def),         // running
		mk("ffffffffffff", "5m", false, def),         // ran a minute ago
		mk("000000000000", "5m", false, def),         // started by the watcher a moment ago
		mk("not-an-id", "5m", false, def),            // not an id
	}
	e, err := Begin("eeeeeeeeeeee")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Finish(Run{Start: now})
	if err := addRun("ffffffffffff", Run{Start: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range DueNow(ss, now, map[string]time.Time{"000000000000": now.Add(-10 * time.Second)}) {
		got = append(got, s.ID)
	}
	if !reflect.DeepEqual(got, []string{"aaaaaaaaaaaa"}) {
		t.Fatalf("due %q", got)
	}
}

func TestModelLists(t *testing.T) {
	help := "  --model <model>                       Model for the current session. Provide\n" +
		"                                        an alias for the latest model (e.g.\n" +
		"                                        'fable', 'opus', or 'sonnet') or a\n" +
		"                                        model's full name.\n" +
		"  -n, --name <name>                     Set a 'display' name\n"
	if got := ClaudeAliases(help); !reflect.DeepEqual(got, []Model{{ID: "fable"}, {ID: "opus"}, {ID: "sonnet"}}) {
		t.Errorf("aliases %+v", got)
	}
	agy := "Fetching available models...\ngemini-4-argon-medium\tGemini 4 Argon (Medium)\nclaude-opus-5-5-low\tClaude Opus 5.5 (Low)\n\n"
	if got := ParseModelList(agy); !reflect.DeepEqual(got, []Model{{"gemini-4-argon-medium", "Gemini 4 Argon (Medium)"}, {"claude-opus-5-5-low", "Claude Opus 5.5 (Low)"}}) {
		t.Errorf("agy models %+v", got)
	}
	if got := ParseModelList("Models:\nanthropic/claude-x\nopenai/gpt-y\n"); len(got) != 2 || got[1].ID != "openai/gpt-y" {
		t.Errorf("opencode models %+v", got)
	}
}

// TestModelsCache: a CLI's list is asked once a day.
func TestModelsCache(t *testing.T) {
	testutil.Isolate(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	bin := filepath.Join(dir, "agy")
	write(t, bin, "#!"+sh+"\necho x >> "+count+"\nprintf 'm1\\tModel One\\n'\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for range 2 {
		if got := Models(context.Background(), session.Antigravity, bin, "", now); len(got) != 1 || got[0].Name != "Model One" {
			t.Fatalf("models %+v", got)
		}
	}
	if b, _ := os.ReadFile(count); strings.Count(string(b), "x") != 1 {
		t.Errorf("asked %d times", strings.Count(string(b), "x"))
	}
	Models(context.Background(), session.Antigravity, bin, "", now.Add(ModelsFor+time.Minute))
	if b, _ := os.ReadFile(count); strings.Count(string(b), "x") != 2 {
		t.Error("a day-old list was kept")
	}
}

func TestNewAgent(t *testing.T) {
	h := testutil.Isolate(t)
	p, err := Create(h.Project, "tag-watch")
	if err != nil || p != filepath.Join(h.Project, ".claude/agents/tag-watch.md") {
		t.Fatalf("%q %v", p, err)
	}
	if fm, body, ok := FrontMatter(p); !ok || fm["name"] != "tag-watch" || !strings.Contains(body, NothingNew) {
		t.Errorf("template %v %q", fm, body)
	}
	if _, err := Create(h.Project, "tag-watch"); err != ErrExists || !Exists(h.Project, "tag-watch") {
		t.Errorf("again: %v", err)
	}
	pr := WritePrompt(h.Project, session.Copilot, "watch the tags", "Turkish")
	if !strings.Contains(pr, filepath.Join(h.Project, ".github/agents")+"/<name>.agent.md") || !strings.HasSuffix(pr, "watch the tags") || !strings.Contains(pr, NothingNew) {
		t.Errorf("write prompt:\n%s", pr)
	}
	if pr := WritePrompt(h.Project, session.Antigravity, "", "Turkish"); !strings.Contains(pr, ".agents/agents/<name>.md") || !strings.HasSuffix(pr, "before you write anything.") || !strings.Contains(pr, "in Turkish") {
		t.Errorf("ask first:\n%s", pr)
	}
}
