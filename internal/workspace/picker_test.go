package workspace

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func writeClaudeSession(t *testing.T, h *testutil.Homes, id, text string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), id+".jsonl")
	testutil.WriteFile(t, p, []byte(`{"type":"user","sessionId":"`+id+`","cwd":"`+h.Project+`","timestamp":"2026-11-05T10:00:00Z","uuid":"a","message":{"role":"user","content":"`+text+`"}}`+"\n"))
	testutil.Touch(t, p, mtime)
}

func writeCodexSession(t *testing.T, h *testutil.Homes, id, text string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(h.Codex, "sessions", "2026", "11", "05", "rollout-2026-11-05T09-00-00-"+id+".jsonl")
	testutil.WriteFile(t, p, []byte(`{"timestamp":"2026-11-05T09:00:00Z","type":"session_meta","payload":{"id":"`+id+`","cwd":"`+h.Project+`"}}`+"\n"+
		`{"timestamp":"2026-11-05T09:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"`+text+`"}]}}`+"\n"))
	testutil.Touch(t, p, mtime)
}

func TestPickerMenuPagesInstalledAgents(t *testing.T) {
	w, _, h := testWS(t)
	f := sources.New(config.Default())
	now := testutil.FrozenTime
	for i := 0; i < 11; i++ {
		id := fmt.Sprintf("11111111-0000-4000-8000-%012d", i)
		writeClaudeSession(t, h, id, fmt.Sprintf("claude task %02d with a rather long first request text", i), now.Add(-time.Duration(i)*time.Minute))
	}
	writeAgy(t, h, "22222222-0000-4000-8000-000000000001", now.Add(-90*time.Second), now.Add(-90*time.Second))
	// Codex is not installed in testWS: its sessions are not listed.
	writeCodexSession(t, h, "33333333-0000-4000-8000-000000000001", "codex task", now)
	p := &Pane{ID: "%1", Agent: "claude", CWD: h.Project}
	ctx := context.Background()
	m := w.PickerMenu(ctx, f, p, "/dev/pts/1", 0)
	if m == nil || !strings.HasPrefix(m.Title, "Sessions · ~/proj (1/2)") {
		t.Fatalf("title %+v", m)
	}
	if len(m.Items) != PickerRows+2 || m.Items[PickerRows+1].Name != "More…" || m.Items[PickerRows+1].Key != "m" {
		t.Fatalf("page 1 items: %d %+v", len(m.Items), m.Items[len(m.Items)-1])
	}
	first := m.Items[0]
	const rowPrefix = "Claude   just now     "
	preview := strings.TrimPrefix(first.Name, rowPrefix)
	if first.Key != "1" || !strings.HasPrefix(first.Name, rowPrefix+"claude task 00 with") ||
		utf8.RuneCountInString(preview) > previewRunes || !strings.HasSuffix(preview, "…") {
		t.Fatalf("row 1 %q", first.Name)
	}
	if !strings.Contains(first.Cmd, "__menu --session claude:11111111-0000-4000-8000-000000000000 --pane %1 --client /dev/pts/1") {
		t.Fatalf("row cmd %q", first.Cmd)
	}
	var sawAgy bool
	for _, it := range m.Items {
		if strings.HasPrefix(it.Name, "Antigravity ") {
			sawAgy = true
		}
		if strings.Contains(it.Name, "codex task") {
			t.Fatal("not-installed agent listed")
		}
	}
	if !sawAgy {
		t.Fatal("antigravity row missing from page 1")
	}
	m2 := w.PickerMenu(ctx, f, p, "", 1)
	if !strings.HasSuffix(m2.Title, "(2/2)") || len(m2.Items) != 3+2 || !strings.Contains(m2.Items[4].Cmd, "--page 0") {
		t.Fatalf("page 2: %q %d", m2.Title, len(m2.Items))
	}
	if w.PickerMenu(ctx, f, &Pane{ID: "%2", CWD: "/home/test/empty"}, "", 0) != nil {
		t.Fatal("empty dir should give no menu")
	}
}

func TestSessionMenu(t *testing.T) {
	w, _, _ := testWS(t)
	ref := session.Ref{Agent: "antigravity", ID: "g-1", UpdatedAt: testutil.FrozenTime.Add(-2 * time.Hour)}
	m := w.SessionMenu(ref, &Pane{ID: "%4", CWD: "/home/test/proj"}, "c1")
	var names []string
	for _, it := range m.Items {
		names = append(names, it.Name)
	}
	got := strings.Join(names, "|")
	want := "Continue in Antigravity (resume)|Continue in Claude (with context)||Preview handoff||New pane: Antigravity (resume)|New pane: Claude (with context)||Pin to the top"
	if got != want {
		t.Fatalf("items\n got %s\nwant %s", got, want)
	}
	if !strings.Contains(m.Items[0].Cmd, "switch antigravity --from-session antigravity:g-1 --pane %4 --client c1") ||
		!strings.Contains(m.Items[1].Cmd, "switch claude --from-session antigravity:g-1") ||
		!strings.Contains(m.Items[3].Cmd, "handoff --session antigravity:g-1 --cwd /home/test/proj --print") ||
		!strings.Contains(m.Items[6].Cmd, "split claude -h --from-session antigravity:g-1") {
		t.Fatalf("commands %+v", m.Items)
	}
	if pin := m.Items[len(m.Items)-1]; pin.Key != "t" || !strings.Contains(pin.Cmd, "__pin antigravity:g-1 --pane %4 --client c1") {
		t.Fatalf("pin item %+v", pin)
	}
	// No native resume when the agent's resume is off.
	w.Cfg.Agents["antigravity"].Resume = false
	if m := w.SessionMenu(ref, &Pane{ID: "%4"}, ""); strings.Contains(m.Items[0].Name, "resume") {
		t.Fatal("resume offered although disabled")
	}
}

// TestPickerPinned: pinned conversations come first (📌), looked up by id
// even past the newest PickerCandidates; the menu offers Unpin for them.
func TestPickerPinned(t *testing.T) {
	w, _, h := testWS(t)
	f := sources.New(config.Default())
	now := testutil.FrozenTime
	var ids []string
	for i := 0; i < PickerCandidates+5; i++ {
		id := fmt.Sprintf("11111111-0000-4000-8000-%012d", i)
		ids = append(ids, id)
		writeClaudeSession(t, h, id, fmt.Sprintf("claude task %02d", i), now.Add(-time.Duration(i)*time.Hour))
	}
	oldest := "claude:" + ids[len(ids)-1]
	w.State().Pin("claude:" + ids[3])
	w.State().Pin(oldest)
	w.State().Pin("claude:gone-0000") // deleted by the agent: not shown
	p := &Pane{ID: "%1", Agent: "claude", CWD: h.Project}
	m := w.PickerMenu(context.Background(), f, p, "", 0)
	if m == nil || len(m.Items) < 3 {
		t.Fatalf("menu %+v", m)
	}
	if !strings.HasPrefix(m.Items[0].Name, "📌 Claude") || !strings.Contains(m.Items[0].Name, fmt.Sprintf("claude task %02d", len(ids)-1)) ||
		!strings.HasPrefix(m.Items[1].Name, "📌 Claude") || !strings.Contains(m.Items[1].Name, "claude task 03") ||
		!strings.HasPrefix(m.Items[2].Name, "   Claude") || !strings.Contains(m.Items[2].Name, "claude task 00") {
		t.Fatalf("pinned first:\n%s\n%s\n%s", m.Items[0].Name, m.Items[1].Name, m.Items[2].Name)
	}
	n := 0
	for pg := 0; ; pg++ {
		mm := w.PickerMenu(context.Background(), f, p, "", pg)
		for _, it := range mm.Items {
			if strings.Contains(it.Name, "claude task 03") {
				n++
			}
		}
		if !strings.Contains(mm.Title, fmt.Sprintf("(%d/", pg+2)) {
			break
		}
	}
	if n != 1 {
		t.Fatalf("a pinned conversation listed %d times", n)
	}
	sm := w.SessionMenu(session.Ref{Agent: "claude", ID: ids[len(ids)-1]}, p, "")
	if last := sm.Items[len(sm.Items)-1]; last.Name != "Unpin" || !strings.Contains(last.Cmd, "__pin "+oldest+" --unpin") {
		t.Fatalf("unpin item %+v", last)
	}
	w.State().Unpin(oldest)
	w.State().Unpin("claude:" + ids[3])
	w.State().Unpin("claude:gone-0000")
	if m := w.PickerMenu(context.Background(), f, p, "", 0); !strings.HasPrefix(m.Items[0].Name, "Claude") {
		t.Fatalf("no pins: no indent: %q", m.Items[0].Name)
	}
}
