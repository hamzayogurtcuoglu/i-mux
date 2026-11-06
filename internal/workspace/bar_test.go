package workspace

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/workspace -update)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from golden file; run with -update to accept.\n--- got ---\n%s", name, got)
	}
}

var barAgentsFixture = []BarAgent{{"claude", true}, {"gemini", true}, {"copilot", false}, {"codex", true}}

func TestAgentBarGolden(t *testing.T) {
	golden(t, "agentbar-clickable.golden", AgentBar(barAgentsFixture, true)+"\n")
	golden(t, "agentbar-plain.golden", AgentBar(barAgentsFixture, false)+"\n")
}

func TestAgentBarRanges(t *testing.T) {
	bar := AgentBar(barAgentsFixture, true)
	re := regexp.MustCompile(`#\[range=user\|([^\]]*)\]`)
	ms := re.FindAllStringSubmatch(bar, -1)
	if len(ms) == 0 {
		t.Fatal("no ranges")
	}
	seen := map[string]bool{}
	for _, m := range ms {
		if len(m[1]) > MaxRangeBytes {
			t.Errorf("range %q is %d bytes (tmux limit %d)", m[1], len(m[1]), MaxRangeBytes)
		}
		if !strings.HasPrefix(m[1], RangePrefix) {
			t.Errorf("range %q lacks the ajan: prefix", m[1])
		}
		seen[m[1]] = true
	}
	for _, want := range []string{"ajan:claude", "ajan:gemini", "ajan:copilot", "ajan:codex", RangeSessions, RangeMenu} {
		if !seen[want] {
			t.Errorf("missing range %s", want)
		}
	}
	// Every range is closed, and nothing else is clickable without ranges.
	if strings.Count(bar, "#[range=") != strings.Count(bar, "#[norange]") {
		t.Fatal("unbalanced ranges")
	}
	if plain := AgentBar(barAgentsFixture, false); strings.Contains(plain, "range=") {
		t.Fatal("a non-clickable bar has ranges")
	}
	for _, r := range []string{RangeSessions, RangeMenu, RangeCur, RangePrefix + "copilot"} {
		if len(r) > MaxRangeBytes {
			t.Errorf("constant %q too long", r)
		}
	}
}

// TestAgentBarOverflow: the outer conditionals pick the full, the
// installed-only and the short-name layouts by client width.
func TestAgentBarOverflow(t *testing.T) {
	full, wFull := barVariant(barAgentsFixture, true, true, false)
	mid, wMid := barVariant(barAgentsFixture, true, false, false)
	short, wShort := barVariant(barAgentsFixture, true, false, true)
	if wFull <= wMid || wMid <= wShort {
		t.Fatalf("widths %d %d %d", wFull, wMid, wShort)
	}
	if !strings.Contains(full, "(not installed)") || strings.Contains(mid, "(not installed)") || strings.Contains(short, "Copilot") {
		t.Fatal("not-installed entries should be dropped first")
	}
	if !strings.Contains(short, " Cl ") || strings.Contains(short, "Claude") || strings.Contains(short, "Sessions") {
		t.Fatal("short layout")
	}
	// With four agents and limit markers the installed-only layout fits a
	// wide terminal; at 80 columns the short layout is used.
	if wMid > 120 || wShort > 80 {
		t.Fatalf("layouts too wide: mid %d short %d", wMid, wShort)
	}
	bar := AgentBar(barAgentsFixture, true)
	if !strings.HasPrefix(bar, "#[align=left]#{?#{e|<:#{client_width},"+itoa(wFull)+"},#{?#{e|<:#{client_width},"+itoa(wMid)+"},") {
		t.Fatalf("outer conditionals: %.120s", bar)
	}
	// Branches of conditionals must not contain bare commas from styles.
	for _, v := range []string{full, mid, short} {
		if regexp.MustCompile(`#\[[^\]]*,[^\]]*\]`).MatchString(v) {
			t.Fatalf("style with a comma inside a conditional branch: %s", v)
		}
	}
}

func TestAgentBarInConf(t *testing.T) {
	w, _, _ := testWS(t)
	v34, _ := tmux.ParseVersion("tmux 3.4")
	v32, _ := tmux.ParseVersion("tmux 3.2a")
	if b := agentBar(w, v34); !strings.Contains(b, "range=user|ajan:claude") {
		t.Fatalf("3.4 bar not clickable: %.100s", b)
	}
	if b := agentBar(w, v32); b == "" || strings.Contains(b, "range=") {
		t.Fatalf("3.2 bar should be an indicator only: %.100s", b)
	}
	w.Cfg.Tmux.Mouse = false
	if b := agentBar(w, v34); strings.Contains(b, "range=") {
		t.Fatal("mouse off: no ranges")
	}
	w.Cfg.Tmux.AgentBar = false
	if agentBar(w, v34) != "" {
		t.Fatal("agent_bar = false must disable the bar")
	}
}
