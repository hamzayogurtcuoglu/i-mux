package sidebar

import (
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestEnvView (T89.4): Env is the rail's seventh view; a row per
// environment with its number of variables, "● this pane" on the target
// pane's, ⚠ on one with skipped lines; a click applies one (its menu on
// the target's own); + and the empty view make one; every row is as wide
// as the sidebar.
func TestEnvView(t *testing.T) {
	i18n.SetLocale("en")
	top, _ := railItems(IconsEmoji)
	if it := top[len(top)-1]; it.view != ViewEnv || it.icon != "🔑" {
		t.Fatalf("last rail item %+v", it)
	}
	m := Model{View: ViewEnv, Icons: IconsEmoji, Target: "%1", TargetEnv: "glm", Envs: []EnvRow{
		{Name: "glm", Vars: 2},
		{Name: "openrouter", Vars: 3, Problems: 1},
	}}
	for _, w := range []int{30, 41, 60} {
		for i, l := range Render(m, w, 0, NoItem).Lines {
			if d := DisplayWidth(Plain(l.Text)); d != w {
				t.Errorf("w=%d row %d %q is %d wide", w, i, Plain(l.Text), d)
			}
		}
	}
	text := screenText(Render(m, 50, 0, NoItem))
	for _, want := range []string{"ENV", "🔑 glm", "● this pane", "🔑 openrouter ⚠", "3", "This pane runs with glm"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	rows, _ := renderEnv(m, 41, 0)
	if a := rows[0].ActionAt(39); a.Kind != ActEnvNew {
		t.Errorf("title's + %+v", a)
	}
	if a := rows[1].Action; a.Kind != ActEnvMenu || a.Path != "glm" {
		t.Errorf("the pane's own %+v", a)
	}
	if a := rows[2].Action; a.Kind != ActEnvApply || a.Path != "openrouter" {
		t.Errorf("another %+v", a)
	}

	// No environment on the target: a click applies each.
	m.TargetEnv = ""
	if rows, _ := renderEnv(m, 41, 0); rows[1].Action.Kind != ActEnvApply || strings.Contains(screenText(Screen{Lines: rows}), "this pane runs") {
		t.Errorf("rows %+v", rows[1].Action)
	}

	// None yet: what it is for (any program), and New environment….
	m.Envs = nil
	rows, _ = renderEnv(m, 41, 0)
	text = screenText(Screen{Lines: rows})
	if !strings.Contains(strings.Join(strings.Fields(text), " "), "for any program: API keys") || rows[len(rows)-1].Action.Kind != ActEnvNew {
		t.Errorf("empty view:\n%s", text)
	}
}

// gatherEnvs reads names, counts and problems, never values.
func TestGatherEnvs(t *testing.T) {
	testutil.Isolate(t)
	testutil.WriteFile(t, envfile.Path("glm"), []byte("A=1\nB=2\nbad\n"))
	got := gatherEnvs()
	if len(got) != 1 || got[0] != (EnvRow{Name: "glm", Vars: 2, Problems: 1}) {
		t.Fatalf("%+v", got)
	}
}
